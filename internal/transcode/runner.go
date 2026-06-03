package transcode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"streaming-transcode/internal/config"
	"streaming-transcode/internal/domain"
)

type Runner struct {
	cfg config.TranscodeConfig
}

func NewFFmpegRunner(cfg config.TranscodeConfig) *Runner {
	return &Runner{cfg: cfg}
}

func (r *Runner) Probe(source string) (domain.MediaInfo, error) {
	return Probe(r.cfg.FFprobePath, source)
}

func (r *Runner) TranscodeRendition(ctx context.Context, source string, rendition domain.Rendition, output string) (domain.RenditionMetrics, error) {
	startedAt := time.Now().UTC()
	preset := strings.TrimSpace(rendition.Preset)
	if preset == "" {
		preset = r.cfg.Preset
	}
	codec := encodingCodecSettings(rendition.Codec, preset)
	sourceInfo, err := r.Probe(source)
	if err != nil {
		return domain.RenditionMetrics{}, err
	}
	sourceSize := sourceInfo.SizeBytes
	if sourceSize == 0 {
		sourceSize = fileSizeBytes(source)
	}

	args := []string{
		"-y",
		"-i", source,
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", rendition.Width, rendition.Height, rendition.Width, rendition.Height),
		"-c:v", codec.encoder,
	}
	if normalizeCodec(rendition.Codec) == "av1" {
		args = append(args, "-b:v", "0", "-crf", av1CRF(rendition))
	} else {
		args = append(args,
			"-b:v", fmt.Sprintf("%dk", rendition.BitrateKbps),
			"-maxrate", fmt.Sprintf("%dk", rendition.BitrateKbps),
			"-bufsize", fmt.Sprintf("%dk", rendition.BitrateKbps*2),
		)
	}
	args = append(args, codec.args...)
	args = append(args,
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
		output,
	)
	resourceUsage, ffmpegOutput, err := runObserved(ctx, r.cfg.FFmpegPath, args...)
	completedAt := time.Now().UTC()
	metrics := domain.RenditionMetrics{
		Status:              "completed",
		StartedAt:           startedAt,
		CompletedAt:         completedAt,
		SourceFileSizeBytes: sourceSize,
		SourceDuration:      sourceInfo.DurationSeconds,
		SourceBitrateKbps:   sourceInfo.BitrateKbps,
		SourceFPS:           sourceInfo.FPS,
		SourceCodec:         sourceInfo.VideoCodec,
		TargetBitrateKbps:   rendition.BitrateKbps,
		ElapsedSeconds:      completedAt.Sub(startedAt).Seconds(),
		ResourceUsage:       resourceUsage,
	}
	if sourceSize > 0 {
		metrics.CompressionRatio = float64(metrics.OutputFileSizeBytes) / float64(sourceSize)
	}
	if sourceInfo.DurationSeconds > 0 {
		metrics.RTF = metrics.ElapsedSeconds / sourceInfo.DurationSeconds
	}
	if err != nil {
		metrics.Status = "failed"
		metrics.ErrorMessage = ffmpegOutput
		return metrics, err
	}

	outputInfo, probeErr := r.Probe(output)
	if probeErr != nil {
		metrics.Status = "failed"
		metrics.ErrorMessage = probeErr.Error()
		return metrics, probeErr
	}
	metrics.OutputFileSizeBytes = outputInfo.SizeBytes
	if metrics.OutputFileSizeBytes == 0 {
		metrics.OutputFileSizeBytes = fileSizeBytes(output)
	}
	metrics.OutputDuration = outputInfo.DurationSeconds
	metrics.OutputBitrateKbps = outputInfo.BitrateKbps
	metrics.OutputFPS = outputInfo.FPS
	metrics.OutputCodec = outputInfo.VideoCodec
	if sourceSize > 0 {
		metrics.CompressionRatio = float64(metrics.OutputFileSizeBytes) / float64(sourceSize)
	}
	return metrics, nil
}

func av1CRF(rendition domain.Rendition) string {
	switch {
	case rendition.Height >= 1080:
		return "30"
	case rendition.Height >= 720:
		return "32"
	default:
		return "34"
	}
}

func (r *Runner) PackageHLS(ctx context.Context, renditionFile string, renditionName string, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	args := []string{
		"-y",
		"-i", renditionFile,
		"-c", "copy",
		"-f", "hls",
		"-hls_time", "6",
		"-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", filepath.Join(outDir, "segment-%05d.m4s"),
		filepath.Join(outDir, "index.m3u8"),
	}
	return run(ctx, r.cfg.FFmpegPath, args...)
}

