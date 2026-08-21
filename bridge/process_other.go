//go:build !windows

package downkit

import (
	"os/exec"
)

func newBridgeCommand() (*exec.Cmd, error) {
	path, err := currentExecutable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(path, "--bridge")
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	return command, nil
}

func newWorkerCommand(token string) (*exec.Cmd, error) {
	path, err := currentExecutable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(path, "--worker", token)
	command.Stdin, command.Stdout, command.Stderr = nil, nil, nil
	return command, nil
}
