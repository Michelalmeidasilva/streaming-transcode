package transcode

import (
	"fmt"
	"strings"

	"streaming-transcode/internal/domain"
)

func PlanProductionRenditions(info domain.MediaInfo) []domain.Rendition {
	return PlanProductionRenditionsForCodecs(info, []string{"h264"})
}

func PlanProductionRenditionsForCodecs(info domain.MediaInfo, codecs []string) []domain.Rendition {
	base := planBaseRenditions(info)
	normalizedCodecs := normalizeCodecs(codecs)
	if len(normalizedCodecs) <= 1 {
		for i := range base {
			base[i].Codec = normalizedCodecs[0]
		}
		return base
	}

	renditions := make([]domain.Rendition, 0, len(base)*len(normalizedCodecs))
	for _, normalized := range normalizedCodecs {
		for _, rendition := range base {
			rendition.Codec = normalized
			rendition.Name = normalized + "-" + rendition.Name
			renditions = append(renditions, rendition)
		}
	}
	return renditions
}

func ResolveRenditions(info domain.MediaInfo, request domain.TranscodeRequest, defaultCodecs []string) []domain.Rendition {
	if len(request.Renditions) > 0 {
		resolved := make([]domain.Rendition, 0, len(request.Renditions))
		for _, requested := range request.Renditions {
			codec := normalizeCodec(requested.Codec)
			if codec == "" {
				if len(request.Codecs) > 0 {
					codec = normalizeCodecs(request.Codecs)[0]
				} else {
					codec = normalizeCodecs(defaultCodecs)[0]
				}
			}
			if requested.Width <= 0 || requested.Height <= 0 || codec == "" {
				continue
			}

			bitrateKbps := requested.BitrateKbps
			if bitrateKbps <= 0 {
				bitrateKbps = defaultBitrateForDimensions(requested.Width, requested.Height)
			}

			preset := strings.TrimSpace(requested.Preset)
			if preset == "" {
				preset = strings.TrimSpace(request.Preset)
			}

			name := strings.TrimSpace(requested.Name)
			if name == "" {
				name = fmt.Sprintf("%s-%s", codec, renditionLabel(requested.Width, requested.Height, info))
			}

			resolved = append(resolved, domain.Rendition{
				Name:        name,
				Width:       requested.Width,
				Height:      requested.Height,
				BitrateKbps: bitrateKbps,
				Codec:       codec,
				Preset:      preset,
			})
		}
		if len(resolved) > 0 {
			return resolved
		}
	}

	renditions := PlanProductionRenditionsForCodecs(info, chooseCodecs(request.Codecs, defaultCodecs))
	if strings.TrimSpace(request.Preset) != "" {
		for i := range renditions {
			renditions[i].Preset = strings.TrimSpace(request.Preset)
		}
	}
	return renditions
}

func planBaseRenditions(info domain.MediaInfo) []domain.Rendition {
	renditions := make([]domain.Rendition, 0, 2)
	if info.Width >= 1920 || info.Height >= 1080 {
		renditions = append(renditions, domain.Rendition{Name: "1080p", Width: 1920, Height: 1080, BitrateKbps: 6000})
	}
	if info.Width >= 1280 || info.Height >= 720 {
		renditions = append(renditions, domain.Rendition{Name: "720p", Width: 1280, Height: 720, BitrateKbps: 3000})
	}
	if len(renditions) == 0 {
		renditions = append(renditions, domain.Rendition{Name: "source", Width: info.Width, Height: info.Height, BitrateKbps: 1800})
	}
	return renditions
}

func chooseCodecs(requested, defaults []string) []string {
	normalized := normalizeCodecs(requested)
	if len(normalized) > 0 {
		return normalized
	}
	return normalizeCodecs(defaults)
}

func defaultBitrateForDimensions(width, height int) int {
	switch {
	case width >= 1920 || height >= 1080:
		return 6000
	case width >= 1280 || height >= 720:
		return 3000
	default:
		return 1800
	}
}

func renditionLabel(width, height int, source domain.MediaInfo) string {
	switch {
	case width == source.Width && height == source.Height:
		return "source"
	case height > 0:
		return fmt.Sprintf("%dp", height)
	default:
		return fmt.Sprintf("%dx%d", width, height)
	}
}

func normalizeCodec(codec string) string {
	switch strings.ToLower(strings.TrimSpace(codec)) {
	case "h264", "h.264", "avc", "libx264":
		return "h264"
	case "h265", "h.265", "hevc", "libx265":
		return "h265"
	case "av1", "aom", "libaom", "libaom-av1", "svt-av1", "libsvtav1":
		return "av1"
	case "vp9", "vp.9", "libvpx", "libvpx-vp9":
		return "vp9"
	case "vvc", "vpc", "h266", "h.266", "h266/vvc", "h.266/vvc", "vvenc", "libvvenc":
		return "vvc"
	default:
		return ""
	}
}

func normalizeCodecs(codecs []string) []string {
	if len(codecs) == 0 {
		return []string{"h264"}
	}
	normalized := make([]string, 0, len(codecs))
	seen := map[string]struct{}{}
	for _, codec := range codecs {
		value := normalizeCodec(codec)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []string{"h264"}
	}
	return normalized
}
