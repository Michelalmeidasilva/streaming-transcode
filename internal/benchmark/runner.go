package benchmark

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"streaming-transcode/internal/domain"
)

// Transcoder is the encode surface the runner needs (transcode.Runner satisfies it).
type Transcoder interface {
	Probe(source string) (domain.MediaInfo, error)
	TranscodeRendition(ctx context.Context, source string, raw *domain.RawVideoParams, rendition domain.Rendition, output string) (domain.RenditionMetrics, error)
}

// Storage is the corpus access the runner needs (storage.ObjectStorage satisfies it).
type Storage interface {
	List(ctx context.Context, bucket, prefix string) ([]string, error)
	Download(ctx context.Context, bucket, key, destination string) error
}

// Deps are the runner's collaborators.
type Deps struct {
	Storage Storage
	Runner  Transcoder
	Sink    ResultSink
	WorkDir string
	Logf    func(format string, args ...any)
}

// Run executes the benchmark matrix serially: for each job, download the clip
// (cached per clip), encode the single rendition (measured), POST the result, and
// delete the output. Encode/post failures are logged and skipped; Run returns an
// error if any job failed so the operator sees a partial matrix.
func Run(ctx context.Context, cfg Config, deps Deps) error {
	logf := deps.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	clips := cfg.Clips
	if len(clips) == 0 {
		listed, err := deps.Storage.List(ctx, cfg.CorpusBucket, cfg.CorpusPrefix)
		if err != nil {
			return fmt.Errorf("list corpus %s/%s: %w", cfg.CorpusBucket, cfg.CorpusPrefix, err)
		}
		clips = listed
	}
	if len(clips) == 0 {
		return fmt.Errorf("corpus is empty (bucket=%s prefix=%s)", cfg.CorpusBucket, cfg.CorpusPrefix)
	}
	cfg.Clips = clips

	if err := os.MkdirAll(deps.WorkDir, 0o755); err != nil {
		return err
	}

	hostname, _ := os.Hostname()
	downloaded := map[string]string{}
	probed := map[string]domain.MediaInfo{}
	var failures int

	for _, job := range ExpandMatrix(cfg) {
		localClip, ok := downloaded[job.Clip]
		if !ok {
			localClip = filepath.Join(deps.WorkDir, fmt.Sprintf("clip-%d%s", len(downloaded), filepath.Ext(job.Clip)))
			if err := deps.Storage.Download(ctx, cfg.CorpusBucket, job.Clip, localClip); err != nil {
				logf("download %s failed: %v", job.Clip, err)
				failures++
				continue
			}
			downloaded[job.Clip] = localClip
			info, perr := deps.Runner.Probe(localClip)
			if perr != nil {
				logf("probe %s failed (continuing without source info): %v", job.Clip, perr)
				info = domain.MediaInfo{}
			}
			probed[job.Clip] = info
		}

		out := filepath.Join(deps.WorkDir, fmt.Sprintf("out-%s-%dx%d-r%d.mp4", job.Codec, job.Resolution.Width, job.Resolution.Height, job.Repetition))
		rendition := domain.Rendition{
			Name:        fmt.Sprintf("%s-%dp", job.Codec, job.Resolution.Height),
			Width:       job.Resolution.Width,
			Height:      job.Resolution.Height,
			BitrateKbps: job.Resolution.BitrateKbps,
			Codec:       job.Codec,
		}
		metrics, err := deps.Runner.TranscodeRendition(ctx, localClip, nil, rendition, out)
		_ = os.Remove(out)
		if err != nil {
			logf("encode %s %s %dp rep%d failed: %v", job.Clip, job.Codec, job.Resolution.Height, job.Repetition, err)
			failures++
			continue
		}

		res := Result{
			Benchmark:             true,
			MachineLabel:          cfg.MachineLabel,
			Hostname:              hostname,
			CPUCores:              runtime.NumCPU(),
			SourceWidth:           probed[job.Clip].Width,
			SourceHeight:          probed[job.Clip].Height,
			SourceDurationSeconds: probed[job.Clip].DurationSeconds,
			SourceFPS:             probed[job.Clip].FPS,
			SourceCodec:           probed[job.Clip].VideoCodec,
			SourceBitrateKbps:     probed[job.Clip].BitrateKbps,
			SourceFileSizeBytes:   probed[job.Clip].SizeBytes,
			Clip:                  job.Clip,
			Repetition:            job.Repetition,
			ElapsedSeconds:        metrics.ElapsedSeconds,
			CompletedAt:           time.Now().UTC().Format(time.RFC3339),
			Renditions: []ResultRendition{{
				Name:              rendition.Name,
				Codec:             job.Codec,
				Width:             job.Resolution.Width,
				Height:            job.Resolution.Height,
				TargetBitrateKbps: job.Resolution.BitrateKbps,
				OutputBitrateKbps: metrics.OutputBitrateKbps,
				ElapsedSeconds:    metrics.ElapsedSeconds,
				AvgCPUPercent:     metrics.ResourceUsage.AvgCPUPercent,
				MaxCPUPercent:     metrics.ResourceUsage.MaxCPUPercent,
				AvgMemoryMB:       metrics.ResourceUsage.AvgMemoryMB,
				MaxMemoryMB:       metrics.ResourceUsage.MaxMemoryMB,
			}},
		}
		if err := postWithRetry(ctx, deps.Sink, res, logf); err != nil {
			logf("post result for %s %s failed after retries: %v", job.Clip, job.Codec, err)
			failures++
		}
	}

	if failures > 0 {
		return fmt.Errorf("benchmark completed with %d failed job(s)", failures)
	}
	return nil
}

func postWithRetry(ctx context.Context, sink ResultSink, res Result, logf func(string, ...any)) error {
	var err error
	for attempt := 1; attempt <= 3; attempt++ {
		if err = sink.Post(ctx, res); err == nil {
			return nil
		}
		logf("post attempt %d failed: %v", attempt, err)
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return err
}
