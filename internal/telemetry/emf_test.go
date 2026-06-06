package telemetry

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestEmitJobWritesValidEMF(t *testing.T) {
	var buf bytes.Buffer
	e := &Emitter{Out: &buf, Now: func() time.Time { return time.UnixMilli(1717689600000) }}

	e.EmitJob("vid123", "success", 12500*time.Millisecond)

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, buf.String())
	}
	if got["JobCount"].(float64) != 1 {
		t.Errorf("JobCount = %v, want 1", got["JobCount"])
	}
	if got["JobDuration"].(float64) != 12500 {
		t.Errorf("JobDuration = %v, want 12500", got["JobDuration"])
	}
	if got["FailureCount"].(float64) != 0 {
		t.Errorf("FailureCount = %v, want 0 for success", got["FailureCount"])
	}
	if got["result"] != "success" || got["video_id"] != "vid123" {
		t.Errorf("dims wrong: %v", got)
	}
	aws := got["_aws"].(map[string]any)
	cwm := aws["CloudWatchMetrics"].([]any)[0].(map[string]any)
	if cwm["Namespace"] != "VOD/streaming-transcode" {
		t.Errorf("Namespace = %v", cwm["Namespace"])
	}
}

func TestEmitJobCountsFailures(t *testing.T) {
	var buf bytes.Buffer
	e := &Emitter{Out: &buf, Now: time.Now}
	e.EmitJob("v", "failed", time.Second)
	var got map[string]any
	_ = json.Unmarshal(buf.Bytes(), &got)
	if got["FailureCount"].(float64) != 1 {
		t.Errorf("FailureCount = %v, want 1", got["FailureCount"])
	}
}
