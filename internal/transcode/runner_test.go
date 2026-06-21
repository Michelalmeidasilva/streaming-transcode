package transcode

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"streaming-transcode/internal/config"
	"streaming-transcode/internal/domain"
)

func TestRunnerProbeUsesConfiguredBinary(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "output.json")
	if err := os.WriteFile(outputPath, []byte(`{"streams":[{"codec_type":"video","codec_name":"h264","width":1280,"height":720,"avg_frame_rate":"60/1"}],"format":{"duration":"12","bit_rate":"4000000"}}`), 0o755); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	script := filepath.Join(tempDir, "ffprobe.sh")
	body := "#!/bin/sh\ncat " + outputPath + "\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}

	runner := NewFFmpegRunner(config.TranscodeConfig{FFprobePath: script})
	info, err := runner.Probe("input.mp4")
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if info.Width != 1280 || info.Height != 720 || info.FPS != 60 {
		t.Fatalf("Probe() = %+v", info)
	}
}

func TestRunnerTranscodeAndPackagingBuildArguments(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "args.log")
	script := filepath.Join(tempDir, "ffmpeg.sh")
	body := "#!/bin/sh\nprintf '%s\n' \"$@\" >" + logPath + "\nlast=''\nfor arg in \"$@\"; do last=\"$arg\"; done\ncase \"$last\" in\n*.mp4|*.m3u8|*.mpd) mkdir -p \"$(dirname \"$last\")\"; touch \"$last\" ;;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}
	ffprobeScript := filepath.Join(tempDir, "ffprobe.sh")
	ffprobeBody := "#!/bin/sh\nlast=''\nfor arg in \"$@\"; do last=\"$arg\"; done\ncase \"$last\" in\nsource.mp4) cat <<'EOF'\n{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\",\"width\":1920,\"height\":1080,\"avg_frame_rate\":\"60000/1001\"}],\"format\":{\"duration\":\"10\",\"bit_rate\":\"8000000\",\"size\":\"20000000\"}}\nEOF\n;;\n*) cat <<'EOF'\n{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\",\"width\":1280,\"height\":720,\"avg_frame_rate\":\"60000/1001\"}],\"format\":{\"duration\":\"10\",\"bit_rate\":\"3000000\",\"size\":\"5000000\"}}\nEOF\n;;\nesac\n"
	if err := os.WriteFile(ffprobeScript, []byte(ffprobeBody), 0o755); err != nil {
		t.Fatalf("WriteFile(ffprobeScript) error = %v", err)
	}

	runner := NewFFmpegRunner(config.TranscodeConfig{FFmpegPath: script, FFprobePath: ffprobeScript, Preset: "fast"})
	rendition := domain.Rendition{Name: "720p", Width: 1280, Height: 720, BitrateKbps: 3000}
	output := filepath.Join(tempDir, "out", "720p.mp4")
	metrics, err := runner.TranscodeRendition(context.Background(), "source.mp4", nil, rendition, output)
	if err != nil {
		t.Fatalf("TranscodeRendition() error = %v", err)
	}
	if metrics.OutputBitrateKbps != 3000 || metrics.OutputFileSizeBytes != 5000000 {
		t.Fatalf("Transcode metrics = %+v", metrics)
	}
	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile(logPath) error = %v", err)
	}
	text := string(args)
	if !strings.Contains(text, "scale=1280:720:force_original_aspect_ratio=decrease,pad=1280:720:(ow-iw)/2:(oh-ih)/2") {
		t.Fatalf("Transcode args missing scale filter: %s", text)
	}
	if !strings.Contains(text, "fast") {
		t.Fatalf("Transcode args missing preset: %s", text)
	}

	if _, err := runner.TranscodeRendition(context.Background(), "source.mp4", nil, domain.Rendition{Name: "h265-720p", Width: 1280, Height: 720, BitrateKbps: 3000, Codec: "h265"}, output); err != nil {
		t.Fatalf("TranscodeRendition(h265) error = %v", err)
	}
	args, _ = os.ReadFile(logPath)
	if !strings.Contains(string(args), "libx265") || !strings.Contains(string(args), "hvc1") {
		t.Fatalf("H.265 transcode args = %s", string(args))
	}

	if _, err := runner.TranscodeRendition(context.Background(), "source.mp4", nil, domain.Rendition{Name: "av1-720p", Width: 1280, Height: 720, BitrateKbps: 3000, Codec: "av1"}, output); err != nil {
		t.Fatalf("TranscodeRendition(av1) error = %v", err)
	}
	args, _ = os.ReadFile(logPath)
	if !strings.Contains(string(args), "libsvtav1") || !strings.Contains(string(args), "-preset") || !strings.Contains(string(args), "-crf") {
		t.Fatalf("AV1 transcode args = %s", string(args))
	}

	if _, err := runner.TranscodeRendition(context.Background(), "source.mp4", nil, domain.Rendition{Name: "vp9-720p", Width: 1280, Height: 720, BitrateKbps: 3000, Codec: "vp9"}, output); err != nil {
		t.Fatalf("TranscodeRendition(vp9) error = %v", err)
	}
	args, _ = os.ReadFile(logPath)
	if !strings.Contains(string(args), "libvpx-vp9") || !strings.Contains(string(args), "-deadline") {
		t.Fatalf("VP9 transcode args = %s", string(args))
	}

	if _, err := runner.TranscodeRendition(context.Background(), "source.mp4", nil, domain.Rendition{Name: "vvc-720p", Width: 1280, Height: 720, BitrateKbps: 3000, Codec: "vvc"}, output); err != nil {
		t.Fatalf("TranscodeRendition(vvc) error = %v", err)
	}
	args, _ = os.ReadFile(logPath)
	if !strings.Contains(string(args), "libvvenc") {
		t.Fatalf("VVC transcode args = %s", string(args))
	}

	hlsDir := filepath.Join(tempDir, "hls")
	if err := runner.PackageHLS(context.Background(), output, "720p", hlsDir, 6); err != nil {
		t.Fatalf("PackageHLS() error = %v", err)
	}
	args, _ = os.ReadFile(logPath)
	if !strings.Contains(string(args), "index.m3u8") {
		t.Fatalf("PackageHLS args = %s", string(args))
	}

	dashDir := filepath.Join(tempDir, "dash")
	second := filepath.Join(tempDir, "out", "1080p.mp4")
	if err := os.WriteFile(second, []byte("data"), 0o644); err != nil {
		t.Fatalf("WriteFile(second) error = %v", err)
	}
	if err := runner.PackageDASH(context.Background(), []string{output, second}, dashDir, 6); err != nil {
		t.Fatalf("PackageDASH() error = %v", err)
	}
	args, _ = os.ReadFile(logPath)
	if !strings.Contains(string(args), "manifest.mpd") || !strings.Contains(string(args), "0:a:0?") {
		t.Fatalf("PackageDASH args = %s", string(args))
	}
}

