//go:build linux

package downkit

import (
	"os/exec"
	"path/filepath"
)

func platformOpenDirectoryCommand(path string) *exec.Cmd {
	return exec.Command("xdg-open", path)
}

func platformOpenFile(path string) error {
	return exec.Command("xdg-open", path).Start()
}

func platformRevealFile(path string) error {
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}
