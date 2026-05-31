package worker

import "testing"

func TestParseUploadCompletedUsesExplicitObjectKey(t *testing.T) {
	event, err := ParseUploadCompleted([]byte(`{"videoId":"v1","filename":"original.mp4","objectKey":"raw/v1/original.mp4","bucket":"videos"}`), "default")
	if err != nil {
		t.Fatalf("ParseUploadCompleted() error = %v", err)
	}
	if event.ObjectKey != "raw/v1/original.mp4" {
		t.Fatalf("ObjectKey = %q", event.ObjectKey)
	}
}

func TestParseUploadCompletedFallsBackToVideoIDFilename(t *testing.T) {
	event, err := ParseUploadCompleted([]byte(`{"videoId":"v1","filename":"original.mp4"}`), "videos")
	if err != nil {
		t.Fatalf("ParseUploadCompleted() error = %v", err)
	}
	if event.ObjectKey != "v1/original.mp4" {
		t.Fatalf("ObjectKey = %q, want v1/original.mp4", event.ObjectKey)
	}
	if event.Bucket != "videos" {
		t.Fatalf("Bucket = %q", event.Bucket)
	}
}

func TestParseUploadCompletedRejectsMissingVideoID(t *testing.T) {
	_, err := ParseUploadCompleted([]byte(`{"filename":"original.mp4"}`), "videos")
	if err == nil {
		t.Fatalf("ParseUploadCompleted() error = nil, want error")
	}
}

func TestParseUploadCompletedFallsBackToSourceKeyAndDefaultProvider(t *testing.T) {
	event, err := ParseUploadCompleted([]byte(`{"videoId":"v1","sourceKey":"raw/v1/source.mp4"}`), "videos")
	if err != nil {
		t.Fatalf("ParseUploadCompleted() error = %v", err)
	}
	if event.ObjectKey != "raw/v1/source.mp4" {
		t.Fatalf("ObjectKey = %q", event.ObjectKey)
	}
	if event.Provider != "minio" {
		t.Fatalf("Provider = %q", event.Provider)
	}
}

func TestParseUploadCompletedKeepsCustomTranscodeRequest(t *testing.T) {
	event, err := ParseUploadCompleted([]byte(`{
		"videoId":"v1",
		"sourceKey":"raw/v1/source.y4m",
		"transcode":{
			"profile":"benchmark-av1",
			"preset":"slow",
			"codecs":["av1"],
			"renditions":[{"width":1280,"height":720,"bitrateKbps":2500,"codec":"av1"}]
		}
	}`), "videos")
	if err != nil {
		t.Fatalf("ParseUploadCompleted() error = %v", err)
	}
	if event.Transcode.Profile != "benchmark-av1" || event.Transcode.Preset != "slow" {
		t.Fatalf("transcode = %+v", event.Transcode)
	}
	if len(event.Transcode.Renditions) != 1 || event.Transcode.Renditions[0].Width != 1280 {
		t.Fatalf("renditions = %+v", event.Transcode.Renditions)
	}
}

func TestParseUploadCompletedRejectsMissingObjectKeyAndFilename(t *testing.T) {
	_, err := ParseUploadCompleted([]byte(`{"videoId":"v1"}`), "videos")
	if err == nil {
		t.Fatalf("ParseUploadCompleted() error = nil, want error")
	}
}

func TestParseUploadCompletedAcceptsSupportedExtensions(t *testing.T) {
	keys := []string{
		"v1/source.mp4", "v1/source.m4v", "v1/source.mov", "v1/source.mkv",
		"v1/source.webm", "v1/source.ts", "v1/source.y4m", "v1/source.m3u8",
	}
	for _, key := range keys {
		body := []byte(`{"videoId":"v1","objectKey":"` + key + `"}`)
		if _, err := ParseUploadCompleted(body, "videos"); err != nil {
			t.Fatalf("ParseUploadCompleted() objectKey=%q error = %v", key, err)
		}
	}
}

func TestParseUploadCompletedRejectsUnsupportedExtension(t *testing.T) {
	_, err := ParseUploadCompleted([]byte(`{"videoId":"v1","objectKey":"v1/source.exe"}`), "videos")
	if err == nil {
		t.Fatalf("ParseUploadCompleted() error = nil, want error for .exe")
	}
}

func TestParseUploadCompletedRejectsNoExtension(t *testing.T) {
	_, err := ParseUploadCompleted([]byte(`{"videoId":"v1","objectKey":"v1/source"}`), "videos")
	if err == nil {
		t.Fatalf("ParseUploadCompleted() error = nil, want error for missing extension")
	}
}

func TestParseUploadCompletedExtensionIsCaseInsensitive(t *testing.T) {
	_, err := ParseUploadCompleted([]byte(`{"videoId":"v1","objectKey":"v1/source.MP4"}`), "videos")
	if err != nil {
		t.Fatalf("ParseUploadCompleted() error = %v, want nil for .MP4", err)
	}
}
