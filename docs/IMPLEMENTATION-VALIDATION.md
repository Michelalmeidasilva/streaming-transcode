# Streaming Transcode - Implementation Validation and Run Guide

Date: 2026-05-09

## 1. Scope

This document validates the current `streaming-transcode` implementation against:

- `streaming-transcode/SPEC-V2.md`
- `streaming-transcode/SPEC.md`

It also explains:

- how the current solution works
- how to run it locally
- how to validate the pipeline end to end

## 2. Executive Summary

The current implementation delivers the MVP defined by `SPEC-V2.md` for the production profile:

- RabbitMQ consumer for `video.upload.completed`
- object download from MinIO/S3-compatible storage
- `ffprobe` media inspection
- H.264/AAC renditions for `1080p` and `720p`
- HLS output
- DASH output
- metrics JSON upload
- status/event publication back through `streaming-ingest`
- `upload-state` patch for ready/error metadata
- Docker image and Compose service

The full benchmark and research scope from `SPEC.md` is not implemented yet. That includes:

- H.265
- VP9
- AV1
- H.266/VVC
- VMAF
- SSIM
- PSNR
- benchmark queue and matrix execution
- cost/performance comparison across local/cloud/GPU

## 3. Validation Result Against SPEC-V2

### 3.1 Architecture and Contracts

Implemented:

- `streaming-platform-upload` emits `upload.completed` with `videoId`, `filename`, `objectKey`, `originalName`, `provider`, `bucket`.
- `streaming-ingest` publishes to RabbitMQ exchange `video_events`.
- `streaming-transcode` consumes `video.upload.completed`.
- `streaming-transcode` writes outputs to `transcoded/{videoId}/...`.
- `streaming-transcode` writes metrics to `metrics/{videoId}/...`.
- `streaming-transcode` calls `POST /api/v1/events` and `PATCH /api/v1/upload-state/videos/{videoId}`.

Partially implemented:

- Input key layout supports the current `${videoId}/${filename}` format and output layout uses `transcoded/{videoId}` and `metrics/{videoId}`, but the `raw/{videoId}/...` migration described in `SPEC-V2.md` was not implemented.
- HLS is generated using fMP4 segments and DASH is generated via FFmpeg, which is compatible with the initial CMAF direction, but there is no dedicated Shaka Packager integration.

Not implemented:

- benchmark queue `transcode.benchmark.jobs`
- routing key `video.benchmark.requested`
- DLQ and retry queues `transcode.retry` and `transcode.dead`

### 3.2 SPEC-V2 Acceptance Criteria

From section `13. Criterios De Aceitacao V2`:

1. Upload completed generates an event consumable by the worker: implemented.
2. Worker consumes `video.upload.completed` from `video_events`: implemented.
3. Worker runs `ffprobe` and persists `media-info.json`: implemented.
4. Worker generates at least H.264/AAC in 1080p and 720p: implemented.
5. Worker generates HLS master playlist and DASH manifest: implemented.
6. Outputs are written to `transcoded/{videoId}/`: implemented.
7. Worker publishes `video.transcode.started`, `video.transcode.completed`, and `video.ready`: implemented.
8. Ingest updates the video document with manifests, renditions, and metrics: implemented.
9. UI stops using fake ready timer and reflects real status: partially implemented.
10. Failures generate `video.transcode.failed` with traceable reason: implemented.
11. Reprocessing the same event does not duplicate outputs or corrupt metadata: partially implemented.
12. Local pipeline runs with `infra/docker-compose.yml`: implemented.

Reason for partial items:

- Item 9 is partial because the fake auto-ready mode is disabled by default and the upload service remains in `processing`, but the UI still does not implement a complete real-time state model for `queued`, `transcoding`, and `packaging` based on transcode events.
- Item 11 is partial because there is basic idempotency by checking existing output manifest, but not the stronger identity defined in `SPEC-V2.md` (`videoId + profile + sourceKey + sourceETag/version`).

### 3.3 Ingest Requirements

Implemented:

- `PATCH /api/v1/upload-state/videos/{videoId}` exists and is used by the worker.
- persisted upload-state model now includes:
  - `processingStatus`
  - `mediaInfo`
  - `transcode`
  - `playback`
  - `metrics`
- `upload.completed` payload normalization supports `occurredAt` and the storage-related fields.

Partially implemented:

- event contracts exist in code and behavior, but the API contract documentation is not fully aligned.

Not implemented:

