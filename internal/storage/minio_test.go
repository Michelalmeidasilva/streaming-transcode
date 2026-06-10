package storage

import (
	"context"
	"errors"
	"testing"

	"streaming-transcode/internal/config"

	"github.com/minio/minio-go/v7"
)

type fakeMinioClient struct {
	getErr   error
	putErr   error
	statInfo minio.ObjectInfo
	statErr  error
}

func (f *fakeMinioClient) FGetObject(_ context.Context, _, _, _ string, _ minio.GetObjectOptions) error {
	return f.getErr
}

func (f *fakeMinioClient) FPutObject(_ context.Context, _, _, _ string, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	return minio.UploadInfo{}, f.putErr
}

func (f *fakeMinioClient) StatObject(_ context.Context, _, _ string, _ minio.StatObjectOptions) (minio.ObjectInfo, error) {
	return f.statInfo, f.statErr
}

func (f *fakeMinioClient) ListObjects(_ context.Context, _ string, _ minio.ListObjectsOptions) <-chan minio.ObjectInfo {
	ch := make(chan minio.ObjectInfo)
	close(ch)
	return ch
}

func TestNewMinIOStorageAndContentTypes(t *testing.T) {
	store, err := NewMinIOStorage(config.StorageConfig{
		Endpoint:  "localhost:9000",
		AccessKey: "admin",
		SecretKey: "password123",
		Region:    "us-east-1",
	})
	if err != nil {
		t.Fatalf("NewMinIOStorage() error = %v", err)
	}
	if store.client == nil {
		t.Fatalf("client = nil")
	}

	cases := map[string]string{
		"master.m3u8":  "application/vnd.apple.mpegurl",
		"manifest.mpd": "application/dash+xml",
		"segment.m4s":  "video/iso.segment",
		"video.mp4":    "video/mp4",
		"data.json":    "application/json",
		"file.bin":     "application/octet-stream",
	}
	for path, want := range cases {
		if got := contentTypeFor(path); got != want {
			t.Fatalf("contentTypeFor(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestMinIOStorageMethods(t *testing.T) {
	store := &MinIOStorage{client: &fakeMinioClient{}}
	if err := store.Download(context.Background(), "videos", "input.mp4", "/tmp/output.mp4"); err != nil {
		t.Fatalf("Download() error = %v", err)
	}
	if err := store.UploadFile(context.Background(), "videos", "out.json", "/tmp/out.json"); err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
	exists, err := store.Exists(context.Background(), "videos", "input.mp4")
	if err != nil || !exists {
		t.Fatalf("Exists() = %v, %v", exists, err)
	}
}

func TestMinIOStorageErrors(t *testing.T) {
	store := &MinIOStorage{client: &fakeMinioClient{getErr: errors.New("download failed")}}
	if err := store.Download(context.Background(), "videos", "input.mp4", "/tmp/output.mp4"); err == nil {
		t.Fatalf("Download() error = nil, want error")
	}

	store = &MinIOStorage{client: &fakeMinioClient{putErr: errors.New("upload failed")}}
	if err := store.UploadFile(context.Background(), "videos", "input.mp4", "/tmp/output.mp4"); err == nil {
		t.Fatalf("UploadFile() error = nil, want error")
	}

	noSuchKey := minio.ErrorResponse{Code: "NoSuchKey", StatusCode: 404}
	store = &MinIOStorage{client: &fakeMinioClient{statErr: noSuchKey}}
	exists, err := store.Exists(context.Background(), "videos", "missing.mp4")
	if err != nil || exists {
		t.Fatalf("Exists(missing) = %v, %v", exists, err)
	}

	store = &MinIOStorage{client: &fakeMinioClient{statErr: errors.New("boom")}}
	if _, err := store.Exists(context.Background(), "videos", "broken.mp4"); err == nil {
		t.Fatalf("Exists() error = nil, want error")
	}
}
