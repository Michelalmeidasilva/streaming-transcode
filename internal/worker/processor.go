package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"streaming-transcode/internal/config"
	"streaming-transcode/internal/domain"
	"streaming-transcode/internal/events"
	"streaming-transcode/internal/queue"
	"streaming-transcode/internal/storage"
	"streaming-transcode/internal/transcode"
)

type TranscodeRunner interface {
	Probe(source string) (domain.MediaInfo, error)
	TranscodeRendition(ctx context.Context, source string, rendition domain.Rendition, output string) (domain.RenditionMetrics, error)
	PackageHLS(ctx context.Context, renditionFile string, renditionName string, outDir string) error
	PackageDASH(ctx context.Context, renditionFiles []string, outDir string) error
}

type Dependencies struct {
	Config   config.Config
	Storage  storage.ObjectStorage
	Events   *events.GatewayClient
	Runner   TranscodeRunner
	Logger   *log.Logger
	ClockNow func() time.Time
}

type Processor struct {
	cfg      config.Config
	storage  storage.ObjectStorage
	events   *events.GatewayClient
	runner   TranscodeRunner
	logger   *log.Logger
	clockNow func() time.Time
}

func NewProcessor(deps Dependencies) *Processor {
	now := deps.ClockNow
	if now == nil {
		now = time.Now
	}
	return &Processor{
		cfg:      deps.Config,
		storage:  deps.Storage,
		events:   deps.Events,
		runner:   deps.Runner,
		logger:   deps.Logger,
		clockNow: now,
	}
}

func (p *Processor) HandleDelivery(ctx context.Context, delivery queue.Delivery) error {
	event, err := ParseUploadCompleted(delivery.Body, p.cfg.Storage.Bucket)
	if err != nil {
		return err
	}

	jobID := fmt.Sprintf("%s-%d", event.VideoID, p.clockNow().UnixNano())
	jobCtx, cancel := context.WithTimeout(ctx, p.cfg.Transcode.JobTimeout)
	defer cancel()

	return p.process(jobCtx, jobID, delivery.Attempt, event)
}

func (p *Processor) Process(ctx context.Context, jobID string, event domain.UploadCompletedEvent) error {
	return p.process(ctx, jobID, 1, event)
}

