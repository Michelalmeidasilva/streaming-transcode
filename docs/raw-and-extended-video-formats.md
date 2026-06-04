# Raw and extended source formats (.mkv, .y4m, .yuv)

## Motivation
The pipeline accepted only container/self-describing formats. We extended the
accepted set to include Matroska (`.mkv`), YUV4MPEG2 (`.y4m`) and headerless raw
YUV (`.yuv`). `.mkv` and `.y4m` were already in the transcoder allowlist and are
handled natively by ffmpeg/ffprobe. `.yuv` is the hard case and is the focus of
this document.

## Why `.yuv` needs special handling
A `.yuv` file is a flat dump of pixel planes with **no header**: it does not
encode resolution, frame rate or pixel format. Consequently:

- `ffprobe -show_streams` returns empty geometry — `Probe()` cannot be used.
- `ffmpeg -i source.yuv ...` fails because ffmpeg cannot guess how to demux the
  bytes.

The geometry therefore has to be supplied **out of band** (at upload time) and
threaded through to ffmpeg as *demuxer* options that must appear **before** `-i`.

## Design
- `domain.RawVideoParams { Width, Height, FPS, PixelFormat }` and
  `UploadCompletedEvent.RawVideo *RawVideoParams`.
- `worker/parser.go` accepts `.yuv` but requires `rawVideo` with positive
  width/height/fps; `PixelFormat` defaults to `yuv420p`
  (`domain.DefaultRawPixelFormat`).
- `transcode.rawInputArgs(source, raw)` returns the input flags:
  - container/self-describing: `-i <source>`
  - raw: `-f rawvideo -pix_fmt <pf> -s <W>x<H> -framerate <fps> -i <source>`
- `TranscodeRendition` and `ExtractThumbnail` take `*RawVideoParams` and use
  `rawInputArgs` so both encoding and the poster frame demux the raw stream
  correctly.
- For raw sources, `MediaInfo` is **synthesized** from `RawVideoParams`
  (`probeSource`) instead of calling ffprobe; duration/bitrate stay zero (unknown
  without a container).

## Data flow (where the geometry comes from)
```
upload UI (.yuv) → rawVideo on upload.started
   → ingest persists rawVideo on the video record
   → storage webhook (ObjectCreated) → ingest attaches rawVideo to
     video.upload.completed
   → transcode consumes the event → rawInputArgs → ffmpeg
```

## Caveats
- The operator must enter correct geometry; a wrong size produces corrupted
  output (ffmpeg cannot detect the mismatch in a headerless stream).
- Raw uploads are large (uncompressed). The 5 GB upload cap still applies.
- `.mkv` may embed subtitle/multiple tracks; only the first video/audio stream is
  used by the existing rendition logic.
