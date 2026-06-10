package benchmark

import (
	"fmt"
	"strconv"
	"strings"
)

// ConfigFromEnv builds a Config from a getenv function. Repeats default to 3.
// Codecs, resolutions, and the ingest URL are required.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	cfg := Config{
		CorpusBucket: getenv("BENCHMARK_CORPUS_BUCKET"),
		CorpusPrefix: getenv("BENCHMARK_CORPUS_PREFIX"),
		Codecs:       splitCSV(getenv("BENCHMARK_CODECS")),
		Repeats:      3,
		MachineLabel: strings.TrimSpace(getenv("BENCHMARK_MACHINE_LABEL")),
		IngestURL:    strings.TrimSpace(getenv("INGEST_BENCHMARK_URL")),
	}
	if clips := splitCSV(getenv("BENCHMARK_CLIPS")); len(clips) > 0 {
		cfg.Clips = clips
	}
	if len(cfg.Codecs) == 0 {
		return Config{}, fmt.Errorf("BENCHMARK_CODECS is required")
	}
	resRaw := strings.TrimSpace(getenv("BENCHMARK_RESOLUTIONS"))
	if resRaw == "" {
		return Config{}, fmt.Errorf("BENCHMARK_RESOLUTIONS is required")
	}
	res, err := ParseResolutions(resRaw)
	if err != nil {
		return Config{}, err
	}
	if len(res) == 0 {
		return Config{}, fmt.Errorf("BENCHMARK_RESOLUTIONS produced no resolutions")
	}
	cfg.Resolutions = res
	if r := strings.TrimSpace(getenv("BENCHMARK_REPEATS")); r != "" {
		n, err := strconv.Atoi(r)
		if err != nil || n < 1 {
			return Config{}, fmt.Errorf("BENCHMARK_REPEATS must be a positive integer, got %q", r)
		}
		cfg.Repeats = n
	}
	if cfg.IngestURL == "" {
		return Config{}, fmt.Errorf("INGEST_BENCHMARK_URL is required")
	}
	return cfg, nil
}

func splitCSV(raw string) []string {
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