func (p *Processor) process(ctx context.Context, jobID string, attempt int, event domain.UploadCompletedEvent) error {
	started := p.clockNow()
	hostname, _ := os.Hostname()
	profile := strings.TrimSpace(event.Transcode.Profile)
	if profile == "" {
		profile = p.cfg.Transcode.Profile
	}
	sourceKey := event.ObjectKey
	hlsManifest := fmt.Sprintf("transcoded/%s/hls/master.m3u8", event.VideoID)
	metricsPath := fmt.Sprintf("metrics/%s/transcode-result.json", event.VideoID)
	observabilityPath := fmt.Sprintf("metrics/%s/observability.json", event.VideoID)
	fingerprint := processingFingerprint(event.VideoID, profile, sourceKey, event.SourceETag, event.SourceVersion)

	if exists, err := p.storage.Exists(ctx, event.Bucket, hlsManifest); err != nil {
		return err
	} else if exists {
		p.logger.Printf("videoId=%s already transcoded, publishing ready", event.VideoID)
		return p.markReady(ctx, event.VideoID)
	}

	if err := p.setProcessingState(ctx, "transcode.queued", event.VideoID, "queued", 5, map[string]any{
		"jobId":       jobID,
		"profile":     profile,
		"sourceKey":   sourceKey,
		"sourceETag":  event.SourceETag,
		"attempt":     attempt,
		"fingerprint": fingerprint,
		"queuedAt":    started.UTC().Format(time.RFC3339),
	}); err != nil {
		p.logger.Printf("queued state publish failed: %v", err)
	}

	if err := p.publishStatus(ctx, "transcode.started", event.VideoID, map[string]any{
		"jobId":       jobID,
		"sourceKey":   sourceKey,
		"sourceETag":  event.SourceETag,
		"attempt":     attempt,
		"fingerprint": fingerprint,
		"profile":     profile,
		"startedAt":   started.UTC().Format(time.RFC3339),
	}); err != nil {
		p.logger.Printf("status publish failed: %v", err)
	}
	if err := p.patchVideo(ctx, event.VideoID, map[string]any{
		"processingStatus": "transcoding",
		"source": map[string]any{
			"bucket":   event.Bucket,
			"key":      sourceKey,
			"provider": event.Provider,
			"size":     event.Size,
			"etag":     event.SourceETag,
			"version":  event.SourceVersion,
		},
		"transcode": map[string]any{
			"jobId":       jobID,
			"profile":     profile,
			"attempt":     attempt,
			"fingerprint": fingerprint,
			"startedAt":   started.UTC().Format(time.RFC3339),
			"error":       nil,
		},
	}); err != nil {
		p.logger.Printf("transcoding state patch failed: %v", err)
	}

	workDir := filepath.Join(p.cfg.Transcode.WorkDir, jobID)
	sourcePath := filepath.Join(workDir, "source", filepath.Base(sourceKey))
	outputDir := filepath.Join(workDir, "output")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(workDir)

	if err := p.storage.Download(ctx, event.Bucket, sourceKey, sourcePath); err != nil {
		_ = p.fail(ctx, event.VideoID, jobID, attempt, fingerprint, "download_failed", err)
		return err
	}
	sourceSizeBytes := fileSizeBytes(sourcePath)
	_ = p.progress(ctx, event.VideoID, jobID, attempt, fingerprint, "downloaded", 20)

	info, err := p.runner.Probe(sourcePath)
	if err != nil {
		_ = p.fail(ctx, event.VideoID, jobID, attempt, fingerprint, "ffprobe_failed", err)
		return err
	}
	_ = p.progress(ctx, event.VideoID, jobID, attempt, fingerprint, "probed", 30)

	renditions := transcode.ResolveRenditions(info, event.Transcode, p.cfg.Transcode.Codecs)
	renditionFiles := make([]string, 0, len(renditions))
	renditionMetrics := make([]domain.RenditionMetrics, 0, len(renditions))
	for i, rendition := range renditions {
		outFile := filepath.Join(outputDir, rendition.Name+".mp4")
		metrics, err := p.runner.TranscodeRendition(ctx, sourcePath, rendition, outFile)
		if err != nil {
			_ = p.fail(ctx, event.VideoID, jobID, attempt, fingerprint, "ffmpeg_failed", err)
			return err
		}
		renditions[i].OutputPath = outFile
		renditions[i].ManifestPath = fmt.Sprintf("transcoded/%s/hls/%s/index.m3u8", event.VideoID, rendition.Name)
		renditions[i].Metrics = &metrics
		renditionFiles = append(renditionFiles, outFile)
		renditionMetrics = append(renditionMetrics, metrics)
		progress := 35 + int(float64(i+1)/float64(len(renditions))*35)
		_ = p.progress(ctx, event.VideoID, jobID, attempt, fingerprint, "rendition.completed", progress, map[string]any{
			"rendition":      rendition.Name,
			"completed":      i + 1,
			"total":          len(renditions),
			"elapsedSeconds": metrics.ElapsedSeconds,
			"avgCpuPercent":  metrics.ResourceUsage.AvgCPUPercent,
			"maxCpuPercent":  metrics.ResourceUsage.MaxCPUPercent,
		})
	}

	hlsDir := filepath.Join(outputDir, "hls")
	if err := p.setProcessingState(ctx, "transcode.progress", event.VideoID, "packaging", 75, map[string]any{
		"jobId":       jobID,
		"attempt":     attempt,
		"fingerprint": fingerprint,
		"phase":       "packaging.started",
	}); err != nil {
		p.logger.Printf("packaging state publish failed: %v", err)
	}
	for _, rendition := range renditions {
		if err := p.runner.PackageHLS(ctx, rendition.OutputPath, rendition.Name, filepath.Join(hlsDir, rendition.Name)); err != nil {
			_ = p.fail(ctx, event.VideoID, jobID, attempt, fingerprint, "hls_failed", err)
			return err
		}
	}
	if err := transcode.WriteHLSMaster(filepath.Join(hlsDir, "master.m3u8"), renditions); err != nil {
		_ = p.fail(ctx, event.VideoID, jobID, attempt, fingerprint, "hls_master_failed", err)
		return err
	}

	dashDir := filepath.Join(outputDir, "dash")
	if err := p.runner.PackageDASH(ctx, renditionFiles, dashDir); err != nil {
		_ = p.fail(ctx, event.VideoID, jobID, attempt, fingerprint, "dash_failed", err)
		return err
	}
	_ = p.progress(ctx, event.VideoID, jobID, attempt, fingerprint, "packaged", 85)

	if err := p.uploadDir(ctx, event.Bucket, filepath.Join(outputDir, "hls"), fmt.Sprintf("transcoded/%s/hls", event.VideoID)); err != nil {
		return err
	}
	if err := p.uploadDir(ctx, event.Bucket, filepath.Join(outputDir, "dash"), fmt.Sprintf("transcoded/%s/dash", event.VideoID)); err != nil {
		return err
	}
	_ = p.progress(ctx, event.VideoID, jobID, attempt, fingerprint, "outputs.uploaded", 92)

	elapsed := p.clockNow().Sub(started).Seconds()
	rtf := 0.0
	if info.DurationSeconds > 0 {
		rtf = elapsed / info.DurationSeconds
	}
	totalOutputSizeBytes := int64(0)
	for _, metrics := range renditionMetrics {
		totalOutputSizeBytes += metrics.OutputFileSizeBytes
	}
	observability := domain.JobObservability{
		Hostname:             hostname,
		CPUCores:             runtime.NumCPU(),
		StartedAt:            started.UTC(),
		CompletedAt:          p.clockNow().UTC(),
		SourceFileSizeBytes:  sourceSizeBytes,
		TotalOutputSizeBytes: totalOutputSizeBytes,
		SuccessfulRenditions: len(renditionMetrics),
		Renditions:           renditionMetrics,
	}

	result := domain.TranscodeResult{
		VideoID:           event.VideoID,
		JobID:             jobID,
		Profile:           profile,
		SourceKey:         sourceKey,
		SourceETag:        event.SourceETag,
		SourceVersion:     event.SourceVersion,
		Fingerprint:       fingerprint,
		Attempt:           attempt,
		MediaInfo:         info,
		Renditions:        renditions,
		HLSManifestPath:   hlsManifest,
		DASHManifestPath:  fmt.Sprintf("transcoded/%s/dash/manifest.mpd", event.VideoID),
		MetricsPath:       metricsPath,
		ObservabilityPath: observabilityPath,
		ElapsedSeconds:    elapsed,
		RTF:               rtf,
		Observability:     observability,
		CompletedAt:       p.clockNow().UTC(),
	}

	if err := p.writeAndUploadJSON(ctx, event.Bucket, filepath.Join(outputDir, "media-info.json"), fmt.Sprintf("metrics/%s/media-info.json", event.VideoID), info); err != nil {
		return err
	}
	if err := p.writeAndUploadJSON(ctx, event.Bucket, filepath.Join(outputDir, "observability.json"), observabilityPath, observability); err != nil {
		return err
	}
	if err := p.writeAndUploadJSON(ctx, event.Bucket, filepath.Join(outputDir, "transcode-result.json"), metricsPath, result); err != nil {
		return err
	}

	if err := p.complete(ctx, result); err != nil {
		return err
	}
	return nil
}

