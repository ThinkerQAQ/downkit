//go:build !windows

package downkit

import "os/exec"

func configureSidecarCommand(command *exec.Cmd) {
	command.Stdin = nil
}
