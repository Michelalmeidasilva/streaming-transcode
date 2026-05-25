package main

import (
	"reflect"
	"testing"
)

func TestExtractVideoID(t *testing.T) {
	tests := map[string]string{
		"https://vimeo.com/1078990193":             "1078990193",
		"https://player.vimeo.com/video/339952895": "339952895",
		"1185327824": "1185327824",
	}

	for input, want := range tests {
		got, err := extractVideoID(input)
		if err != nil {
			t.Fatalf("extractVideoID(%q) error = %v", input, err)
		}
		if got != want {
			t.Fatalf("extractVideoID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestCollectManifestURLs(t *testing.T) {
	format := ytDLPFormat{
		FormatID:    "dash-video-11777",
		Protocol:    "http_dash_segments",
		ManifestURL: "https://example.com/manifest.mpd",
		URL:         "https://example.com/video.mp4",
		Width:       3840,
		Height:      2160,
		FPS:         24,
		TBR:         11777,
		VCodec:      "avc1.640034",
	}
	info := ytDLPVideo{ID: "1", Title: "Demo", License: "Creative Commons"}

	row, ok := buildRow("https://vimeo.com/1", info, format)
	if !ok {
		t.Fatal("buildRow() returned ok=false")
	}
	if row.Delivery != "dash" || row.BitrateKbps != 11777 || row.Height != 2160 || row.License != "Creative Commons" {
		t.Fatalf("row = %+v", row)
	}
}

func TestBuildRowAudioFormat(t *testing.T) {
	format := ytDLPFormat{
		FormatID:   "dash-audio-196",
		Protocol:   "http_dash_segments",
		TBR:        196,
		ACodec:     "mp4a.40.2",
		Resolution: "audio only",
	}
	info := ytDLPVideo{ID: "1", Title: "Demo", License: "Standard Vimeo License"}

	row, ok := buildRow("https://vimeo.com/1", info, format)
	if !ok {
		t.Fatal("buildRow() returned ok=false")
	}
	if row.Delivery != "dash" || row.Codec != "mp4a.40.2" || row.Variant != "audio only" || row.License != "Standard Vimeo License" {
		t.Fatalf("row = %+v", row)
	}
}

func TestInferDelivery(t *testing.T) {
	tests := []struct {
		format ytDLPFormat
		want   string
	}{
		{format: ytDLPFormat{FormatID: "http-720p", Protocol: "https"}, want: "progressive"},
		{format: ytDLPFormat{FormatID: "hls-1306-0", Protocol: "m3u8_native"}, want: "hls"},
		{format: ytDLPFormat{FormatID: "dash-video-1465", Protocol: "http_dash_segments"}, want: "dash"},
	}

	for _, tc := range tests {
		if got := inferDelivery(tc.format); got != tc.want {
			t.Fatalf("inferDelivery(%+v) = %q, want %q", tc.format, got, tc.want)
		}
	}
}

func TestDedupe(t *testing.T) {
	got := dedupe([]string{"a", "b", "a", "", "b", "c"})
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupe() = %#v, want %#v", got, want)
	}
}
