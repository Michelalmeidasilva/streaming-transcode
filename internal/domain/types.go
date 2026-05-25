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
}

type RequestedRendition struct {
	Name        string `json:"name,omitempty"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	BitrateKbps int    `json:"bitrateKbps,omitempty"`
	Codec       string `json:"codec,omitempty"`
	Preset      string `json:"preset,omitempty"`
}

type TranscodeRequest struct {
	Profile    string               `json:"profile,omitempty"`
	Codecs     []string             `json:"codecs,omitempty"`
	Preset     string               `json:"preset,omitempty"`
	Renditions []RequestedRendition `json:"renditions,omitempty"`
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
}

type Rendition struct {
	Name         string            `json:"name"`
	Width        int               `json:"width"`
	Height       int               `json:"height"`
	BitrateKbps  int               `json:"bitrateKbps"`
	Codec        string            `json:"codec"`
	Preset       string            `json:"preset,omitempty"`
	OutputPath   string            `json:"outputPath,omitempty"`
	ManifestPath string            `json:"manifestPath,omitempty"`
	Metrics      *RenditionMetrics `json:"metrics,omitempty"`
}

type JobObservability struct {
	Hostname             string             `json:"hostname"`
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
	HLSManifestPath   string           `json:"hlsManifestPath"`
	DASHManifestPath  string           `json:"dashManifestPath"`
	MetricsPath       string           `json:"metricsPath"`
	ObservabilityPath string           `json:"observabilityPath"`
	ElapsedSeconds    float64          `json:"elapsedSeconds"`
	RTF               float64          `json:"rtf"`
	Observability     JobObservability `json:"observability"`
	CompletedAt       time.Time        `json:"completedAt"`
}
