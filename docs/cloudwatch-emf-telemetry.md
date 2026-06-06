# CloudWatch EMF Telemetry

## Motivation

`streaming-transcode` runs on AWS Batch (or locally as a Docker container), where
pull-based Prometheus scrape is not viable: jobs are ephemeral, there is no persistent
HTTP server, and no Prometheus-compatible collector runs alongside the Batch compute
environment. The previous pipeline — `internal/otel/` + a per-job OTel span pushing to
an OTLP endpoint — was dead weight once the collector was removed; with no receiver on
`:4317` the exporters flooded logs every interval. Stdout is the natural, zero-infra
output channel for Batch jobs.

## What Changed

- **Removed:** `internal/otel/` package (OTLP gRPC push pipeline, `otel.Init`, per-job
  span). The `OTEL_SDK_DISABLED`/`OTEL_EXPORTER_OTLP_ENDPOINT` env vars are no longer
  needed.
- **Added:** `internal/telemetry/emf.go` — a helper called once per job that emits a
  single structured JSON log line in CloudWatch Embedded Metric Format (EMF). The record
  captures job-level metrics (`JobCount`, `JobDuration`, `FailureCount`) with dimension
  `result`, plus top-level fields `video_id` and `result` for filtering in Logs Insights.

## EMF Contract

Each completed transcode job produces a single line written to stdout:

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
  "video_id": "ebd23f99-c89b-43d9-a72a-1928fb211e40",
  "JobCount": 1,
  "JobDuration": 142300,
  "FailureCount": 0
}
```

`result` is `"success"` or `"failure"`. `FailureCount` is `1` on failure, otherwise `0`.

## Dev / Prod Data Flow

The same EMF JSON is emitted to stdout in both environments; in production, AWS Batch
routes job stdout to CloudWatch Logs, and CloudWatch Logs automatically extracts the
embedded metrics into the `VOD/streaming-transcode` namespace — no collector, no sidecar.
Local wiring (LocalStack + log group) is covered by Plan 2 of the observability migration
(infra work, not in scope here).

## Reference

Design spec: `infra/docs/design-docs/specs/2026-06-06-cloudwatch-observability-migration-design.md`
