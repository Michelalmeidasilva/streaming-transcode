# HLS master audio-codec advertisement

**Date:** 2026-05-31
**Status:** Fixed

## Problem

`WriteHLSMaster` emitted every `#EXT-X-STREAM-INF` with a fixed
`CODECS="<video>,mp4a.40.2"`, i.e. it always claimed an AAC audio track. For
video-only sources (e.g. muted Pixabay clips) the packaged fMP4 segments contain
no audio. Players (Shaka) then create an MSE audio SourceBuffer based on the
advertised codec and fail the segment append:

```
Shaka Error 3014 (MEDIA_SOURCE_OPERATION_FAILED)
```

This broke HLS playback even though the media itself was valid (DASH, which only
advertised the codecs present, played fine).

## Fix

`WriteHLSMaster(path, renditions, hasAudio bool)` only appends `,mp4a.40.2` when
`hasAudio` is true. The worker derives it from ffprobe:

```go
hasAudio := strings.TrimSpace(info.AudioCodec) != ""
transcode.WriteHLSMaster(filepath.Join(hlsDir, "master.m3u8"), renditions, hasAudio)
```

`MediaInfo.AudioCodec` is populated by `ffprobe.go` from the source's audio
stream and is empty when there is none.

## Result (verified 2026-05-31)

| Source | Audio? | HLS master CODECS |
|---|---|---|
| `beauty-medium-001.mp4` (muted) | no | `avc1.640028` |
| `similar-fashion-rocha-activewear-008.webm` | yes | `avc1.640028,mp4a.40.2` |

Both play in Shaka Player via HLS (currentTime advances, no error 3014).

## Note

`TranscodeRendition` keeps `-c:a aac -b:a 128k`; with no audio input ffmpeg
simply produces a video-only rendition (no forced audio map), so this change is
purely about what the master advertises.
