# Streaming Transcode - Pending Implementation Roadmap

Date: 2026-05-09

## 1. Objective

This document maps all known items that are still not fully implemented after the current MVP, based on:

- `streaming-transcode/SPEC-V2.md`
- `streaming-transcode/SPEC.md`
- `streaming-transcode/IMPLEMENTATION-VALIDATION.md`

It also defines a practical implementation plan to close those gaps.

## 2. Current Baseline

Already implemented:

- worker consuming `video.upload.completed`
- MinIO/S3-compatible storage download/upload
- `ffprobe` metadata extraction
- H.264/AAC `1080p` and `720p` renditions
- HLS output
- DASH output
- metrics JSON upload
- status publication through `streaming-ingest`
- `upload-state` patch to ready/error
- Docker image and Compose integration
- CI pipeline and unit test coverage above 80%

The remaining gaps are not generic polish. They are real product, resilience, benchmark, and contract items still open.

## 3. Pending Items

## 3.1 Event Contracts and API Documentation

Status: implemented for the current production contract

What exists:

- worker publishes events through `POST /api/v1/events`
- worker patches state through `PATCH /api/v1/upload-state/videos/{videoId}`
- ingest persists extra V2 fields
- `streaming-ingest/swagger.yaml` documents transcode lifecycle fields
- `EVENT-CONTRACTS.md` documents routing keys, payloads, retry, and DLQ behavior
- contract tests validate transcode lifecycle routing keys

What is missing:

- explicit schema versioning or compatibility notes for worker payloads

Impact:

- integration consumers now have the current contract documented
- future schema evolution still needs explicit versioning

Implementation plan:

1. Freeze a V2 payload schema and document fallback behavior for legacy fields.
2. Add schema-version fields if multiple consumers begin depending on incompatible payload shapes.

## 3.2 Upload Source Key Normalization

Status: migration support implemented, full adapter propagation pending

What exists:

- worker accepts current `${videoId}/${filename}` layout
- worker supports `objectKey` fallback from `filename`
- upload can optionally write to `raw/{videoId}/{filename}` with `UPLOAD_RAW_PREFIX_ENABLED=true`
- upload, ingest, and transcode types support source `etag` and `version`

What is missing:

- enabling canonical `raw/{videoId}/{originalName}` as the default storage layout
- adapter-level stable propagation of source `etag` or `version` from every upload path

Impact:

- idempotency remains weaker than required
- source storage layout is inconsistent with the intended V2 model

Implementation plan:

1. Promote `raw/{videoId}/{filename}` to the default once migration is accepted.
2. Finish ETag/version propagation inside every upload adapter.
3. Keep worker backward-compatible with current key layout during migration.

## 3.3 Strong Idempotency

Status: partially implemented

What exists:

- worker avoids reprocessing when HLS master already exists
- worker calculates a deterministic fingerprint from:
  - `videoId`
  - profile
  - source key
  - source ETag
  - source version
- worker persists fingerprint and source identity in upload-state patches and result metrics

What is missing:

- persisted processing ledger
- safe distinction between:
  - same source re-delivery
  - new source version for same video

Impact:

- duplicate events are tolerated only in the simplest case
- source updates can be misclassified

Implementation plan:

1. Add a persisted processing ledger.
2. Skip reprocessing only when ledger fingerprint matches.
3. Reprocess cleanly when source changes under same `videoId`.

## 3.4 Retry, Backoff, and DLQ

Status: implemented with fixed-delay retry

What exists:

- on processing failure the worker publishes `transcode.failed`
- RabbitMQ topology declares `transcode.jobs`, `transcode.retry`, and `transcode.dead`
- retry attempts are counted in `x-transcode-attempt`
- terminal errors and exhausted attempts are routed to `transcode.dead`
- transient failures are routed to `transcode.retry`

What is missing:

- exponential backoff with per-attempt delay
- redrive procedure documentation

Impact:

- retry and poison-message behavior is explicit
- delay is currently fixed by retry queue TTL

Implementation plan:

1. Add exponential retry support if fixed TTL becomes insufficient.
2. Add redrive procedure documentation.
3. Add alerts for DLQ growth.

## 3.5 Structured Progress Reporting

Status: implemented for worker-side lifecycle

What exists:

- worker publishes `transcode.started`
- worker publishes `packaging.completed`
- worker publishes `transcode.completed`
- worker publishes `ready`
- worker publishes `transcode.queued`
- worker publishes `transcode.progress`
- worker patches upload-state through `queued`, `transcoding`, `packaging`, `ready`, and `failed`
- worker includes progress percentages and phase names

What is missing:

