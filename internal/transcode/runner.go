package transcode

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
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

// probeSource returns media info for the source. For headerless raw streams
// ffprobe cannot read geometry, so the info is synthesized from the
// upload-supplied RawVideoParams instead.
func (r *Runner) probeSource(source string, raw *domain.RawVideoParams) (domain.MediaInfo, error) {
	if raw == nil {
		return r.Probe(source)
	}
	return rawMediaInfo(source, raw), nil
}

// rawMediaInfo builds MediaInfo for a raw stream. Duration/bitrate are unknown
// (no container), so they stay zero; size falls back to the file on disk.
func rawMediaInfo(source string, raw *domain.RawVideoParams) domain.MediaInfo {
	return domain.MediaInfo{
		Width:      raw.Width,
		Height:     raw.Height,
		FPS:        raw.FPS,
		VideoCodec: "rawvideo",
		SizeBytes:  fileSizeBytes(source),
	}
}

// rawInputArgs returns the ffmpeg flags that select the input. Headerless raw
// streams (.yuv) carry no geometry, so they must be demuxed as rawvideo with an
// explicit pixel format, frame size and rate before -i; everything else lets
// ffmpeg probe the container.
func rawInputArgs(source string, raw *domain.RawVideoParams) []string {
	if raw == nil {
		return []string{"-i", source}
	}
	pixfmt := strings.TrimSpace(raw.PixelFormat)
	if pixfmt == "" {
		pixfmt = domain.DefaultRawPixelFormat
	}
	return []string{
		"-f", "rawvideo",
		"-pix_fmt", pixfmt,
		"-s", fmt.Sprintf("%dx%d", raw.Width, raw.Height),
		"-framerate", strconv.FormatFloat(raw.FPS, 'f', -1, 64),
		"-i", source,
	}
}

