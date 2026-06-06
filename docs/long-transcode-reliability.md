# Long-transcode reliability (timeouts, broker, telemetry)

## Motivation
A 70-minute 4K HDR (H.265) source uploaded through the platform failed to
transcode. The worker crashed ~1h into the job and the video stayed stuck at
`processingStatus: transcoding`. Investigation found three independent failure
modes that only surface on long-running encodes, plus an operational pitfall.

## Failure modes and fixes

### 1. Job context timeout SIGKILLs ffmpeg
`Processor.HandleDelivery` wraps each job in
`context.WithTimeout(ctx, JobTimeout)` where `JobTimeout =
TRANSCODE_JOB_TIMEOUT_SECONDS` (default **3600s**). Every `ffmpeg`/`ffprobe`
invocation runs under that context (`exec.CommandContext`), so when the deadline
fires the child process is SIGKILLed → ffmpeg fails with `signal: killed` and the
job errors out. A multi-rendition 4K/HDR encode needs far more than 1h.

**Fix:** default raised to **10800s (3h)** in `streaming-transcode/.env`
(`TRANSCODE_JOB_TIMEOUT_SECONDS=10800`). Tune per environment/source length.

### 2. RabbitMQ consumer_timeout closes the channel
The worker consumes with `Qos(1)` and processes the delivery **synchronously**,
acking only after the whole transcode completes. RabbitMQ 3.13 enforces a default
**`consumer_timeout` of 30 minutes**: a delivery left unacked longer is treated as
a stuck consumer, the channel is force-closed and the message requeued. The worker
then sees `delivery channel closed`, fails to publish, and exits.

**Fix:** `infra/rabbitmq/rabbitmq.conf` sets `consumer_timeout = 10800000` (3h),
mounted into `/etc/rabbitmq/conf.d/`. Keep it ≥ `TRANSCODE_JOB_TIMEOUT_SECONDS`.

### 3. Telemetry exporter floods logs
`otel.Init` always built OTLP gRPC exporters pointing at
`OTEL_EXPORTER_OTLP_ENDPOINT` (default `localhost:4317`). With no collector
running, the periodic reader logged an export failure every interval, burying the
real ffmpeg output.

**Stop-gap:** `OTEL_SDK_DISABLED=true` was added to `streaming-transcode/.env` as a
manual environment-level stop-gap — `otel.Init` itself was never modified to check
this flag. The underlying issue was resolved properly by the CloudWatch EMF migration
(2026-06-06), which removed the OTel SDK entirely (`internal/otel/` deleted).

## Operational pitfall: RabbitMQ node identity
RabbitMQ stores durable queues/messages under a directory keyed by node name
(`rabbit@<hostname>`). The container hostname defaulted to the random container id,
so **every `docker compose` recreate started a fresh node** and orphaned the
queued jobs in the data volume. `infra/docker-compose.yml` now pins
`hostname: rabbitmq` so the node is always `rabbit@rabbitmq` and survives recreates.

## Caveat: sidecar subtitles must exist before transcode
`processSubtitles` downloads each subtitle referenced on the `upload.completed`
event and treats a missing object as a **terminal** failure (`subtitle_failed`).
The upload client uploads `.srt` sidecars best-effort, so a dropped subtitle PUT
will fail the transcode. Ensure every `subtitles[].objectKey` is present in storage
(e.g. `subtitles/<videoId>/<lang>.srt`) before publishing the event.

## Shedding load via flags (temporary relief)
When the host cannot finish the full profile in time, two env flags reduce the
work without code changes (revert when capacity allows):
- `TRANSCODE_CODECS=h264` — drop the slow `h265` (libx265) passes; on a 4K source
  these are the dominant cost.
- `TRANSCODE_MAX_HEIGHT=1080` — cap the ladder so no rendition exceeds 1080p
  (`transcode.CapRenditionsByHeight`). 0 means uncapped.

Together they turn a 4K `h264+h265` job (h264/h265 × 1080p/720p = 4 renditions,
two of them 4K-decode libx265) into a light `h264` 1080p+720p ladder.

## Resource note
4K/HDR H.265 → multi-rendition (h264 + h265) HLS/DASH on a CPU-only, memory-shared
Docker VM is heavy (hours of wall time, multiple GB of RAM for libx265). Raising
the timeouts removes the artificial kill, but the host must still have the CPU/RAM
and time to finish, or the encode can be OOM-killed. For pipeline validation prefer
a short SD/HD clip.
