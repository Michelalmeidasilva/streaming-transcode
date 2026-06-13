package transcode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// vmafLog is the subset of ffmpeg libvmaf's JSON output we read.
type vmafLog struct {
	PooledMetrics struct {
		VMAF *struct {
			Mean float64 `json:"mean"`
		} `json:"vmaf"`
		PSNRY *struct {
			Mean float64 `json:"mean"`
		} `json:"psnr_y"`
	} `json:"pooled_metrics"`
}

func parseVMAFLog(b []byte) (vmaf, psnr float64, err error) {
	var l vmafLog
	if err = json.Unmarshal(b, &l); err != nil {
		return 0, 0, err
	}
	if l.PooledMetrics.VMAF == nil {
		return 0, 0, fmt.Errorf("vmaf metric absent in libvmaf log")
	}
	psnr = 0
	if l.PooledMetrics.PSNRY != nil {
		psnr = l.PooledMetrics.PSNRY.Mean
	}
	return l.PooledMetrics.VMAF.Mean, psnr, nil
}

// Quality runs libvmaf comparing the distorted (encoded) file against the
// reference (source) scaled+padded to the encode geometry — the same filter the
// encode used, so frames align. Returns mean VMAF and mean PSNR-Y.
func (r *Runner) Quality(ctx context.Context, reference, distorted string, width, height int) (float64, float64, error) {
	logFile, err := os.CreateTemp("", "vmaf-*.json")
	if err != nil {
		return 0, 0, err
	}
	logName := logFile.Name()
	logFile.Close()
	defer os.Remove(logName)

	scalePad := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2", width, height, width, height)
	// input 0 = distorted (encoded), input 1 = reference (source). libvmaf takes
	// [distorted][reference]. Normalize timebase/pts so frame N aligns to frame N.
	filter := fmt.Sprintf(
		"[0:v]settb=AVTB,setpts=PTS-STARTPTS[dist];[1:v]%s,settb=AVTB,setpts=PTS-STARTPTS[ref];[dist][ref]libvmaf=feature=name=psnr:log_fmt=json:log_path=%s",
		scalePad, logName,
	)
	cmd := exec.CommandContext(ctx, r.cfg.FFmpegPath, "-i", distorted, "-i", reference, "-lavfi", filter, "-f", "null", "-")
	if out, err := cmd.CombinedOutput(); err != nil {
		return 0, 0, fmt.Errorf("libvmaf failed: %v: %s", err, truncate(string(out), 500))
	}
	b, err := os.ReadFile(logName)
	if err != nil {
		return 0, 0, err
	}
	return parseVMAFLog(b)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
