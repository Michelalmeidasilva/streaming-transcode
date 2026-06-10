package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ResultRendition mirrors the ingest RunRendition JSON shape.
type ResultRendition struct {
	Name              string  `json:"name"`
	Codec             string  `json:"codec"`
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	Preset            string  `json:"preset,omitempty"`
	TargetBitrateKbps int     `json:"targetBitrateKbps"`
	OutputBitrateKbps int64   `json:"outputBitrateKbps"`
	ElapsedSeconds    float64 `json:"elapsedSeconds"`
	AvgCPUPercent     float64 `json:"avgCpuPercent"`
	MaxCPUPercent     float64 `json:"maxCpuPercent"`
	AvgMemoryMB       float64 `json:"avgMemoryMb"`
	MaxMemoryMB       float64 `json:"maxMemoryMb"`
}

// Result is one benchmark measurement, matching the ingest Run JSON shape.
type Result struct {
	Benchmark      bool              `json:"benchmark"`
	MachineLabel   string            `json:"machineLabel"`
	Hostname       string            `json:"hostname"`
	CPUCores       int               `json:"cpuCores"`
	Clip           string            `json:"clip"`
	Repetition     int               `json:"repetition"`
	ElapsedSeconds float64           `json:"elapsedSeconds"`
	Renditions     []ResultRendition `json:"renditions"`
	CompletedAt    string            `json:"completedAt"`
}

// ResultSink persists a single benchmark measurement.
type ResultSink interface {
	Post(ctx context.Context, res Result) error
}

type httpSink struct {
	url    string // ingest base, e.g. https://host/api/v1
	client *http.Client
}

func NewHTTPSink(baseURL string, client *http.Client) ResultSink {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &httpSink{url: strings.TrimRight(baseURL, "/"), client: client}
}

func (s *httpSink) Post(ctx context.Context, res Result) error {
	body, err := json.Marshal(res)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url+"/benchmark-runs", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("benchmark-runs POST: status %d", resp.StatusCode)
	}
	return nil
}

var _ ResultSink = (*httpSink)(nil)
