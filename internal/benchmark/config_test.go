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
