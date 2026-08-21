//go:build windows

package downkit

import (
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup = 0x00000200
	createNewConsole      = 0x00000010
	createNoWindow        = 0x08000000
)

func newBridgeCommand() (*exec.Cmd, error) {
	path, err := currentExecutable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(path, "--bridge")
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | createNoWindow, HideWindow: true}
	return command, nil
}

func newWorkerCommand(token string) (*exec.Cmd, error) {
	path, err := currentExecutable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(path, "--worker", token)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewProcessGroup | createNoWindow, HideWindow: true}
	return command, nil
}
