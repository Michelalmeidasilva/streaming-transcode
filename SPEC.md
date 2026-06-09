# streaming-transcode — SPEC

Transcoding worker (pipeline stage 4). AWS Batch job / Docker container.
Go + FFmpeg + shaka-packager. Consumes RabbitMQ, never exposes HTTP.

## Trigger / Invocation Modes

Two entrypoints drive the **same** pipeline (`worker.Processor.Process`):

- **`cmd/worker` (dev / RabbitMQ):** consumes `video.upload.completed` from
  exchange `video_events`, queue `transcode.jobs`. One job per message. Retry with
  backoff (`transcode.retry`) + dead-letter queue (`transcode.dead`). Carries full
  upload metadata (sidecar subtitles, raw-stream geometry).
- **`cmd/transcode-local <s3-key>` (prod / AWS Batch):** invoked by
  `S3 ObjectCreated(raw/) → EventBridge → Batch SubmitJob`, which passes the object
  key as the single positional argument (`command = ["transcode-local", "Ref::s3_key"]`).
  The binary derives `videoId` from `raw/<videoId>/<object>`, rebuilds a minimal
  `UploadCompletedEvent{VideoID, ObjectKey, Bucket, Provider:"aws-s3"}`, and runs the
  pipeline. The S3 event carries no upload-time metadata, so **sidecar subtitles are
  not advertised** and **headerless raw `.yuv` is rejected** (no geometry available).
  Exit 0 = `SUCCEEDED`, exit ≠0 = `FAILED` (reprocessable). The same binary keeps its
  flag-based local-file mode (`--input/--output`) for ad-hoc dev transcodes; a
  positional argument selects Batch mode.

## Pipeline per Job

1. Download source video from object storage at the event's `objectKey` — the
   full key `raw/<videoId>/<filename>` (both MinIO and AWS). The key comes from
   the event (`objectKey`/`sourceKey`, or the Batch entrypoint's argument); only
   when absent is it reconstructed as `<videoId>/<filename>` via `resolveObjectKey`.
2. Probe source with `ffprobe`; synthesize `MediaInfo` from supplied geometry for
   headerless `.yuv` sources (geometry provided in the event payload).
3. Extract thumbnail via ffmpeg (`-ss <t> -frames:v 1 -vf scale=640:-2 -q:v 3`),
   upload to `thumbnails/<videoId>.jpg`, PATCH ingest record with
   `thumbnail_status: "ready"`.
4. Transcode each rendition with ffmpeg (aligned GOPs: `-g 60 -keyint_min 60
   -sc_threshold 0`). Single AAC 128k audio track shared across renditions.
5. Convert sidecar `.srt` subtitles to WebVTT; package HLS subtitle media playlists.
6. Package DASH + HLS with shaka-packager; write to `transcoded/<videoId>/`.
7. Persist playback metadata (status `ready`, `mediaInfo`, subtitle tracks, etc.)
   by calling the **Event Gateway** (`streaming-ingest`): `PATCH
   /api/v1/upload-state/videos/:id` + status events on `POST /api/v1/events`. The
   gateway is the single writer of the shared `videos` collection that
   `streaming-distribution` reads — transcode never opens MongoDB directly. In
   prod the gateway base URL comes from `EVENT_GATEWAY_URL` (the ingest Lambda
   Function URL + `/api/v1`).

**Event publish vs. job success:** the `PATCH /upload-state/videos/:id` that marks the
video `ready` (the catalog state) is the success criterion; the lifecycle **events**
(`POST /api/v1/events`, which the gateway forwards to RabbitMQ) are **best-effort**. A
failure to publish an event — e.g. the gateway returning 500 because its RabbitMQ publish
failed — is logged but does **not** fail the job once the media is produced and the video
is marked ready. (Previously the `ready` event being fatal made Batch report `FAILED` on
jobs whose output was complete and serving.)

**Job state lifecycle:** `queued → transcoding → packaging → ready | failed`.

Packaged HLS/DASH output (many small segments per rendition) is uploaded to
object storage with bounded parallelism (`maxUploadConcurrency = 8`); the first
upload error cancels the remaining in-flight uploads and fails the job.

## Bitrate Ladder

| Resolution | Video bitrate | Codec |
|------------|--------------|-------|
| 360p | 800 kbps | H.264 |
| 480p | 1400 kbps | H.264 |
| 720p | 2800 kbps | H.264 |
| 1080p | 5000 kbps | H.264 |
| Audio | 128 kbps | AAC (shared) |

GOP alignment (`-g 60 -keyint_min 60 -sc_threshold 0`) is non-negotiable.

**Upscaling guard:** when explicit renditions are requested (via `TranscodeRequest.Renditions`), any rendition taller than the source is silently dropped. If all requested renditions exceed the source height, one rendition per distinct codec is produced at the source dimensions, so the ladder is never empty. Events without an explicit `transcode` field continue to use the production ladder defaults from `PlanProductionRenditionsForCodecs`.

## Telemetry (CloudWatch EMF)

One EMF JSON line is written to stdout per job:

```json
{
  "_aws": {
    "Timestamp": 1717689600000,
    "CloudWatchMetrics": [{
      "Namespace": "VOD/streaming-transcode",
      "Dimensions": [["result"]],
      "Metrics": [
        {"Name":"JobCount","Unit":"Count"},
        {"Name":"JobDuration","Unit":"Milliseconds"},
        {"Name":"FailureCount","Unit":"Count"}
      ]
    }]
  },
  "result": "success",
  "video_id": "<videoId>",
  "JobCount": 1,
  "JobDuration": 142300,
  "FailureCount": 0
}
```

No `/metrics` endpoint. No OTel push pipeline. See `docs/cloudwatch-emf-telemetry.md`.

## Storage

Backend selected at runtime via `STORAGE_PROVIDER` (`minio` | `aws-s3`).
`storage.New(cfg.Storage)` dispatches to `NewMinIOStorage` or `NewS3Storage`.

## Env

```
STORAGE_PROVIDER         minio | aws-s3
STORAGE_BUCKET           videos
MINIO_ENDPOINT           http://minio:9000
MINIO_ACCESS_KEY         admin
MINIO_SECRET_KEY         password123
AWS_REGION               us-east-1
AWS_ACCESS_KEY_ID        (AWS only)
AWS_SECRET_ACCESS_KEY    (AWS only)
RABBITMQ_URL             amqp://guest:guest@rabbitmq:5672/
MONGODB_URI              mongodb://mongodb:27017/streaming
TRANSCODE_JOB_TIMEOUT_SECONDS  10800
TRANSCODE_MAX_HEIGHT     0    (0 = uncapped)
TRANSCODE_CODECS         h264
INGEST_BASE_URL          http://streaming-ingest:8080/api/v1
```