- richer phase-level duration analytics for dashboards

Impact:

- UI can consume worker-side progress/status fields
- dashboard-grade duration analytics are still shallow

Implementation plan:

1. Add phase duration analytics for dashboards.
2. Add alerts or traces for jobs stuck in a phase.

## 3.6 UI Lifecycle Integration

Status: partially implemented

What exists:

- upload service no longer marks ready by default
- types support `queued`, `transcoding`, `packaging`, `ready`, `failed`
- upload area recognizes those states

What is missing:

- real event-driven transition handling from transcode events
- subscription/update model for queued/transcoding/packaging
- display of generated HLS/DASH manifest paths
- playback flow using generated manifests

Impact:

- UI model is ahead of the actual live integration
- users still mainly observe `processing -> ready`

Implementation plan:

1. Decide event delivery mechanism to front-end:
   - polling via upload-state
   - SSE
   - websocket
2. Map transcode events to UI state transitions.
3. Update upload list and detail components to show:
   - `queued`
   - `transcoding`
   - `packaging`
   - `ready`
   - `failed`
4. Surface playback info from `playback.hlsManifestPath` and `dashManifestPath`.
5. Add integration tests for real status progression.

## 3.7 Playback Delivery Experience

Status: partially implemented

What exists:

- HLS and DASH files are generated and stored
- playback metadata is patched into the video document

What is missing:

- actual player integration for generated manifest playback
- CDN or direct manifest URL resolution strategy
- manifest URL exposure policy in APIs

Impact:

- assets are produced but not fully exposed as a complete playback product

Implementation plan:

1. Decide whether playback uses:
   - signed manifest URLs
   - proxy endpoint
   - direct object URLs
2. Update ingest/video APIs to expose playback URLs safely.
3. Update UI playback modal/player to use HLS first and DASH fallback if needed.
4. Validate browser compatibility and auth behavior.

## 3.8 Packaging Tooling Depth

Status: partially implemented

What exists:

- HLS and DASH are generated with FFmpeg
- HLS uses fMP4 segmenting

What is missing:

- dedicated Shaka Packager integration
- explicit CMAF validation rules
- packaging strategy comparison

Impact:

- current packaging is good enough for MVP
- long-term interoperability and packaging control are still limited

Implementation plan:

1. Evaluate whether FFmpeg-only packaging remains sufficient.
2. If not, add Shaka Packager as an optional packaging stage.
3. Add CMAF compatibility validation to output checks.
4. Add packaging regression tests using sample assets.

## 3.9 Benchmark Mode

Status: not implemented

What exists:

- only production profile `production-h264-hls-dash`

What is missing:

- separate benchmark queue
- benchmark request workflow
- benchmark result storage
- matrix execution engine

Impact:

- `SPEC.md` research scope is still pending

Implementation plan:

1. Add benchmark queue:
   - `transcode.benchmark.jobs`
   - routing key `video.benchmark.requested`
2. Create benchmark job schema:
   - source video
   - codec
   - target resolution
   - quality mode
   - preset
3. Implement benchmark worker mode separated from production path.
4. Persist benchmark outputs under:
   - `metrics/benchmark/{runId}/...`
5. Add benchmark summary generators:
   - jsonl
   - csv

## 3.10 Additional Codecs

Status: not implemented

Missing codecs:

- H.265
- VP9
- AV1
- H.266/VVC

Impact:

- benchmark and codec recommendation goals from `SPEC.md` are blocked

Implementation plan:

1. Introduce profile abstraction for encoder strategy.
2. Add production-independent benchmark encoder profiles:
   - `benchmark-h265`
   - `benchmark-vp9`
   - `benchmark-av1`
   - `benchmark-vvc`
3. Validate container/runtime dependencies for each encoder.
4. Gate VVC/H.266 behind dedicated research builds if operational cost is too high.

## 3.11 Quality Metrics

Status: not implemented

Missing metrics:

- VMAF
- SSIM
- PSNR

Impact:

- current metrics describe processing, not perceptual quality
- codec comparisons cannot be defended rigorously

Implementation plan:

1. Add optional benchmark-only quality analysis stage.
2. Install/build dependencies for `libvmaf`.
3. Compute and store:
   - VMAF
   - SSIM
   - PSNR
4. Include metrics in benchmark result schema.
5. Add threshold and comparison reports per codec/resolution.

## 3.12 Cost and Performance Benchmarking

Status: not implemented

Missing areas:

- CPU benchmark matrix
- GPU benchmark matrix
- EC2 profile comparison
- local energy/amortization model
- throughput and queue capacity planning

Impact:

