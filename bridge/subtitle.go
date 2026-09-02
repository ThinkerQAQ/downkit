package downkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const ytDLPSubtitleRecordPrefix = "downkit-subtitle:"

// SubtitleRequest is part of the Bridge task protocol. Phase one supports
// source-site subtitles only; the shape leaves ASR and translation policy out
// of the transport until those engines are actually available.
type SubtitleRequest struct {
	Mode             string   `json:"mode,omitempty"`
	Languages        []string `json:"languages,omitempty"`
	IncludeAutomatic bool     `json:"includeAutomatic,omitempty"`
	Format           string   `json:"format,omitempty"`
}

func (r SubtitleRequest) enabled() bool {
	return strings.EqualFold(strings.TrimSpace(r.Mode), "site")
}

func normalizeSubtitleRequest(request SubtitleRequest) (SubtitleRequest, error) {
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode == "" {
		request.Mode = "none"
	}
	if request.Mode != "none" && request.Mode != "site" {
		return request, errors.New("字幕模式必须是 none 或 site")
	}
	request.Format = strings.ToLower(strings.TrimSpace(request.Format))
	if request.Format == "" {
		request.Format = "best"
	}
	switch request.Format {
	case "best", "srt", "vtt", "ass":
	default:
		return request, errors.New("字幕格式必须是 best、srt、vtt 或 ass")
	}

	languages := make([]string, 0, len(request.Languages))
	seen := make(map[string]bool)
	for _, language := range request.Languages {
		language = strings.TrimSpace(language)
		if language == "" {
			continue
		}
		if len(language) > 64 || strings.ContainsAny(language, "\r\n,") {
			return request, errors.New("字幕语言标记无效")
		}
		key := strings.ToLower(language)
		if !seen[key] {
			seen[key] = true
			languages = append(languages, language)
		}
		if len(languages) > 20 {
			return request, errors.New("字幕语言不能超过 20 项")
		}
	}
	if request.Mode == "site" && len(languages) == 0 {
		languages = []string{"all", "-live_chat"}
	}
	request.Languages = languages
	return request, nil
}

// SubtitlePipeline coordinates subtitle acquisition and, later, ASR and
// translation. The first implementation only acquires source-site subtitles.
type SubtitlePipeline interface {
	Process(context.Context, SubtitleRequest, []MediaOutput) ([]SubtitleArtifact, error)
}

// ASREngine is the future boundary for whisper.cpp or another local engine.
type ASREngine interface {
	Transcribe(context.Context, MediaOutput, string) (SubtitleArtifact, error)
}

// Translator is the future boundary for llama.cpp or another local model.
type Translator interface {
	Translate(context.Context, SubtitleArtifact, string, bool) (SubtitleArtifact, error)
}

type defaultSubtitlePipeline struct {
	app        *app
	asr        ASREngine
	translator Translator
}

func newSubtitlePipeline(a *app, asr ASREngine, translator Translator) SubtitlePipeline {
	return &defaultSubtitlePipeline{app: a, asr: asr, translator: translator}
}

func (p *defaultSubtitlePipeline) Process(ctx context.Context, request SubtitleRequest, media []MediaOutput) ([]SubtitleArtifact, error) {
	existing := subtitleArtifactsFromMedia(media)
	if !request.enabled() || len(existing) > 0 || sourceSubtitleAttemptComplete(media) {
		return existing, nil
	}
	return p.app.downloadSiteSubtitles(ctx, request, media)
}

func sourceSubtitleAttemptComplete(media []MediaOutput) bool {
	if len(media) == 0 {
		return false
	}
	for _, output := range media {
		if !output.SubtitleAttempted {
			return false
		}
	}
	return true
}

func (a *app) downloadSiteSubtitles(ctx context.Context, request SubtitleRequest, media []MediaOutput) ([]SubtitleArtifact, error) {
	if len(media) != 1 {
		// yt-dlp page downloads obtain playlist subtitles in the media command.
		// Avoid a second playlist run when the site returned none.
		return nil, nil
	}
	sourceURL := subtitleSourceURL(a.opts)
	access, err := a.prepareYTDLPAccess(sourceURL)
	if err != nil {
		return nil, err
	}
	defer access.cleanup()

	record := filepath.Join(a.workDir, "yt-dlp-subtitles.txt")
	_ = os.Remove(record)
	var lastErr error
	for attemptIndex, attempt := range access.attempts {
		if attemptIndex > 0 {
			fmt.Fprintln(consoleOut, "原站字幕获取改用备用网络线路重试。")
		}
		args := a.ytDLPSubtitleOnlyArgs(sourceURL, media[0], record, request, attempt)
		if runErr, detail := a.runYTDLPCommandContext(ctx, args); runErr != nil {
			lastErr = describeYTDLPFailure(runErr, detail)
			continue
		}
		artifacts, readErr := readYTDLPSubtitleArtifacts(record, media, a.opts.outputDir)
		if readErr != nil {
			return nil, readErr
		}
		if len(artifacts) == 0 {
			fmt.Fprintln(consoleOut, "原站未提供符合所选语言的字幕，媒体文件仍会正常保留。")
		}
		return artifacts, nil
	}
	if lastErr == nil {
		lastErr = errors.New("没有可用的 yt-dlp 网络尝试")
	}
	return nil, lastErr
}

