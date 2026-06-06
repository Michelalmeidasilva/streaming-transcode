// Package telemetry emits CloudWatch EMF job records to stdout for the transcode worker.
package telemetry

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

type Emitter struct {
	Out io.Writer
	Now func() time.Time
	mu  sync.Mutex
}

func New() *Emitter { return &Emitter{Out: os.Stdout, Now: time.Now} }

// EmitJob writes a single job-level EMF record. result is "success" or "failed".
func (e *Emitter) EmitJob(videoID, result string, dur time.Duration) {
	failure := 0
	if result != "success" {
		failure = 1
	}
	record := map[string]any{
		"_aws": map[string]any{
			"Timestamp": e.Now().UnixMilli(),
			"CloudWatchMetrics": []map[string]any{{
				"Namespace":  "VOD/streaming-transcode",
				"Dimensions": [][]string{{"result"}},
				"Metrics": []map[string]string{
					{"Name": "JobCount", "Unit": "Count"},
					{"Name": "JobDuration", "Unit": "Milliseconds"},
					{"Name": "FailureCount", "Unit": "Count"},
				},
			}},
		},
		"video_id":     videoID,
		"result":       result,
		"JobCount":     1,
		"JobDuration":  float64(dur.Microseconds()) / 1000.0,
		"FailureCount": failure,
	}
	b, err := json.Marshal(record)
	if err != nil {
		log.Printf("telemetry: failed to marshal EMF job record: %v", err)
		return
	}
	e.mu.Lock()
	_, _ = e.Out.Write(append(b, '\n'))
	e.mu.Unlock()
}