- full Swagger/OpenAPI update for the transcode lifecycle events
- official documentation of all routing keys in the ingest API contract

### 3.4 UI Integration Requirements

Implemented:

- upload service no longer marks a video ready by default after a fixed timer
- upload emits richer `upload.completed` metadata
- types now include:
  - `processingStatus`
  - `mediaInfo`
  - `transcode`
  - `playback`
  - `metrics`
- upload UI supports status types for `queued`, `transcoding`, and `packaging`

Partially implemented:

- the front-end model is ready for richer processing states, but the operational UI flow is still centered on `processing` to `ready`
- playback manifest presentation is not surfaced as a complete HLS/DASH delivery UX

Not implemented:

- complete event-driven UI progression across `queued`, `transcoding`, `packaging`
- dedicated HLS/DASH playback experience from the generated manifests

### 3.5 Resilience Requirements

Implemented:

- worker publishes failure event on processing error
- worker patches error state on failure
- worker has basic idempotency on existing HLS manifest

Not implemented:

- retry with backoff
- DLQ
- separate retry queue
- structured attempt management beyond fixed `attempt: 1`
- stronger idempotency using source version/etag

## 4. Validation Result Against SPEC.md

`SPEC.md` is broader than the implemented MVP. It defines a benchmarking and research platform, not only the production worker.

### 4.1 Production-Relevant Parts Implemented

Implemented:

- video processing worker architecture
- storage-based source retrieval
- metadata extraction via `ffprobe`
- rendition planning
- HLS packaging
- DASH packaging
- basic elapsed time and RTF measurement
- Docker-based local execution

### 4.2 Benchmark and Research Scope Not Implemented

Not implemented:

- 30-video benchmark dataset execution
- H.265 encoding
- H.266/VVC with VVenC
- VP9 encoding
- AV1 encoding
- codec comparison matrix
- VMAF
- SSIM
- PSNR
- EC2/GPU benchmark automation
- cost model execution
- benchmark result dataset generation
- benchmark-specific queueing mode

### 4.3 Conclusion Against SPEC.md

The current code is compliant with the production-oriented slice that `SPEC-V2.md` extracted from `SPEC.md`.

It is not compliant with the full benchmark scope of `SPEC.md`, because the benchmark mode was explicitly deferred to future phases.

## 5. Evidence Collected

Validated during this session:

- `go test ./...` in `streaming-transcode`: passed
- `docker compose config --quiet` in `infra`: passed

Previously validated and still consistent with the current codebase:

- Docker image for `streaming-transcode` builds successfully
- Compose service `streaming-transcode` starts successfully
- Docker-backed E2E smoke completed successfully with:
  - upload event publication
  - HLS and DASH output generation
  - metrics file upload
  - `upload-state` patched to `ready`
  - events persisted:
    - `upload.completed`
    - `transcode.started`
    - `packaging.completed`
    - `transcode.completed`
    - `ready`

## 6. How the Current System Works

Current flow:

1. `streaming-platform-upload` uploads the original file to object storage.
2. `streaming-platform-upload` notifies `streaming-ingest` with `upload.completed`.
3. `streaming-ingest` publishes the payload to RabbitMQ exchange `video_events` with routing key `video.upload.completed`.
4. `streaming-transcode` consumes that event from queue `transcode.jobs`.
5. The worker checks whether the HLS manifest already exists.
6. If not already processed, the worker downloads the source object.
7. The worker runs `ffprobe` and extracts media metadata.
8. The worker plans the renditions:
   - `1080p`
   - `720p`
   - or source fallback when upscale is not appropriate
9. The worker transcodes renditions with FFmpeg.
10. The worker packages HLS and DASH outputs.
11. The worker uploads:
   - `transcoded/{videoId}/hls/...`
   - `transcoded/{videoId}/dash/...`
   - `metrics/{videoId}/media-info.json`
   - `metrics/{videoId}/transcode-result.json`
12. The worker publishes lifecycle events through `streaming-ingest`.
13. The worker patches `upload-state` so the video becomes `ready` with playback and metrics metadata.

## 7. How to Run Locally

## 7.1 Prerequisites

- Docker running
- Docker Compose available

Optional for local host execution outside containers:

- Go 1.26+
- FFmpeg
- ffprobe

## 7.2 Run the Shared Infrastructure

From `infra`:

```bash
cd /Users/user/workspace-personal/video-on-demand-arch/microsservices/infra
docker compose up -d --build event-gateway streaming-transcode
```

This ensures the latest local code is used for:

