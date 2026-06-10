# Changelog

## [Unreleased] 2026-06-10
### Added
- cmd/benchmark: encode-only codec benchmark over an S3 corpus (matrix codec×resolution×clip×repeat); posts results to ingest /benchmark-runs; machine label from IMDS. Storage gains List.

## [Unreleased] 2026-06-10
### Added
- `make image-push-multiarch` — builds and pushes a multi-arch (amd64+arm64) `vod-transcode` image to ECR (override `PLATFORMS`/`IMAGE_TAG`/`AWS_REGION`/`ECR_REPO`). Enables Graviton benchmark runs while keeping the amd64 Fargate path on the same tag.

## [Unreleased] 2026-06-09
### Added
- TRANSCODE_MACHINE_LABEL env var; jobs now tag observability and the transcode.completed event with a machine label for benchmark runs (falls back to hostname).

## [Unreleased] 2026-06-09
### Added
- Guard de upscaling: `ResolveRenditions` descarta renditions acima da altura da fonte; fallback de 1 rendition por codec na altura da fonte. Mantém retrocompat: evento sem `transcode` usa os defaults.

## [Unreleased] 2026-06-08 — worker: lifecycle event publish is best-effort (não falha o job)
### Fixed
- Jobs do Batch reportavam `FAILED` (exit 1) mesmo com a saída transcodada **completa e
  servindo** no catálogo. Causa: em `internal/worker/processor.go`, `complete()` tratava o
  evento `ready` (`markReady` → `POST /api/v1/events`) como fatal, e o atalho "already
  transcoded" também retornava esse erro. Quando o gateway de ingest retornava **500** ao
  publicar no RabbitMQ (`POST /events`), o `PATCH /upload-state/videos/:id` (que marca
  `ready` no Mongo) **funcionava** — o vídeo aparecia no catálogo — mas o job saía com erro.
  Agora os **eventos** de ciclo de vida são best-effort (logados, não fatais); o sucesso do
  job é decidido pela produção da mídia + o PATCH `ready`. Ver `docs/event-publish-nonfatal.md`.
  Imagem rebuildada e publicada no ECR (`vod-transcode:latest`).

## [Unreleased] 2026-06-08 — Dockerfile: CMD instead of ENTRYPOINT for Batch
### Fixed
- AWS Batch job failed with `rabbitmq init failed: connect rabbitmq: dial tcp 127.0.0.1:5672: connection refused` (exit 1). The Dockerfile pinned `ENTRYPOINT ["streaming-transcode"]` (the RabbitMQ worker), but the Batch job definition sets `command = ["transcode-local", "<s3-key>"]`. In Docker, `command` is **appended** to `ENTRYPOINT`, so Batch actually ran `streaming-transcode transcode-local <key>` — the RabbitMQ worker, which dies trying to reach a non-existent broker. Changed to `CMD ["streaming-transcode"]` so the Batch `command` **replaces** the default and runs the `transcode-local` binary directly (both binaries are on `PATH`). Verified E2E: S3 `raw/` → EventBridge → Batch → ffmpeg → shaka-packager → DASH segments uploaded to `transcoded/`. Dev worker behavior unchanged (CMD default).

## [Unreleased] 2026-06-07 — Source download uses the event's full objectKey
### Fixed
- The dev RabbitMQ→worker path failed to download `raw/`-prefixed sources (`The specified key does not exist`): `streaming-ingest` published only `videoId`/`filename` (basename), so `resolveObjectKey` rebuilt `<videoId>/<filename>` and dropped `raw/`. Fixed in `streaming-ingest` by publishing the full `objectKey`; the worker already prefers `event.ObjectKey`, so it now downloads the exact key. No code change here — `SPEC.md` clarified (step 1) that the source key comes from the event. See `streaming-ingest/docs/object-key-preservation.md`.

