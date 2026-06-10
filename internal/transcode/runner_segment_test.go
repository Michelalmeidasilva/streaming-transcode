package transcode

import (
	"strings"
	"testing"
)

func TestHLSArgsUseSegmentSeconds(t *testing.T) {
	args := hlsArgs("/in/720p.mp4", "/out/720p", 4)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-hls_time 4") {
		t.Fatalf("expected -hls_time 4 in args, got: %s", joined)
	}
	if !strings.Contains(joined, "-hls_segment_type fmp4") {
		t.Fatalf("expected fmp4 segments preserved, got: %s", joined)
	}
}

func TestHLSArgsDefaultsToSix(t *testing.T) {
	joined := strings.Join(hlsArgs("/in/720p.mp4", "/out/720p", 0), " ")
	if !strings.Contains(joined, "-hls_time 6") {
		t.Fatalf("expected -hls_time 6 default, got: %s", joined)
	}
}

func TestDASHArgsUseSegmentSeconds(t *testing.T) {
	args := dashArgs([]string{"/in/720p.mp4"}, "/out/dash", 2)
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-seg_duration 2") {
		t.Fatalf("expected -seg_duration 2 in args, got: %s", joined)
	}
}
