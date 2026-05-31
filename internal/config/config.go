package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	RabbitMQURL     string
	EventGatewayURL string
	Storage         StorageConfig
	Queue           QueueConfig
	Transcode       TranscodeConfig
}

type StorageConfig struct {
	Provider       string
	Bucket         string
	Endpoint       string
	AccessKey      string
	SecretKey      string
	Region         string
	UseSSL         bool
	ForcePathStyle bool
}

type QueueConfig struct {
	Exchange          string
	Name              string
	BindingKey        string
	RetryName         string
	DeadName          string
	MaxAttempts       int
	RetryDelaySeconds int
}

type TranscodeConfig struct {
	WorkDir          string
	Profile          string
	Codecs           []string
	FFmpegPath       string
	FFprobePath      string
	Preset           string
	JobTimeout       time.Duration
	MaxFileSizeBytes int64
}

func FromEnv() Config {
	endpoint, useSSL := normalizeEndpoint(env("MINIO_ENDPOINT", "http://localhost:9000"))
	return Config{
		RabbitMQURL:     env("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		EventGatewayURL: strings.TrimRight(env("EVENT_GATEWAY_URL", "http://localhost:8080/api/v1"), "/"),
		Storage: StorageConfig{
			Provider:       env("STORAGE_PROVIDER", "minio"),
			Bucket:         env("STORAGE_BUCKET", "videos"),
			Endpoint:       endpoint,
			AccessKey:      firstEnv("MINIO_ACCESS_KEY", "MINIO_ROOT_USER", "admin"),
			SecretKey:      firstEnv("MINIO_SECRET_KEY", "MINIO_ROOT_PASSWORD", "password123"),
			Region:         env("AWS_REGION", "us-east-1"),
			UseSSL:         useSSL,
			ForcePathStyle: true,
		},
		Queue: QueueConfig{
			Exchange:          env("TRANSCODE_EXCHANGE", "video_events"),
			Name:              env("TRANSCODE_QUEUE", "transcode.jobs"),
			BindingKey:        env("TRANSCODE_BINDING_KEY", "video.upload.completed"),
			RetryName:         env("TRANSCODE_RETRY_QUEUE", "transcode.retry"),
			DeadName:          env("TRANSCODE_DEAD_QUEUE", "transcode.dead"),
			MaxAttempts:       envInt("TRANSCODE_MAX_ATTEMPTS", 3),
			RetryDelaySeconds: envInt("TRANSCODE_RETRY_DELAY_SECONDS", 60),
		},
		Transcode: TranscodeConfig{
			WorkDir:          env("TRANSCODE_WORKDIR", "/tmp/transcode"),
			Profile:          env("TRANSCODE_PROFILE", "production-h264-hls-dash"),
			Codecs:           envList("TRANSCODE_CODECS", []string{"h264"}),
			FFmpegPath:       env("FFMPEG_PATH", "ffmpeg"),
			FFprobePath:      env("FFPROBE_PATH", "ffprobe"),
			Preset:           env("FFMPEG_PRESET", "veryfast"),
			JobTimeout:       time.Duration(envInt("TRANSCODE_JOB_TIMEOUT_SECONDS", 3600)) * time.Second,
			MaxFileSizeBytes: int64(envInt("TRANSCODE_MAX_FILE_SIZE_MB", 0)) * 1024 * 1024,
		},
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func firstEnv(primary, secondary, fallback string) string {
	if value := os.Getenv(primary); value != "" {
		return value
	}
	return env(secondary, fallback)
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envList(key string, fallback []string) []string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		normalized := normalizeCodec(part)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func normalizeCodec(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "h264", "h.264", "avc", "libx264":
		return "h264"
	case "h265", "h.265", "hevc", "libx265":
		return "h265"
	case "av1", "aom", "libaom", "libaom-av1", "svt-av1", "libsvtav1":
		return "av1"
	case "vp9", "vp.9", "libvpx", "libvpx-vp9":
		return "vp9"
	case "vvc", "vpc", "h266", "h.266", "h266/vvc", "h.266/vvc", "vvenc", "libvvenc":
		return "vvc"
	default:
		return ""
	}
}

func normalizeEndpoint(raw string) (string, bool) {
	endpoint := strings.TrimSpace(raw)
	useSSL := false
	if strings.HasPrefix(endpoint, "https://") {
		useSSL = true
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}
	endpoint = strings.TrimPrefix(endpoint, "http://")
	return strings.TrimRight(endpoint, "/"), useSSL
}
