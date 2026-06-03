# Changelog

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
