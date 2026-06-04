package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"streaming-transcode/internal/config"
	"streaming-transcode/internal/domain"
	"streaming-transcode/internal/events"
	"streaming-transcode/internal/queue"
)

type fakeStorage struct {
	existsMap map[string]bool
	downloads []string
	uploads   []string
	existsErr error
	uploadErr error
}

func (s *fakeStorage) Download(_ context.Context, _, key, destination string) error {
	s.downloads = append(s.downloads, key)
	return os.WriteFile(destination, []byte("source"), 0o644)
}

func (s *fakeStorage) UploadFile(_ context.Context, _, key, source string) error {
	if s.uploadErr != nil {
		return s.uploadErr
	}
	s.uploads = append(s.uploads, key+"="+filepath.Base(source))
	return nil
}

func (s *fakeStorage) Exists(_ context.Context, _, key string) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	return s.existsMap[key], nil
}

type fakeRunner struct {
	info           domain.MediaInfo
	probeErr       error
	transcodeErr   error
	packageHLSErr  error
	packageDASH    error
	seenRenditions []domain.Rendition
	thumbnailCalls int
	thumbnailErr   error

	subtitleConversions int
	subtitleErr         error
}

func (r *fakeRunner) Probe(string) (domain.MediaInfo, error) {
	return r.info, r.probeErr
}

func (r *fakeRunner) TranscodeRendition(_ context.Context, _ string, _ *domain.RawVideoParams, rendition domain.Rendition, output string) (domain.RenditionMetrics, error) {
	r.seenRenditions = append(r.seenRenditions, rendition)
	if r.transcodeErr != nil {
		return domain.RenditionMetrics{}, r.transcodeErr
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return domain.RenditionMetrics{}, err
	}
	if err := os.WriteFile(output, []byte("rendition"), 0o644); err != nil {
		return domain.RenditionMetrics{}, err
	}
	return domain.RenditionMetrics{
		Status:              "completed",
		SourceFileSizeBytes: 6,
		OutputFileSizeBytes: 9,
		SourceDuration:      10,
		OutputDuration:      10,
		SourceCodec:         "h264",
		OutputCodec:         rendition.Codec,
		TargetBitrateKbps:   rendition.BitrateKbps,
		ElapsedSeconds:      1.5,
		RTF:                 0.15,
		ResourceUsage: domain.ResourceUsage{
			SampleCount:   2,
			AvgCPUPercent: 120,
			MaxCPUPercent: 150,
			AvgMemoryMB:   64,
			MaxMemoryMB:   80,
		},
	}, nil
}

func TestProcessorProcessUsesCustomTranscodeRequest(t *testing.T) {
	store := &fakeStorage{existsMap: map[string]bool{}}
	runner := &fakeRunner{info: domain.MediaInfo{
		Width:           1920,
		Height:          1080,
		DurationSeconds: 10,
		VideoCodec:      "h264",
		AudioCodec:      "aac",
	}}
	var requests []capturedRequest
	processor := newTestProcessor(t, store, runner, &requests)

	event := domain.UploadCompletedEvent{
		VideoID:   "video-custom",
		Filename:  "source.y4m",
		ObjectKey: "video-custom/source.y4m",
		Bucket:    "videos",
		Transcode: domain.TranscodeRequest{
			Profile: "benchmark-av1-720p",
			Preset:  "slow",
			Renditions: []domain.RequestedRendition{
				{Name: "custom-av1", Width: 1280, Height: 720, BitrateKbps: 2500, Codec: "av1"},
			},
		},
	}

	if err := processor.Process(context.Background(), "job-custom", event); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(runner.seenRenditions) != 1 {
		t.Fatalf("seenRenditions len = %d, want 1", len(runner.seenRenditions))
	}
	got := runner.seenRenditions[0]
	if got.Name != "custom-av1" || got.Codec != "av1" || got.Width != 1280 || got.Height != 720 || got.BitrateKbps != 2500 || got.Preset != "slow" {
		t.Fatalf("seen rendition = %+v", got)
	}
}

func (r *fakeRunner) PackageHLS(_ context.Context, _ string, _ string, outDir string) error {
	if r.packageHLSErr != nil {
		return r.packageHLSErr
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "index.m3u8"), []byte("#EXTM3U"), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "segment-00000.m4s"), []byte("segment"), 0o644)
}

func (r *fakeRunner) PackageDASH(_ context.Context, _ []string, outDir string) error {
	if r.packageDASH != nil {
		return r.packageDASH
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outDir, "manifest.mpd"), []byte("<MPD/>"), 0o644)
}

func (r *fakeRunner) ExtractThumbnail(_ context.Context, _ string, _ *domain.RawVideoParams, output string, _ float64) error {
	r.thumbnailCalls++
	if r.thumbnailErr != nil {
		return r.thumbnailErr
	}
	return os.WriteFile(output, []byte("jpeg-bytes"), 0o644)
}

