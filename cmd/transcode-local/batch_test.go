package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"streaming-transcode/internal/config"
	"streaming-transcode/internal/domain"
)

type fakeProcessor struct {
	calls    int
	gotJobID string
	gotEvent domain.UploadCompletedEvent
	err      error
}

func (f *fakeProcessor) Process(_ context.Context, jobID string, event domain.UploadCompletedEvent) error {
	f.calls++
	f.gotJobID = jobID
	f.gotEvent = event
	return f.err
}

func TestExtractVideoID(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		want    string
		wantErr bool
	}{
		{name: "flat key", key: "raw/abc123/video.mp4", want: "abc123"},
		{name: "nested object keeps first segment", key: "raw/abc123/sub/dir/original.mov", want: "abc123"},
		{name: "uuid id", key: "raw/9f1c2e7a-1/source.mkv", want: "9f1c2e7a-1"},
		{name: "missing raw prefix", key: "uploads/abc/video.mp4", wantErr: true},
		{name: "empty id segment", key: "raw//video.mp4", wantErr: true},
		{name: "no object after id", key: "raw/abc123", wantErr: true},
		{name: "id but trailing slash only", key: "raw/abc123/", wantErr: true},
		{name: "empty key", key: "", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := extractVideoID(tc.key)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("extractVideoID(%q) = %q, want error", tc.key, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("extractVideoID(%q) unexpected error: %v", tc.key, err)
			}
			if got != tc.want {
				t.Fatalf("extractVideoID(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestBuildBatchEvent(t *testing.T) {
	cfg := config.Config{Storage: config.StorageConfig{Bucket: "vod-prod"}}

	t.Run("populates event from key and config", func(t *testing.T) {
		event, err := buildBatchEvent(cfg, "raw/vid-1/source.mp4")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if event.VideoID != "vid-1" {
			t.Errorf("VideoID = %q, want %q", event.VideoID, "vid-1")
		}
		if event.ObjectKey != "raw/vid-1/source.mp4" {
			t.Errorf("ObjectKey = %q, want the full key", event.ObjectKey)
		}
		if event.Bucket != "vod-prod" {
			t.Errorf("Bucket = %q, want %q", event.Bucket, "vod-prod")
		}
		if event.Provider != "aws-s3" {
			t.Errorf("Provider = %q, want %q", event.Provider, "aws-s3")
		}
	})

	t.Run("rejects headerless raw .yuv without geometry", func(t *testing.T) {
		if _, err := buildBatchEvent(cfg, "raw/vid-2/source.yuv"); err == nil {
			t.Fatal("expected error for .yuv key, got nil")
		}
	})

	t.Run("propagates invalid key error", func(t *testing.T) {
		if _, err := buildBatchEvent(cfg, "uploads/vid-3/source.mp4"); err == nil {
			t.Fatal("expected error for key without raw/ prefix, got nil")
		}
	})
}

func TestBatchJobID(t *testing.T) {
	t.Run("prefers AWS_BATCH_JOB_ID", func(t *testing.T) {
		t.Setenv("AWS_BATCH_JOB_ID", "batch-xyz")
		if got := batchJobID("vid-1"); got != "batch-xyz" {
			t.Fatalf("batchJobID = %q, want %q", got, "batch-xyz")
		}
	})

	t.Run("falls back to a video-scoped id", func(t *testing.T) {
		t.Setenv("AWS_BATCH_JOB_ID", "")
		got := batchJobID("vid-1")
		if !strings.HasPrefix(got, "vid-1-") {
			t.Fatalf("batchJobID = %q, want prefix %q", got, "vid-1-")
		}
	})
}

func TestRunBatchJob(t *testing.T) {
	cfg := config.Config{Storage: config.StorageConfig{Bucket: "vod-prod"}}

	t.Run("processes a valid key", func(t *testing.T) {
		proc := &fakeProcessor{}
		if err := runBatchJob(context.Background(), proc, cfg, "raw/vid-1/source.mp4"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if proc.calls != 1 {
			t.Fatalf("Process called %d times, want 1", proc.calls)
		}
		if proc.gotEvent.VideoID != "vid-1" {
			t.Errorf("event VideoID = %q, want %q", proc.gotEvent.VideoID, "vid-1")
		}
		if proc.gotJobID == "" {
			t.Error("jobID passed to Process was empty")
		}
	})

	t.Run("does not call Process on an invalid key", func(t *testing.T) {
		proc := &fakeProcessor{}
		if err := runBatchJob(context.Background(), proc, cfg, "uploads/vid-1/source.mp4"); err == nil {
			t.Fatal("expected error for invalid key, got nil")
		}
		if proc.calls != 0 {
			t.Fatalf("Process called %d times, want 0", proc.calls)
		}
	})

	t.Run("propagates a processing error", func(t *testing.T) {
		sentinel := errors.New("ffmpeg blew up")
		proc := &fakeProcessor{err: sentinel}
		if err := runBatchJob(context.Background(), proc, cfg, "raw/vid-1/source.mp4"); !errors.Is(err, sentinel) {
			t.Fatalf("runBatchJob error = %v, want %v", err, sentinel)
		}
	})
}