func TestWriteHLSMasterAndRunFailure(t *testing.T) {
	tempDir := t.TempDir()
	manifest := filepath.Join(tempDir, "master.m3u8")
	renditions := []domain.Rendition{
		{Name: "1080p", Width: 1920, Height: 1080, BitrateKbps: 6000},
		{Name: "720p", Width: 1280, Height: 720, BitrateKbps: 3000},
	}
	if err := WriteHLSMaster(manifest, renditions, true); err != nil {
		t.Fatalf("WriteHLSMaster() error = %v", err)
	}
	data, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "#EXTM3U") || !strings.Contains(text, "1080p/index.m3u8") || !strings.Contains(text, "720p/index.m3u8") {
		t.Fatalf("manifest = %s", text)
	}
	if !strings.Contains(text, "avc1.640028") {
		t.Fatalf("manifest missing H.264 codec string = %s", text)
	}
	if !strings.Contains(text, "mp4a.40.2") {
		t.Fatalf("manifest with audio should advertise mp4a.40.2 = %s", text)
	}

	// Video-only source: the master must NOT advertise an audio codec.
	noAudioManifest := filepath.Join(tempDir, "no-audio-master.m3u8")
	if err := WriteHLSMaster(noAudioManifest, renditions, false); err != nil {
		t.Fatalf("WriteHLSMaster(noAudio) error = %v", err)
	}
	noAudioData, err := os.ReadFile(noAudioManifest)
	if err != nil {
		t.Fatalf("ReadFile(noAudio) error = %v", err)
	}
	if strings.Contains(string(noAudioData), "mp4a") {
		t.Fatalf("video-only master must not advertise audio codec = %s", string(noAudioData))
	}

	h265Manifest := filepath.Join(tempDir, "h265-master.m3u8")
	if err := WriteHLSMaster(h265Manifest, []domain.Rendition{{Name: "h265-720p", Width: 1280, Height: 720, BitrateKbps: 3000, Codec: "h265"}}, true); err != nil {
		t.Fatalf("WriteHLSMaster(h265) error = %v", err)
	}
	h265Data, err := os.ReadFile(h265Manifest)
	if err != nil {
		t.Fatalf("ReadFile(h265Manifest) error = %v", err)
	}
	if !strings.Contains(string(h265Data), "hvc1.1.6.L120.90") {
		t.Fatalf("H.265 manifest = %s", string(h265Data))
	}

	advancedManifest := filepath.Join(tempDir, "advanced-master.m3u8")
	if err := WriteHLSMaster(advancedManifest, []domain.Rendition{
		{Name: "av1-720p", Width: 1280, Height: 720, BitrateKbps: 3000, Codec: "av1"},
		{Name: "vp9-720p", Width: 1280, Height: 720, BitrateKbps: 3000, Codec: "vp9"},
		{Name: "vvc-720p", Width: 1280, Height: 720, BitrateKbps: 3000, Codec: "vvc"},
	}, true); err != nil {
		t.Fatalf("WriteHLSMaster(advanced) error = %v", err)
	}
	advancedData, err := os.ReadFile(advancedManifest)
	if err != nil {
		t.Fatalf("ReadFile(advancedManifest) error = %v", err)
	}
	advancedText := string(advancedData)
	for _, want := range []string{"av01.0.08M.08", "vp09.00.51.08", "vvc1.1.L120.C0"} {
		if !strings.Contains(advancedText, want) {
			t.Fatalf("advanced manifest missing %q: %s", want, advancedText)
		}
	}

	failScript := filepath.Join(tempDir, "fail.sh")
	if err := os.WriteFile(failScript, []byte("#!/bin/sh\necho failure\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(failScript) error = %v", err)
	}
	if err := run(context.Background(), failScript); err == nil || !strings.Contains(err.Error(), "failure") {
		t.Fatalf("run() error = %v, want command output", err)
	}
}

