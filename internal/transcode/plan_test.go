package transcode

import (
	"testing"

	"streaming-transcode/internal/domain"
)

func TestPlanProductionRenditionsFor1080Source(t *testing.T) {
	renditions := PlanProductionRenditions(domain.MediaInfo{Width: 1920, Height: 1080})
	if len(renditions) != 2 {
		t.Fatalf("renditions len = %d, want 2", len(renditions))
	}
	if renditions[0].Name != "1080p" || renditions[1].Name != "720p" {
		t.Fatalf("renditions = %+v", renditions)
	}
}

func TestPlanProductionRenditionsAvoidsUpscaleForSmallSource(t *testing.T) {
	renditions := PlanProductionRenditions(domain.MediaInfo{Width: 854, Height: 480})
	if len(renditions) != 1 || renditions[0].Name != "source" {
		t.Fatalf("renditions = %+v", renditions)
	}
}

func TestPlanProductionRenditionsForMultipleCodecs(t *testing.T) {
	renditions := PlanProductionRenditionsForCodecs(domain.MediaInfo{Width: 1280, Height: 720}, []string{"h264", "h265", "av1", "vp9", "vpc", "unknown", "hevc"})
	if len(renditions) != 5 {
		t.Fatalf("renditions len = %d, want 5", len(renditions))
	}
	if renditions[0].Name != "h264-720p" || renditions[0].Codec != "h264" {
		t.Fatalf("first rendition = %+v", renditions[0])
	}
	if renditions[1].Name != "h265-720p" || renditions[1].Codec != "h265" {
		t.Fatalf("second rendition = %+v", renditions[1])
	}
	if renditions[2].Name != "av1-720p" || renditions[2].Codec != "av1" {
		t.Fatalf("third rendition = %+v", renditions[2])
	}
	if renditions[3].Name != "vp9-720p" || renditions[3].Codec != "vp9" {
		t.Fatalf("fourth rendition = %+v", renditions[3])
	}
	if renditions[4].Name != "vvc-720p" || renditions[4].Codec != "vvc" {
		t.Fatalf("fifth rendition = %+v", renditions[4])
	}
}

func TestPlanProductionRenditionsFallbacksToH264WhenCodecsAreInvalid(t *testing.T) {
	renditions := PlanProductionRenditionsForCodecs(domain.MediaInfo{Width: 1280, Height: 720}, []string{"unknown"})
	if len(renditions) != 1 || renditions[0].Codec != "h264" || renditions[0].Name != "720p" {
		t.Fatalf("renditions = %+v", renditions)
	}
}

func TestResolveRenditionsUsesExplicitRequest(t *testing.T) {
	info := domain.MediaInfo{Width: 1920, Height: 1080}
	renditions := ResolveRenditions(info, domain.TranscodeRequest{
		Preset: "slow",
		Renditions: []domain.RequestedRendition{
			{Width: 1280, Height: 720, BitrateKbps: 2500, Codec: "av1"},
		},
	}, []string{"h264"})
	if len(renditions) != 1 {
		t.Fatalf("renditions len = %d, want 1", len(renditions))
	}
	got := renditions[0]
	if got.Name != "av1-720p" || got.Codec != "av1" || got.BitrateKbps != 2500 || got.Preset != "slow" {
		t.Fatalf("rendition = %+v", got)
	}
}

func TestResolveRenditionsFallsBackToRequestCodecsAndPreset(t *testing.T) {
	info := domain.MediaInfo{Width: 1280, Height: 720}
	renditions := ResolveRenditions(info, domain.TranscodeRequest{
		Codecs: []string{"vp9"},
		Preset: "medium",
	}, []string{"h264"})
	if len(renditions) != 1 {
		t.Fatalf("renditions len = %d, want 1", len(renditions))
	}
	if renditions[0].Codec != "vp9" || renditions[0].Preset != "medium" || renditions[0].Name != "720p" {
		t.Fatalf("rendition = %+v", renditions[0])
	}
}

func TestValidateTranscodeRequestAcceptsEmptyRequest(t *testing.T) {
	if err := ValidateTranscodeRequest(domain.TranscodeRequest{}); err != nil {
		t.Fatalf("ValidateTranscodeRequest() error = %v, want nil for empty request", err)
	}
}

func TestValidateTranscodeRequestAcceptsKnownCodecs(t *testing.T) {
	req := domain.TranscodeRequest{Codecs: []string{"h264", "av1", "vp9"}}
	if err := ValidateTranscodeRequest(req); err != nil {
		t.Fatalf("ValidateTranscodeRequest() error = %v", err)
	}
}

func TestValidateTranscodeRequestRejectsUnknownCodecInList(t *testing.T) {
	req := domain.TranscodeRequest{Codecs: []string{"h264", "xyz"}}
	if err := ValidateTranscodeRequest(req); err == nil {
		t.Fatalf("ValidateTranscodeRequest() error = nil, want error for unknown codec")
	}
}

func TestValidateTranscodeRequestRejectsRenditionWithZeroDimensions(t *testing.T) {
	req := domain.TranscodeRequest{
		Renditions: []domain.RequestedRendition{
			{Width: 0, Height: 720, Codec: "h264"},
		},
	}
	if err := ValidateTranscodeRequest(req); err == nil {
		t.Fatalf("ValidateTranscodeRequest() error = nil, want error for width=0")
	}
}

