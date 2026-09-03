//go:build windows

package downkit

import (
	"os/exec"
	"syscall"
)

func configureSidecarCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | createNoWindow, HideWindow: true}
}
