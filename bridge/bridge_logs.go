package downkit

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxLogEntries  = 1000
	maxLogFileTail = 512 * 1024
)

type runtimeLogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Raw     string `json:"raw"`
}

// Only known runtime components are readable/clearable. The API never
// accepts a filesystem path, to avoid exposing downloaded media or tokens.
var logFiles = []string{"bridge.log.1", "bridge.log", "worker.log.1", "worker.log"}

func readRuntimeLogFile(path string) ([]runtimeLogEntry, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("log file is not regular: %s", filepath.Base(path))
	}
	if stat.Size() > maxLogFileTail {
		if _, err := file.Seek(stat.Size()-maxLogFileTail, io.SeekStart); err != nil {
			return nil, err
		}
	}
	reader := bufio.NewReader(file)
	if stat.Size() > maxLogFileTail {
		// Drop the incomplete first entry without discarding buffered bytes.
		if _, err := reader.ReadString('\n'); err != nil && !errors.Is(err, io.EOF) {
			return nil, err
		}
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 64*1024)
	var entries []runtimeLogEntry
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.SplitN(line, " ", 2)
		if len(fields) < 2 {
			continue
		}
		stamp, err := time.Parse(time.RFC3339, fields[0])
		if err != nil {
			continue
		}
		message := fields[1]
		level := "INFO"
		if strings.Contains(message, "event=failed") || strings.Contains(message, "event=error") {
			level = "ERROR"
		} else if strings.Contains(message, "event=warn") {
			level = "WARN"
		}
		entries = append(entries, runtimeLogEntry{
			Time:    stamp.Format(time.RFC3339),
			Level:   level,
			Message: message,
			Raw:     "[" + stamp.Format(time.RFC3339) + "] " + level + " " + message,
		})
	}
	return entries, scanner.Err()
}

func readRuntimeLogs(directory string, limit int) ([]runtimeLogEntry, error) {
	if limit <= 0 || limit > maxLogEntries {
		limit = maxLogEntries
	}
	var entries []runtimeLogEntry
	for _, name := range logFiles {
		items, err := readRuntimeLogFile(filepath.Join(directory, name))
		if err != nil {
			return nil, err
		}
		entries = append(entries, items...)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Time < entries[j].Time })
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}
	if entries == nil {
		entries = []runtimeLogEntry{}
	}
	return entries, nil
}

func clearRuntimeLogs(directory string) error {
	for _, name := range logFiles {
		path := filepath.Join(directory, name)
		err := os.Truncate(path, 0)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (s *bridgeServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	s.allowExtension(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !s.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "unauthorized"})
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	directory, err := bridgeDataPath("logs")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "logs unavailable"})
		return
	}
	if r.Method == http.MethodDelete {
		if err := clearRuntimeLogs(directory); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "cannot clear logs"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	entries, err := readRuntimeLogs(directory, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "cannot read logs"})
		return
	}
	s.mu.Lock()
	enabled := s.config.LogEnabled
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true, "entries": entries, "path": directory, "level": "info", "enabled": enabled,
	})
}
