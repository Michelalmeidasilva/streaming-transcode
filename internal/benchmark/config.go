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
	cfg.Mode = strings.TrimSpace(getenv("BENCHMARK_MODE"))
	if cfg.Mode == "" {
		cfg.Mode = "throughput"
	}
	if cfg.Mode != "throughput" && cfg.Mode != "rd" {
		return Config{}, fmt.Errorf("BENCHMARK_MODE %q is not valid (throughput|rd)", cfg.Mode)
	}
	if cfg.Mode == "rd" {
		qpRaw := strings.TrimSpace(getenv("BENCHMARK_QUALITY_POINTS"))
		if qpRaw == "" {
			return Config{}, fmt.Errorf("BENCHMARK_QUALITY_POINTS is required when BENCHMARK_MODE=rd")
		}
		qp, err := parseQualityPoints(qpRaw)
		if err != nil {
			return Config{}, err
		}
		cfg.QualityPoints = qp
	}
	return cfg, nil
}

func parseQualityPoints(raw string) (map[string][]int, error) {
	out := map[string][]int{}
	for _, group := range strings.Split(raw, ";") {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		codec, list, ok := strings.Cut(group, "=")
		if !ok {
			return nil, fmt.Errorf("quality points %q must be codec=v1,v2,...", group)
		}
		vals := []int{}
		for _, v := range strings.Split(list, ",") {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			n, err := strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("quality point %q: %w", v, err)
			}
			vals = append(vals, n)
		}
		out[strings.TrimSpace(codec)] = vals
	}
	return out, nil
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
