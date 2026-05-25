# Streaming Transcode - Implementation Checkpoints

Date: 2026-05-09

## Checkpoint 1 - Roadmap Intake

Status: completed

What was reviewed:

- `PENDING-IMPLEMENTATION-ROADMAP.md`
- current worker code
- current queue/storage/event abstractions
- current test coverage

Decision:

- Implement the production-hardening items first:
  - event contract documentation
  - stronger source identity/idempotency
  - retry/backoff/DLQ topology
  - structured progress/status events
- Keep benchmark codecs, VMAF/SSIM/PSNR, GPU/cloud benchmarking, and full UI playback as explicit future checkpoints because they require larger cross-service and runtime dependency work.

Progress:

- Started implementation in the `streaming-transcode` worker and docs.

Challenges:

- The roadmap includes multiple project phases. This execution focuses on production worker hardening rather than mixing benchmark research features into the serving pipeline.

## Checkpoint 2 - Production Worker Hardening

Status: completed

Implemented:

- Added source identity fields:
  - `sourceETag`
  - `sourceVersion`
  - deterministic processing `fingerprint`
- Added retry/dead queue configuration:
  - `TRANSCODE_RETRY_QUEUE`
  - `TRANSCODE_DEAD_QUEUE`
  - `TRANSCODE_MAX_ATTEMPTS`
  - `TRANSCODE_RETRY_DELAY_SECONDS`
- Added structured progress events:
  - `transcode.queued`
  - `transcode.progress`
- Added upload-state patches for:
  - `queued`
  - `transcoding`
  - `packaging`
  - `ready`
  - `failed`
- Added retry/DLQ RabbitMQ behavior:
  - transient failures go to `transcode.retry`
  - terminal failures and exhausted attempts go to `transcode.dead`
- Updated `infra/docker-compose.yml` with retry/dead queue environment variables.
- Added `EVENT-CONTRACTS.md`.

Validation:

- `go test ./...` passed after worker and queue updates.

Challenges:

- Retry queue delay uses RabbitMQ TTL plus dead-lettering back to the main exchange. This is operationally simple, but future per-attempt exponential delays may need multiple retry queues or delayed-message plugin support.

## Checkpoint 3 - Contracts and Source Key Normalization

Status: completed

Implemented:

- Updated `streaming-ingest/swagger.yaml` with transcode lifecycle event names and payload fields.
- Added `streaming-ingest` contract tests for routing keys:
  - `video.transcode.queued`
  - `video.transcode.started`
  - `video.transcode.progress`
  - `video.packaging.completed`
  - `video.transcode.completed`
  - `video.transcode.failed`
  - `video.ready`
- Added optional canonical raw upload layout in `streaming-platform-upload`:
  - current default remains `{videoId}/{filename}`
  - setting `UPLOAD_RAW_PREFIX_ENABLED=true` writes `raw/{videoId}/{filename}`
- Expanded upload-side types with source `etag`, source `version`, and transcode `fingerprint`.
- Updated worker README with retry/dead queue environment variables.

Validation:

- Pending full cross-project test run after these edits.

Challenges:

- The upload service cannot always provide a stable source ETag today because multipart/direct upload adapters return different values. The worker and types now support ETag/version, but complete source-version propagation still needs adapter-level work.

## Checkpoint 4 - Test Coverage and Validation

Status: completed

Implemented:

- Added focused unit tests for terminal worker errors.
- Expanded RabbitMQ retry/DLQ tests for:
  - channel initialization failure
  - retry attempt parsing
  - terminal errors routed to `transcode.dead`
  - max-attempt failures routed to `transcode.dead`
- Raised `streaming-transcode` package coverage above the 80% target.

Validation:

- `streaming-transcode`: `go test ./...` passed.
- `streaming-transcode`: `go test ./internal/... -coverprofile=coverage.out -covermode=atomic` passed with total coverage `81.7%`.
- `streaming-ingest`: `go test ./...` passed.
- `streaming-platform-upload`: `npm test -- --runInBand src/lib/services/__tests__/UploadService.test.ts` passed.
- `streaming-platform-upload`: `npx tsc --noEmit` passed.
- `infra`: `docker compose config --quiet` passed.

Challenges:

- Local sandbox blocked Go build cache access and local `httptest` sockets. The final Go validations were rerun with elevated local permissions.
- The upload unit test suite still prints an expected `console.error` from the existing thumbnail failure-path test; the suite passes.

## Checkpoint 5 - Remaining Roadmap Items

Status: pending for future implementation

Not implemented in this pass:

- Full benchmark matrix for codec ladders, bitrate/quality trade-offs, and runtime comparisons.
- Objective quality metrics such as VMAF, SSIM, and PSNR.
- GPU/cloud transcoding comparison and cost benchmark.
- End-to-end playback UI validation against generated HLS/DASH manifests.
- Adapter-level propagation of stable source ETag/version from every upload path.
- Per-attempt exponential backoff. Current retry behavior uses one RabbitMQ retry queue with fixed TTL.

Recommended next plan:

- Add adapter-level source identity propagation in `streaming-platform-upload`.
- Add a benchmark harness in `streaming-transcode` that can run FFmpeg profiles against fixture videos and persist metrics.
- Add VMAF/SSIM/PSNR tooling behind optional runtime dependencies so normal CI remains lightweight.
- Add an end-to-end smoke test using Docker Compose: upload raw video, emit event, process transcode, validate manifests, and verify upload state.
