package downkit

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var runtimeLogMu sync.Mutex
var runtimeLogAtom = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var runtimeLogKeys = map[string]bool{
	"jobId": true, "phase": true, "progress": true, "fileCount": true,
	"subtitleCount": true, "durationMs": true, "reason": true,
	"mode": true, "count": true, "operation": true,
}

// The installation root is the parent of bin/. During development, if the
// executable is not under bin/, it is the executable's own directory.
func dataRootForExecutable(executable string) string {
	executableDir := filepath.Dir(executable)
	if strings.EqualFold(filepath.Base(executableDir), "bin") {
		return filepath.Join(filepath.Dir(executableDir), "Data")
	}
	return filepath.Join(executableDir, "Data")
}

func bridgeDataPath(name string) (string, error) {
	exe, err := currentExecutable()
	if err != nil {
		return "", err
	}
	return filepath.Join(dataRootForExecutable(exe), name), nil
}

// appendRuntimeLog preserves old lifecycle call sites with a single INFO event.
func appendRuntimeLog(component, event string) {
	level := "INFO"
	if event == "failed" || event == "error" {
		level = "ERROR"
	}
	appendRuntimeEvent(level, component, event)
}

// Only fixed-key atomic values may enter persistent logs. Browser URLs,
// cookies, tokens, titles, paths, command output and raw error text must never
// pass through this sink. The worker-generated jobId is only 12 hex digits.
func runtimeLogLine(level, component, event string, fields ...string) string {
	if level != "INFO" && level != "WARN" && level != "ERROR" && level != "DEBUG" {
		return ""
	}
	if !runtimeLogAtom.MatchString(component) || !runtimeLogAtom.MatchString(event) || len(fields)%2 != 0 {
		return ""
	}
	var details strings.Builder
	for i := 0; i < len(fields); i += 2 {
		key, value := fields[i], fields[i+1]
		if !runtimeLogKeys[key] || !runtimeLogAtom.MatchString(value) {
			return ""
		}
		if key == "jobId" && (len(value) != 12 || !isHexJobID(value)) {
			return ""
		}
		if key == "progress" {
			number, err := strconv.Atoi(value)
			if err != nil || number < 0 || number > 100 {
				return ""
			}
		}
		details.WriteString(" ")
		details.WriteString(key)
		details.WriteString("=")
		details.WriteString(value)
	}
	return fmt.Sprintf("%s level=%s component=%s event=%s%s\n",
		time.Now().Format(time.RFC3339), level, component, event, details.String())
}

func isHexJobID(value string) bool {
	for _, c := range value {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func appendRuntimeEvent(level, component, event string, fields ...string) {
	line := runtimeLogLine(level, component, event, fields...)
	if line == "" {
		return
	}
	cfg := loadBridgeConfig()
	if !cfg.LogEnabled {
		return
	}
	path, err := bridgeDataPath(filepath.Join("logs", component+".log"))
	if err != nil {
		return
	}
	runtimeLogMu.Lock()
	defer runtimeLogMu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	// Rotate before writing, under one lock to avoid concurrent lost lines.
	if info, err := os.Stat(path); err == nil && info.Size()+int64(len(line)) > int64(cfg.LogMaxMB)*1024*1024 {
		_ = os.Remove(path + ".1")
		_ = os.Rename(path, path+".1")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = file.WriteString(line)
}
