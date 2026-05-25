package main

import (
	"bufio"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	videoIDPattern = regexp.MustCompile(`\b\d{6,}\b`)
	defaultURLs    = []string{
		"https://vimeo.com/1078990193",
		"https://vimeo.com/339952895",
		"https://vimeo.com/1185327824",
		"https://vimeo.com/158920985",
		"https://vimeo.com/158921536",
		"https://vimeo.com/544796409",
		"https://vimeo.com/158921536",
		"https://vimeo.com/1075261593",
		"https://vimeo.com/765449139",
		"https://vimeo.com/797434674",
		"https://vimeo.com/1117476549",
		"https://vimeo.com/1145209824",
		"https://vimeo.com/714049017",
		"https://vimeo.com/374557639",
		"https://vimeo.com/1145891430",
		"https://vimeo.com/833927212",
		"https://vimeo.com/866373019",
		"https://vimeo.com/1114576994",
		"https://vimeo.com/853253127",
	}
)

type resultRow struct {
	InputURL     string `json:"input_url"`
	VideoID      string `json:"video_id"`
	Title        string `json:"title,omitempty"`
	License      string `json:"license,omitempty"`
	Delivery     string `json:"delivery,omitempty"`
	Variant      string `json:"variant,omitempty"`
	Codec        string `json:"codec,omitempty"`
	Width        int    `json:"width,omitempty"`
	Height       int    `json:"height,omitempty"`
	FPS          string `json:"fps,omitempty"`
	BitrateKbps  int    `json:"bitrate_kbps,omitempty"`
	BandwidthBps int    `json:"bandwidth_bps,omitempty"`
	SourceURL    string `json:"source_url,omitempty"`
	Error        string `json:"error,omitempty"`
}

type analysisResult struct {
	InputURL string      `json:"input_url"`
	VideoID  string      `json:"video_id,omitempty"`
	Title    string      `json:"title,omitempty"`
	Rows     []resultRow `json:"rows,omitempty"`
	Error    string      `json:"error,omitempty"`
}

type ytDLPVideo struct {
	ID      string          `json:"id"`
	Title   string          `json:"title"`
	License string          `json:"license"`
	Formats []ytDLPFormat   `json:"formats"`
	Error   *ytDLPErrorNode `json:"_error"`
}

type ytDLPErrorNode struct {
	Message string `json:"message"`
}

