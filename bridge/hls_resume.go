package downkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const hlsResumePlanVersion = 1

type hlsResumePlan struct {
	Version       int             `json:"version"`
	Segments      []resumeSegment `json:"segments"`
	AudioSegments []resumeSegment `json:"audioSegments,omitempty"`
	AudioInput    string          `json:"audioInput,omitempty"`
}

type resumeSegment struct {
	URL         string `json:"url"`
	Name        string `json:"name"`
	RangeStart  int64  `json:"rangeStart,omitempty"`
	RangeLength int64  `json:"rangeLength,omitempty"`
	Encrypted   bool   `json:"encrypted,omitempty"`
}

func hlsResumePlanPath(workDir string) string {
	return filepath.Join(workDir, "resume-plan.json")
}

func resumeSegments(values []segment) []resumeSegment {
	result := make([]resumeSegment, 0, len(values))
	for _, value := range values {
		result = append(result, resumeSegment{
			URL: value.url, Name: value.name, RangeStart: value.rangeStart,
			RangeLength: value.rangeLength, Encrypted: value.encrypted,
		})
	}
	return result
}

func restoreSegments(values []resumeSegment, directory string) []segment {
	result := make([]segment, 0, len(values))
	for _, value := range values {
		result = append(result, segment{
			url: value.URL, name: value.Name, path: filepath.Join(directory, value.Name),
			rangeStart: value.RangeStart, rangeLength: value.RangeLength, encrypted: value.Encrypted,
		})
	}
	return result
}

func saveHLSResumePlan(a *app, segments, audioSegments []segment, audioInput string) error {
	plan := hlsResumePlan{
		Version: hlsResumePlanVersion, Segments: resumeSegments(segments),
		AudioSegments: resumeSegments(audioSegments), AudioInput: audioInput,
	}
	data, err := json.Marshal(plan)
	if err != nil {
		return err
	}
	path := hlsResumePlanPath(a.workDir)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	return replaceFile(temporary, path)
}

func loadHLSResumePlan(a *app) (hlsResumePlan, bool) {
	var plan hlsResumePlan
	data, err := os.ReadFile(hlsResumePlanPath(a.workDir))
	if err != nil || json.Unmarshal(data, &plan) != nil || plan.Version != hlsResumePlanVersion || len(plan.Segments) == 0 {
		return hlsResumePlan{}, false
	}
	if _, err := os.Stat(a.localPlaylist); err != nil {
		return hlsResumePlan{}, false
	}
	if len(plan.AudioSegments) > 0 {
		if _, err := os.Stat(filepath.Join(a.workDir, "audio", "local.m3u8")); err != nil {
			return hlsResumePlan{}, false
		}
	}
	return plan, true
}

func validateHLSResumePlan(plan hlsResumePlan) error {
	if plan.Version != hlsResumePlanVersion || len(plan.Segments) == 0 {
		return errors.New("invalid HLS resume plan")
	}
	return nil
}

func (a *app) resumeHLS(plan hlsResumePlan) error {
	if err := validateHLSResumePlan(plan); err != nil {
		return err
	}
	segments := restoreSegments(plan.Segments, a.segmentDir)
	// A paused worker loses its in-memory cookie jar and transport choice. Touch
	// the original playlist before using the stable checkpoint so that rotated
	// cookies are observed again and a Cloudflare/CDN 403 can switch this run to
	// the existing curl compatibility transport. The checkpoint still decides
	// which segment URLs and files are resumed.
	publishJobPhaseProgress("resolving", a.segmentDownloadProgress(segmentCompletionPercent(segments)), "正在恢复媒体会话", segmentDownloadedBytes(segments), segmentTotalBytes(segments), 0)
	if _, err := a.fetchPlaylist(a.opts.sourceURL, "resume-session.m3u8"); err != nil {
		return keepWorkError(a.workDir, fmt.Errorf("恢复媒体会话失败：%w", err))
	}
	audioSegments := restoreSegments(plan.AudioSegments, filepath.Join(a.workDir, "audio", "segments"))
	var audioApp *app
	if len(audioSegments) > 0 {
		preparedAudioApp := *a
		preparedAudioApp.workDir = filepath.Join(a.workDir, "audio")
		preparedAudioApp.segmentDir = filepath.Join(preparedAudioApp.workDir, "segments")
		preparedAudioApp.localPlaylist = filepath.Join(preparedAudioApp.workDir, "local.m3u8")
		audioApp = &preparedAudioApp
		totalSegments := len(segments) + len(audioSegments)
		videoSpan := len(segments) * 100 / totalSegments
		a.segmentProgressSpan = videoSpan
		audioApp.segmentProgressStart = videoSpan
		audioApp.segmentProgressSpan = 100 - videoSpan
	}
	publishJobPhaseProgress("downloading", a.segmentDownloadProgress(segmentCompletionPercent(segments)), "正在恢复已有媒体分片", segmentDownloadedBytes(segments), segmentTotalBytes(segments), 0)
	if err := a.downloadSegments(segments); err != nil {
		return keepWorkError(a.workDir, err)
	}
	if audioApp != nil {
		if err := audioApp.downloadSegments(audioSegments); err != nil {
			return keepWorkError(a.workDir, err)
		}
	}
	outputPath, err := resumableOutputPath(a.workDir, a.opts.outputDir, a.opts.title)
	if err != nil {
		return keepWorkError(a.workDir, err)
	}
	publishJobPhaseProgress("processing", 0, "正在封装 MP4", 0, 0, 0)
	if err := a.mux(outputPath, plan.AudioInput); err != nil {
		return keepWorkError(a.workDir, err)
	}
	publishJobOutput(outputPath)
	if !a.opts.keepWork {
		if err := os.RemoveAll(a.workDir); err != nil {
			fmt.Fprintln(consoleOut, "警告：无法清理工作目录：", err)
		}
	}
	fmt.Fprintln(consoleOut, "完成：", outputPath)
	return nil
}
