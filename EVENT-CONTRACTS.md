# Streaming Transcode - Event Contracts

Date: 2026-05-09

## RabbitMQ Topology

Exchange:

```text
video_events
```

Queues:

```text
transcode.jobs
transcode.retry
transcode.dead
```

Bindings:

```text
transcode.jobs <- video.upload.completed
```

Retry behavior:

- failed transient jobs are republished to `transcode.retry`
- `transcode.retry` has a message TTL
- expired retry messages are dead-lettered back to `video_events` with routing key `video.upload.completed`
- terminal failures and exhausted attempts are published to `transcode.dead`

## Input Event

HTTP event sent to `streaming-ingest`:

```json
{
  "eventType": "upload.completed",
  "payload": {
    "videoId": "uuid",
    "filename": "uuid/original-file.mp4",
    "originalName": "original-file.mp4",
    "size": 1048576000,
    "provider": "minio",
    "bucket": "videos",
    "objectKey": "uuid/original-file.mp4",
    "sourceETag": "optional-etag",
    "sourceVersion": "optional-version",
    "url": "http://localhost:9000/videos/uuid/original-file.mp4",
    "occurredAt": "2026-05-09T12:00:00Z"
  }
}
```

RabbitMQ routing key produced by ingest:

```text
video.upload.completed
```

Backward compatibility:

- if `objectKey` is missing, the worker uses `sourceKey`
- if `sourceKey` is missing, the worker resolves from `filename`
- if `bucket` is missing, the worker uses `STORAGE_BUCKET`
- if `provider` is missing, the worker uses `minio`

## Output Events

All events are sent through `POST /api/v1/events`. `streaming-ingest` converts each `eventType` to routing key `video.<eventType>`.

### transcode.queued

```json
{
  "eventType": "transcode.queued",
  "payload": {
    "videoId": "uuid",
    "jobId": "uuid-123",
    "profile": "production-h264-hls-dash",
    "sourceKey": "uuid/original-file.mp4",
    "sourceETag": "optional-etag",
    "attempt": 1,
    "fingerprint": "sha256",
    "processingStatus": "queued",
    "progress": 5,
    "queuedAt": "2026-05-09T12:00:00Z"
  }
}
```

### transcode.started

```json
{
  "eventType": "transcode.started",
  "payload": {
    "videoId": "uuid",
    "jobId": "uuid-123",
    "profile": "production-h264-hls-dash",
    "sourceKey": "uuid/original-file.mp4",
    "sourceETag": "optional-etag",
    "attempt": 1,
    "fingerprint": "sha256",
    "startedAt": "2026-05-09T12:01:00Z"
  }
}
```

### transcode.progress

```json
{
  "eventType": "transcode.progress",
  "payload": {
    "videoId": "uuid",
    "jobId": "uuid-123",
    "attempt": 1,
    "fingerprint": "sha256",
    "phase": "rendition.completed",
    "progress": 65,
    "rendition": "720p",
    "completed": 2,
    "total": 2,
    "updatedAt": "2026-05-09T12:02:00Z"
  }
}
```

Known phases:

```text
downloaded
probed
rendition.completed
packaging.started
packaged
outputs.uploaded
```

### packaging.completed

```json
{
  "eventType": "packaging.completed",
  "payload": {
    "videoId": "uuid",
    "jobId": "uuid-123",
    "status": "completed",
    "profile": "production-h264-hls-dash",
    "sourceKey": "uuid/original-file.mp4",
    "durationSeconds": 300,
    "elapsedSeconds": 180,
    "rtf": 0.6,
    "renditions": [],
    "hlsManifestPath": "transcoded/uuid/hls/master.m3u8",
    "dashManifestPath": "transcoded/uuid/dash/manifest.mpd",
    "metricsPath": "metrics/uuid/transcode-result.json",
    "attempt": 1,
    "fingerprint": "sha256",
    "completedAt": "2026-05-09T12:04:00Z"
  }
}
```

### transcode.completed

Uses the same payload shape as `packaging.completed`.

### ready

```json
{
  "eventType": "ready",
  "payload": {
    "videoId": "uuid",
    "status": "ready"
  }
}
```

### transcode.failed

```json
{
  "eventType": "transcode.failed",
  "payload": {
    "videoId": "uuid",
    "jobId": "uuid-123",
    "attempt": 1,
    "fingerprint": "sha256",
    "reason": "ffmpeg_failed",
    "message": "ffmpeg exited with status 1"
  }
}
```

## Upload-State Patch

The worker patches:

```text
PATCH /api/v1/upload-state/videos/{videoId}
```

Main fields:

```json
{
  "status": "ready",
  "processingStatus": "ready",
  "source": {
    "bucket": "videos",
    "key": "uuid/original-file.mp4",
    "provider": "minio",
    "size": 1048576000,
    "etag": "optional-etag",
    "version": "optional-version"
  },
  "mediaInfo": {},
  "transcode": {
    "jobId": "uuid-123",
    "profile": "production-h264-hls-dash",
    "attempt": 1,
    "fingerprint": "sha256",
    "startedAt": "2026-05-09T12:01:00Z",
    "completedAt": "2026-05-09T12:04:00Z",
    "error": null
  },
  "playback": {
    "hlsManifestPath": "transcoded/uuid/hls/master.m3u8",
    "dashManifestPath": "transcoded/uuid/dash/manifest.mpd",
    "renditions": []
  },
  "metrics": {
    "rtf": 0.6,
    "elapsedSeconds": 180,
    "metricsPath": "metrics/uuid/transcode-result.json"
  }
}
```

