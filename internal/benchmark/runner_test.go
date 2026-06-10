package benchmark

import (
	"context"
	"os"
	"testing"

	"streaming-transcode/internal/domain"
)

type fakeStorage struct{ listed []string }

func (f *fakeStorage) List(_ context.Context, _, _ string) ([]string, error) { return f.listed, nil }
func (f *fakeStorage) Download(_ context.Context, _, _, dest string) error {
	return os.WriteFile(dest, []byte("x"), 0o644)
}

type fakeRunner struct {
	calls      int
	probeCalls int
}

func (f *fakeRunner) Probe(_ string) (domain.MediaInfo, error) {
	f.probeCalls++
	return domain.MediaInfo{
		Width: 1920, Height: 1080, DurationSeconds: 31.5, FPS: 30,
		VideoCodec: "vp9", BitrateKbps: 4200, SizeBytes: 1234,
	}, nil
}

func (f *fakeRunner) TranscodeRendition(_ context.Context, _ string, _ *domain.RawVideoParams, r domain.Rendition, _ string) (domain.RenditionMetrics, error) {
	f.calls++
	return domain.RenditionMetrics{
		ElapsedSeconds:    10,
		OutputBitrateKbps: int64(r.BitrateKbps - 50),
		ResourceUsage:     domain.ResourceUsage{AvgCPUPercent: 150, MaxCPUPercent: 300},
	}, nil
}

type fakeSink struct{ posted []Result }

func (f *fakeSink) Post(_ context.Context, res Result) error {
	f.posted = append(f.posted, res)
	return nil
}

func TestRunPostsOneResultPerJob(t *testing.T) {
	storage := &fakeStorage{}
	runner := &fakeRunner{}
	sink := &fakeSink{}
	cfg := Config{
		CorpusBucket: "b", Clips: []string{"a.mp4"},
		Codecs: []string{"h264", "av1"}, Resolutions: []Resolution{{1280, 720, 3000}},
		Repeats: 1, MachineLabel: "c5.xlarge",
	}
	deps := Deps{Storage: storage, Runner: runner, Sink: sink, WorkDir: t.TempDir()}

	if err := Run(context.Background(), cfg, deps); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 2 || len(sink.posted) != 2 {
		t.Fatalf("want 2 encodes/posts, got %d/%d", runner.calls, len(sink.posted))
	}
	r0 := sink.posted[0]
	if !r0.Benchmark || r0.MachineLabel != "c5.xlarge" || r0.Clip != "a.mp4" || len(r0.Renditions) != 1 {
		t.Fatalf("bad result: %#v", r0)
	}
	if r0.Renditions[0].Codec != "h264" || r0.Renditions[0].ElapsedSeconds != 10 || r0.Renditions[0].OutputBitrateKbps != 2950 {
		t.Fatalf("bad rendition: %#v", r0.Renditions[0])
	}
}

func TestRunListsCorpusWhenNoClips(t *testing.T) {
	storage := &fakeStorage{listed: []string{"benchmark/corpus/x.mp4"}}
	runner := &fakeRunner{}
	sink := &fakeSink{}
	cfg := Config{CorpusBucket: "b", CorpusPrefix: "benchmark/corpus/",
		Codecs: []string{"h264"}, Resolutions: []Resolution{{1280, 720, 3000}}, Repeats: 1}
	deps := Deps{Storage: storage, Runner: runner, Sink: sink, WorkDir: t.TempDir()}
	if err := Run(context.Background(), cfg, deps); err != nil {
		t.Fatal(err)
	}
	if len(sink.posted) != 1 || sink.posted[0].Clip != "benchmark/corpus/x.mp4" {
		t.Fatalf("expected 1 posted job for listed clip, got %#v", sink.posted)
	}
}

func TestRunPopulatesSourceFromProbe(t *testing.T) {
	storage := &fakeStorage{}
	runner := &fakeRunner{}
	sink := &fakeSink{}
	cfg := Config{
		CorpusBucket: "b", Clips: []string{"a.mp4"},
		Codecs: []string{"h264", "av1"}, Resolutions: []Resolution{{1280, 720, 3000}},
		Repeats: 1, MachineLabel: "c5.xlarge",
	}
	deps := Deps{Storage: storage, Runner: runner, Sink: sink, WorkDir: t.TempDir()}
	if err := Run(context.Background(), cfg, deps); err != nil {
		t.Fatal(err)
	}
	if runner.probeCalls != 1 {
		t.Fatalf("want 1 probe (cached per clip), got %d", runner.probeCalls)
	}
	if len(sink.posted) != 2 {
		t.Fatalf("want 2 posts, got %d", len(sink.posted))
	}
	r := sink.posted[0]
	if r.SourceWidth != 1920 || r.SourceHeight != 1080 || r.SourceDurationSeconds != 31.5 ||
		r.SourceFPS != 30 || r.SourceCodec != "vp9" || r.SourceBitrateKbps != 4200 || r.SourceFileSizeBytes != 1234 {
		t.Fatalf("source fields not populated: %#v", r)
	}
}
