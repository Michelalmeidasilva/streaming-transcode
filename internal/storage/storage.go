package storage

import (
	"fmt"
	"strings"

	"streaming-transcode/internal/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// New selects the object-storage backend from cfg.Provider, mirroring the
// provider switch used by streaming-ingest and streaming-distribution. Both
// backends speak the S3 protocol via minio-go, so they share the MinIOStorage
// implementation and differ only in endpoint/TLS wiring.
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

// NewS3Storage builds a minio-go client pointed at the AWS S3 regional endpoint
// with TLS enforced, the same way streaming-ingest/streaming-distribution reach
// real S3. Credentials come from the resolved config (AWS_* env vars).
func NewS3Storage(cfg config.StorageConfig) (*MinIOStorage, error) {
	client, err := minio.New(awsS3Endpoint(cfg.Region), &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Region: cfg.Region,
		Secure: true,
	})
	if err != nil {
		return nil, fmt.Errorf("create s3 client: %w", err)
	}
	return &MinIOStorage{client: client}, nil
}

// awsS3Endpoint returns the regional virtual-hosted S3 endpoint, defaulting to
// us-east-1 when no region is configured.
func awsS3Endpoint(region string) string {
	if region == "" {
		region = "us-east-1"
	}
	return fmt.Sprintf("s3.%s.amazonaws.com", region)
}