- `SPEC.md` cost/performance decision goals remain unresolved

Implementation plan:

1. Define benchmark result schema for:
   - elapsed time
   - RTF
   - CPU
   - GPU
   - RAM
   - I/O
2. Add host metrics collection during benchmark jobs.
3. Add run metadata for machine type and environment.
4. Build scripts to compare:
   - local
   - cloud CPU
   - cloud GPU
5. Generate a recommendation matrix by codec and workload.

## 3.13 AWS Operationalization

Status: partially implemented at infra planning level only

What exists:

- Terraform for storage and IAM around S3

What is missing:

- compute for transcode workers
- GPU-capable benchmark environment
- managed broker strategy
- managed Mongo strategy
- IAM role for worker runtime
- deployment topology

Impact:

- local pipeline exists, but production cloud rollout is not complete

Implementation plan:

1. Choose runtime:
   - ECS
   - EC2 autoscaling
   - AWS Batch
2. Create worker IAM permissions.
3. Define storage, broker, and database endpoints by environment.
4. Add deployment manifests and environment configuration.
5. Add cloud smoke validation flow.

## 3.14 Observability and Operations

Status: partially implemented

What exists:

- worker logs
- basic metrics JSON output

What is missing:

- structured logs by `jobId`
- dashboard-friendly metrics
- alerts for repeated failures
- DLQ alerts
- benchmark reporting dashboards

Impact:

- operational visibility is still shallow

Implementation plan:

1. Standardize structured logs with:
   - `videoId`
   - `jobId`
   - profile
   - attempt
   - phase
2. Export machine-readable counters and timings.
3. Add alerts for:
   - failed jobs
   - repeated retries
   - DLQ growth
4. Add runbook for common failure types.

## 4. Recommended Implementation Phases

## Phase 1 - Contracts and State Accuracy

Goal:

- stabilize the system contract before adding more processing complexity

Deliverables:

- Swagger/OpenAPI transcode contract update
- routing key documentation
- source key normalization plan
- explicit `queued/transcoding/packaging/ready/failed` state model

Exit criteria:

- all worker-facing contracts are documented
- UI and ingest share the same status vocabulary

## Phase 2 - Resilience and Idempotency

Goal:

- make the current production worker safe for repeated events and failures

Deliverables:

- stronger idempotency fingerprint
- retry queue
- DLQ
- attempt management
- transient vs terminal error classification

Exit criteria:

- duplicate events are safe
- poison messages do not loop indefinitely

## Phase 3 - Full UI and Playback Integration

Goal:

- expose real transcode lifecycle and playback value to users

Deliverables:

- event-driven or polling-driven status updates
- manifest path exposure
- HLS playback integration
- end-to-end UI validation

Exit criteria:

- user can observe real states
- ready videos can be played from generated outputs

## Phase 4 - Packaging and Validation Hardening

Goal:

- make packaging behavior robust and standards-aware

Deliverables:

- CMAF validation
- optional Shaka Packager evaluation/integration
- output verification suite

Exit criteria:

- packaging outputs are validated, not only generated

## Phase 5 - Benchmark Platform

Goal:

- implement the research/decision platform described in `SPEC.md`

Deliverables:

- benchmark queue
- codec matrix execution
- result schemas
- benchmark storage layout

Exit criteria:

- benchmark jobs can run independently from production jobs

## Phase 6 - Advanced Codec and Quality Analysis

Goal:

- collect evidence for codec and cost decisions

Deliverables:

- H.265
- VP9
- AV1
- H.266/VVC
- VMAF
- SSIM
- PSNR

Exit criteria:

- codec comparisons are data-backed

## Phase 7 - Cloud Benchmark and Production Rollout

Goal:

- complete the local-to-cloud path

Deliverables:

- cloud worker topology
- IAM/runtime configs
- EC2/GPU benchmark runs
- cost/performance reports

Exit criteria:

- production and benchmark deployment strategy is defined and validated

## 5. Suggested Priority Order

Highest priority:

1. contracts and API documentation
2. stronger idempotency
3. retry/backoff/DLQ
4. UI real lifecycle integration

Medium priority:

5. playback UX
6. packaging validation
7. structured observability

Later priority:

8. benchmark queue and matrix
9. additional codecs
10. quality metrics
11. cloud cost/performance benchmarking

## 6. Final Recommendation

Do not start with benchmark codecs next.

The technically correct next step is to harden the current production worker first:

- document the contract
- make retries and DLQ explicit
- strengthen idempotency
- finish the real UI lifecycle

After that, the benchmark platform from `SPEC.md` can be added without mixing production reliability work with research work.
