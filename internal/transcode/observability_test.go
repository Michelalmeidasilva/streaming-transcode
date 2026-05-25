package transcode

import (
	"math"
	"os"
	"testing"
)

func TestParseProcStatCPUSeconds(t *testing.T) {
	data := []byte("1234 (ffmpeg) R 1 2 3 4 5 6 7 8 9 10 200 50 14 15 16 17 18 19 20 21 22 23 24 25")
	got, err := parseProcStatCPUSeconds(data)
	if err != nil {
		t.Fatalf("parseProcStatCPUSeconds() error = %v", err)
	}
	want := 2.5
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("parseProcStatCPUSeconds() = %.4f, want %.4f", got, want)
	}
}

func TestParseProcStatmMemoryMB(t *testing.T) {
	pages := 1024.0
	data := []byte("4096 1024 0 0 0 0 0")
	got, err := parseProcStatmMemoryMB(data)
	if err != nil {
		t.Fatalf("parseProcStatmMemoryMB() error = %v", err)
	}
	want := pages * float64(os.Getpagesize()) / (1024.0 * 1024.0)
	if math.Abs(got-want) > 0.0001 {
		t.Fatalf("parseProcStatmMemoryMB() = %.4f, want %.4f", got, want)
	}
}
