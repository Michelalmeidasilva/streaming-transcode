package main

import (
	"context"
	"log"
	"os"
	"path/filepath"

	"streaming-transcode/internal/benchmark"
	"streaming-transcode/internal/config"
	"streaming-transcode/internal/storage"
	"streaming-transcode/internal/transcode"
)

func main() {
	logger := log.New(os.Stdout, "benchmark ", log.LstdFlags|log.LUTC)

	cfg, err := benchmark.ConfigFromEnv(os.Getenv)
	if err != nil {
		logger.Fatalf("config: %v", err)
	}

	appCfg := config.FromEnv()
	if cfg.CorpusBucket == "" {
		cfg.CorpusBucket = appCfg.Storage.Bucket
	}
	if cfg.MachineLabel == "" {
		if it := benchmark.InstanceType(context.Background()); it != "" {
			cfg.MachineLabel = it
		} else if host, _ := os.Hostname(); host != "" {
			cfg.MachineLabel = host
		}
	}

	store, err := storage.New(appCfg.Storage)
	if err != nil {
		logger.Fatalf("storage: %v", err)
	}

	deps := benchmark.Deps{
		Storage: store,
		Runner:  transcode.NewFFmpegRunner(appCfg.Transcode),
		Sink:    benchmark.NewHTTPSink(cfg.IngestURL, nil),
		WorkDir: filepath.Join(appCfg.Transcode.WorkDir, "benchmark"),
		Logf:    logger.Printf,
	}

	logger.Printf("benchmark start machine=%s bucket=%s prefix=%s codecs=%v repeats=%d",
		cfg.MachineLabel, cfg.CorpusBucket, cfg.CorpusPrefix, cfg.Codecs, cfg.Repeats)
	if err := benchmark.Run(context.Background(), cfg, deps); err != nil {
		logger.Fatalf("benchmark failed: %v", err)
	}
	logger.Printf("benchmark done")
}
