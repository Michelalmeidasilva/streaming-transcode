# Streaming Format Controls (Transcoder)

## Motivation

The transcoder now honors three upload-time choices carried on the `transcode` request, in
addition to the existing codec/resolution selection: **protocols** (which of HLS/DASH to
package), **segmentSeconds** (segment duration), and per-rendition **bitrateKbps**.

## Request fields

`domain.TranscodeRequest` gains:

```go
Protocols      []string  // subset of {"hls","dash"}; empty = both (legacy)
SegmentSeconds int       // 2|4|6; anything else -> 6
```

`RequestedRendition.BitrateKbps` already existed and is honored by `ResolveRenditions`.

## Resolution (`internal/transcode/protocols.go`)

- `ResolveProtocols(requested)` → deterministic, deduplicated subset of `{"hls","dash"}` in
  canonical order; empty/unknown falls back to both (preserves the original always-both
  behavior for legacy events).
- `ResolveSegmentSeconds(requested)` → accepts only `2/4/6`; anything else returns `6`.
- `HasProtocol(set, name)` → membership helper used by the processor.

## Packaging (`internal/worker/processor.go`, `internal/transcode/runner.go`)

- The processor resolves the protocol set and segment duration once per job, then gates the
  HLS block (per-rendition `PackageHLS` + subtitles + `WriteHLSMaster`) and the DASH block
  (`PackageDASH`) — and their uploads — on `wantHLS`/`wantDASH`.
- `PackageHLS(..., segmentSeconds)` → ffmpeg `-hls_time <segmentSeconds>`.
- `PackageDASH(..., segmentSeconds)` → ffmpeg `-seg_duration <segmentSeconds> -use_timeline 1
  -use_template 1`.
- The idempotency probe (the "already transcoded" short-circuit) checks the HLS master when
  HLS is requested, otherwise the DASH `.mpd`.
- `TranscodeResult.Protocols` records the produced set; `HLSManifestPath`/`DASHManifestPath`
  are empty for a protocol that wasn't produced.
- `complete()` writes `playback.protocols` into the shared `videos` record so
  `streaming-distribution` can advertise only the produced protocols.

## GOP / keyframe-alignment constraint

The encoder keeps `-g 60` (a 2 s GOP at 30 fps). The UI only offers segment presets 2/4/6 s,
all clean multiples of that GOP, so HLS/DASH segments stay keyframe-aligned and players can
switch renditions cleanly. Do not introduce non-multiple presets without revisiting GOP size.

## Known limitations

- **Subtitles ride the HLS master only.** A DASH-only upload that includes sidecar subtitles
  will not surface them (subtitle processing happens inside the HLS branch). Operators who
  need subtitles should keep HLS selected.
- Legacy videos transcoded before this change have no `playback.protocols`; distribution
  treats them as "both".

## Related

- `internal/transcode/codec_trace_test.go` — pins each codec to its ffmpeg encoder
- `../../docs/design-docs/specs/2026-06-10-streaming-format-controls-design.md` — design spec
