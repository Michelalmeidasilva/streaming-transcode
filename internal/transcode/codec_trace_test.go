package transcode

import (
	"testing"

	"streaming-transcode/internal/domain"
)

// TestEncodingCodecSettingsPerUICodec pins each codec selectable in the upload
// UI to the ffmpeg encoder it must reach, so a normalization or wiring change
// can't silently swap encoders.
func TestEncodingCodecSettingsPerUICodec(t *testing.T) {
	want := map[string]string{
		"h264": "libx264",
		"h265": "libx265",
		"av1":  "libsvtav1",
	}
	for codec, encoder := range want {
		got, err := encodingCodecSettings(codec, "medium", "software")
		if err != nil {
			t.Fatalf("software backend should not error for %q: %v", codec, err)
		}
		if got.encoder != encoder {
			t.Fatalf("codec %q: encoder = %q, want %q", codec, got.encoder, encoder)
		}
	}
}

// TestResolveRenditionsPreservesRequestedCodec proves a UI selection (codec +
// per-rendition bitrate override) survives ResolveRenditions end-to-end — the
// path the bitrate value travels once ingest stops dropping it.
func TestResolveRenditionsPreservesRequestedCodec(t *testing.T) {
	info := domain.MediaInfo{Width: 1920, Height: 1080}
	req := domain.TranscodeRequest{
		Codecs: []string{"av1"},
		Renditions: []domain.RequestedRendition{
			{Width: 1280, Height: 720, Codec: "av1", BitrateKbps: 2500},
		},
	}
	got := ResolveRenditions(info, req, []string{"h264"})
	if len(got) != 1 {
		t.Fatalf("expected 1 rendition, got %d", len(got))
	}
	if got[0].Codec != "av1" {
		t.Fatalf("codec = %q, want av1", got[0].Codec)
	}
	if got[0].BitrateKbps != 2500 {
		t.Fatalf("bitrate = %d, want 2500 (UI override must survive)", got[0].BitrateKbps)
	}
}
