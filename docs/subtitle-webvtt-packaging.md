# Subtitle packaging (.srt → WebVTT)

## Motivation
Operators upload `.srt` subtitles alongside a video. Players consume WebVTT, so
the transcoder converts each `.srt` and advertises it in the HLS manifest so the
track is selectable.

## End-to-end flow
```
upload (.mp4 + movie.srt) → subtitles[] on upload.started (objectKey/lang/label)
  → ingest persists + attaches subtitles to video.upload.completed
  → transcode: download .srt → ffmpeg -c:s webvtt → .vtt
              → HLS subtitle media playlist per language
              → #EXT-X-MEDIA:TYPE=SUBTITLES in master + SUBTITLES="subs" on variants
              → tracks in playback metadata
  → distribution exposes subtitle URLs
  → web-client Shaka addTextTrackAsync
```

## What this service does
For each `UploadCompletedEvent.Subtitles[i]` (`SubtitleInput{ObjectKey, Language,
Label}`), `processSubtitles`:
1. Downloads the `.srt` from storage.
2. Converts it to WebVTT via `Runner.ConvertSRTToVTT` (`ffmpeg -i in.srt -c:s
   webvtt -f webvtt out.vtt`).
3. Writes a per-language HLS media playlist
   (`BuildSubtitleMediaPlaylist`) into `hlsDir/subtitles/<lang>/index.m3u8`
   pointing at the single `<lang>.vtt` (the whole VTT advertised as one segment
   spanning the program duration).
4. Returns a `domain.SubtitleTrack` (language, label, `VTTPath`, `ManifestPath`,
   `Default`).

`WriteHLSMaster` then emits an `#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",...`
line per track (first/`Default` is `DEFAULT=YES,AUTOSELECT=YES`) and appends
`SUBTITLES="subs"` to every `#EXT-X-STREAM-INF`.

## Design decisions
- **Packaged at transcode time** (subtitles arrive with the upload), so the
  master playlist is written once with the tracks — no post-hoc manifest
  rewrite.
- **Languages** are sanitized (`SanitizeLanguage`) to filename/GROUP-ID-safe
  tokens; duplicate codes are disambiguated (`pt`, `pt-2`).
- **Failure is terminal**: a requested subtitle that cannot be downloaded or
  converted fails the job (`subtitle_failed`) rather than silently shipping a
  video without the track the operator asked for.

## Caveats
- DASH (`manifest.mpd`) is still produced by ffmpeg without an embedded text
  AdaptationSet; DASH playback relies on the player side-loading the WebVTT URL
  (the web-client uses Shaka `addTextTrackAsync`). HLS native players get the
  track from the master playlist.
- The whole VTT is one HLS "segment"; this is standard for VOD sidecar
  subtitles and keeps packaging simple.
