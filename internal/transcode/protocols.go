package transcode

import "strings"

// defaultProtocols is the packaging set used when an event omits the protocols
// field (legacy events) or names nothing recognizable — preserves the original
// "always produce both" behavior.
var defaultProtocols = []string{"hls", "dash"}

// ResolveProtocols normalizes the requested protocol set to a deterministic,
// deduplicated subset of {"hls","dash"} in canonical order. Empty/unknown input
// falls back to both.
func ResolveProtocols(requested []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, 2)
	for _, p := range requested {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "hls":
			seen["hls"] = true
		case "dash":
			seen["dash"] = true
		}
	}
	for _, p := range defaultProtocols { // canonical order: hls, dash
		if seen[p] {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), defaultProtocols...)
	}
	return out
}

// HasProtocol reports whether name is in the resolved protocol set.
func HasProtocol(set []string, name string) bool {
	for _, p := range set {
		if p == name {
			return true
		}
	}
	return false
}

// ResolveSegmentSeconds accepts only the GOP-aligned presets 2/4/6 seconds;
// anything else falls back to 6 so segments stay multiples of the 2s GOP and
// remain keyframe-aligned for clean rendition switching.
func ResolveSegmentSeconds(requested int) int {
	switch requested {
	case 2, 4, 6:
		return requested
	default:
		return 6
	}
}
