package transcode

import "testing"

func TestParseVMAFLog(t *testing.T) {
	js := []byte(`{"pooled_metrics":{"vmaf":{"min":70.1,"max":99.0,"mean":92.345},"psnr_y":{"mean":41.2}}}`)
	v, p, err := parseVMAFLog(js)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if v != 92.345 {
		t.Fatalf("vmaf = %v, want 92.345", v)
	}
	if p != 41.2 {
		t.Fatalf("psnr = %v, want 41.2", p)
	}
}

func TestParseVMAFLogMissing(t *testing.T) {
	if _, _, err := parseVMAFLog([]byte(`{"pooled_metrics":{}}`)); err == nil {
		t.Fatal("expected error when vmaf metric is absent")
	}
}
