# Changelog

## [Unreleased] 2026-06-04
### Added
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
