package benchmark

import "testing"

func TestConfigFromEnvParsesLists(t *testing.T) {
	env := map[string]string{
		"BENCHMARK_CORPUS_BUCKET": "vod",
		"BENCHMARK_CORPUS_PREFIX": "benchmark/corpus/",
		"BENCHMARK_CODECS":        "h264, av1",
		"BENCHMARK_RESOLUTIONS":   "1280x720:3000,1920x1080:6000",
		"BENCHMARK_REPEATS":       "3",
		"BENCHMARK_MACHINE_LABEL": "c5.xlarge",
		"INGEST_BENCHMARK_URL":    "https://host/api/v1",
	}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CorpusBucket != "vod" || len(cfg.Codecs) != 2 || cfg.Codecs[1] != "av1" {
		t.Fatalf("bad codecs/bucket: %#v", cfg)
	}
	if len(cfg.Resolutions) != 2 || cfg.Resolutions[0].Height != 720 || cfg.Repeats != 3 {
		t.Fatalf("bad resolutions/repeats: %#v", cfg)
	}
	if cfg.MachineLabel != "c5.xlarge" || cfg.IngestURL != "https://host/api/v1" {
		t.Fatalf("bad label/url: %#v", cfg)
	}
}

func TestConfigFromEnvRequiresCodecsResolutionsURL(t *testing.T) {
	_, err := ConfigFromEnv(func(string) string { return "" })
	if err == nil {
		t.Fatal("expected error when required envs are missing")
	}
}

func TestConfigFromEnvRejectsInvalidMode(t *testing.T) {
	env := map[string]string{
		"BENCHMARK_CODECS":      "h264",
		"BENCHMARK_RESOLUTIONS": "1920x1080:6000",
		"INGEST_BENCHMARK_URL":  "https://host/api/v1",
		"BENCHMARK_MODE":        "RD", // typo: not throughput|rd
	}
	if _, err := ConfigFromEnv(func(k string) string { return env[k] }); err == nil {
		t.Fatal("expected error for invalid BENCHMARK_MODE")
	}
}

func TestConfigReadsSessionID(t *testing.T) {
	env := map[string]string{
		"BENCHMARK_CODECS":      "h264",
		"BENCHMARK_RESOLUTIONS": "1920x1080:6000",
		"INGEST_BENCHMARK_URL":  "https://host/api/v1",
		"BENCHMARK_SESSION_ID":  "123e4567-e89b-42d3-a456-426614174000",
	}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.SessionID != "123e4567-e89b-42d3-a456-426614174000" {
		t.Fatalf("SessionID = %q, want UUID", cfg.SessionID)
	}
}

func TestConfigSessionIDEmptyWhenUnset(t *testing.T) {
	env := map[string]string{
		"BENCHMARK_CODECS":      "h264",
		"BENCHMARK_RESOLUTIONS": "1920x1080:6000",
		"INGEST_BENCHMARK_URL":  "https://host/api/v1",
	}
	cfg, err := ConfigFromEnv(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.SessionID != "" {
		t.Fatalf("SessionID should be empty, got %q", cfg.SessionID)
	}
}

func TestParseQualityPoints(t *testing.T) {
	qp, err := parseQualityPoints("h264=19,25,31;av1=20,40,55")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(qp["h264"]) != 3 || qp["h264"][1] != 25 {
		t.Fatalf("h264 points = %v", qp["h264"])
	}
	if len(qp["av1"]) != 3 || qp["av1"][2] != 55 {
		t.Fatalf("av1 points = %v", qp["av1"])
	}
}
