package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"streaming-transcode/internal/config"
	"streaming-transcode/internal/domain"
	"streaming-transcode/internal/transcode"
)

func main() {
	var (
		input     = flag.String("input", "", "arquivo de entrada")
		output    = flag.String("output", "", "arquivo de saida")
		codec     = flag.String("codec", "av1", "codec de saida")
		width     = flag.Int("width", 1280, "largura alvo")
		height    = flag.Int("height", 720, "altura alvo")
		bitrate   = flag.Int("bitrate-kbps", 3000, "bitrate alvo em kbps")
		timeout   = flag.Duration("timeout", 2*time.Hour, "timeout do job")
		rawWidth  = flag.Int("raw-width", 0, "largura do raw .yuv (obrigatorio p/ .yuv)")
		rawHeight = flag.Int("raw-height", 0, "altura do raw .yuv (obrigatorio p/ .yuv)")
		rawFPS    = flag.Float64("raw-fps", 0, "fps do raw .yuv (obrigatorio p/ .yuv)")
		rawPixFmt = flag.String("raw-pixfmt", domain.DefaultRawPixelFormat, "pixel format do raw .yuv")
	)
	flag.Parse()

	// Batch mode: AWS Batch invokes `transcode-local <s3-key>` — a single
	// positional argument, no flags. The triggering S3 ObjectCreated event
	// carries only the key, so we rebuild a minimal event and run the full
	// pipeline (download -> ladder -> package -> upload -> persist via the Event
	// Gateway). The flag-based local-file mode below stays for dev use.
	if flag.NArg() >= 1 {
		os.Exit(runBatchMode(flag.Arg(0)))
	}

	if *input == "" || *output == "" {
		fmt.Fprintln(os.Stderr, "--input e --output sao obrigatorios")
		os.Exit(1)
	}

	// Headerless raw streams (.yuv) cannot be probed; geometry comes from flags.
	var raw *domain.RawVideoParams
	if strings.EqualFold(filepath.Ext(*input), ".yuv") {
		if *rawWidth <= 0 || *rawHeight <= 0 || *rawFPS <= 0 {
			fmt.Fprintln(os.Stderr, "--raw-width, --raw-height e --raw-fps sao obrigatorios para .yuv")
			os.Exit(1)
		}
		raw = &domain.RawVideoParams{Width: *rawWidth, Height: *rawHeight, FPS: *rawFPS, PixelFormat: *rawPixFmt}
	}

	cfg := config.FromEnv()
	runner := transcode.NewFFmpegRunner(cfg.Transcode)

	info := domain.MediaInfo{Width: *rawWidth, Height: *rawHeight, FPS: *rawFPS, VideoCodec: "rawvideo"}
	if raw == nil {
		probed, err := runner.Probe(*input)
		if err != nil {
			fmt.Fprintf(os.Stderr, "probe falhou: %v\n", err)
			os.Exit(1)
		}
		info = probed
	}

	if err := os.MkdirAll(filepath.Dir(*output), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir falhou: %v\n", err)
		os.Exit(1)
	}

	rendition := domain.Rendition{
		Name:        fmt.Sprintf("%s-%dp", *codec, *height),
		Width:       *width,
		Height:      *height,
		BitrateKbps: *bitrate,
		Codec:       *codec,
	}

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	fmt.Printf("source=%s\n", *input)
	fmt.Printf("media=%dx%d %.3ffps codec=%s bitrate=%dkbps\n", info.Width, info.Height, info.FPS, info.VideoCodec, info.BitrateKbps)
	fmt.Printf("rendition=%+v\n", rendition)

	metrics, err := runner.TranscodeRendition(ctx, *input, raw, rendition, *output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "transcode falhou: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("output=%s\n", *output)
	fmt.Printf("observability supported=%t samples=%d elapsed=%.3fs rtf=%.3f avgCpu=%.2f%% maxCpu=%.2f%% outputSize=%d outputBitrate=%dkbps error=%q\n",
		metrics.ResourceUsage.Supported,
		metrics.ResourceUsage.SampleCount,
		metrics.ElapsedSeconds,
		metrics.RTF,
		metrics.ResourceUsage.AvgCPUPercent,
		metrics.ResourceUsage.MaxCPUPercent,
		metrics.OutputFileSizeBytes,
		metrics.OutputBitrateKbps,
		metrics.ResourceUsage.CollectionError,
	)
}
