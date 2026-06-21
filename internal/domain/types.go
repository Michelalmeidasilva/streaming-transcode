package domain

import "time"

type UploadCompletedEvent struct {
	EventType     string           `json:"eventType,omitempty"`
	VideoID       string           `json:"videoId"`
	Filename      string           `json:"filename"`
	OriginalName  string           `json:"originalName,omitempty"`
	ObjectKey     string           `json:"objectKey,omitempty"`
	SourceKey     string           `json:"sourceKey,omitempty"`
	SourceETag    string           `json:"sourceETag,omitempty"`
	SourceVersion string           `json:"sourceVersion,omitempty"`
	Provider      string           `json:"provider,omitempty"`
	Bucket        string           `json:"bucket,omitempty"`
	Size          int64            `json:"size,omitempty"`
	URL           string           `json:"url,omitempty"`
	OccurredAt    string           `json:"occurredAt,omitempty"`
	Transcode     TranscodeRequest `json:"transcode,omitempty"`
	// RawVideo carries the geometry/format of headerless raw sources (.yuv).
	// Containers and self-describing formats (.mp4/.mkv/.y4m/...) leave it nil
	// and rely on ffprobe instead.
	RawVideo *RawVideoParams `json:"rawVideo,omitempty"`
	// Subtitles are sidecar .srt tracks uploaded with the video. The transcoder
	// converts each to WebVTT and advertises it in the HLS/DASH manifests.
	Subtitles []SubtitleInput `json:"subtitles,omitempty"`
}

// SubtitleInput is a sidecar subtitle uploaded alongside the video. ObjectKey
// points at the source .srt in storage; Language is a BCP-47/ISO code (e.g.
// "pt", "en") and Label is the human-readable track name shown in the player.
type SubtitleInput struct {
	ObjectKey string `json:"objectKey"`
	Language  string `json:"language,omitempty"`
	Label     string `json:"label,omitempty"`
}

// SubtitleTrack is a packaged WebVTT track referenced from the manifests and the
// playback metadata. ManifestPath is the per-language HLS media playlist.
type SubtitleTrack struct {
	Language     string `json:"language"`
	Label        string `json:"label"`
	VTTPath      string `json:"vttPath"`
	ManifestPath string `json:"manifestPath,omitempty"`
	Default      bool   `json:"default,omitempty"`
}

// RawVideoParams describes a headerless raw video stream (e.g. .yuv). ffprobe
// cannot infer these from the file, so they must be supplied at upload time and
// fed to ffmpeg as demuxer options (-f rawvideo -pix_fmt -s WxH -framerate)
// before the -i flag.
type RawVideoParams struct {
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         float64 `json:"fps"`
	PixelFormat string  `json:"pixelFormat,omitempty"`
}

// DefaultRawPixelFormat is used when an uploaded raw stream omits the pixel
// format. yuv420p is the planar 8-bit layout produced by the bitrate ladder.
const DefaultRawPixelFormat = "yuv420p"