func (r *Runner) TranscodeRendition(ctx context.Context, source string, raw *domain.RawVideoParams, rendition domain.Rendition, output string) (domain.RenditionMetrics, error) {
	startedAt := time.Now().UTC()
	preset := strings.TrimSpace(rendition.Preset)
	if preset == "" {
		preset = r.cfg.Preset
	}
	codec, codecErr := encodingCodecSettings(rendition.Codec, preset, r.cfg.EncoderBackend)
	if codecErr != nil {
		return domain.RenditionMetrics{Status: "failed", StartedAt: startedAt, CompletedAt: time.Now().UTC(), ErrorMessage: codecErr.Error()}, codecErr
	}
	sourceInfo, err := r.probeSource(source, raw)
	if err != nil {
		return domain.RenditionMetrics{}, err
	}
	sourceSize := sourceInfo.SizeBytes
	if sourceSize == 0 {
		sourceSize = fileSizeBytes(source)
	}

	args := []string{"-y"}
	args = append(args, rawInputArgs(source, raw)...)
	args = append(args,
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", rendition.Width, rendition.Height, rendition.Width, rendition.Height),
		"-c:v", codec.encoder,
	)
	if rendition.QualityValue > 0 {
		// R-D sweep: constant quality, bitrate floats.
		args = append(args, constantQualityArgs(r.cfg.EncoderBackend, rendition.QualityValue)...)
	} else if normalizeCodec(rendition.Codec) == "av1" && !r.cfg.ForceCappedVBR {
		// av1 defaults to CRF for quality. In a throughput benchmark this is set aside
		// (ForceCappedVBR) so av1 is rate-controlled identically to the other codecs and
		// av1+NVENC does not receive a -crf it would silently ignore.
		args = append(args, "-b:v", "0", "-crf", av1CRF(rendition))
	} else {
		args = append(args, cappedVBRArgs(rendition.Codec, rendition.BitrateKbps)...)
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

// cappedVBRArgs builds constrained-VBR rate control targeting kbps. h264/h265/vp9/vvc
// use maxrate == target (near-CBR, unchanged production behavior). libsvtav1 rejects
// maxrate == target ("Max Bitrate must be greater than Target Bitrate"), so av1 gets a
// modest 1.5× ceiling — still constraining peaks while hitting the same average bitrate,
// which is what a throughput comparison needs across codecs/backends.
func cappedVBRArgs(codec string, kbps int) []string {
	maxKbps := kbps
	if normalizeCodec(codec) == "av1" {
		maxKbps = kbps * 3 / 2
	}
	return []string{
		"-b:v", fmt.Sprintf("%dk", kbps),
		"-maxrate", fmt.Sprintf("%dk", maxKbps),
		"-bufsize", fmt.Sprintf("%dk", kbps*2),
	}
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

// hlsArgs builds the ffmpeg argument list for packaging one rendition into an
// fMP4 HLS media playlist. segmentSeconds controls -hls_time; non-positive
// values fall back to the 6s default.
func hlsArgs(renditionFile, outDir string, segmentSeconds int) []string {
	if segmentSeconds <= 0 {
		segmentSeconds = 6
	}
	return []string{
		"-y",
		"-i", renditionFile,
		"-c", "copy",
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", segmentSeconds),
		"-hls_playlist_type", "vod",
		"-hls_segment_type", "fmp4",
		"-hls_fmp4_init_filename", "init.mp4",
		"-hls_segment_filename", filepath.Join(outDir, "segment-%05d.m4s"),
		filepath.Join(outDir, "index.m3u8"),
	}
}

func (r *Runner) PackageHLS(ctx context.Context, renditionFile string, renditionName string, outDir string, segmentSeconds int) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	return run(ctx, r.cfg.FFmpegPath, hlsArgs(renditionFile, outDir, segmentSeconds)...)
}

// dashArgs builds the ffmpeg argument list for packaging all renditions into a
// single DASH manifest. segmentSeconds controls -seg_duration; non-positive
// values fall back to the 6s default.
func dashArgs(renditionFiles []string, outDir string, segmentSeconds int) []string {
	if segmentSeconds <= 0 {
		segmentSeconds = 6
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
		"-seg_duration", fmt.Sprintf("%d", segmentSeconds),
		"-use_timeline", "1",
		"-use_template", "1",
		filepath.Join(outDir, "manifest.mpd"),
	)
	return args
}

func (r *Runner) PackageDASH(ctx context.Context, renditionFiles []string, outDir string, segmentSeconds int) error {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	return run(ctx, r.cfg.FFmpegPath, dashArgs(renditionFiles, outDir, segmentSeconds)...)
}

// WriteHLSMaster writes the HLS multivariant playlist. The audio codec
// (mp4a.40.2) is only advertised when hasAudio is true: advertising an audio
// track that the (video-only) segments do not contain makes players initialize
// the MSE audio SourceBuffer and then fail the append (Shaka error 3014).
func WriteHLSMaster(path string, renditions []domain.Rendition, hasAudio bool, subtitles ...domain.SubtitleTrack) error {
	var builder strings.Builder
	builder.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n")
	// Subtitle renditions are declared up front; each variant then references the
	// group via the SUBTITLES attribute.
	builder.WriteString(hlsSubtitleMediaLines(subtitles))
	subtitlesAttr := ""
	if len(subtitles) > 0 {
		subtitlesAttr = fmt.Sprintf(",SUBTITLES=\"%s\"", hlsSubtitleGroupID)
	}
	for _, rendition := range renditions {
		bandwidth := rendition.BitrateKbps * 1000
		codecs := hlsCodecString(rendition.Codec)
		if hasAudio {
			codecs += ",mp4a.40.2"
		}
		builder.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,CODECS=\"%s\"%s\n", bandwidth, rendition.Width, rendition.Height, codecs, subtitlesAttr))
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

// encodingCodecSettings returns the ffmpeg encoder + args for a codec under the
// chosen backend. "software" (default) uses libx264/libx265/libsvtav1/libvpx/libvvenc;
// "nvenc" uses NVIDIA hardware encoders (h264/hevc/av1 only).
func encodingCodecSettings(codec, preset, backend string) (codecSettings, error) {
	if strings.EqualFold(strings.TrimSpace(backend), "nvenc") {
		return nvencCodecSettings(codec)
	}
	return softwareCodecSettings(codec, preset), nil
}

func softwareCodecSettings(codec string, preset string) codecSettings {
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

func nvencCodecSettings(codec string) (codecSettings, error) {
	base := []string{"-preset", "p4", "-rc", "vbr", "-pix_fmt", "yuv420p"}
	switch normalizeCodec(codec) {
	case "h264":
		return codecSettings{encoder: "h264_nvenc", args: base}, nil
	case "h265":
		return codecSettings{encoder: "hevc_nvenc", args: append(append([]string{}, base...), "-tag:v", "hvc1")}, nil
	case "av1":
		return codecSettings{encoder: "av1_nvenc", args: base}, nil
	default:
		return codecSettings{}, fmt.Errorf("codec %q is not supported by the nvenc backend (only h264, h265, av1)", codec)
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

func buildThumbnailArgs(source string, raw *domain.RawVideoParams, output string, atSeconds float64) []string {
	args := []string{"-ss", fmt.Sprintf("%.3f", atSeconds)}
	args = append(args, rawInputArgs(source, raw)...)
	return append(args,
		"-frames:v", "1",
		"-vf", "scale=640:-2",
		"-q:v", "3",
		"-y", output,
	)
}

// ExtractThumbnail captures a single poster frame at atSeconds into output (jpg).
func (r *Runner) ExtractThumbnail(ctx context.Context, source string, raw *domain.RawVideoParams, output string, atSeconds float64) error {
	if atSeconds < 1 {
		atSeconds = 1
	}
	return run(ctx, r.cfg.FFmpegPath, buildThumbnailArgs(source, raw, output, atSeconds)...)
}
