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
