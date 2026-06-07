package worker

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// concurrencyProbeStorage records the maximum number of concurrent UploadFile
// calls, so a test can assert that directory upload is bounded-parallel.
type concurrencyProbeStorage struct {
	mu          sync.Mutex
	inFlight    int
	maxInFlight int
	uploaded    int
}

func (s *concurrencyProbeStorage) Download(context.Context, string, string, string) error {
	return nil
}

func (s *concurrencyProbeStorage) Exists(context.Context, string, string) (bool, error) {
	return false, nil
}

func (s *concurrencyProbeStorage) UploadFile(_ context.Context, _, _, _ string) error {
	s.mu.Lock()
	s.inFlight++
	if s.inFlight > s.maxInFlight {
		s.maxInFlight = s.inFlight
	}
	s.uploaded++
	s.mu.Unlock()

	// Hold the slot briefly so concurrent calls actually overlap.
	time.Sleep(5 * time.Millisecond)

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()
	return nil
}

func TestUploadDir_BoundedParallel(t *testing.T) {
	dir := t.TempDir()
	const fileCount = 20
	for i := 0; i < fileCount; i++ {
		name := filepath.Join(dir, fmt.Sprintf("segment-%02d.m4s", i))
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}
	}

	probe := &concurrencyProbeStorage{}
	p := &Processor{storage: probe, logger: log.New(io.Discard, "", 0)}

	if err := p.uploadDir(context.Background(), "videos", dir, "transcoded/v1/hls"); err != nil {
		t.Fatalf("uploadDir: %v", err)
	}

	if probe.uploaded != fileCount {
		t.Errorf("uploaded %d files, want %d", probe.uploaded, fileCount)
	}
	if probe.maxInFlight <= 1 {
		t.Errorf("expected parallel uploads (max in-flight > 1), got %d", probe.maxInFlight)
	}
	if probe.maxInFlight > maxUploadConcurrency {
		t.Errorf("upload concurrency %d exceeded bound %d", probe.maxInFlight, maxUploadConcurrency)
	}
}

// errOnNthStorage fails the Nth UploadFile, to verify error propagation under
// concurrent upload.
type errOnNthStorage struct {
	mu    sync.Mutex
	n     int
	count int
	err   error
}

func (s *errOnNthStorage) Download(context.Context, string, string, string) error { return nil }
func (s *errOnNthStorage) Exists(context.Context, string, string) (bool, error)   { return false, nil }
func (s *errOnNthStorage) UploadFile(context.Context, string, string, string) error {
	s.mu.Lock()
	s.count++
	hit := s.count == s.n
	s.mu.Unlock()
	if hit {
		return s.err
	}
	return nil
}

func TestUploadDir_PropagatesError(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 10; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f-%d", i)), []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	store := &errOnNthStorage{n: 3, err: fmt.Errorf("boom")}
	p := &Processor{storage: store, logger: log.New(io.Discard, "", 0)}

	if err := p.uploadDir(context.Background(), "videos", dir, "p"); err == nil {
		t.Fatal("expected uploadDir to return an error when an upload fails")
	}
}