func TestValidateTranscodeRequestRejectsRenditionWithNegativeDimensions(t *testing.T) {
	req := domain.TranscodeRequest{
		Renditions: []domain.RequestedRendition{
			{Width: 1280, Height: -1, Codec: "h264"},
		},
	}
	if err := ValidateTranscodeRequest(req); err == nil {
		t.Fatalf("ValidateTranscodeRequest() error = nil, want error for height=-1")
	}
}

func TestValidateTranscodeRequestRejectsRenditionWithUnknownCodec(t *testing.T) {
	req := domain.TranscodeRequest{
		Renditions: []domain.RequestedRendition{
			{Width: 1280, Height: 720, Codec: "xyz"},
		},
	}
	if err := ValidateTranscodeRequest(req); err == nil {
		t.Fatalf("ValidateTranscodeRequest() error = nil, want error for unknown rendition codec")
	}
}

func TestValidateTranscodeRequestAcceptsRenditionWithNoCodec(t *testing.T) {
	req := domain.TranscodeRequest{
		Renditions: []domain.RequestedRendition{
			{Width: 1280, Height: 720},
		},
	}
	if err := ValidateTranscodeRequest(req); err != nil {
		t.Fatalf("ValidateTranscodeRequest() error = %v, want nil when codec is empty (uses default)", err)
	}
}

func TestResolveRenditionsDropsRenditionsAboveSource(t *testing.T) {
	info := domain.MediaInfo{Width: 1280, Height: 720}
	req := domain.TranscodeRequest{Renditions: []domain.RequestedRendition{
		{Width: 1280, Height: 720, Codec: "h264"},
		{Width: 1920, Height: 1080, Codec: "h264"}, // above source -> dropped
	}}
	got := ResolveRenditions(info, req, []string{"h264"})
	if len(got) != 1 || got[0].Height != 720 {
		t.Fatalf("expected only the 720p rendition, got %+v", got)
	}
}

func TestResolveRenditionsFallsBackToSourceHeightPerCodec(t *testing.T) {
	info := domain.MediaInfo{Width: 1280, Height: 720}
	req := domain.TranscodeRequest{Renditions: []domain.RequestedRendition{
		{Width: 1920, Height: 1080, Codec: "h264"}, // all above source
		{Width: 1920, Height: 1080, Codec: "av1"},
	}}
	got := ResolveRenditions(info, req, []string{"h264"})
	if len(got) != 2 {
		t.Fatalf("expected one rendition per codec at source height, got %+v", got)
	}
	for _, r := range got {
		if r.Height != 720 || r.Width != 1280 {
			t.Fatalf("expected source dimensions, got %+v", r)
		}
	}
}

func TestEncodingCodecSettingsBackends(t *testing.T) {
	sw, err := encodingCodecSettings("h265", "medium", "software")
	if err != nil || sw.encoder != "libx265" {
		t.Fatalf("software h265 = %q err=%v, want libx265", sw.encoder, err)
	}
	cases := map[string]string{"h264": "h264_nvenc", "h265": "hevc_nvenc", "av1": "av1_nvenc"}
	for codec, want := range cases {
		got, err := encodingCodecSettings(codec, "medium", "nvenc")
		if err != nil || got.encoder != want {
			t.Fatalf("nvenc %s = %q err=%v, want %s", codec, got.encoder, err, want)
		}
	}
	if _, err := encodingCodecSettings("vp9", "medium", "nvenc"); err == nil {
		t.Fatal("nvenc vp9 should error")
	}
	if _, err := encodingCodecSettings("vvc", "medium", "nvenc"); err == nil {
		t.Fatal("nvenc vvc should error")
	}
}

func TestCapRenditionsByHeight(t *testing.T) {
	ladder := PlanProductionRenditionsForCodecs(
		domain.MediaInfo{Width: 3840, Height: 2160},
		[]string{"h264", "h265"},
	)
	// 4K source × 2 codecs => h264/h265 × {1080p, 720p} = 4 renditions.
	if len(ladder) != 4 {
		t.Fatalf("ladder len = %d, want 4 (%+v)", len(ladder), ladder)
	}

	// maxHeight 0 = uncapped.
	if got := CapRenditionsByHeight(ladder, 0); len(got) != 4 {
		t.Fatalf("uncapped len = %d, want 4", len(got))
	}

	// Cap at 1080 keeps every rendition <= 1080p (1080p + 720p, here both codecs).
	capped := CapRenditionsByHeight(ladder, 1080)
	for _, r := range capped {
		if r.Height > 1080 {
			t.Fatalf("rendition %s height %d exceeds cap", r.Name, r.Height)
		}
	}

	// h264-only ladder capped at 1080 on a 4K source => 1080p + 720p, no h265.
	h264 := CapRenditionsByHeight(
		PlanProductionRenditionsForCodecs(domain.MediaInfo{Width: 3840, Height: 2160}, []string{"h264"}),
		1080,
	)
	for _, r := range h264 {
		if r.Codec != "h264" {
			t.Fatalf("unexpected codec %q in %+v", r.Codec, h264)
		}
		if r.Height > 1080 {
			t.Fatalf("height %d exceeds cap", r.Height)
		}
	}

	// Cap below the whole ladder keeps exactly the shortest rendition.
	tiny := CapRenditionsByHeight(ladder, 100)
	if len(tiny) != 1 {
		t.Fatalf("sub-ladder cap len = %d, want 1", len(tiny))
	}
}
