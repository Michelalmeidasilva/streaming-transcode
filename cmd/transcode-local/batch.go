package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"streaming-transcode/internal/config"
	"streaming-transcode/internal/domain"
	"streaming-transcode/internal/events"
	"streaming-transcode/internal/storage"
	"streaming-transcode/internal/transcode"
	"streaming-transcode/internal/worker"
)

// rawKeyPrefix is the storage prefix the upload platform writes original sources
// under (raw/{videoID}/{filename}). The S3 ObjectCreated -> EventBridge -> Batch
// rule filters on this prefix, so every Batch job key starts with it.
const rawKeyPrefix = "raw/"

// extractVideoID derives the video id from a raw object key of the form
// raw/{videoID}/{object...}. It returns an error for any key that does not
// carry both a non-empty id segment and an object beneath it.
func extractVideoID(key string) (string, error) {
	rest, ok := strings.CutPrefix(key, rawKeyPrefix)
	if !ok {
		return "", fmt.Errorf("key %q does not start with %q", key, rawKeyPrefix)
	}
	id, object, ok := strings.Cut(rest, "/")
	if !ok {
		return "", fmt.Errorf("key %q has no object beneath the video id", key)
	}
	if id == "" {
		return "", fmt.Errorf("key %q has an empty video id segment", key)
	}
	if strings.TrimSpace(object) == "" {
		return "", fmt.Errorf("key %q has no object name after the video id", key)
	}
	return id, nil
}

// buildBatchEvent reconstructs the minimal UploadCompletedEvent the Processor
// needs from just the S3 key the Batch job receives. The S3 ObjectCreated event
// carries no upload-time metadata (sidecar subtitles, raw-stream geometry), so
// those stay empty and the Processor relies on ffprobe and the configured
// profile. Headerless raw sources (.yuv) cannot be probed and have no geometry
// to supply here, so they are rejected up front with a clear error.
func buildBatchEvent(cfg config.Config, key string) (domain.UploadCompletedEvent, error) {
	videoID, err := extractVideoID(key)
	if err != nil {
		return domain.UploadCompletedEvent{}, err
	}
	if strings.EqualFold(filepath.Ext(key), ".yuv") {
		return domain.UploadCompletedEvent{}, fmt.Errorf("raw source %q is unsupported via the Batch trigger: headerless .yuv needs geometry not present in the S3 event", key)
	}
	return domain.UploadCompletedEvent{
		VideoID:   videoID,
		ObjectKey: key,
		Bucket:    cfg.Storage.Bucket,
		Provider:  "aws-s3",
	}, nil
}

// batchProcessor is the slice of worker.Processor the Batch entrypoint drives.
// Declaring it here lets runBatchJob be tested with a fake instead of a real
// ffmpeg/storage-backed processor.
type batchProcessor interface {
	Process(ctx context.Context, jobID string, event domain.UploadCompletedEvent) error
}

// batchJobID names the job for logs and the per-job work directory. AWS Batch
// injects AWS_BATCH_JOB_ID into the container; outside Batch (or in tests) we
// synthesize a video-scoped, time-unique id.
func batchJobID(videoID string) string {
	if id := strings.TrimSpace(os.Getenv("AWS_BATCH_JOB_ID")); id != "" {
		return id
	}
	return fmt.Sprintf("%s-%d", videoID, time.Now().UnixNano())
}

// runBatchJob is the Batch-mode entrypoint: turn the triggering S3 key into an
// event and hand it to the Processor, which runs the full download -> ladder ->
// package -> upload -> persist pipeline. An invalid key fails before any work
// starts; a processing failure propagates so main can exit non-zero (Batch then
// marks the job FAILED and it can be reprocessed).
func runBatchJob(ctx context.Context, proc batchProcessor, cfg config.Config, key string) error {
	event, err := buildBatchEvent(cfg, key)
	if err != nil {
		return err
	}
	return proc.Process(ctx, batchJobID(event.VideoID), event)
}

// runBatchMode wires the production dependencies (S3-backed storage, the Event
// Gateway client, the ffmpeg runner) and runs the job for key, returning the
// process exit code: 0 = SUCCEEDED, 1 = FAILED (reprocessable by Batch). It
// mirrors cmd/worker's wiring but is driven by a single S3 key instead of a
// RabbitMQ delivery.
func runBatchMode(key string) int {
	cfg := config.FromEnv()
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := storage.New(cfg.Storage)
	if err != nil {
		logger.Printf("storage init failed: %v", err)
		return 1
	}

	proc := worker.NewProcessor(worker.Dependencies{
		Config:  cfg,
		Storage: store,
		Events:  events.NewGatewayClient(cfg.EventGatewayURL),
		Runner:  transcode.NewFFmpegRunner(cfg.Transcode),
		Logger:  logger,
	})

	logger.Printf("transcode batch job started key=%s bucket=%s gateway=%s", key, cfg.Storage.Bucket, cfg.EventGatewayURL)
	if err := runBatchJob(ctx, proc, cfg, key); err != nil {
		logger.Printf("transcode batch job failed key=%s: %v", key, err)
		return 1
	}
	logger.Printf("transcode batch job succeeded key=%s", key)
	return 0
}
