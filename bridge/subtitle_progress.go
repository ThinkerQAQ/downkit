package downkit

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ffmpegDurationPattern  = regexp.MustCompile(`Duration:\s*(\d+):(\d{2}):(\d{2}(?:\.\d+)?)`)
	whisperProgressPattern = regexp.MustCompile(`(?i)progress\s*=\s*(\d{1,3})%`)
)

// progressLineWriter turns the line-oriented status output from sidecar tools
// into callbacks without assuming that a Write call contains a complete line.
type progressLineWriter struct {
	mu      sync.Mutex
	pending string
	onLine  func(string)
}

func newProgressLineWriter(onLine func(string)) *progressLineWriter {
	return &progressLineWriter{onLine: onLine}
}

func (w *progressLineWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	w.pending += strings.ReplaceAll(string(data), "\r", "\n")
	parts := strings.Split(w.pending, "\n")
	w.pending = parts[len(parts)-1]
	lines := append([]string(nil), parts[:len(parts)-1]...)
	w.mu.Unlock()
	for _, line := range lines {
		if w.onLine != nil {
			w.onLine(strings.TrimSpace(line))
		}
	}
	return len(data), nil
}

type ffmpegAudioProgressReporter struct {
	mu           sync.Mutex
	duration     time.Duration
	position     time.Duration
	speed        string
	lastProgress int
	report       func(int, string)
}

func newFFmpegAudioProgressReporter(report func(int, string)) *ffmpegAudioProgressReporter {
	return &ffmpegAudioProgressReporter{lastProgress: -1, report: report}
}

func (r *ffmpegAudioProgressReporter) logLine(line string) {
	match := ffmpegDurationPattern.FindStringSubmatch(line)
	if len(match) != 4 {
		return
	}
	duration, ok := parseClockDuration(match[1], match[2], match[3])
	if !ok {
		return
	}
	r.mu.Lock()
	r.duration = duration
	progress, detail, changed := r.snapshotLocked(false)
	r.mu.Unlock()
	if changed {
		r.report(progress, detail)
	}
}

func (r *ffmpegAudioProgressReporter) progressLine(line string) {
	key, value, ok := strings.Cut(line, "=")
	if !ok {
		return
	}
	r.mu.Lock()
	shouldReport := false
	force := false
	switch strings.TrimSpace(key) {
	case "out_time_us":
		if microseconds, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64); err == nil && microseconds >= 0 {
			r.position = time.Duration(microseconds) * time.Microsecond
		}
	case "out_time":
		if match := ffmpegDurationPattern.FindStringSubmatch("Duration: " + strings.TrimSpace(value)); len(match) == 4 {
			if position, valid := parseClockDuration(match[1], match[2], match[3]); valid {
				r.position = position
			}
		}
	case "speed":
		r.speed = strings.TrimSpace(value)
	case "progress":
		shouldReport = true
		if strings.TrimSpace(value) == "end" {
			r.position = r.duration
			force = true
		}
	}
	if !shouldReport {
		r.mu.Unlock()
		return
	}
	progress, detail, changed := r.snapshotLocked(force)
	r.mu.Unlock()
	if changed {
		r.report(progress, detail)
	}
}

func (r *ffmpegAudioProgressReporter) snapshotLocked(force bool) (int, string, bool) {
	progress := 0
	if r.duration > 0 {
		progress = min(100, max(0, int(r.position*100/r.duration)))
	}
	changed := force || progress > r.lastProgress
	if changed {
		r.lastProgress = progress
	}
	detail := "正在提取 16 kHz 音频"
	if r.duration > 0 {
		detail = fmt.Sprintf("已处理 %s / %s", formatMediaDuration(r.position), formatMediaDuration(r.duration))
	} else if r.position > 0 {
		detail = fmt.Sprintf("已处理 %s（媒体总时长未知）", formatMediaDuration(r.position))
	}
	if r.speed != "" && r.speed != "N/A" {
		detail += " · " + r.speed
	}
	return progress, detail, changed
}

func parseClockDuration(hours, minutes, seconds string) (time.Duration, bool) {
	hour, hourErr := strconv.Atoi(hours)
	minute, minuteErr := strconv.Atoi(minutes)
	second, secondErr := strconv.ParseFloat(seconds, 64)
	if hourErr != nil || minuteErr != nil || secondErr != nil || minute < 0 || minute >= 60 || second < 0 || second >= 60 {
		return 0, false
	}
	return time.Duration(float64(time.Hour)*float64(hour) + float64(time.Minute)*float64(minute) + float64(time.Second)*second), true
}

func formatMediaDuration(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	totalSeconds := int64(value.Round(time.Second) / time.Second)
	hours := totalSeconds / 3600
	minutes := totalSeconds % 3600 / 60
	seconds := totalSeconds % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

type whisperProgressReporter struct {
	mu           sync.Mutex
	lastProgress int
	report       func(int, string)
}

func newWhisperProgressReporter(report func(int, string)) *whisperProgressReporter {
	return &whisperProgressReporter{lastProgress: -1, report: report}
}

func (r *whisperProgressReporter) line(line string) {
	match := whisperProgressPattern.FindStringSubmatch(line)
	if len(match) != 2 {
		return
	}
	progress, err := strconv.Atoi(match[1])
	if err != nil {
		return
	}
	progress = min(100, max(0, progress))
	r.mu.Lock()
	if progress <= r.lastProgress {
		r.mu.Unlock()
		return
	}
	r.lastProgress = progress
	r.mu.Unlock()
	r.report(progress, fmt.Sprintf("Whisper 已处理 %d%% 音频", progress))
}
