//go:build !windows

package downkit

import "os/exec"

func stopProcess(command *exec.Cmd) error {
	if command == nil || command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}