## [Unreleased] 2026-06-07 — Batch entrypoint (transcode-local)
### Added
- `cmd/transcode-local` now doubles as the **AWS Batch job entrypoint**. Invoked with a single positional S3 key (`transcode-local raw/<videoId>/<object>`), it derives the `videoId`, rebuilds a minimal `UploadCompletedEvent`, and runs the existing `worker.Processor` pipeline. Exit 0 = job SUCCEEDED, exit ≠0 = FAILED (reprocessable). The flag-based local-file mode (`--input/--output`) is preserved; a positional argument selects Batch mode. New `batch.go` (`extractVideoID`, `buildBatchEvent`, `batchJobID`, `runBatchJob`, `runBatchMode`) with unit tests.
### Notes
- Persistence stays via the Event Gateway (`PATCH /api/v1/upload-state/videos/:id`), not direct MongoDB — `streaming-ingest` remains the single writer of the `videos` collection. Batch mode does not advertise sidecar subtitles and rejects headerless `.yuv` (no geometry in the S3 event).

## [Unreleased] 2026-06-07
### Changed
- Packaged output upload (`uploadDir`) is now bounded-parallel (`maxUploadConcurrency = 8`) instead of one file at a time. HLS produces many small segments per rendition; sequential upload dominated job wall-clock. The first upload error cancels remaining in-flight uploads and is returned.

## [Unreleased] 2026-06-06 — CloudWatch EMF telemetry
### Changed
- Telemetry now emits CloudWatch EMF to stdout (job-level metrics per transcode job: JobCount, JobDuration, FailureCount).
### Removed
- OTel SDK push pipeline (`internal/otel/`, `otel.Init`, the per-job OTel span). No `/metrics` endpoint existed.

## [Unreleased] 2026-06-06
### Added
- Explicit object-storage provider selection via `STORAGE_PROVIDER` (`minio` |
  `aws-s3`), matching the factory pattern already used by `streaming-ingest` and
  `streaming-distribution`. `storage.New(cfg)` now dispatches to `NewMinIOStorage`
  or `NewS3Storage`; the latter targets `s3.<AWS_REGION>.amazonaws.com` over TLS
  and reads `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`.
### Changed
- The worker no longer hard-wires `storage.NewMinIOStorage`; `cmd/worker/main.go`
  calls `storage.New(cfg.Storage)`. The previously dead `STORAGE_PROVIDER` /
  `Storage.Provider` config value is now honored — running against real S3 is a
  config change, not a code change.

## [Unreleased] 2026-06-04
### Fixed
- Long encodes (e.g. a 70-min 4K HDR source) no longer die mid-job. Three causes
  were addressed:
  - **Job timeout too short.** `HandleDelivery` wraps the job in a context with
    `TRANSCODE_JOB_TIMEOUT_SECONDS` (default 3600s). ffmpeg runs under that context,
    so at 1h it was SIGKILLed (`signal: killed`) mid-encode. Default raised to
    10800s (3h) via `streaming-transcode/.env`.
  - **Broker consumer_timeout.** RabbitMQ 3.13 force-closes a channel whose
    delivery is unacked for 30 min (the worker holds the delivery for the whole
    synchronous transcode), requeuing the message and crashing the worker with
    "delivery channel closed". Raised `consumer_timeout` to 3h via
    `infra/rabbitmq/rabbitmq.conf`.
  - **Telemetry log flood.** With no OTLP collector on `:4317`, the OTel periodic
    exporters failed every interval and flooded the logs. `OTEL_SDK_DISABLED=true`
    was added to `.env` as a manual stop-gap (`otel.Init` was not modified to read
    it). Resolved properly by the 2026-06-06 CloudWatch EMF migration, which removed
    the OTel SDK entirely.
- Missing sidecar subtitle made the job fail terminally: `processSubtitles`
  downloads each referenced `.srt` and errors if absent. Documented that the
  subtitle object must exist at its `objectKey` before the transcode runs.

