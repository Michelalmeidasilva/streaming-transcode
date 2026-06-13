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
`streaming-ingest` tagged `benchmark=true`. Per-rendition metrics include
`outputFileSizeBytes` (int64) — the size of the encoded output file in bytes.

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

## Source Characterization

Before encoding each clip, the harness calls `transcode.Runner.Probe` to read the source
video properties. The result is cached per clip key so that each corpus clip is probed
exactly once, regardless of how many codec×resolution×repeat iterations reference it.

Probe failure is **non-fatal**: if `ffprobe` cannot read a clip, all source fields are
left at their zero values (`0` / `""`) and the matrix continues normally. A warning is
logged with the clip key and the error.

The following source fields are added to every run document posted to
`POST /api/v1/benchmark-runs`:

| Field (JSON) | Type | Description |
|---|---|---|
| `sourceWidth` | int | Width of the source clip in pixels |
| `sourceHeight` | int | Height of the source clip in pixels |
| `sourceDurationSeconds` | float64 | Duration of the source clip in seconds |
| `sourceFps` | float64 | Frame rate of the source clip |
| `sourceCodec` | string | Video codec of the source clip (e.g. `h264`, `vp9`, `hevc`) |
| `sourceBitrateKbps` | int | Container bitrate of the source clip in kbps |
| `sourceFileSizeBytes` | int64 | File size of the source clip in bytes |

These fields allow the Benchmark view (`streaming-platform-upload` Metrics tab) to display
**Video** (clip title with full S3 key in the tooltip) and **Source** (`{w}×{h} ·
{duration}s · {codec}`) columns, and to key aggregations by `clip × codec × resolution`
so results from different source clips are not blended together.

## Rendition Fields

Each rendition entry in a posted run document includes:

| Field (JSON) | Type | Description |
|---|---|---|
| `name` | string | Rendition label, e.g. `720p` |
| `codec` | string | Codec ID, e.g. `h264`, `av1` |
| `width` | int | Output width in pixels |
| `height` | int | Output height in pixels |
| `preset` | string | ffmpeg preset, e.g. `fast` |
| `targetBitrateKbps` | int | Target bitrate in kbps |
| `outputBitrateKbps` | int | Measured output bitrate in kbps |
| `outputFileSizeBytes` | int64 | Encoded output file size in bytes |
| `elapsedSeconds` | float64 | Wall-clock encode time for this rendition |
| `avgCpuPercent` | float64 | Average CPU utilisation during encoding |
| `maxCpuPercent` | float64 | Peak CPU utilisation during encoding |
| `avgMemoryMb` | float64 | Average RSS memory in MB |
| `maxMemoryMb` | float64 | Peak RSS memory in MB |

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

## GPU (NVENC)

### Images

Two GPU images are provided:

| Image | Dockerfile | Arch | Notes |
|---|---|---|---|
| `vod-transcode-gpu` | `Dockerfile.gpu` | `amd64` | BtbN static ffmpeg + NVENC; `h264_nvenc`, `hevc_nvenc`, `av1_nvenc` |
| `vod-transcode-gpu` (arm64 tag) | `Dockerfile.gpu.arm64` | `arm64` | ffmpeg compiled from source; `h264_nvenc`, `hevc_nvenc` only (no `av1_nvenc` on T4G) |

Both images contain only the `benchmark` binary (`CMD ["benchmark"]`). They are stored in
the `vod-transcode-gpu` ECR repository.

Build and push:

```bash
REG=151803906541.dkr.ecr.us-east-2.amazonaws.com
REGION=us-east-2
aws ecr get-login-password --region $REGION | docker login --username AWS --password-stdin "$REG"

# amd64 GPU image (g4dn / g6 / g6e):
docker build --platform linux/amd64 -f Dockerfile.gpu \
  -t "$REG/vod-transcode-gpu:latest" . && docker push "$REG/vod-transcode-gpu:latest"

# arm64 GPU image (g5g):
docker build --platform linux/arm64 -f Dockerfile.gpu.arm64 \
  -t "$REG/vod-transcode-gpu:arm64" . && docker push "$REG/vod-transcode-gpu:arm64"
```

### Encoder Backend

The GPU images set `TRANSCODE_ENCODER_BACKEND=nvenc`, which routes each codec to its NVENC
encoder. See the **Encoder Backend** section in `SPEC.md` for the full codec→encoder mapping.
vp9 and vvc are unsupported under `nvenc` — affected runs are logged failed and the matrix
continues.

### Per-Device Codec Matrix

| Device | GPU chip | Arch | Supported codecs |
|---|---|---|---|
| g4dn | T4 | x86_64 | `h264`, `h265` |
| g5g | T4G | arm64 | `h264`, `h265` |
| g6 | L4 | x86_64 | `h264`, `h265`, `av1` |
| g6e | L40S | x86_64 | `h264`, `h265`, `av1` |

`av1_nvenc` is only available on Ada Lovelace and newer (L4 / L40S). Do not include `av1`
in `benchmark_codecs` for g4dn or g5g runs.

CPU Graviton instances (c7g, c8g) run software codecs via the `vod-transcode` image
(`TRANSCODE_ENCODER_BACKEND=software`); `h264`, `h265`, and `av1` are all supported.

### Service Quota Prerequisite

GPU instances require a **Service Quotas increase** before `terraform apply` can launch them:

- x86_64 GPU (g4dn, g6, g6e): request **"Running On-Demand G and VT instances"** in the
  EC2 console → Service Quotas. Minimum: 4 vCPUs (g4dn.xlarge / g6.xlarge).
- arm64 GPU (g5g): request **"Running On-Demand G instances"** for arm64.

Without the quota increase, `terraform apply` succeeds but the EC2 instance launch fails
(`InsufficientInstanceCapacity` or quota error) and the benchmark never starts.
