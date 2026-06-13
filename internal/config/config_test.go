package config

import (
	"os"
	"testing"
)

func TestFromEnvAppliesOverridesAndNormalization(t *testing.T) {
	t.Setenv("MINIO_ENDPOINT", "https://minio.example.com:9443/")
	t.Setenv("MINIO_ACCESS_KEY", "access")
	t.Setenv("MINIO_SECRET_KEY", "secret")
	t.Setenv("AWS_REGION", "sa-east-1")
	t.Setenv("RABBITMQ_URL", "amqp://example")
	t.Setenv("EVENT_GATEWAY_URL", "http://gateway/api/v1/")
	t.Setenv("TRANSCODE_EXCHANGE", "exchange")
	t.Setenv("TRANSCODE_QUEUE", "queue")
	t.Setenv("TRANSCODE_BINDING_KEY", "video.ready")
	t.Setenv("TRANSCODE_RETRY_QUEUE", "retry")
	t.Setenv("TRANSCODE_DEAD_QUEUE", "dead")
	t.Setenv("TRANSCODE_MAX_ATTEMPTS", "5")
	t.Setenv("TRANSCODE_RETRY_DELAY_SECONDS", "30")
	t.Setenv("TRANSCODE_WORKDIR", "/tmp/work")
	t.Setenv("TRANSCODE_PROFILE", "custom")
	t.Setenv("TRANSCODE_CODECS", "h.264, hevc, av1, libvpx-vp9, vpc, unknown, h264")
	t.Setenv("FFMPEG_PATH", "/usr/bin/ffmpeg")
	t.Setenv("FFPROBE_PATH", "/usr/bin/ffprobe")
	t.Setenv("FFMPEG_PRESET", "slow")
	t.Setenv("TRANSCODE_JOB_TIMEOUT_SECONDS", "120")

	cfg := FromEnv()
	if cfg.Storage.Endpoint != "minio.example.com:9443" {
		t.Fatalf("Endpoint = %q", cfg.Storage.Endpoint)
	}
	if !cfg.Storage.UseSSL {
		t.Fatalf("UseSSL = false, want true")
	}
	if cfg.EventGatewayURL != "http://gateway/api/v1" {
		t.Fatalf("EventGatewayURL = %q", cfg.EventGatewayURL)
	}
	if cfg.Queue.Exchange != "exchange" || cfg.Queue.Name != "queue" || cfg.Queue.BindingKey != "video.ready" {
		t.Fatalf("Queue = %+v", cfg.Queue)
	}
	if cfg.Queue.RetryName != "retry" || cfg.Queue.DeadName != "dead" || cfg.Queue.MaxAttempts != 5 || cfg.Queue.RetryDelaySeconds != 30 {
		t.Fatalf("Queue retry config = %+v", cfg.Queue)
	}
	if cfg.Transcode.JobTimeout.Seconds() != 120 {
		t.Fatalf("JobTimeout = %v", cfg.Transcode.JobTimeout)
	}
	if len(cfg.Transcode.Codecs) != 5 ||
		cfg.Transcode.Codecs[0] != "h264" ||
		cfg.Transcode.Codecs[1] != "h265" ||
		cfg.Transcode.Codecs[2] != "av1" ||
		cfg.Transcode.Codecs[3] != "vp9" ||
		cfg.Transcode.Codecs[4] != "vvc" {
		t.Fatalf("Codecs = %+v", cfg.Transcode.Codecs)
	}
}

func TestHelperFallbacks(t *testing.T) {
	if env("UNSET_KEY", "fallback") != "fallback" {
		t.Fatalf("env fallback failed")
	}
	t.Setenv("PRIMARY_ENV", "")
	t.Setenv("SECONDARY_ENV", "secondary")
	if firstEnv("PRIMARY_ENV", "SECONDARY_ENV", "fallback") != "secondary" {
		t.Fatalf("firstEnv secondary fallback failed")
	}
	t.Setenv("INT_ENV", "invalid")
	if envInt("INT_ENV", 42) != 42 {
		t.Fatalf("envInt fallback failed")
	}
	endpoint, useSSL := normalizeEndpoint("http://localhost:9000/")
	if endpoint != "localhost:9000" || useSSL {
		t.Fatalf("normalizeEndpoint(http) = %q %v", endpoint, useSSL)
	}
}

