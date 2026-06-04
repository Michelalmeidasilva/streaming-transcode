package transcode

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"streaming-transcode/internal/domain"
)

// ConvertSRTToVTT converts a SubRip (.srt) file to WebVTT (.vtt) using ffmpeg's
// webvtt muxer. WebVTT is the format HLS/DASH players (and Shaka) consume.
func (r *Runner) ConvertSRTToVTT(ctx context.Context, srcSRT, outVTT string) error {
	if err := os.MkdirAll(filepath.Dir(outVTT), 0o755); err != nil {
		return err
	}
	return run(ctx, r.cfg.FFmpegPath, "-y", "-i", srcSRT, "-c:s", "webvtt", "-f", "webvtt", outVTT)
}

// SanitizeLanguage normalizes a subtitle language code into a lowercase token
// safe for filenames and HLS GROUP-ID/URI use. Falls back to "und" (undefined).
func SanitizeLanguage(language string) string {
	lang := strings.ToLower(strings.TrimSpace(language))
	var b strings.Builder
	for _, r := range lang {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "und"
	}
	return b.String()
}

// subtitleLabel returns a display label, defaulting to the language code.
func subtitleLabel(track domain.SubtitleTrack) string {
	if strings.TrimSpace(track.Label) != "" {
		return track.Label
	}
	if strings.TrimSpace(track.Language) != "" {
		return track.Language
	}
	return "Subtitles"
}

// BuildSubtitleMediaPlaylist renders the per-language HLS media playlist that
// points at the single WebVTT file. HLS subtitles need their own playlist; the
// whole VTT is advertised as one segment spanning the program duration.
func BuildSubtitleMediaPlaylist(vttFilename string, durationSeconds float64) string {
	if durationSeconds <= 0 {
		// Players require a positive EXTINF; default to a long window.
		durationSeconds = 3600
	}
	target := int(math.Ceil(durationSeconds))
	var b strings.Builder
	b.WriteString("#EXTM3U\n")
	b.WriteString("#EXT-X-VERSION:7\n")
	fmt.Fprintf(&b, "#EXT-X-TARGETDURATION:%d\n", target)
	b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	fmt.Fprintf(&b, "#EXTINF:%.3f,\n", durationSeconds)
	b.WriteString(vttFilename + "\n")
	b.WriteString("#EXT-X-ENDLIST\n")
	return b.String()
}

// hlsSubtitleMediaLines renders the #EXT-X-MEDIA:TYPE=SUBTITLES entries for the
// master playlist. The first track is marked DEFAULT/AUTOSELECT.
func hlsSubtitleMediaLines(subtitles []domain.SubtitleTrack) string {
	var b strings.Builder
	for i, track := range subtitles {
		lang := SanitizeLanguage(track.Language)
		def := "NO"
		auto := "NO"
		if i == 0 || track.Default {
			def = "YES"
			auto = "YES"
		}
		uri := track.ManifestPath
		if uri == "" {
			uri = fmt.Sprintf("subtitles/%s/index.m3u8", lang)
		}
		fmt.Fprintf(&b,
			"#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"%s\",NAME=\"%s\",LANGUAGE=\"%s\",DEFAULT=%s,AUTOSELECT=%s,FORCED=NO,URI=\"%s\"\n",
			hlsSubtitleGroupID, subtitleLabel(track), lang, def, auto, uri,
		)
	}
	return b.String()
}

const hlsSubtitleGroupID = "subs"