### Added
- `TRANSCODE_MAX_HEIGHT` env flag caps the output ladder to renditions no taller
  than the given height (0 = uncapped). Combined with `TRANSCODE_CODECS=h264` it
  lets operators temporarily shed heavy renditions — e.g. limit a 4K HDR source to
  a single-codec 1080p ladder — without editing the profile. Applied via
  `transcode.CapRenditionsByHeight` after rendition resolution.
- Headerless raw video (`.yuv`) is now an accepted source format. Because raw
  streams carry no container/geometry, the upload-supplied `rawVideo`
  (`width`, `height`, `fps`, `pixelFormat`) on the event is required and fed to
  ffmpeg as demuxer options (`-f rawvideo -pix_fmt <pf> -s <W>x<H>
  -framerate <fps>`) before `-i`. ffprobe is skipped for raw sources and
  `MediaInfo` is synthesized from the supplied geometry instead.
- `domain.RawVideoParams` and `UploadCompletedEvent.RawVideo`; parser validates
  that `.yuv` events include positive width/height/fps (pixel format defaults to
  `yuv420p`).
- `cmd/transcode-local` gained `--raw-width/--raw-height/--raw-fps/--raw-pixfmt`
  flags so `.yuv` can be transcoded locally.
- Sidecar subtitles (`.srt`) are now packaged. `UploadCompletedEvent.Subtitles`
  carries the source object keys; the processor converts each to WebVTT
  (`Runner.ConvertSRTToVTT`, `ffmpeg -c:s webvtt`), writes a per-language HLS
  media playlist under `transcoded/<id>/hls/subtitles/<lang>/`, advertises the
  `#EXT-X-MEDIA:TYPE=SUBTITLES` group in the HLS master (each variant gains
  `SUBTITLES="subs"`), and includes the tracks in the playback metadata
  (`domain.SubtitleTrack`). A subtitle that cannot be produced fails the job.

### Changed
- `TranscodeRunner.TranscodeRendition` and `ExtractThumbnail` now take an
  optional `*domain.RawVideoParams` so raw demuxer options are applied to both
  encoding and thumbnail extraction. `.mkv` and `.y4m` were already accepted and
  remain handled natively by ffmpeg.

## [Unreleased] 2026-06-03
### Added
- `Runner.ExtractThumbnail(ctx, source, output, atSeconds)` — extracts a single
  poster frame via ffmpeg (`-ss <t> -i <src> -frames:v 1 -vf scale=640:-2 -q:v 3
  -y <out>`), scaling to 640px wide with preserved aspect ratio.
- Processor now extracts a thumbnail before starting rendition transcoding: probes
  source duration, picks `max(1s, duration*0.1)` as the seek offset, uploads the
  JPEG to `thumbnails/<videoId>.jpg` in the bucket, and PATCHes the ingest record
  with `thumbnail_status: "ready"`. Any failure is logged and silently skipped —
  the transcode job continues regardless.

## [Unreleased] 2026-05-31 — HLS master audio-codec fix
### Fixed
- `WriteHLSMaster` no longer hardcodes the AAC audio codec (`mp4a.40.2`) in the
  HLS multivariant playlist. It now takes a `hasAudio bool` and advertises
  `mp4a.40.2` only when an audio track is present (`processor` passes
  `info.AudioCodec != ""`). Advertising audio on video-only (e.g. muted) sources
  made players initialize an MSE audio SourceBuffer and then fail the append
  (Shaka error 3014 = MEDIA_SOURCE_OPERATION_FAILED), breaking HLS playback.
### Tests
- `internal/transcode/runner_test.go`: asserts the master includes `mp4a.40.2`
  when `hasAudio=true` and omits any `mp4a` when `hasAudio=false`.
### Verified
- Re-transcoded two dataset videos: a muted MP4 → master `CODECS="avc1.640028"`
  (no audio); a WebM with audio → `CODECS="avc1.640028,mp4a.40.2"`. Both play in
  Shaka Player via HLS (currentTime advances; no 3014).