- `event-gateway`
- `streaming-transcode`

and starts:

- MongoDB
- RabbitMQ
- Redis
- MinIO
- Event Gateway
- streaming-transcode

## 7.3 Rebuild the Worker

```bash
cd /Users/user/workspace-personal/video-on-demand-arch/microsservices/infra
docker compose build event-gateway streaming-transcode
docker compose up -d --no-build event-gateway streaming-transcode
```

## 7.4 Check Service Status

```bash
docker compose ps
docker compose logs --tail=100 streaming-transcode
```

Expected worker startup log:

```text
streaming-transcode worker started queue=transcode.jobs binding=video.upload.completed
```

## 8. How to Validate Locally

Two validation paths are recommended.

### 8.1 Validation Through the Real Upload Flow

Use `streaming-platform-upload` and complete an upload normally.

Validate:

- the video remains `processing` after upload completion
- `streaming-ingest` receives `upload.completed`
- RabbitMQ queue `transcode.jobs` is consumed
- MinIO receives:
  - `transcoded/{videoId}/hls/...`
  - `transcoded/{videoId}/dash/...`
  - `metrics/{videoId}/media-info.json`
  - `metrics/{videoId}/transcode-result.json`
- `upload-state` is patched to `ready`

### 8.2 Manual Validation Without the Upload UI

1. Ensure infrastructure is running.
2. Upload a sample file to MinIO.
3. Create or update the corresponding upload-state record.
4. Publish `upload.completed`.
5. Inspect outputs and patched metadata.

Example sequence:

Upload `sample.mp4` to MinIO:

```bash
cd /Users/user/workspace-personal/video-on-demand-arch/microsservices/infra
docker run --rm --network vod-network --entrypoint sh \
  -v /Users/user/workspace-personal/video-on-demand-arch/microsservices/streaming-transcode:/data \
  minio/mc \
  -c 'mc alias set local http://minio:9000 admin password123 >/dev/null && mc cp /data/sample.mp4 local/videos/e2e-sample/sample.mp4'
```

Create the upload-state record:

```bash
curl -sS -X PUT http://127.0.0.1:8080/api/v1/upload-state/videos/e2e-sample \
  -H 'Content-Type: application/json' \
  -d '{"id":"e2e-sample","filename":"e2e-sample/sample.mp4","originalName":"sample.mp4","title":"sample.mp4","size":687391,"status":"processing","progress":100,"createdAt":"2026-05-09T12:00:00Z","updatedAt":"2026-05-09T12:00:00Z","provider":"minio"}'
```

Publish `upload.completed`:

```bash
curl -sS -X POST http://127.0.0.1:8080/api/v1/events \
  -H 'Content-Type: application/json' \
  -d '{"eventType":"upload.completed","payload":{"videoId":"e2e-sample","filename":"e2e-sample/sample.mp4","objectKey":"e2e-sample/sample.mp4","originalName":"sample.mp4","size":687391,"provider":"minio","bucket":"videos"}}'
```

Inspect uploaded outputs:

```bash
docker run --rm --network vod-network --entrypoint sh minio/mc \
  -c 'mc alias set local http://minio:9000 admin password123 >/dev/null && mc ls --recursive local/videos/transcoded/e2e-sample && mc ls --recursive local/videos/metrics/e2e-sample'
```

Inspect resulting state:

```bash
curl -sS http://127.0.0.1:8080/api/v1/upload-state/videos/e2e-sample
```

Expected outcome:

- `status = ready`
- `processingStatus = ready`
- `playback.hlsManifestPath` exists
- `playback.dashManifestPath` exists
- `metrics.metricsPath` exists

## 9. What Is Still Missing

The following gaps remain open relative to the full specs:

- benchmark mode
- additional codecs
- VMAF, SSIM, PSNR
- benchmark queueing model
- retry with backoff
- DLQ
- stronger idempotency key
- complete Swagger/OpenAPI transcode contract
- full UI event-driven transcode lifecycle
- playback UX based on generated manifests

## 10. Final Conclusion

The current implementation is valid as the MVP production worker defined by `SPEC-V2.md`.

It is not a full implementation of `SPEC.md`, because the benchmark, research, resilience, and advanced observability layers remain future work.

For the current codebase, the correct conclusion is:

- `SPEC-V2.md`: mostly implemented, with known partial items around UI richness, idempotency depth, and resilience
- `SPEC.md`: partially implemented only in the production MVP slice, not in the benchmark/research scope
