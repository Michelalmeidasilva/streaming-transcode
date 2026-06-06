# streaming-transcode — SPEC

Transcoding worker (pipeline stage 4). AWS Batch job / Docker container.
Go + FFmpeg + shaka-packager. Consumes RabbitMQ, never exposes HTTP.

## Trigger

Consumes `video.upload.completed` from RabbitMQ exchange `video_events`, queue
`transcode.jobs`. One job per message. Implements retry with backoff
(`transcode.retry`) and dead-letter queue (`transcode.dead`).

## Pipeline per Job

1. Download source video from object storage (`raw/<videoId>/<filename>` on AWS,
   `<videoId>/<filename>` on MinIO).
2. Probe source with `ffprobe`; synthesize `MediaInfo` from supplied geometry for
   headerless `.yuv` sources (geometry provided in the event payload).
3. Extract thumbnail via ffmpeg (`-ss <t> -frames:v 1 -vf scale=640:-2 -q:v 3`),
   upload to `thumbnails/<videoId>.jpg`, PATCH ingest record with
   `thumbnail_status: "ready"`.
4. Transcode each rendition with ffmpeg (aligned GOPs: `-g 60 -keyint_min 60
   -sc_threshold 0`). Single AAC 128k audio track shared across renditions.
5. Convert sidecar `.srt` subtitles to WebVTT; package HLS subtitle media playlists.
6. Package DASH + HLS with shaka-packager; write to `transcoded/<videoId>/`.
7. Write playback metadata to MongoDB `videos` collection (status `ready`,
   subtitle tracks, etc.).

**Job state lifecycle:** `queued → transcoding → packaging → ready | failed`.

## Bitrate Ladder

| Resolution | Video bitrate | Codec |
|------------|--------------|-------|
| 360p | 800 kbps | H.264 |
| 480p | 1400 kbps | H.264 |
| 720p | 2800 kbps | H.264 |
| 1080p | 5000 kbps | H.264 |
| Audio | 128 kbps | AAC (shared) |

GOP alignment (`-g 60 -keyint_min 60 -sc_threshold 0`) is non-negotiable.

## Telemetry (CloudWatch EMF)

One EMF JSON line is written to stdout per job:

```json
{
  "_aws": {
    "Timestamp": 1717689600000,
    "CloudWatchMetrics": [{
      "Namespace": "VOD/streaming-transcode",
      "Dimensions": [["result"]],
      "Metrics": [
        {"Name":"JobCount","Unit":"Count"},
        {"Name":"JobDuration","Unit":"Milliseconds"},
        {"Name":"FailureCount","Unit":"Count"}
      ]
    }]
  },
  "result": "success",
  "video_id": "<videoId>",
  "JobCount": 1,
  "JobDuration": 142300,
  "FailureCount": 0
}
```

No `/metrics` endpoint. No OTel push pipeline. See `docs/cloudwatch-emf-telemetry.md`.

## Storage

Backend selected at runtime via `STORAGE_PROVIDER` (`minio` | `aws-s3`).
`storage.New(cfg.Storage)` dispatches to `NewMinIOStorage` or `NewS3Storage`.

## Env

```
STORAGE_PROVIDER         minio | aws-s3
STORAGE_BUCKET           videos
MINIO_ENDPOINT           http://minio:9000
MINIO_ACCESS_KEY         admin
MINIO_SECRET_KEY         password123
AWS_REGION               us-east-1
AWS_ACCESS_KEY_ID        (AWS only)
AWS_SECRET_ACCESS_KEY    (AWS only)
RABBITMQ_URL             amqp://guest:guest@rabbitmq:5672/
MONGODB_URI              mongodb://mongodb:27017/streaming
TRANSCODE_JOB_TIMEOUT_SECONDS  10800
TRANSCODE_MAX_HEIGHT     0    (0 = uncapped)
TRANSCODE_CODECS         h264
INGEST_BASE_URL          http://streaming-ingest:8080/api/v1
```