func (p *Processor) uploadDir(ctx context.Context, bucket, dir, prefix string) error {
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(filepath.Join(prefix, rel))
		return p.storage.UploadFile(ctx, bucket, key, path)
	})
}

func (p *Processor) writeAndUploadJSON(ctx context.Context, bucket, localPath, key string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		return err
	}
	return p.storage.UploadFile(ctx, bucket, key, localPath)
}

func (p *Processor) complete(ctx context.Context, result domain.TranscodeResult) error {
	payload := map[string]any{
		"jobId":             result.JobID,
		"status":            "completed",
		"profile":           result.Profile,
		"sourceKey":         result.SourceKey,
		"durationSeconds":   result.MediaInfo.DurationSeconds,
		"elapsedSeconds":    result.ElapsedSeconds,
		"rtf":               result.RTF,
		"renditions":        result.Renditions,
		"hlsManifestPath":   result.HLSManifestPath,
		"dashManifestPath":  result.DASHManifestPath,
		"metricsPath":       result.MetricsPath,
		"observabilityPath": result.ObservabilityPath,
		"observability":     result.Observability,
		"sourceETag":        result.SourceETag,
		"sourceVersion":     result.SourceVersion,
		"attempt":           result.Attempt,
		"fingerprint":       result.Fingerprint,
		"completedAt":       result.CompletedAt.Format(time.RFC3339),
	}
	if err := p.publishStatus(ctx, "packaging.completed", result.VideoID, payload); err != nil {
		p.logger.Printf("packaging event failed: %v", err)
	}
	if err := p.publishStatus(ctx, "transcode.completed", result.VideoID, payload); err != nil {
		p.logger.Printf("transcode completed event failed: %v", err)
	}
	patch := map[string]any{
		"status":           "ready",
		"processingStatus": "ready",
		"mediaInfo":        result.MediaInfo,
		"transcode": map[string]any{
			"jobId":       result.JobID,
			"profile":     result.Profile,
			"attempt":     result.Attempt,
			"fingerprint": result.Fingerprint,
			"completedAt": result.CompletedAt.Format(time.RFC3339),
			"error":       nil,
		},
		"playback": map[string]any{
			"hlsManifestPath":  result.HLSManifestPath,
			"dashManifestPath": result.DASHManifestPath,
			"renditions":       result.Renditions,
		},
		"metrics": map[string]any{
			"rtf":                  result.RTF,
			"elapsedSeconds":       result.ElapsedSeconds,
			"metricsPath":          result.MetricsPath,
			"observabilityPath":    result.ObservabilityPath,
			"sourceFileSizeBytes":  result.Observability.SourceFileSizeBytes,
			"totalOutputSizeBytes": result.Observability.TotalOutputSizeBytes,
		},
	}
	if err := p.events.PatchVideo(ctx, result.VideoID, patch); err != nil {
		return err
	}
	return p.markReady(ctx, result.VideoID)
}