func TestRawInputArgs(t *testing.T) {
	if got := strings.Join(rawInputArgs("/in/source.mp4", nil), " "); got != "-i /in/source.mp4" {
		t.Fatalf("rawInputArgs(nil) = %q", got)
	}
	raw := &domain.RawVideoParams{Width: 1920, Height: 1080, FPS: 29.97, PixelFormat: "yuv420p"}
	joined := strings.Join(rawInputArgs("/in/source.yuv", raw), " ")
	for _, want := range []string{"-f rawvideo", "-pix_fmt yuv420p", "-s 1920x1080", "-framerate 29.97", "-i /in/source.yuv"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("rawInputArgs(raw) = %q, missing %q", joined, want)
		}
	}
	// Pixel format falls back to the default when blank.
	if got := strings.Join(rawInputArgs("/in/x.yuv", &domain.RawVideoParams{Width: 2, Height: 2, FPS: 1}), " "); !strings.Contains(got, "-pix_fmt yuv420p") {
		t.Fatalf("rawInputArgs default pixfmt = %q", got)
	}
}

func TestBuildThumbnailArgs(t *testing.T) {
	args := buildThumbnailArgs("/in/source.mp4", nil, "/out/thumb.jpg", 2.5)
	joined := strings.Join(args, " ")

	for _, want := range []string{"-ss 2.500", "-i /in/source.mp4", "-frames:v 1", "-vf scale=640:-2", "-q:v 3", "-y /out/thumb.jpg"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args %q missing %q", joined, want)
		}
	}
}

