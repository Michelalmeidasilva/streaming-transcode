# Transcode Benchmark Machine Label

## Motivation

The transcode worker already captures per-rendition elapsed time, ffmpeg CPU usage, codec, and bitrate for each job. To compare codec processing performance across different EC2 instance types (e.g. `c5.xlarge` vs `c5.2xlarge` vs `c7g.xlarge`), jobs need a stable, human-readable label identifying the worker host. Without this, all runs land in MongoDB with only the container hostname — which is opaque and changes across Batch invocations.

The `TRANSCODE_MACHINE_LABEL` env var solves this: set it to the EC2 instance type (e.g. `c5.xlarge`) before the worker starts, and every `transcode.completed` event carries that label. The Event Gateway stores it in the `transcode_runs` collection, enabling the upload platform to group and compare runs by machine type.

## Env Var

| Variable | Default | Description |
|----------|---------|-------------|
| `TRANSCODE_MACHINE_LABEL` | `os.Hostname()` | Human-readable label for the worker host. Set to the EC2 instance type or any descriptive name. Falls back to the container/instance hostname when unset. |

### Example

```
TRANSCODE_MACHINE_LABEL=c5.xlarge
```

## JobObservability Contract

The worker populates `JobObservability.MachineLabel` with the resolved label (env var or hostname fallback). The struct is serialised into the `transcode.completed` event body, which is sent to `POST /api/v1/events` on the Event Gateway.

### transcode.completed payload (relevant fields)

```json
{
  "eventType": "transcode.completed",
  "videoId": "<uuid>",
  "jobId": "<uuid>",
  "machineLabel": "c5.xlarge",
  "observability": {
    "machineLabel": "c5.xlarge",
    "hostname": "ip-10-0-1-42.us-east-2.compute.internal",
    "cpuCores": 4,
    "profile": "production",
    "elapsedSeconds": 142.3,
    "rtf": 0.58,
    "sourceFileSizeBytes": 524288000,
    "totalOutputSizeBytes": 89128960,
    "renditions": [
      {
        "name": "360p",
        "codec": "h264",
        "width": 640,
        "height": 360,
        "preset": "fast",
        "targetBitrateKbps": 800,
        "outputBitrateKbps": 793,
        "elapsedSeconds": 28.1,
        "avgCpuPercent": 92.4,
        "maxCpuPercent": 99.1,
        "avgMemoryMb": 310.5,
        "maxMemoryMb": 412.0
      }
    ]
  }
}
```

`machineLabel` is duplicated as a top-level field so the Event Gateway can index it directly without deserialising the full observability struct.

## Benchmark Workflow

This feature is designed for a specific manual workflow:

1. Bring up a specific EC2 instance type via `terraform apply` with `enable_transcode_benchmark=true` and `benchmark_instance_type=<type>` (see `infra/docs/transcode-ec2-benchmark.md`).
2. Upload ONE video through the normal upload UI (`streaming-platform-upload`).
3. Wait for the transcode job to complete.
4. Read the Metrics tab in the upload platform to see the run result for that machine.
5. Change `benchmark_instance_type` in `terraform.tfvars`, re-apply, upload another video, repeat.

Each run is stored separately in MongoDB keyed by `jobId`, so results from different machine types accumulate and can be compared side by side.

## Caveats

- The label is static for the lifetime of the worker process. On EC2 Batch jobs, the instance type is fixed per job definition; set `TRANSCODE_MACHINE_LABEL` in the user-data or job environment.
- When `TRANSCODE_MACHINE_LABEL` is not set, the fallback is `os.Hostname()`, which is the container ID on ECS/Fargate — not human-readable. Always set it explicitly for benchmark runs.
- The EC2 benchmark module defaults to `x86_64` (`c5.xlarge`, `al2023-ami-*-x86_64`). Benchmarking Graviton instances (`c7g.*`) requires building an `arm64` ECR image first.
