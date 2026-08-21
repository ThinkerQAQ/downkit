//go:build darwin

package downkit

import "os/exec"

func platformOpenDirectoryCommand(path string) *exec.Cmd {
	return exec.Command("open", path)
}

func platformOpenFile(path string) error {
	return exec.Command("open", path).Start()
}

func platformRevealFile(path string) error {
	return exec.Command("open", "-R", path).Start()
}