func (p *Processor) markReady(ctx context.Context, videoID string) error {
	return p.publishStatus(ctx, "ready", videoID, map[string]any{"status": "ready"})
}

func (p *Processor) fail(ctx context.Context, videoID, jobID string, attempt int, fingerprint, reason string, cause error) error {
	patch := map[string]any{
		"status":           "error",
		"processingStatus": "failed",
		"transcode": map[string]any{
			"jobId":       jobID,
			"attempt":     attempt,
			"fingerprint": fingerprint,
			"error": map[string]any{
				"reason":  reason,
				"message": cause.Error(),
			},
		},
	}
	_ = p.events.PatchVideo(ctx, videoID, patch)
	return p.publishStatus(ctx, "transcode.failed", videoID, map[string]any{
		"jobId":       jobID,
		"attempt":     attempt,
		"fingerprint": fingerprint,
		"reason":      reason,
		"message":     cause.Error(),
	})
}

func (p *Processor) publishStatus(ctx context.Context, eventType, videoID string, payload map[string]any) error {
	payload["videoId"] = videoID
	return p.events.SendEvent(ctx, eventType, payload)
}

func (p *Processor) patchVideo(ctx context.Context, videoID string, patch map[string]any) error {
	return p.events.PatchVideo(ctx, videoID, patch)
}

func (p *Processor) setProcessingState(ctx context.Context, eventType, videoID, status string, percent int, payload map[string]any) error {
	payload["processingStatus"] = status
	payload["progress"] = percent
	if err := p.publishStatus(ctx, eventType, videoID, payload); err != nil {
		return err
	}
	return p.patchVideo(ctx, videoID, map[string]any{
		"processingStatus": status,
		"progress":         percent,
	})
}

func (p *Processor) progress(ctx context.Context, videoID, jobID string, attempt int, fingerprint, phase string, percent int, extra ...map[string]any) error {
	payload := map[string]any{
		"jobId":       jobID,
		"attempt":     attempt,
		"fingerprint": fingerprint,
		"phase":       phase,
		"progress":    percent,
		"updatedAt":   p.clockNow().UTC().Format(time.RFC3339),
	}
	if len(extra) > 0 {
		for key, value := range extra[0] {
			payload[key] = value
		}
	}
	return p.publishStatus(ctx, "transcode.progress", videoID, payload)
}

func processingFingerprint(videoID, profile, sourceKey, sourceETag, sourceVersion string) string {
	hash := sha256.Sum256([]byte(videoID + "\x00" + profile + "\x00" + sourceKey + "\x00" + sourceETag + "\x00" + sourceVersion))
	return hex.EncodeToString(hash[:])
}

func fileSizeBytes(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