func (r *fakeRunner) ConvertSRTToVTT(_ context.Context, _ string, outVTT string) error {
	r.subtitleConversions++
	if r.subtitleErr != nil {
		return r.subtitleErr
	}
	if err := os.MkdirAll(filepath.Dir(outVTT), 0o755); err != nil {
		return err
	}
	return os.WriteFile(outVTT, []byte("WEBVTT\n"), 0o644)
}

type capturedRequest struct {
	method string
	path   string
	body   map[string]any
}

func newTestProcessor(t *testing.T, store *fakeStorage, runner *fakeRunner, requests *[]capturedRequest) *Processor {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		body := map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode request: %v", err)
		}
		*requests = append(*requests, capturedRequest{method: r.Method, path: r.URL.Path, body: body})
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	now := time.Date(2026, 5, 9, 22, 0, 0, 0, time.UTC)
	cfg := config.Config{
		EventGatewayURL: server.URL + "/api/v1",
		Storage: config.StorageConfig{
			Bucket: "videos",
		},
		Transcode: config.TranscodeConfig{
			WorkDir:    t.TempDir(),
			Profile:    "production-h264-hls-dash",
			JobTimeout: time.Minute,
		},
	}
	return NewProcessor(Dependencies{
		Config:   cfg,
		Storage:  store,
		Events:   events.NewGatewayClient(cfg.EventGatewayURL),
		Runner:   runner,
		Logger:   log.New(io.Discard, "", 0),
		ClockNow: func() time.Time { return now },
	})
}