func TestCappedVBRArgs(t *testing.T) {
	// h264/h265/vp9/vvc: maxrate == target (near-CBR, unchanged production behavior).
	h264 := strings.Join(cappedVBRArgs("h264", 3000), " ")
	if !strings.Contains(h264, "-b:v 3000k") || !strings.Contains(h264, "-maxrate 3000k") || !strings.Contains(h264, "-bufsize 6000k") {
		t.Fatalf("h264 capped VBR args = %q", h264)
	}
	// av1 (libsvtav1) rejects maxrate == target, so it gets a 1.5x ceiling.
	av1 := strings.Join(cappedVBRArgs("av1", 3000), " ")
	if !strings.Contains(av1, "-b:v 3000k") || !strings.Contains(av1, "-maxrate 4500k") || !strings.Contains(av1, "-bufsize 6000k") {
		t.Fatalf("av1 capped VBR args = %q", av1)
	}
}

func TestForceCappedVBRRoutesAV1ThroughBitrate(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "args.log")
	script := filepath.Join(tempDir, "ffmpeg.sh")
	body := "#!/bin/sh\nprintf '%s\n' \"$@\" >" + logPath + "\nlast=''\nfor arg in \"$@\"; do last=\"$arg\"; done\ncase \"$last\" in\n*.mp4|*.m3u8|*.mpd) mkdir -p \"$(dirname \"$last\")\"; touch \"$last\" ;;\nesac\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("WriteFile(script) error = %v", err)
	}
	ffprobeScript := filepath.Join(tempDir, "ffprobe.sh")
	ffprobeBody := "#!/bin/sh\ncat <<'EOF'\n{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"av1\",\"width\":1280,\"height\":720,\"avg_frame_rate\":\"30/1\"}],\"format\":{\"duration\":\"10\",\"bit_rate\":\"3000000\",\"size\":\"5000000\"}}\nEOF\n"
	if err := os.WriteFile(ffprobeScript, []byte(ffprobeBody), 0o755); err != nil {
		t.Fatalf("WriteFile(ffprobeScript) error = %v", err)
	}
	output := filepath.Join(tempDir, "out", "av1.mp4")
	av1 := domain.Rendition{Name: "av1-720p", Width: 1280, Height: 720, BitrateKbps: 3000, Codec: "av1"}

	// Default: av1 uses CRF, no -maxrate.
	def := NewFFmpegRunner(config.TranscodeConfig{FFmpegPath: script, FFprobePath: ffprobeScript, Preset: "fast"})
	if _, err := def.TranscodeRendition(context.Background(), "source.mp4", nil, av1, output); err != nil {
		t.Fatalf("default av1 error = %v", err)
	}
	got, _ := os.ReadFile(logPath)
	if !strings.Contains(string(got), "-crf") || strings.Contains(string(got), "-maxrate") {
		t.Fatalf("default av1 should use -crf and no -maxrate; args = %s", got)
	}

	// ForceCappedVBR: av1 uses bitrate target + maxrate (> target), no -crf.
	forced := NewFFmpegRunner(config.TranscodeConfig{FFmpegPath: script, FFprobePath: ffprobeScript, Preset: "fast", ForceCappedVBR: true})
	if _, err := forced.TranscodeRendition(context.Background(), "source.mp4", nil, av1, output); err != nil {
		t.Fatalf("forced av1 error = %v", err)
	}
	got, _ = os.ReadFile(logPath)
	// The fake ffmpeg logs one arg per line, so assert on individual tokens.
	gotStr := string(got)
	if strings.Contains(gotStr, "-crf") {
		t.Fatalf("ForceCappedVBR av1 must not use -crf; args = %s", gotStr)
	}
	for _, want := range []string{"-b:v", "3000k", "-maxrate", "4500k", "-bufsize", "6000k"} {
		if !strings.Contains(gotStr, want) {
			t.Fatalf("ForceCappedVBR av1 args missing %q; args = %s", want, gotStr)
		}
	}
}
