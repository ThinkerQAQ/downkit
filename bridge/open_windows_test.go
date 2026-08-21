//go:build windows

package downkit

import (
	"os"
	"testing"
	"time"
)

func TestWindowsRevealFileCommandKeepsPathInSeparateArgument(t *testing.T) {
	path := `C:\Users\test\Downloads\带 空格，标点.mp4`
	command := windowsRevealFileCommand(path)
	if len(command.Args) != 3 || command.Args[1] != "/select," || command.Args[2] != path {
		t.Fatalf("unexpected explorer arguments: %#v", command.Args)
	}
}

func TestWindowsOpenActionsIntegration(t *testing.T) {
	path := os.Getenv("DOWNKIT_TEST_OPEN_FILE")
	if path == "" {
		t.Skip("set DOWNKIT_TEST_OPEN_FILE to run the shell integration test")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	if err := platformOpenFile(path); err != nil {
		t.Fatalf("open file: %v", err)
	}
	time.Sleep(time.Second)
	if err := platformRevealFile(path); err != nil {
		t.Fatalf("reveal file: %v", err)
	}
}