type RequestedRendition struct {
	Name        string `json:"name,omitempty"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	BitrateKbps int    `json:"bitrateKbps,omitempty"`
	Codec       string `json:"codec,omitempty"`
	Preset      string `json:"preset,omitempty"`
}

type TranscodeRequest struct {
	Profile        string               `json:"profile,omitempty"`
	Codecs         []string             `json:"codecs,omitempty"`
	Protocols      []string             `json:"protocols,omitempty"`
	SegmentSeconds int                  `json:"segmentSeconds,omitempty"`
	Preset         string               `json:"preset,omitempty"`
	Renditions     []RequestedRendition `json:"renditions,omitempty"`
}

type MediaInfo struct {
	DurationSeconds float64 `json:"durationSeconds"`
	Width           int     `json:"width"`
	Height          int     `json:"height"`
	FPS             float64 `json:"fps"`
	VideoCodec      string  `json:"videoCodec"`
	AudioCodec      string  `json:"audioCodec,omitempty"`
	BitrateKbps     int64   `json:"bitrateKbps,omitempty"`
	SizeBytes       int64   `json:"sizeBytes,omitempty"`
}

type ResourceUsage struct {
	Supported       bool    `json:"supported"`
	SampleCount     int     `json:"sampleCount"`
	AvgCPUPercent   float64 `json:"avgCpuPercent"`
	MaxCPUPercent   float64 `json:"maxCpuPercent"`
	AvgMemoryMB     float64 `json:"avgMemoryMb"`
	MaxMemoryMB     float64 `json:"maxMemoryMb"`
	CollectionError string  `json:"collectionError,omitempty"`
}

type RenditionMetrics struct {
	Status              string        `json:"status"`
	ErrorMessage        string        `json:"errorMessage,omitempty"`
	StartedAt           time.Time     `json:"startedAt"`
	CompletedAt         time.Time     `json:"completedAt"`
	SourceFileSizeBytes int64         `json:"sourceFileSizeBytes"`
	OutputFileSizeBytes int64         `json:"outputFileSizeBytes"`
	SourceDuration      float64       `json:"sourceDurationSeconds"`
	OutputDuration      float64       `json:"outputDurationSeconds"`
	SourceBitrateKbps   int64         `json:"sourceBitrateKbps"`
	OutputBitrateKbps   int64         `json:"outputBitrateKbps"`
	SourceFPS           float64       `json:"sourceFps"`
	OutputFPS           float64       `json:"outputFps"`
	SourceCodec         string        `json:"sourceCodec"`
	OutputCodec         string        `json:"outputCodec"`
	TargetBitrateKbps   int           `json:"targetBitrateKbps"`
	ElapsedSeconds      float64       `json:"elapsedSeconds"`
	RTF                 float64       `json:"rtf"`
	CompressionRatio    float64       `json:"compressionRatio"`
	ResourceUsage       ResourceUsage `json:"resourceUsage"`
	// Provenance — the exact encode parameters used, for reproducibility/audit.
	Encoder     string `json:"encoder,omitempty"`     // libx264 / h264_nvenc / ...
	Preset      string `json:"preset,omitempty"`      // effective ffmpeg preset
	GOPSize     int    `json:"gopSize,omitempty"`     // -g / -keyint_min
	RateControl string `json:"rateControl,omitempty"` // capped-vbr | crf | cq
	Threads     int    `json:"threads,omitempty"`     // -threads (0 = auto/all cores)
	FFmpegArgs  string `json:"ffmpegArgs,omitempty"`  // full effective argument line
}

type Rendition struct {
	Name         string            `json:"name"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	BitrateKbps  int               `json:"bitrateKbps"`
	Codec        string            `json:"codec"`
	Preset       string            `json:"preset,omitempty"`
	QualityValue int               `json:"qualityValue,omitempty"`
	OutputPath   string            `json:"outputPath,omitempty"`
	ManifestPath string            `json:"manifestPath,omitempty"`
	Metrics      *RenditionMetrics `json:"metrics,omitempty"`
}

type JobObservability struct {
	Hostname             string             `json:"hostname"`
	MachineLabel         string             `json:"machineLabel,omitempty"`
	CPUCores             int                `json:"cpuCores"`
	StartedAt            time.Time          `json:"startedAt"`
	CompletedAt          time.Time          `json:"completedAt"`
	SourceFileSizeBytes  int64              `json:"sourceFileSizeBytes"`
	TotalOutputSizeBytes int64              `json:"totalOutputSizeBytes"`
	SuccessfulRenditions int                `json:"successfulRenditions"`
	FailedRenditions     int                `json:"failedRenditions"`
	Renditions           []RenditionMetrics `json:"renditions"`
}

type TranscodeResult struct {
	VideoID           string           `json:"videoId"`
	JobID             string           `json:"jobId"`
	Profile           string           `json:"profile"`
	SourceKey         string           `json:"sourceKey"`
	SourceETag        string           `json:"sourceETag,omitempty"`
	SourceVersion     string           `json:"sourceVersion,omitempty"`
	Fingerprint       string           `json:"fingerprint"`
	Attempt           int              `json:"attempt"`
	MediaInfo         MediaInfo        `json:"mediaInfo"`
	Renditions        []Rendition      `json:"renditions"`
	Subtitles         []SubtitleTrack  `json:"subtitles,omitempty"`
	HLSManifestPath   string           `json:"hlsManifestPath"`
	DASHManifestPath  string           `json:"dashManifestPath"`
	Protocols         []string         `json:"protocols,omitempty"`
	MetricsPath       string           `json:"metricsPath"`
	ObservabilityPath string           `json:"observabilityPath"`
	ElapsedSeconds    float64          `json:"elapsedSeconds"`
	RTF               float64          `json:"rtf"`
	Observability     JobObservability `json:"observability"`
	CompletedAt       time.Time        `json:"completedAt"`
}