func TestProcessorProcessSuccess(t *testing.T) {
	store := &fakeStorage{existsMap: map[string]bool{}}
	runner := &fakeRunner{info: domain.MediaInfo{
		Width:           1920,
		Height:          1080,
		DurationSeconds: 10,
		VideoCodec:      "h264",
		AudioCodec:      "aac",
	}}
	var requests []capturedRequest
	processor := newTestProcessor(t, store, runner, &requests)

	event := domain.UploadCompletedEvent{
		VideoID:   "video-1",
		Filename:  "source.mp4",
		ObjectKey: "video-1/source.mp4",
		Bucket:    "videos",
	}
	if err := processor.Process(context.Background(), "job-1", event); err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(store.downloads) != 1 {
		t.Fatalf("downloads = %v", store.downloads)
	}
	if len(store.uploads) < 6 {
		t.Fatalf("uploads = %v", store.uploads)
	}
	var sawObservability bool
	for _, upload := range store.uploads {
		if strings.Contains(upload, "metrics/video-1/observability.json") {
			sawObservability = true
			break
		}
	}
	if !sawObservability {
		t.Fatalf("uploads missing observability.json = %v", store.uploads)
	}

	var sawPatch, sawReady bool
	for _, req := range requests {
		if req.method == http.MethodPatch && strings.Contains(req.path, "/upload-state/videos/video-1") {
			if req.body["status"] == "ready" && req.body["processingStatus"] == "ready" {
				sawPatch = true
				metrics, _ := req.body["metrics"].(map[string]any)
				if metrics["observabilityPath"] == nil {
					t.Fatalf("patch metrics missing observabilityPath: %#v", req.body)
				}
			}
		}
		if req.method == http.MethodPost && strings.Contains(req.path, "/events") && req.body["eventType"] == "ready" {
			sawReady = true
		}
	}
	if !sawPatch || !sawReady {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestNewProcessorUsesDefaultClockWhenNil(t *testing.T) {
	processor := NewProcessor(Dependencies{
		Config: config.Config{
			Transcode: config.TranscodeConfig{JobTimeout: time.Second},
		},
		Logger: log.New(io.Discard, "", 0),
	})
	if processor.clockNow == nil {
		t.Fatalf("clockNow = nil")
	}
}

func TestProcessorAlreadyTranscodedOnlyMarksReady(t *testing.T) {
	store := &fakeStorage{existsMap: map[string]bool{"transcoded/video-1/hls/master.m3u8": true}}
	runner := &fakeRunner{}
	var requests []capturedRequest
	processor := newTestProcessor(t, store, runner, &requests)

	err := processor.Process(context.Background(), "job-1", domain.UploadCompletedEvent{
		VideoID:   "video-1",
		ObjectKey: "video-1/source.mp4",
		Bucket:    "videos",
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if len(store.downloads) != 0 {
		t.Fatalf("downloads = %v, want none", store.downloads)
	}
	if len(requests) != 1 || requests[0].body["eventType"] != "ready" {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestProcessorRejectsInvalidTranscodeRequest(t *testing.T) {
	store := &fakeStorage{existsMap: map[string]bool{}}
	runner := &fakeRunner{info: domain.MediaInfo{Width: 1920, Height: 1080, DurationSeconds: 10, VideoCodec: "h264"}}
	var requests []capturedRequest
	processor := newTestProcessor(t, store, runner, &requests)

	event := domain.UploadCompletedEvent{
		VideoID:   "video-bad-codec",
		ObjectKey: "video-bad-codec/source.mp4",
		Bucket:    "videos",
		Transcode: domain.TranscodeRequest{
			Renditions: []domain.RequestedRendition{
				{Width: 0, Height: 720, Codec: "h264"},
			},
		},
	}
	err := processor.Process(context.Background(), "job-bad", event)
	if err == nil {
		t.Fatalf("Process() error = nil, want error for invalid rendition dimensions")
	}

	var sawFailed bool
	for _, req := range requests {
		if req.method == http.MethodPost && req.body["eventType"] == "transcode.failed" {
			payload, _ := req.body["payload"].(map[string]any)
			if payload["reason"] == "invalid_transcode_request" {
				sawFailed = true
			}
		}
	}
	if !sawFailed {
		t.Fatalf("expected transcode.failed with reason=invalid_transcode_request, got requests = %#v", requests)
	}
	if len(store.downloads) != 0 {
		t.Fatalf("downloads = %v, want none (should fail before download)", store.downloads)
	}
}

func TestProcessorRejectsFileTooLarge(t *testing.T) {
	store := &fakeStorage{existsMap: map[string]bool{}}
	runner := &fakeRunner{info: domain.MediaInfo{Width: 1920, Height: 1080, DurationSeconds: 10, VideoCodec: "h264"}}
	var requests []capturedRequest
	processor := newTestProcessor(t, store, runner, &requests)
	processor.cfg.Transcode.MaxFileSizeBytes = 100 * 1024 * 1024 // 100 MB

	event := domain.UploadCompletedEvent{
		VideoID:   "video-huge",
		ObjectKey: "video-huge/source.mp4",
		Bucket:    "videos",
		Size:      500 * 1024 * 1024, // 500 MB
	}
	err := processor.Process(context.Background(), "job-huge", event)
	if err == nil {
		t.Fatalf("Process() error = nil, want error for oversized file")
	}

	var sawFailed bool
	for _, req := range requests {
		if req.method == http.MethodPost && req.body["eventType"] == "transcode.failed" {
			payload, _ := req.body["payload"].(map[string]any)
			if payload["reason"] == "file_too_large" {
				sawFailed = true
			}
		}
	}
	if !sawFailed {
		t.Fatalf("expected transcode.failed with reason=file_too_large, got requests = %#v", requests)
	}
	if len(store.downloads) != 0 {
		t.Fatalf("downloads = %v, want none (should fail before download)", store.downloads)
	}
}

func TestProcessorHandleDeliveryAndFailure(t *testing.T) {
	store := &fakeStorage{existsMap: map[string]bool{}}
	runner := &fakeRunner{probeErr: errors.New("probe failed")}
	var requests []capturedRequest
	processor := newTestProcessor(t, store, runner, &requests)

	delivery := queue.Delivery{
		Body: []byte(`{"videoId":"video-2","filename":"source.mp4","bucket":"videos"}`),
	}
	err := processor.HandleDelivery(context.Background(), delivery)
	if err == nil || !strings.Contains(err.Error(), "probe failed") {
		t.Fatalf("HandleDelivery() error = %v", err)
	}

	var sawFailed bool
	for _, req := range requests {
		if req.method == http.MethodPost && req.body["eventType"] == "transcode.failed" {
			sawFailed = true
		}
	}
	if !sawFailed {
		t.Fatalf("requests = %#v", requests)
	}
}

func TestProcessorGeneratesThumbnail(t *testing.T) {
	store := &fakeStorage{existsMap: map[string]bool{}}
	runner := &fakeRunner{info: domain.MediaInfo{
		Width:           1920,
		Height:          1080,
		DurationSeconds: 60,
		VideoCodec:      "h264",
		AudioCodec:      "aac",
	}}
	var requests []capturedRequest
	processor := newTestProcessor(t, store, runner, &requests)

	event := domain.UploadCompletedEvent{
		VideoID:   "video-thumb",
		Filename:  "source.mp4",
		ObjectKey: "video-thumb/source.mp4",
		Bucket:    "videos",
	}
	if err := processor.Process(context.Background(), "job-thumb", event); err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	// 1. ExtractThumbnail must have been called exactly once.
	if runner.thumbnailCalls != 1 {
		t.Fatalf("thumbnailCalls = %d, want 1", runner.thumbnailCalls)
	}

	// 2. Storage must have received an UploadFile for thumbnails/<videoID>.jpg.
	wantUploadKey := "thumbnails/video-thumb.jpg"
	var sawThumbnailUpload bool
	for _, upload := range store.uploads {
		if strings.HasPrefix(upload, wantUploadKey+"=") {
			sawThumbnailUpload = true
			break
		}
	}
	if !sawThumbnailUpload {
		t.Fatalf("uploads missing %s; got %v", wantUploadKey, store.uploads)
	}

	// 3. A PATCH to ingest for video-thumb must set thumbnail_status = "ready".
	var sawThumbnailPatch bool
	for _, req := range requests {
		if req.method == http.MethodPatch && strings.Contains(req.path, "/upload-state/videos/video-thumb") {
			if req.body["thumbnail_status"] == "ready" {
				sawThumbnailPatch = true
				break
			}
		}
	}
	if !sawThumbnailPatch {
		t.Fatalf("expected PATCH with thumbnail_status=ready for video-thumb; got requests = %#v", requests)
	}
}