type ytDLPFormat struct {
	FormatID    string  `json:"format_id"`
	FormatNote  string  `json:"format_note"`
	Protocol    string  `json:"protocol"`
	ManifestURL string  `json:"manifest_url"`
	URL         string  `json:"url"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FPS         float64 `json:"fps"`
	TBR         float64 `json:"tbr"`
	VBR         float64 `json:"vbr"`
	ABR         float64 `json:"abr"`
	VCodec      string  `json:"vcodec"`
	ACodec      string  `json:"acodec"`
	Resolution  string  `json:"resolution"`
	Ext         string  `json:"ext"`
	Format      string  `json:"format"`
}

func main() {
	var (
		inputFile          = flag.String("input", "", "arquivo com uma URL/ID do Vimeo por linha")
		format             = flag.String("format", "csv", "formato de saida: csv ou json")
		timeout            = flag.Duration("timeout", 45*time.Second, "timeout por video")
		ytDLPPath          = flag.String("yt-dlp-path", "yt-dlp", "caminho do executavel yt-dlp")
		cookiesFromBrowser = flag.String("cookies-from-browser", "", "browser para ler cookies, ex: chrome, firefox, safari")
	)
	flag.Parse()

	inputs, err := loadInputs(*inputFile, flag.Args())
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao carregar entradas: %v\n", err)
		os.Exit(1)
	}

	results := make([]analysisResult, 0, len(inputs))
	for _, input := range inputs {
		results = append(results, analyzeVideo(*ytDLPPath, *cookiesFromBrowser, *timeout, input))
	}

	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		if err := writeJSON(os.Stdout, results); err != nil {
			fmt.Fprintf(os.Stderr, "erro ao escrever JSON: %v\n", err)
			os.Exit(1)
		}
	case "csv":
		if err := writeCSV(os.Stdout, flattenResults(results)); err != nil {
			fmt.Fprintf(os.Stderr, "erro ao escrever CSV: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "formato invalido: %s\n", *format)
		os.Exit(1)
	}
}

func loadInputs(inputFile string, args []string) ([]string, error) {
	if inputFile != "" {
		file, err := os.Open(inputFile)
		if err != nil {
			return nil, err
		}
		defer file.Close()

		var inputs []string
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			inputs = append(inputs, line)
		}
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return dedupe(inputs), nil
	}

	if len(args) > 0 {
		return dedupe(args), nil
	}

	return dedupe(defaultURLs), nil
}

func analyzeVideo(ytDLPPath, cookiesFromBrowser string, timeout time.Duration, input string) analysisResult {
	videoID, err := extractVideoID(input)
	if err != nil {
		return analysisResult{InputURL: input, Error: err.Error()}
	}

	info, err := fetchWithYTDLP(ytDLPPath, cookiesFromBrowser, timeout, input)
	if err != nil {
		return analysisResult{InputURL: input, VideoID: videoID, Error: err.Error()}
	}

	if info.ID == "" {
		info.ID = videoID
	}

	rows := make([]resultRow, 0, len(info.Formats))
	for _, format := range info.Formats {
		if row, ok := buildRow(input, info, format); ok {
			rows = append(rows, row)
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Delivery != rows[j].Delivery {
			return rows[i].Delivery < rows[j].Delivery
		}
		if rows[i].Height != rows[j].Height {
			return rows[i].Height > rows[j].Height
		}
		return rows[i].BandwidthBps > rows[j].BandwidthBps
	})

	if len(rows) == 0 {
		return analysisResult{
			InputURL: input,
			VideoID:  info.ID,
			Title:    info.Title,
			Error:    "yt-dlp nao retornou formatos utilizaveis",
		}
	}

	return analysisResult{
		InputURL: input,
		VideoID:  info.ID,
		Title:    info.Title,
		Rows:     rows,
	}
}

func fetchWithYTDLP(ytDLPPath, cookiesFromBrowser string, timeout time.Duration, input string) (ytDLPVideo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{"-J", "--no-warnings", "--skip-download"}
	if cookiesFromBrowser != "" {
		args = append(args, "--cookies-from-browser", cookiesFromBrowser)
	}
	args = append(args, input)

	cmd := exec.CommandContext(ctx, ytDLPPath, args...)
	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			msg := strings.TrimSpace(string(exitErr.Stderr))
			if msg == "" {
				msg = err.Error()
			}
			return ytDLPVideo{}, fmt.Errorf("yt-dlp falhou: %s", msg)
		}
		if ctx.Err() == context.DeadlineExceeded {
			return ytDLPVideo{}, fmt.Errorf("yt-dlp excedeu timeout de %s", timeout)
		}
		return ytDLPVideo{}, fmt.Errorf("falha ao executar yt-dlp: %w", err)
	}

	var info ytDLPVideo
	if err := json.Unmarshal(output, &info); err != nil {
		return ytDLPVideo{}, fmt.Errorf("falha ao parsear JSON do yt-dlp: %w", err)
	}
	if info.Error != nil && strings.TrimSpace(info.Error.Message) != "" {
		return ytDLPVideo{}, fmt.Errorf("yt-dlp retornou erro: %s", info.Error.Message)
	}
	return info, nil
}

func buildRow(input string, info ytDLPVideo, format ytDLPFormat) (resultRow, bool) {
	bitrate := bitrateFromFormat(format)
	if bitrate <= 0 {
		return resultRow{}, false
	}

	delivery := inferDelivery(format)
	codec := inferCodec(format)
	variant := inferVariant(format)
	sourceURL := firstNonEmpty(format.ManifestURL, format.URL)

	return resultRow{
		InputURL:     input,
		VideoID:      info.ID,
		Title:        info.Title,
		License:      info.License,
		Delivery:     delivery,
		Variant:      variant,
		Codec:        codec,
		Width:        format.Width,
		Height:       format.Height,
		FPS:          formatFPS(format.FPS),
		BitrateKbps:  int(math.Round(bitrate)),
		BandwidthBps: int(math.Round(bitrate * 1000)),
		SourceURL:    sourceURL,
	}, true
}

func bitrateFromFormat(format ytDLPFormat) float64 {
	switch {
	case format.TBR > 0:
		return format.TBR
	case format.VBR > 0:
		return format.VBR
	case format.ABR > 0:
		return format.ABR
	default:
		return 0
	}
}

func inferDelivery(format ytDLPFormat) string {
	lowerID := strings.ToLower(format.FormatID)
	lowerProtocol := strings.ToLower(format.Protocol)
	lowerManifest := strings.ToLower(format.ManifestURL)

	switch {
	case strings.Contains(lowerID, "dash") || strings.Contains(lowerProtocol, "dash") || strings.Contains(lowerManifest, ".mpd"):
		return "dash"
	case strings.Contains(lowerID, "hls") || strings.Contains(lowerProtocol, "m3u8") || strings.Contains(lowerManifest, ".m3u8"):
		return "hls"
	case strings.HasPrefix(lowerID, "http-") || strings.HasPrefix(lowerProtocol, "http") || strings.HasPrefix(lowerProtocol, "https"):
		return "progressive"
	default:
		return firstNonEmpty(format.Protocol, "unknown")
	}
}

func inferCodec(format ytDLPFormat) string {
	switch {
	case format.VCodec != "" && format.VCodec != "none":
		return format.VCodec
	case format.ACodec != "" && format.ACodec != "none":
		return format.ACodec
	case format.Ext != "":
		return format.Ext
	default:
		return ""
	}
}

func inferVariant(format ytDLPFormat) string {
	switch {
	case format.Height > 0:
		return resolutionLabel(format.Height)
	case strings.TrimSpace(format.Resolution) != "":
		return format.Resolution
	case strings.TrimSpace(format.FormatNote) != "":
		return format.FormatNote
	default:
		return format.FormatID
	}
}

func formatFPS(value float64) string {
	if value <= 0 {
		return ""
	}
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", value), "0"), ".")
}

func extractVideoID(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", errors.New("entrada vazia")
	}
	if videoIDPattern.MatchString(trimmed) && !strings.Contains(trimmed, "/") {
		return videoIDPattern.FindString(trimmed), nil
	}
	if match := videoIDPattern.FindString(trimmed); match != "" {
		return match, nil
	}
	return "", fmt.Errorf("video_id nao encontrado em %q", input)
}

func resolutionLabel(height int) string {
	if height == 0 {
		return ""
	}
	return fmt.Sprintf("%dp", height)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func flattenResults(results []analysisResult) []resultRow {
	var rows []resultRow
	for _, result := range results {
		if result.Error != "" {
			rows = append(rows, resultRow{
				InputURL: result.InputURL,
				VideoID:  result.VideoID,
				Title:    result.Title,
				License:  "",
				Error:    result.Error,
			})
			continue
		}
		rows = append(rows, result.Rows...)
	}
	return rows
}

func writeCSV(out io.Writer, rows []resultRow) error {
	writer := csv.NewWriter(out)
	defer writer.Flush()

	header := []string{
		"input_url",
		"video_id",
		"title",
		"license",
		"delivery",
		"variant",
		"codec",
		"width",
		"height",
		"fps",
		"bitrate_kbps",
		"bandwidth_bps",
		"source_url",
		"error",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	for _, row := range rows {
		record := []string{
			row.InputURL,
			row.VideoID,
			row.Title,
			row.License,
			row.Delivery,
			row.Variant,
			row.Codec,
			itoaIfPositive(row.Width),
			itoaIfPositive(row.Height),
			row.FPS,
			itoaIfPositive(row.BitrateKbps),
			itoaIfPositive(row.BandwidthBps),
			row.SourceURL,
			row.Error,
		}
		if err := writer.Write(record); err != nil {
			return err
		}
	}
	return writer.Error()
}

func writeJSON(out io.Writer, results []analysisResult) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	return encoder.Encode(results)
}

func itoaIfPositive(value int) string {
	if value <= 0 {
		return ""
	}
	return strconv.Itoa(value)
}

func dedupe(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