func (r *Runner) PackageDASH(ctx context.Context, renditionFiles []string, outDir string) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	args := []string{"-y"}
	for _, file := range renditionFiles {
		args = append(args, "-i", file)
	}
	for index := range renditionFiles {
		args = append(args, "-map", fmt.Sprintf("%d:v:0", index))
		if index == 0 {
			args = append(args, "-map", fmt.Sprintf("%d:a:0?", index))
		}
	}
	args = append(args,
		"-c", "copy",
		"-f", "dash",
		filepath.Join(outDir, "manifest.mpd"),
	)
	return run(ctx, r.cfg.FFmpegPath, args...)
}

// WriteHLSMaster writes the HLS multivariant playlist. The audio codec
// (mp4a.40.2) is only advertised when hasAudio is true: advertising an audio
// track that the (video-only) segments do not contain makes players initialize
// the MSE audio SourceBuffer and then fail the append (Shaka error 3014).
func WriteHLSMaster(path string, renditions []domain.Rendition, hasAudio bool) error {
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n")
	for _, rendition := range renditions {
		bandwidth := rendition.BitrateKbps * 1000
		codecs := hlsCodecString(rendition.Codec)
		if hasAudio {
			codecs += ",mp4a.40.2"
		}
		builder.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=\"%s\"\n", bandwidth, rendition.Width, rendition.Height, codecs))
		builder.WriteString(fmt.Sprintf("%s/index.m3u8\n", rendition.Name))
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func hlsCodecString(codec string) string {
	switch normalizeCodec(codec) {
	case "h265":
		return "hvc1.1.6.L120.90"
	case "av1":
		return "av01.0.08M.08"
	case "vp9":
		return "vp09.00.51.08"
	case "vvc":
		return "vvc1.1.L120.C0"
	default:
		return "avc1.640028"
	}
}

type codecSettings struct {
	encoder string
	args    []string
}

func encodingCodecSettings(codec string, preset string) codecSettings {
	switch normalizeCodec(codec) {
	case "h265":
		return codecSettings{
			encoder: "libx265",
			args:    []string{"-preset", preset, "-pix_fmt", "yuv420p", "-tag:v", "hvc1"},
		}
	case "av1":
		return codecSettings{
			encoder: "libsvtav1",
			args:    []string{"-preset", av1Preset(preset), "-pix_fmt", "yuv420p"},
		}
	case "vp9":
		return codecSettings{
			encoder: "libvpx-vp9",
			args:    []string{"-deadline", "good", "-cpu-used", vp9CPUUsed(preset), "-pix_fmt", "yuv420p"},
		}
	case "vvc":
		return codecSettings{
			encoder: "libvvenc",
			args:    []string{"-preset", vvcPreset(preset), "-pix_fmt", "yuv420p"},
		}
	default:
		return codecSettings{
			encoder: "libx264",
			args:    []string{"-preset", preset, "-pix_fmt", "yuv420p"},
		}
	}
}

func av1Preset(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "veryslow", "slower", "slow":
		return "4"
	case "medium":
		return "6"
	case "fast":
		return "8"
	case "faster", "veryfast", "superfast", "ultrafast":
		return "10"
	default:
		return "8"
	}
}

func vp9CPUUsed(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "veryslow", "slower", "slow":
		return "1"
	case "medium":
		return "2"
	case "fast":
		return "3"
	case "faster", "veryfast", "superfast", "ultrafast":
		return "5"
	default:
		return "3"
	}
}

func vvcPreset(preset string) string {
	switch strings.ToLower(strings.TrimSpace(preset)) {
	case "veryslow", "slower", "slow", "medium", "fast", "faster":
		return strings.ToLower(strings.TrimSpace(preset))
	case "veryfast", "superfast", "ultrafast":
		return "faster"
	default:
		return "medium"
	}
}

func run(ctx context.Context, binary string, args ...string) error {
	_, _, err := runObserved(ctx, binary, args...)
	return err
}

func buildThumbnailArgs(source, output string, atSeconds float64) []string {
	return []string{
		"-ss", fmt.Sprintf("%.3f", atSeconds),
		"-i", source,
		"-frames:v", "1",
		"-vf", "scale=640:-2",
		"-q:v", "3",
		"-y", output,
	}
}

// ExtractThumbnail captures a single poster frame at atSeconds into output (jpg).
func (r *Runner) ExtractThumbnail(ctx context.Context, source, output string, atSeconds float64) error {
	if atSeconds < 1 {
		atSeconds = 1
	}
	return run(ctx, r.cfg.FFmpegPath, buildThumbnailArgs(source, output, atSeconds)...)
}
