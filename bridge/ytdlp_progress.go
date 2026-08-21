package downkit

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

const (
	ytDLPProgressPrefix    = "downkit-progress:"
	ytDLPPostprocessPrefix = "downkit-postprocess:"
	ytDLPOutputPrefix      = "downkit-output:"
)

// ytDLPProgressWriter keeps yt-dlp's ordinary console output visible while
// turning the machine-readable progress lines into bridge job updates.
type ytDLPProgressWriter struct {
	mu      sync.Mutex
	dest    io.Writer
	pending []byte
}

func newYTDLPProgressWriter(dest io.Writer) *ytDLPProgressWriter {
	return &ytDLPProgressWriter{dest: dest}
}

func (w *ytDLPProgressWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = append(w.pending, data...)
	for {
		newline := bytes.IndexByte(w.pending, '\n')
		if newline < 0 {
			break
		}
		line := strings.TrimSuffix(string(w.pending[:newline]), "\r")
		w.pending = w.pending[newline+1:]
		if err := w.writeLine(line, true); err != nil {
			return 0, err
		}
	}
	return len(data), nil
}

func (w *ytDLPProgressWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.pending) == 0 {
		return
	}
	_ = w.writeLine(strings.TrimSuffix(string(w.pending), "\r"), false)
	w.pending = nil
}

func (w *ytDLPProgressWriter) writeLine(line string, newline bool) error {
	if progress, ok := parseYTDLPProgress(line); ok {
		publishJobProgress(progress.percent, "yt-dlp 下载中", progress.downloaded, progress.total, progress.speed)
		return nil
	}
	if strings.HasPrefix(strings.TrimSpace(line), ytDLPPostprocessPrefix) {
		publishJobPhaseProgress("processing", 0, "正在合并媒体", 0, 0, 0)
		return nil
	}
	if output, ok := parseYTDLPOutput(line); ok {
		publishJobOutputFile(output.path, output.index, output.id)
		if newline {
			_, err := fmt.Fprintln(w.dest, "完成："+output.path)
			return err
		}
		_, err := fmt.Fprint(w.dest, "完成："+output.path)
		return err
	}
	if newline {
		_, err := fmt.Fprintln(w.dest, line)
		return err
	}
	_, err := fmt.Fprint(w.dest, line)
	return err
}

type ytDLPOutput struct {
	index int
	id    string
	path  string
}

func parseYTDLPOutput(line string) (ytDLPOutput, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, ytDLPOutputPrefix) {
		return ytDLPOutput{}, false
	}
	fields := strings.SplitN(strings.TrimPrefix(line, ytDLPOutputPrefix), "|", 3)
	if len(fields) != 3 || strings.TrimSpace(fields[2]) == "" {
		return ytDLPOutput{}, false
	}
	index, _ := strconv.Atoi(strings.TrimSpace(fields[0]))
	return ytDLPOutput{index: max(index, 0), id: strings.TrimSpace(fields[1]), path: strings.TrimSpace(fields[2])}, true
}

type ytDLPProgress struct {
	percent    int
	downloaded int64
	total      int64
	speed      int64
}

func parseYTDLPProgress(line string) (ytDLPProgress, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, ytDLPProgressPrefix) {
		return ytDLPProgress{}, false
	}
	fields := strings.Split(strings.TrimPrefix(line, ytDLPProgressPrefix), "|")
	if len(fields) != 5 {
		return ytDLPProgress{}, false
	}
	downloaded := parseYTDLPNumber(fields[0])
	total := parseYTDLPNumber(fields[1])
	if total <= 0 {
		total = parseYTDLPNumber(fields[2])
	}
	speed := parseYTDLPNumber(fields[3])
	percentValue, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(fields[4], "%")), 64)
	if total > 0 && downloaded >= 0 {
		percentValue = float64(downloaded) * 100 / float64(total)
	}
	percentValue = min(max(percentValue, 0), 100)
	return ytDLPProgress{
		percent:    int(percentValue),
		downloaded: max(downloaded, 0),
		total:      max(total, 0),
		speed:      max(speed, 0),
	}, true
}

func parseYTDLPNumber(value string) int64 {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || number <= 0 {
		return 0
	}
	return int64(number)
}