func TestFirstEnvUsesPrimaryWhenSet(t *testing.T) {
	t.Setenv("PRIMARY_ENV", "primary")
	t.Setenv("SECONDARY_ENV", "secondary")
	if got := firstEnv("PRIMARY_ENV", "SECONDARY_ENV", "fallback"); got != "primary" {
		t.Fatalf("firstEnv() = %q", got)
	}
}

func TestEnvIntUsesPositiveValue(t *testing.T) {
	t.Setenv("INT_ENV", "15")
	if got := envInt("INT_ENV", 10); got != 15 {
		t.Fatalf("envInt() = %d", got)
	}
}

func TestEnvReadsSetValue(t *testing.T) {
	const key = "EXPLICIT_ENV"
	t.Setenv(key, "value")
	if got := env(key, "fallback"); got != "value" {
		t.Fatalf("env() = %q", got)
	}
}

func TestFromEnvUsesAWSCredentialsForS3Provider(t *testing.T) {
	os.Unsetenv("MINIO_ACCESS_KEY")
	os.Unsetenv("MINIO_SECRET_KEY")
	os.Unsetenv("MINIO_ROOT_USER")
	os.Unsetenv("MINIO_ROOT_PASSWORD")
	t.Setenv("STORAGE_PROVIDER", "aws-s3")
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIAEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "aws-secret")
	t.Setenv("AWS_REGION", "eu-west-1")

	cfg := FromEnv()
	if cfg.Storage.Provider != "aws-s3" {
		t.Fatalf("Provider = %q", cfg.Storage.Provider)
	}
	if cfg.Storage.AccessKey != "AKIAEXAMPLE" || cfg.Storage.SecretKey != "aws-secret" {
		t.Fatalf("Storage credentials = %q/%q", cfg.Storage.AccessKey, cfg.Storage.SecretKey)
	}
}

func TestFromEnvUsesSecondaryCredentialsFallback(t *testing.T) {
	os.Unsetenv("MINIO_ACCESS_KEY")
	os.Unsetenv("MINIO_SECRET_KEY")
	t.Setenv("MINIO_ROOT_USER", "root-user")
	t.Setenv("MINIO_ROOT_PASSWORD", "root-password")

	cfg := FromEnv()
	if cfg.Storage.AccessKey != "root-user" || cfg.Storage.SecretKey != "root-password" {
		t.Fatalf("Storage credentials = %q/%q", cfg.Storage.AccessKey, cfg.Storage.SecretKey)
	}
}

func TestFromEnvMachineLabel(t *testing.T) {
	t.Setenv("TRANSCODE_MACHINE_LABEL", "c7g.xlarge")
	cfg := FromEnv()
	if cfg.Transcode.MachineLabel != "c7g.xlarge" {
		t.Fatalf("MachineLabel = %q, want c7g.xlarge", cfg.Transcode.MachineLabel)
	}
}

func TestFromEnvEncoderBackend(t *testing.T) {
	if got := FromEnv().Transcode.EncoderBackend; got != "software" {
		t.Fatalf("default EncoderBackend = %q, want software", got)
	}
	t.Setenv("TRANSCODE_ENCODER_BACKEND", "nvenc")
	if got := FromEnv().Transcode.EncoderBackend; got != "nvenc" {
		t.Fatalf("EncoderBackend = %q, want nvenc", got)
	}
}

func TestFromEnvVMAFFFmpegPath(t *testing.T) {
	// defaults to the encode ffmpeg path
	t.Setenv("FFMPEG_PATH", "ffmpeg")
	if got := FromEnv().Transcode.VMAFFFmpegPath; got != "ffmpeg" {
		t.Fatalf("default VMAFFFmpegPath = %q, want ffmpeg (= FFMPEG_PATH)", got)
	}
	// explicit override wins (a separate libvmaf-capable binary)
	t.Setenv("VMAF_FFMPEG_PATH", "ffmpeg-vmaf")
	if got := FromEnv().Transcode.VMAFFFmpegPath; got != "ffmpeg-vmaf" {
		t.Fatalf("VMAFFFmpegPath = %q, want ffmpeg-vmaf", got)
	}
}