func subtitleSourceURL(opts options) string {
	for _, candidate := range []string{opts.pageURL, opts.referer, opts.sourceURL} {
		candidate = strings.TrimSpace(candidate)
		if strings.HasPrefix(strings.ToLower(candidate), "http://") || strings.HasPrefix(strings.ToLower(candidate), "https://") {
			return candidate
		}
	}
	return opts.sourceURL
}

func (a *app) ytDLPSubtitleOnlyArgs(pageURL string, media MediaOutput, record string, request SubtitleRequest, attempt ytDLPAttempt) []string {
	base := strings.TrimSuffix(media.Path, filepath.Ext(media.Path))
	outputTemplate := strings.ReplaceAll(base, "%", "%%") + ".%(ext)s"
	args := []string{
		"--no-color", "--newline", "--skip-download", "--write-subs",
		"--sub-langs", strings.Join(request.Languages, ","), "--sub-format", request.Format,
		"--output", outputTemplate,
		"--print-to-file", "after_video:" + ytDLPSubtitleRecordPrefix + "0||%(requested_subtitles)j", strings.ReplaceAll(record, "%", "%%"),
	}
	if request.IncludeAutomatic {
		args = append(args, "--write-auto-subs")
	}
	args = a.appendYTDLPAccessArgs(args, pageURL, attempt)
	return append(args, pageURL)
}

type ytDLPRequestedSubtitle struct {
	Filepath string `json:"filepath"`
	Ext      string `json:"ext"`
}

func readYTDLPSubtitleArtifacts(record string, media []MediaOutput, outputDir string) ([]SubtitleArtifact, error) {
	data, err := os.ReadFile(record)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("无法读取 yt-dlp 字幕记录：%w", err)
	}
	var artifacts []SubtitleArtifact
	for _, line := range splitLines(string(data)) {
		index, itemID, requested, ok := parseYTDLPSubtitleRecord(line)
		if !ok {
			continue
		}
		associated := matchingMediaOutput(media, index, itemID)
		for language, subtitle := range requested {
			path := strings.TrimSpace(subtitle.Filepath)
			if path == "" {
				continue
			}
			if !filepath.IsAbs(path) {
				path = filepath.Join(outputDir, filepath.FromSlash(path))
			}
			path = filepath.Clean(path)
			if info, statErr := os.Stat(path); statErr != nil || !info.Mode().IsRegular() {
				continue
			}
			format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
			if format == "" {
				format = strings.ToLower(strings.TrimSpace(subtitle.Ext))
			}
			artifacts = appendUniqueSubtitleArtifacts(artifacts, SubtitleArtifact{
				Path: path, Language: language, Format: format, Source: "site",
				MediaPath: associated.Path, MediaIndex: associated.Index, MediaID: associated.ItemID,
			})
		}
	}
	return artifacts, nil
}

func parseYTDLPSubtitleRecord(line string) (int, string, map[string]ytDLPRequestedSubtitle, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, ytDLPSubtitleRecordPrefix) {
		return 0, "", nil, false
	}
	parts := strings.SplitN(strings.TrimPrefix(line, ytDLPSubtitleRecordPrefix), "|", 3)
	if len(parts) != 3 {
		return 0, "", nil, false
	}
	index, _ := strconv.Atoi(strings.TrimSpace(parts[0]))
	requested := make(map[string]ytDLPRequestedSubtitle)
	if json.Unmarshal([]byte(parts[2]), &requested) != nil {
		return 0, "", nil, false
	}
	return max(index, 0), strings.TrimSpace(parts[1]), requested, true
}

func matchingMediaOutput(media []MediaOutput, index int, itemID string) MediaOutput {
	for _, output := range media {
		if itemID != "" && strings.EqualFold(output.ItemID, itemID) {
			return output
		}
		if index > 0 && output.Index == index {
			return output
		}
	}
	if len(media) == 1 {
		return media[0]
	}
	return MediaOutput{Index: index, ItemID: itemID}
}
