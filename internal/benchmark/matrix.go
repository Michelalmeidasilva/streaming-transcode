package benchmark

import (
	"fmt"
	"strconv"
	"strings"
)

// Resolution is one output geometry + target bitrate to benchmark.
type Resolution struct {
	Width       int
	Height      int
	BitrateKbps int
}

// Config is the fully-resolved benchmark request (one machine's matrix).
type Config struct {
	CorpusBucket  string
	CorpusPrefix  string
	Clips         []string // object keys under CorpusBucket; if empty, runner lists CorpusPrefix
	Codecs        []string
	Resolutions   []Resolution
	Repeats       int
	MachineLabel  string
	IngestURL     string           // base, e.g. https://host/api/v1
	Mode          string           // "throughput" (default) or "rd"
	QualityPoints map[string][]int // codec -> CRF/CQ values, used in rd mode
}

// Job is one measurement unit: a clip encoded with a codec at a resolution, once.
type Job struct {
	Clip       string
	Codec      string
	Resolution Resolution
	Repetition int
	Quality    int // CRF/CQ value in rd mode; 0 = throughput (fixed bitrate)
}

// ExpandMatrix produces the ordered job list: clip → codec → resolution → repeat,
// so repetitions of the same combination are adjacent (clean serial measurement).
// In "rd" mode the quality dimension replaces fixed bitrate: for each codec the
// QualityPoints values are swept and the job carries the CRF/CQ value in Quality.
func ExpandMatrix(cfg Config) []Job {
	repeats := cfg.Repeats
	if repeats < 1 {
		repeats = 1
	}
	jobs := []Job{}
	for _, clip := range cfg.Clips {
		for _, codec := range cfg.Codecs {
			for _, res := range cfg.Resolutions {
				if cfg.Mode == "rd" {
					for _, q := range cfg.QualityPoints[codec] {
						for rep := 1; rep <= repeats; rep++ {
							jobs = append(jobs, Job{Clip: clip, Codec: codec, Resolution: res, Repetition: rep, Quality: q})
						}
					}
					continue
				}
				for rep := 1; rep <= repeats; rep++ {
					jobs = append(jobs, Job{Clip: clip, Codec: codec, Resolution: res, Repetition: rep})
				}
			}
		}
	}
	return jobs
}

// ParseResolutions parses "WxH:bitrateKbps,WxH:bitrateKbps" into Resolutions.
func ParseResolutions(raw string) ([]Resolution, error) {
	out := []Resolution{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		geo, br, ok := strings.Cut(part, ":")
		if !ok {
			return nil, fmt.Errorf("resolution %q must be WxH:bitrateKbps", part)
		}
		w, h, ok := strings.Cut(geo, "x")
		if !ok {
			return nil, fmt.Errorf("resolution %q must be WxH:bitrateKbps", part)
		}
		width, err := strconv.Atoi(strings.TrimSpace(w))
		if err != nil {
			return nil, fmt.Errorf("resolution %q width: %w", part, err)
		}
		height, err := strconv.Atoi(strings.TrimSpace(h))
		if err != nil {
			return nil, fmt.Errorf("resolution %q height: %w", part, err)
		}
		bitrate, err := strconv.Atoi(strings.TrimSpace(br))
		if err != nil {
			return nil, fmt.Errorf("resolution %q bitrate: %w", part, err)
		}
		out = append(out, Resolution{Width: width, Height: height, BitrateKbps: bitrate})
	}
	return out, nil
}
