package storage

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"streaming-transcode/internal/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type ObjectStorage interface {
	Download(ctx context.Context, bucket, key, destination string) error
	UploadFile(ctx context.Context, bucket, key, source string) error
	Exists(ctx context.Context, bucket, key string) (bool, error)
}

type minioClient interface {
	FGetObject(ctx context.Context, bucketName, objectName, filePath string, opts minio.GetObjectOptions) error
	FPutObject(ctx context.Context, bucketName, objectName, filePath string, opts minio.PutObjectOptions) (minio.UploadInfo, error)
	StatObject(ctx context.Context, bucketName, objectName string, opts minio.StatObjectOptions) (minio.ObjectInfo, error)
}

type MinIOStorage struct {
	client minioClient
}

func NewMinIOStorage(cfg config.StorageConfig) (*MinIOStorage, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}
	return &MinIOStorage{client: client}, nil
}

func (s *MinIOStorage) Download(ctx context.Context, bucket, key, destination string) error {
	if err := s.client.FGetObject(ctx, bucket, key, destination, minio.GetObjectOptions{}); err != nil {
		return fmt.Errorf("download %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (s *MinIOStorage) UploadFile(ctx context.Context, bucket, key, source string) error {
	contentType := contentTypeFor(source)
	_, err := s.client.FPutObject(ctx, bucket, key, source, minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("upload %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (s *MinIOStorage) Exists(ctx context.Context, bucket, key string) (bool, error) {
	_, err := s.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err == nil {
		return true, nil
	}
	if minio.ToErrorResponse(err).Code == "NoSuchKey" || minio.ToErrorResponse(err).StatusCode == 404 {
		return false, nil
	}
	return false, err
}

func contentTypeFor(path string) string {
	switch {
	case strings.HasSuffix(path, ".m3u8"):
		return "application/vnd.apple.mpegurl"
	case strings.HasSuffix(path, ".mpd"):
		return "application/dash+xml"
	case strings.HasSuffix(path, ".m4s"):
		return "video/iso.segment"
	case strings.HasSuffix(path, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(path, ".json"):
		return "application/json"
	}
	if value := mime.TypeByExtension(filepath.Ext(path)); value != "" {
		return value
	}
	return "application/octet-stream"
}
