package downkit

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

func appendRuntimeLog(component, event string) {
	cfg := loadBridgeConfig()
	if !cfg.LogEnabled {
		return
	}
	path, err := bridgeDataPath(filepath.Join("logs", component+".log"))
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return
	}
	// Keep logs bounded without introducing a second configuration backend.
	if info, err := os.Stat(path); err == nil && info.Size() >= int64(cfg.LogMaxMB)*1024*1024 {
		_ = os.Rename(path, path+".1")
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	defer file.Close()
	_, _ = fmt.Fprintf(file, "%s component=%s event=%s\n", time.Now().Format(time.RFC3339), component, event)
}
