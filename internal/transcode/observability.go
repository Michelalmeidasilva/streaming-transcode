package transcode

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"streaming-transcode/internal/domain"
)

type processObserver struct {
	pid         int
	interval    time.Duration
	mu          sync.Mutex
	prev        *procSample
	sampleCount int
	cpuSum      float64
	cpuMax      float64
	memSumMB    float64
	memMaxMB    float64
	firstError  string
}

type procSample struct {
	capturedAt      time.Time
	totalCPUSeconds float64
	memoryMB        float64
}

const assumedLinuxClockTicksPerSecond = 100.0

func newProcessObserver(pid int, interval time.Duration) *processObserver {
	return &processObserver{
		pid:      pid,
		interval: interval,
	}
}

func (o *processObserver) Observe(ctx context.Context) {
	o.capture()

	ticker := time.NewTicker(o.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			o.capture()
		}
	}
}

func (o *processObserver) Snapshot() domain.ResourceUsage {
	o.mu.Lock()
	defer o.mu.Unlock()

	usage := domain.ResourceUsage{
		Supported:       o.sampleCount > 0,
		SampleCount:     o.sampleCount,
		MaxCPUPercent:   o.cpuMax,
		MaxMemoryMB:     o.memMaxMB,
		CollectionError: o.firstError,
	}
	if o.sampleCount > 0 {
		usage.AvgCPUPercent = o.cpuSum / float64(o.sampleCount)
		usage.AvgMemoryMB = o.memSumMB / float64(o.sampleCount)
	}
	return usage
}

func (o *processObserver) capture() {
	cpuPercent, memoryMB, next, err := sampleProcess(o.pid, o.prev)
	if err != nil {
		o.mu.Lock()
		if o.firstError == "" {
			o.firstError = err.Error()
		}
		o.mu.Unlock()
		return
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	o.prev = next

	if next == nil {
		return
	}
	if cpuPercent < 0 {
		return
	}

	o.sampleCount++
	o.cpuSum += cpuPercent
	o.memSumMB += memoryMB
	if cpuPercent > o.cpuMax {
		o.cpuMax = cpuPercent
	}
	if memoryMB > o.memMaxMB {
		o.memMaxMB = memoryMB
	}
}

func sampleProcess(pid int, prev *procSample) (float64, float64, *procSample, error) {
	if next, err := readProcSample(pid); err == nil {
		if prev == nil {
			return -1, next.memoryMB, next, nil
		}
		elapsed := next.capturedAt.Sub(prev.capturedAt).Seconds()
		if elapsed <= 0 {
			return -1, next.memoryMB, next, nil
		}
		deltaCPU := next.totalCPUSeconds - prev.totalCPUSeconds
		if deltaCPU < 0 {
			return -1, next.memoryMB, next, nil
		}
		return (deltaCPU / elapsed) * 100.0, next.memoryMB, next, nil
	}

	cmd := exec.Command("ps", "-o", "%cpu=,rss=", "-p", strconv.Itoa(pid))
	output, err := cmd.Output()
	if err != nil {
		return 0, 0, nil, err
	}

	fields := strings.Fields(string(output))
	if len(fields) < 2 {
		return 0, 0, nil, fmt.Errorf("unexpected ps output: %q", string(output))
	}

	cpuPercent, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("parse cpu percent: %w", err)
	}
	rssKB, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("parse rss kb: %w", err)
	}
	return cpuPercent, rssKB / 1024.0, &procSample{
		capturedAt: time.Now(),
		memoryMB:   rssKB / 1024.0,
	}, nil
}

func readProcSample(pid int) (*procSample, error) {
	statPath := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	statData, err := os.ReadFile(statPath)
	if err != nil {
		return nil, err
	}
	totalCPUSeconds, err := parseProcStatCPUSeconds(statData)
	if err != nil {
		return nil, err
	}

	statmPath := filepath.Join("/proc", strconv.Itoa(pid), "statm")
	statmData, err := os.ReadFile(statmPath)
	if err != nil {
		return nil, err
	}
	memoryMB, err := parseProcStatmMemoryMB(statmData)
	if err != nil {
		return nil, err
	}

	return &procSample{
		capturedAt:      time.Now(),
		totalCPUSeconds: totalCPUSeconds,
		memoryMB:        memoryMB,
	}, nil
}

func parseProcStatCPUSeconds(data []byte) (float64, error) {
	text := strings.TrimSpace(string(data))
	closing := strings.LastIndex(text, ")")
	if closing == -1 || closing+2 >= len(text) {
		return 0, fmt.Errorf("unexpected /proc stat format")
	}
	fields := strings.Fields(text[closing+2:])
	if len(fields) < 13 {
		return 0, fmt.Errorf("unexpected /proc stat field count: %d", len(fields))
	}

	utimeTicks, err := strconv.ParseFloat(fields[11], 64)
	if err != nil {
		return 0, fmt.Errorf("parse utime: %w", err)
	}
	stimeTicks, err := strconv.ParseFloat(fields[12], 64)
	if err != nil {
		return 0, fmt.Errorf("parse stime: %w", err)
	}
	return (utimeTicks + stimeTicks) / assumedLinuxClockTicksPerSecond, nil
}

func parseProcStatmMemoryMB(data []byte) (float64, error) {
	fields := strings.Fields(string(data))
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected /proc statm format")
	}
	residentPages, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return 0, fmt.Errorf("parse resident pages: %w", err)
	}
	return residentPages * float64(os.Getpagesize()) / (1024.0 * 1024.0), nil
}

func fileSizeBytes(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

func hostObservability() (string, int) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}
	return hostname, runtime.NumCPU()
}

func runObserved(ctx context.Context, binary string, args ...string) (domain.ResourceUsage, string, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	if err := cmd.Start(); err != nil {
		return domain.ResourceUsage{}, "", err
	}

	observeCtx, cancel := context.WithCancel(context.Background())
	observer := newProcessObserver(cmd.Process.Pid, 500*time.Millisecond)
	done := make(chan struct{})
	go func() {
		defer close(done)
		observer.Observe(observeCtx)
	}()

	err := cmd.Wait()
	cancel()
	<-done

	text := output.String()
	if err != nil {
		return observer.Snapshot(), text, fmt.Errorf("%s %s failed: %w: %s", binary, strings.Join(args, " "), err, text)
	}
	return observer.Snapshot(), text, nil
}
