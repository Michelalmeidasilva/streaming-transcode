package worker

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"streaming-transcode/internal/domain"
)

var supportedVideoExtensions = map[string]struct{}{
	".mp4":  {},
	".m4v":  {},
	".mov":  {},
	".mkv":  {},
	".webm": {},
	".ts":   {},
	".y4m":  {},
	".yuv":  {},
	".m3u8": {},
}

// rawVideoExtensions are headerless streams ffprobe cannot inspect; they require
// RawVideo geometry to be supplied on the event.
var rawVideoExtensions = map[string]struct{}{
	".yuv": {},
}

func isRawVideoExtension(ext string) bool {
	_, ok := rawVideoExtensions[ext]
	return ok
}

func isSupportedVideoExtension(ext string) bool {
	_, ok := supportedVideoExtensions[ext]
	return ok
}

func ParseUploadCompleted(body []byte, defaultBucket string) (domain.UploadCompletedEvent, error) {
	var event domain.UploadCompletedEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return event, terminal(fmt.Errorf("decode upload event: %w", err))
	}

	if event.VideoID == "" {
		return event, terminal(errors.New("videoId is required"))
	}

	if event.ObjectKey == "" {
		event.ObjectKey = event.SourceKey
	}
	if event.ObjectKey == "" {
		event.ObjectKey = resolveObjectKey(event.VideoID, event.Filename)
	}
	if event.ObjectKey == "" {
		return event, terminal(errors.New("objectKey or filename is required"))
	}

	ext := strings.ToLower(filepath.Ext(event.ObjectKey))
	if !isSupportedVideoExtension(ext) {
		return event, terminal(fmt.Errorf("unsupported file extension %q; supported: .mp4, .m4v, .mov, .mkv, .webm, .ts, .y4m, .yuv, .m3u8", ext))
	}

	if isRawVideoExtension(ext) {
		if event.RawVideo == nil {
			return event, terminal(fmt.Errorf("raw source %q requires rawVideo metadata (width, height, fps)", ext))
		}
		if event.RawVideo.Width <= 0 || event.RawVideo.Height <= 0 {
			return event, terminal(fmt.Errorf("raw source %q requires positive rawVideo width and height", ext))
		}
		if event.RawVideo.FPS <= 0 {
			return event, terminal(fmt.Errorf("raw source %q requires positive rawVideo fps", ext))
		}
		if strings.TrimSpace(event.RawVideo.PixelFormat) == "" {
			event.RawVideo.PixelFormat = domain.DefaultRawPixelFormat
		}
	}

	if event.Bucket == "" {
		event.Bucket = defaultBucket
	}
	if event.Provider == "" {
		event.Provider = "minio"
	}
	return event, nil
}

func resolveObjectKey(videoID, filename string) string {
	filename = strings.TrimLeft(strings.TrimSpace(filename), "/")
	if filename == "" {
		return ""
	}
	if strings.Contains(filename, "/") {
		return filename
	}
	return strings.TrimRight(videoID, "/") + "/" + filename
}
