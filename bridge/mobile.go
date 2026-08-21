package downkit

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// PlatformMuxer is implemented by the Android/iOS host application. The Go
// engine downloads and decrypts media; the platform owns final MP4 muxing.
// Its simple string-only API is intentionally compatible with gomobile bind.
type PlatformMuxer interface {
	Mux(requestID, workDir, videoInput, audioInput, output string) error
}

// mobileTask is serialized as JSON across the browser/app and gomobile
// boundaries so adding optional fields does not break generated bindings.
type mobileTask struct {
	RequestID   string            `json:"requestId"`
	URL         string            `json:"url"`
	Title       string            `json:"title"`
	Referer     string            `json:"referer"`
	Origin      string            `json:"origin"`
	UserAgent   string            `json:"ua"`
	Proxy       string            `json:"proxy"`
	OutputDir   string            `json:"outputDir"`
	Quality     string            `json:"quality"`
	Concurrent  int               `json:"concurrent"`
	KeepWork    bool              `json:"keepWork"`
	ResolvePage bool              `json:"resolvePage"`
	Headers     map[string]string `json:"headers"`
	Cookies     []browserCookie   `json:"cookies"`
}

type platformMuxerAdapter struct {
	host      PlatformMuxer
	requestID string
}

func (m platformMuxerAdapter) Mux(request muxRequest) error {
	if m.host == nil {
		return errors.New("移动端没有提供媒体封装器")
	}
	return m.host.Mux(m.requestID, request.WorkDir, request.Video, request.Audio, request.Output)
}

// DownloadMobile runs one foreground mobile download. Android should call it
// from its foreground service and provide an implementation of PlatformMuxer.
func DownloadMobile(taskJSON string, muxer PlatformMuxer) (resultErr error) {
	startedAt := time.Now()
	requestID := "unknown"
	defer func() {
		status := "succeeded"
		errorType := "none"
		if resultErr != nil {
			status = "failed"
			errorType = fmt.Sprintf("%T", resultErr)
		}
		fmt.Fprintf(
			consoleOut,
			"requestId=%q node=%q operation=%q status=%q durationMs=%d errorType=%q\n",
			requestID,
			"mobile-core",
			"download",
			status,
			time.Since(startedAt).Milliseconds(),
			errorType,
		)
	}()

	var task mobileTask
	if err := json.Unmarshal([]byte(taskJSON), &task); err != nil {
		return fmt.Errorf("无效移动端任务：%w", err)
	}
	requestID = safeMobileRequestID(task.RequestID)
	fmt.Fprintf(consoleOut, "requestId=%q node=%q operation=%q status=%q\n", requestID, "mobile-core", "download", "start")
	if task.OutputDir == "" {
		return errors.New("移动端任务缺少 outputDir")
	}
	concurrent := task.Concurrent
	if concurrent <= 0 {
		concurrent = 8
	}
	opts := options{
		sourceURL:      task.URL,
		title:          task.Title,
		referer:        task.Referer,
		origin:         task.Origin,
		userAgent:      task.UserAgent,
		proxy:          task.Proxy,
		outputDir:      task.OutputDir,
		playlistMode:   "single",
		concurrent:     concurrent,
		keepWork:       task.KeepWork,
		resolvePage:    task.ResolvePage,
		requestHeaders: task.Headers,
		mediaCookies:   sanitizeBrowserCookies(task.Cookies),
	}
	if opts.userAgent == "" {
		opts.userAgent = defaultUA
	}
	if task.Quality == "" {
		task.Quality = "best"
	}
	if err := setQuality(&opts, task.Quality); err != nil {
		return err
	}
	var err error
	opts, err = normalizeOptions(opts)
	if err != nil {
		return err
	}
	return runWithOptions(opts, platformMuxerAdapter{host: muxer, requestID: requestID})
}

func safeMobileRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return "unknown"
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '-' && char != '_' {
			return "unknown"
		}
	}
	return value
}
