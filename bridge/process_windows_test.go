//go:build windows

package downkit

import "testing"

func TestWorkerCommandDoesNotCreateConsole(t *testing.T) {
	command, err := newWorkerCommand("test-token")
	if err != nil {
		t.Fatal(err)
	}
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow {
		t.Fatal("worker command must hide its window")
	}
	flags := command.SysProcAttr.CreationFlags
	if flags&createNoWindow == 0 || flags&createNewConsole != 0 {
		t.Fatalf("unexpected worker creation flags: %#x", flags)
	}
}
