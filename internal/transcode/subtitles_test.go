package transcode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"streaming-transcode/internal/domain"
)

func TestSanitizeLanguage(t *testing.T) {
	cases := map[string]string{
		"PT":      "pt",
		" en ":    "en",
		"pt-BR":   "pt-br",
		"es_419":  "es419", // underscore stripped
		"":        "und",
		"@@@":     "und",
		"Français": "franais",
	}
	for in, want := range cases {
		if got := SanitizeLanguage(in); got != want {
			t.Errorf("SanitizeLanguage(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildSubtitleMediaPlaylist(t *testing.T) {
	pl := BuildSubtitleMediaPlaylist("pt.vtt", 12.5)
	for _, want := range []string{
		"#EXTM3U", "#EXT-X-VERSION:7", "#EXT-X-TARGETDURATION:13",
		"#EXT-X-PLAYLIST-TYPE:VOD", "#EXTINF:12.500,", "pt.vtt", "#EXT-X-ENDLIST",
	} {
		if !strings.Contains(pl, want) {
			t.Errorf("playlist missing %q:\n%s", want, pl)
		}
	}
	// A non-positive duration still yields a valid, positive EXTINF.
	if strings.Contains(BuildSubtitleMediaPlaylist("x.vtt", 0), "#EXTINF:0.000,") {
		t.Errorf("zero duration should not produce a zero EXTINF")
	}
}

func TestWriteHLSMasterWithSubtitles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.m3u8")
	renditions := []domain.Rendition{{Name: "720p", Width: 1280, Height: 720, BitrateKbps: 3000}}
	subs := []domain.SubtitleTrack{
		{Language: "pt", Label: "Português", ManifestPath: "subtitles/pt/index.m3u8"},
		{Language: "en", Label: "English", ManifestPath: "subtitles/en/index.m3u8"},
	}
	if err := WriteHLSMaster(path, renditions, true, subs...); err != nil {
		t.Fatalf("WriteHLSMaster() error = %v", err)
	}
	data, _ := os.ReadFile(path)
	text := string(data)
	for _, want := range []string{
		`#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID="subs",NAME="Português",LANGUAGE="pt",DEFAULT=YES,AUTOSELECT=YES,FORCED=NO,URI="subtitles/pt/index.m3u8"`,
		`NAME="English",LANGUAGE="en",DEFAULT=NO,AUTOSELECT=NO`,
		`SUBTITLES="subs"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("master missing %q:\n%s", want, text)
		}
	}
}

func TestWriteHLSMasterWithoutSubtitlesOmitsGroup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "master.m3u8")
	if err := WriteHLSMaster(path, []domain.Rendition{{Name: "720p", Width: 1280, Height: 720, BitrateKbps: 3000}}, true); err != nil {
		t.Fatalf("WriteHLSMaster() error = %v", err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "SUBTITLES") {
		t.Errorf("master should not reference a subtitles group when none provided:\n%s", string(data))
	}
}
