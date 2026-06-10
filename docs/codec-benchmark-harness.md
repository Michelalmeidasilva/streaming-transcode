# Codec Benchmark Harness

## Overview

The codec benchmark harness is an isolated, on-demand, scale-to-zero tool for measuring
ffmpeg encode time, CPU utilisation, and output bitrate per codec × resolution across EC2
instance types. It runs entirely outside the production transcode path (S3 → EventBridge →
Batch) and produces no catalog side-effects.

## Binary

`cmd/benchmark` is built into the same `vod-transcode` Docker image alongside `cmd/worker`
and `cmd/transcode-local`. The active binary is selected by setting the container `command`:

```
command = ["benchmark"]
```

No image rebuild is required to switch between the production worker and the benchmark
harness — only the command changes.

## Corpus Convention

Corpus clips live in S3 under a dedicated prefix, separate from production video objects:

```
s3://<BENCHMARK_CORPUS_BUCKET>/<BENCHMARK_CORPUS_PREFIX>
     e.g. s3://vod-storage-2026/benchmark/corpus/
```

Upload representative clips once. The harness calls `ObjectStorage.List(ctx, bucket,
prefix)` to enumerate them automatically when `BENCHMARK_CLIPS` is not explicitly set.

## Benchmark Matrix

The harness iterates the full product **serially**:

```
for clip in clips:
  for codec in BENCHMARK_CODECS:
    for resolution in BENCHMARK_RESOLUTIONS:
      for repeat in 1..BENCHMARK_REPEATS:
        measure TranscodeRendition(clip, codec, resolution)
        POST /api/v1/benchmark-runs
```

Only `TranscodeRendition` is called — **no packaging, no segment upload, no PATCH to
upload-state, no catalog write**. Each cell produces exactly one run document in
`streaming-ingest` tagged `benchmark=true`.

## Machine Label

The machine label that identifies each run is resolved in priority order:

1. `BENCHMARK_MACHINE_LABEL` env var (explicit, e.g. set by the infra module).
2. IMDSv2 `http://169.254.169.254/latest/meta-data/instance-type` — works on EC2 with an
   instance profile that does not restrict IMDS. Returns the instance type string (e.g.
   `c5.xlarge`) without requiring any additional IAM permission.
3. `os.Hostname()` — fallback for non-EC2 environments.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `BENCHMARK_CORPUS_BUCKET` | `STORAGE_BUCKET` | Bucket holding corpus clips |
| `BENCHMARK_CORPUS_PREFIX` | — | Key prefix to list (e.g. `benchmark/corpus/`) |
| `BENCHMARK_CODECS` | — | Comma-separated codec IDs, e.g. `h264,av1` |
| `BENCHMARK_RESOLUTIONS` | — | Comma-separated `WxH:bitrateKbps` tuples, e.g. `1280x720:2800,1920x1080:5000` |
| `BENCHMARK_REPEATS` | `3` | Encode repetitions per codec×resolution×clip cell |
| `BENCHMARK_CLIPS` | — | Optional explicit S3 keys (comma-separated); overrides corpus listing |
| `BENCHMARK_MACHINE_LABEL` | — | Force a machine label (skips IMDS; falls back to hostname when both are unset) |
| `INGEST_BENCHMARK_URL` | **required** | Full URL for `POST /api/v1/benchmark-runs` on `streaming-ingest` |

All storage provider env vars (`STORAGE_PROVIDER`, `STORAGE_BUCKET`, `MINIO_ENDPOINT`,
`AWS_REGION`, `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`) apply unchanged.

## Isolation from Production

| Concern | Production path | Benchmark path |
|---|---|---|
| Trigger | S3 ObjectCreated → EventBridge → Batch | Self-terminating EC2 / manual container run |
| Pipeline stages | transcode + package + upload + catalog patch | transcode only (`TranscodeRendition`) |
| RabbitMQ | Consumes `video.upload.completed` | Not connected |
| MongoDB | Never writes directly | Never writes directly |
| Ingest endpoint | `POST /api/v1/events`, `PATCH /upload-state/videos/:id` | `POST /api/v1/benchmark-runs` only |
| Run documents | `benchmark=false` (production partition) | `benchmark=true` (benchmark partition) |

## ObjectStorage.List

The `ObjectStorage` port gained a `List(ctx context.Context, bucket, prefix string)
([]string, error)` method. Both `MinIOStorage` and `S3Storage` implement it. The benchmark
harness uses it to build the clip set when `BENCHMARK_CLIPS` is not provided.

## Workflow

1. Upload representative clips to `s3://<bucket>/benchmark/corpus/` once.
2. Set `enable_transcode_benchmark_harness=true` in `infra/terraform.tfvars` and choose
   an instance type (`benchmark_instance_type`, e.g. `c5.xlarge`).
3. `terraform apply` in `infra/aws/`. The EC2 instance pulls the `vod-transcode` image,
   runs the benchmark container over the corpus, and **self-terminates** when done
   (`instance_initiated_shutdown_behavior = terminate`).
4. Open the Metrics tab in `streaming-platform-upload` → Benchmark view. Results appear
   grouped by machine label (instance type) with per codec×resolution tables.
5. Change `benchmark_instance_type` (e.g. `c5.2xlarge`, `c7g.xlarge`) and re-apply.
   Each run accumulates in `transcode_runs` and all instance types are visible side by side.
6. Set `enable_transcode_benchmark_harness=false` and re-apply to clean up.

## arm64 / Graviton

The default image is `amd64`. To benchmark Graviton instances:

```bash
# Build and push a multi-arch image (keeps amd64 Fargate path working):
make -C streaming-transcode image-push-multiarch

# Or a dedicated arm64 tag:
make -C streaming-transcode image-push-multiarch PLATFORMS=linux/arm64 IMAGE_TAG=arm64
```

Then in `terraform.tfvars`:

```hcl
benchmark_instance_type = "c7g.xlarge"
benchmark_ami_arch      = "arm64"
benchmark_image_tag     = "latest"   # multi-arch, or "arm64"
```

If `benchmark_ami_arch` and the image architecture disagree, the container fails with
`exec format error` — keep them consistent.
