# Upscaling Guard

## Rule

When a `TranscodeRequest` carries explicit `Renditions`, any rendition whose
`Height` exceeds the source video height is dropped before transcoding begins.
Upscaling wastes CPU and produces no quality gain — the guard enforces this at
the plan level rather than relying on callers to check.

Implementation: `capRenditionsToSource` in `internal/transcode/plan.go`, called
from `ResolveRenditions` immediately after the explicit-rendition list is built.

## Fallback

If _every_ requested rendition exceeds the source height, the guard does not
return an empty ladder. Instead it emits one fallback rendition per distinct
codec at the source dimensions (`Width × Height`), with the bitrate selected by
`defaultBitrateForDimensions`. This preserves the invariant that a transcode job
always produces at least one output per requested codec.

Example: source is 720p, user requests `[{1080p, h264}, {1080p, av1}]` → guard
produces `[{720p, h264}, {720p, av1}]`.

## Retrocompat

Events that do not include a `transcode` field (i.e. `TranscodeRequest` is
zero-valued, `Renditions` is nil/empty) are **unaffected**. In that path
`ResolveRenditions` falls through to `PlanProductionRenditionsForCodecs`, which
already avoids upscaling via `planBaseRenditions` (it only adds renditions at or
below the source resolution).

## Tests

`TestResolveRenditionsDropsRenditionsAboveSource` — verifies that a mixed list
(one below-source, one above-source rendition) returns only the valid one.

`TestResolveRenditionsFallsBackToSourceHeightPerCodec` — verifies the fallback:
all above-source renditions → one per codec at source height.
