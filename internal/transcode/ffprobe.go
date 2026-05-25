package transcode

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"streaming-transcode/internal/domain"
)

type ffprobeOutput struct {
	Streams []struct {
		CodecType    string `json:"codec_type"`
		CodecName    string `json:"codec_name"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		AvgFrameRate string `json:"avg_frame_rate"`
	} `json:"streams"`
	Format struct {
		Duration string `json:"duration"`
		BitRate  string `json:"bit_rate"`
		Size     string `json:"size"`
	} `json:"format"`
}

func Probe(ffprobePath, source string) (domain.MediaInfo, error) {
	cmd := exec.Command(ffprobePath, "-v", "error", "-print_format", "json", "-show_format", "-show_streams", source)
	output, err := cmd.Output()
	if err != nil {
		return domain.MediaInfo{}, fmt.Errorf("ffprobe failed: %w", err)
	}
	return ParseProbeOutput(output)
}

func ParseProbeOutput(data []byte) (domain.MediaInfo, error) {
	var parsed ffprobeOutput
	if err := json.Unmarshal(data, &parsed); err != nil {
		return domain.MediaInfo{}, fmt.Errorf("parse ffprobe json: %w", err)
	}

	info := domain.MediaInfo{
		DurationSeconds: parseFloat(parsed.Format.Duration),
		BitrateKbps:     parseInt64(parsed.Format.BitRate) / 1000,
		SizeBytes:       parseInt64(parsed.Format.Size),
	}

	for _, stream := range parsed.Streams {
		switch stream.CodecType {
		case "video":
			if info.VideoCodec == "" {
				info.VideoCodec = stream.CodecName
				info.Width = stream.Width
				info.Height = stream.Height
				info.FPS = parseRatio(stream.AvgFrameRate)
			}
		case "audio":
			if info.AudioCodec == "" {
				info.AudioCodec = stream.CodecName
			}
		}
	}

	if info.VideoCodec == "" || info.Width == 0 || info.Height == 0 {
		return info, fmt.Errorf("ffprobe did not return a video stream")
	}
	return info, nil
}

func parseFloat(value string) float64 {
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}

func parseRatio(value string) float64 {
	parts := strings.Split(value, "/")
	if len(parts) != 2 {
		return parseFloat(value)
	}
	numerator := parseFloat(parts[0])
	denominator := parseFloat(parts[1])
	if denominator == 0 {
		return 0
	}
	return numerator / denominator
}
