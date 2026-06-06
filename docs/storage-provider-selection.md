# Storage Provider Selection

## Motivation

The transcode worker reads source video from object storage and writes back
thumbnails, renditions, and HLS/DASH manifests. Every other Go service that
touches storage — `streaming-ingest` and `streaming-distribution` — picks its
backend (`minio` local vs `aws-s3` cloud) explicitly at runtime through a small
adapter factory.

The transcode worker did **not**. `cmd/worker/main.go` hard-wired
`storage.NewMinIOStorage(cfg.Storage)`, and although `config.FromEnv` already
read a `STORAGE_PROVIDER` value into `Storage.Provider`, nothing consumed it —
the field was dead. Running against real AWS S3 only worked by the coincidence
that `minio-go` also speaks the S3 protocol, and required hand-pointing
`MINIO_ENDPOINT` at an S3 host. This made the worker the one inconsistent corner
of an otherwise uniform storage abstraction.

## Design

A factory mirrors `streaming-distribution`'s `newStorageAdapter`:

```go
store, err := storage.New(cfg.Storage)   // cmd/worker/main.go
```

```go
func New(cfg config.StorageConfig) (ObjectStorage, error) {
    switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
    case "aws-s3", "s3":
        return NewS3Storage(cfg)
    case "minio", "":
        return NewMinIOStorage(cfg)
    default:
        return nil, fmt.Errorf("unsupported storage provider: %q", cfg.Provider)
    }
}
```

Both backends share the same `MinIOStorage` implementation (Download / UploadFile
/ Exists) because `minio-go` is the underlying client for both. They differ only
in how the client is wired:

| Provider | Endpoint | TLS | Credentials |
|---|---|---|---|
| `minio` (default) | `MINIO_ENDPOINT` (e.g. `minio:9000`) | from URL scheme | `MINIO_ACCESS_KEY`/`MINIO_ROOT_USER`, `MINIO_SECRET_KEY`/`MINIO_ROOT_PASSWORD` |
| `aws-s3` | derived: `s3.<AWS_REGION>.amazonaws.com` | always on | `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` |

`NewS3Storage` ignores `MINIO_ENDPOINT` entirely and derives the host from
`AWS_REGION`, exactly like the ingest and distribution S3 adapters — so an
operator deploying to AWS sets `STORAGE_PROVIDER=aws-s3` and AWS credentials, and
never has to know the S3 hostname format.

## Configuration

```bash
# Local (default) — nothing extra needed
STORAGE_PROVIDER=minio
MINIO_ENDPOINT=http://minio:9000
MINIO_ROOT_USER=admin
MINIO_ROOT_PASSWORD=password123

# Production AWS S3
STORAGE_PROVIDER=aws-s3
AWS_REGION=us-east-2
AWS_ACCESS_KEY_ID=AKIA...
AWS_SECRET_ACCESS_KEY=...
```

`config.FromEnv` only swaps in the `AWS_*` credential vars when the provider is
`aws-s3`/`s3`, falling back to the MinIO names so existing local setups are
unaffected.

## Caveats

- **S3-compatible third parties** (Ceph, Wasabi, LocalStack) are not reachable
  through `aws-s3`, which forces the `s3.<region>.amazonaws.com` host. Point them
  at `STORAGE_PROVIDER=minio` with a custom `MINIO_ENDPOINT` instead — the same
  escape hatch the other services use.
- The default credentials (`admin`/`password123`) are MinIO-only. With
  `aws-s3`, missing `AWS_*` vars fall back to those placeholders and will simply
  fail authentication against AWS — set real credentials.
- Bucket layout and object keys are identical across providers; only the client
  wiring changes, so the rest of the pipeline (`processor.go`) is untouched.
