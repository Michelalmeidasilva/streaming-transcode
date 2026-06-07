# Batch Entrypoint (`transcode-local` as the AWS Batch job)

## Motivation

In production the transcode stage is **AWS Batch (Fargate Spot)**, not the
RabbitMQ consumer. The trigger is:

```
S3 ObjectCreated(raw/) → EventBridge rule → Batch SubmitJob
```

EventBridge passes the S3 object key as the Batch job parameter `Ref::s3_key`,
and the job's container command is `["transcode-local", "Ref::s3_key"]`. So the
`transcode-local` binary — until now a local-file dev utility — becomes the
production job entrypoint, receiving the key as `argv[1]`.

The RabbitMQ worker (`cmd/worker`) stays for local dev; it is no longer the
production path.

## Design

`transcode-local` runs in one of two modes, selected by argument shape:

| Invocation | Mode | Behavior |
|---|---|---|
| `transcode-local --input a.mp4 --output b.mp4 ...` | dev local-file | unchanged: single rendition from flags |
| `transcode-local raw/<videoId>/<object>` | **Batch** | full pipeline from the S3 key |

Selection: after `flag.Parse()`, a positional argument (`flag.NArg() >= 1`)
means Batch mode.

### Batch flow (`batch.go`)

1. **`extractVideoID(key)`** — `raw/<videoId>/<object>` → `videoId`. Rejects keys
   without the `raw/` prefix, an empty id segment, or no object beneath the id.
2. **`buildBatchEvent(cfg, key)`** — builds the minimal
   `UploadCompletedEvent{VideoID, ObjectKey: key, Bucket: cfg.Storage.Bucket,
   Provider: "aws-s3"}`. Rejects `.yuv` up front (headerless raw needs geometry
   the S3 event does not carry).
3. **`batchJobID(videoId)`** — uses `AWS_BATCH_JOB_ID` when present, else
   `videoId-<unixnano>`. Names logs and the per-job work directory.
4. **`runBatchJob(ctx, proc, cfg, key)`** — ties the above together and calls
   `proc.Process`. An invalid key fails before any work; a processing error
   propagates so `main` exits non-zero.
5. **`runBatchMode(key)`** — wires the production dependencies (S3-backed
   storage, Event Gateway client, ffmpeg runner) and returns the process exit
   code: `0` = SUCCEEDED, `1` = FAILED (Batch reprocesses).

The pipeline body itself is the **unchanged** `worker.Processor.Process`
(download → bitrate ladder → HLS/DASH packaging → upload → persist). Idempotency
is inherited: the processor short-circuits to "ready" if the HLS master already
exists, so Batch retries are safe.

## Persistence: via the Event Gateway, not MongoDB

`streaming-distribution` reads the shared **`videos`** collection, which
`streaming-ingest` owns as the single writer. The transcode job persists by
calling the gateway (`PATCH /api/v1/upload-state/videos/:id`, status events on
`POST /api/v1/events`) — exactly as the RabbitMQ worker already does. It does
**not** open MongoDB. There is no `manifests` collection.

> This corrects the original AWS IaC runbook, which assumed the Batch job would
> write MongoDB (`manifests`/`transcoding_jobs`) directly. See
> `infra/aws/RUNBOOK.md` (P1) and `infra/CHANGELOG.md`.

## Configuration (Batch task env)

| Var | Value | Source |
|---|---|---|
| `STORAGE_PROVIDER` | `s3` | module env |
| `STORAGE_BUCKET` | raw/transcoded bucket | module env |
| `AWS_REGION` | `us-east-2` | module env |
| `EVENT_GATEWAY_URL` | ingest Function URL + `/api/v1` | module env (`event_gateway_url`) |
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | S3 IAM-user creds | SSM (`S3_ACCESS_KEY_ID`/`S3_SECRET_ACCESS_KEY`) |

`config.FromEnv()` reads `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY` for the `s3`
provider (minio-go uses static creds). The Terraform module maps the `S3_*` SSM
parameters onto those env names.

Leave `TRANSCODE_MAX_HEIGHT` unset so the full ladder up to 1080p runs (decision
D11).

## Caveats (v1)

- **No sidecar subtitles** in Batch mode — the S3 event lists only the video
  object. Subtitle upload remains a dev/RabbitMQ-path feature.
- **`.yuv` unsupported** in Batch mode — rejected early with a clear error.
- Many small per-job HTTP calls to the ingest Function URL (progress events).
  Acceptable; the Function URL is public (`authorization_type = NONE`) and the
  `/events`/`/upload-state` routes carry no header gate.

## Tests

`cmd/transcode-local/batch_test.go` covers `extractVideoID` (valid/nested/missing
-prefix/empty-id/no-object/empty), `buildBatchEvent` (population, `.yuv` reject,
invalid-key propagation), `batchJobID` (env vs fallback), and `runBatchJob`
(success, no-Process-on-bad-key, error propagation) with a fake processor.
`Processor.Process` itself is already covered in `internal/worker`.
