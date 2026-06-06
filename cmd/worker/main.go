package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"streaming-transcode/internal/config"
	"streaming-transcode/internal/events"
	intotel "streaming-transcode/internal/otel"
	"streaming-transcode/internal/queue"
	"streaming-transcode/internal/storage"
	"streaming-transcode/internal/transcode"
	"streaming-transcode/internal/worker"
)

func main() {
	cfg := config.FromEnv()
	logger := log.New(os.Stdout, "", log.LstdFlags|log.LUTC)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	otelShutdown, err := intotel.Init(ctx)
	if err != nil {
		logger.Printf("WARNING: OTEL init failed (continuing without tracing): %v", err)
	} else {
		defer func() {
			shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutCancel()
			_ = otelShutdown(shutCtx)
		}()
	}

	store, err := storage.New(cfg.Storage)
	if err != nil {
		logger.Fatalf("storage init failed: %v", err)
	}

	eventClient := events.NewGatewayClient(cfg.EventGatewayURL)
	processor := worker.NewProcessor(worker.Dependencies{
		Config:   cfg,
		Storage:  store,
		Events:   eventClient,
		Runner:   transcode.NewFFmpegRunner(cfg.Transcode),
		Logger:   logger,
		ClockNow: nil,
	})

	consumer, err := queue.NewRabbitConsumer(cfg.RabbitMQURL, cfg.Queue, logger)
	if err != nil {
		logger.Fatalf("rabbitmq init failed: %v", err)
	}
	defer consumer.Close()

	logger.Printf("streaming-transcode worker started queue=%s binding=%s", cfg.Queue.Name, cfg.Queue.BindingKey)
	if err := consumer.Run(ctx, processor.HandleDelivery); err != nil {
		logger.Fatalf("worker stopped with error: %v", err)
	}
}
