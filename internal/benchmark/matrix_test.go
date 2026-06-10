package benchmark

import "testing"

func TestExpandMatrix(t *testing.T) {
	cfg := Config{
		Clips:       []string{"a.mp4", "b.mp4"},
		Codecs:      []string{"h264", "av1"},
		Resolutions: []Resolution{{Width: 1280, Height: 720, BitrateKbps: 3000}},
		Repeats:     2,
	}
	jobs := ExpandMatrix(cfg)
	// 2 clips × 2 codecs × 1 res × 2 repeats = 8
	if len(jobs) != 8 {
		t.Fatalf("want 8 jobs, got %d", len(jobs))
	}
	first := jobs[0]
	if first.Clip != "a.mp4" || first.Codec != "h264" || first.Resolution.Height != 720 || first.Repetition != 1 {
		t.Fatalf("unexpected first job: %#v", first)
	}
	if jobs[1].Repetition != 2 {
		t.Fatalf("want repetition 2 for second job, got %d", jobs[1].Repetition)
	}
}

func TestParseResolutions(t *testing.T) {
	res, err := ParseResolutions("1280x720:3000,1920x1080:6000")
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || res[1].Width != 1920 || res[1].Height != 1080 || res[1].BitrateKbps != 6000 {
		t.Fatalf("bad parse: %#v", res)
	}
}
