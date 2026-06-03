# Thumbnail Generation

## Motivation

Consumer clients (streaming-web-client, streaming-app-client) need a poster image
to display in the catalog before the user starts playback. Generating the thumbnail
inside the transcode worker is the natural place: the worker already has the raw
source file, the ffmpeg binary, and the storage credentials — no new infrastructure
is required.

## Where in the Pipeline

Thumbnail extraction happens **after** the source is probed (so duration is known)
and **before** rendition transcoding begins. This means the thumbnail is available
as soon as the video appears in the catalog, not only after multi-bitrate encoding
finishes.

```
probe source
  └─ ExtractThumbnail   ← poster frame at ~10% of duration
       └─ upload thumbnails/<videoId>.jpg
            └─ PATCH ingest  thumbnail_status: "ready"
transcode renditions (360p → 1080p)
package HLS + DASH
```

## ffmpeg Command

```bash
ffmpeg -ss <atSeconds> -i <source> -frames:v 1 -vf scale=640:-2 -q:v 3 -y <output>
```

| Flag | Purpose |
|---|---|
| `-ss <t>` | Seek to `t` seconds before opening the input (fast, keyframe-accurate) |
| `-frames:v 1` | Capture exactly one frame |
| `-vf scale=640:-2` | Scale to 640 px wide; `-2` keeps aspect ratio and ensures height is even (required by libx264/yuvj420p) |
| `-q:v 3` | JPEG quality 3 (scale 1–31, lower = better); good balance of size vs. sharpness |
| `-y` | Overwrite output without prompting |

### Seek Offset

```
atSeconds = max(1, int(duration * 0.1))
```

Clamped to a minimum of 1 s to avoid black frames at the very start of videos that
open with a fade-in or title card.

## Storage Key Convention

`thumbnails/<videoId>.jpg`

The bucket is the same one used for transcoded output (env `STORAGE_BUCKET`).
`streaming-distribution` and `streaming-platform-upload` both derive the public URL
from this key, so the convention must not change without updating both services.

## snake_case PATCH Key Rationale

The PATCH body sent to the ingest service uses `thumbnail_status` (snake_case) to
match the MongoDB field name. The ingest `PatchVideo` handler performs a raw
`$set` on whatever keys are present in the request body, so the Go struct tag
`bson:"thumbnail_status"` on the distribution read model maps directly to what
the worker writes. Using a different case would silently store a parallel field and
`thumbnail_status == "ready"` would never be true.

## Best-Effort Failure Handling

Thumbnail extraction is **non-fatal**. Any error (ffmpeg not installed, source
unreadable at the seek point, storage upload failure, ingest PATCH timeout) is
logged at WARN level and execution continues with rendition transcoding. The video
will still be transcoded and playable; `thumbnail_status` will simply remain unset
(`""`), and consumer clients fall back to `default-thumbnail.png`.

This prevents a cosmetic feature from blocking the critical transcode path.
