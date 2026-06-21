package transcode

import (
	"os/exec"
	"strings"
)

// FFmpegVersion returns the first line of `ffmpeg -version` (e.g.
// "ffmpeg version 5.1.9-0+deb12u1 ..."), used as benchmark provenance so runs
// recorded from the CPU image and the GPU image (different ffmpeg builds) are
// distinguishable and reproducible. Returns "" when the binary cannot be run.
func FFmpegVersion(ffmpegPath string) string {
	out, err := exec.Command(ffmpegPath, "-version").Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(line)
}
