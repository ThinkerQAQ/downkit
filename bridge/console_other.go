//go:build !windows

package downkit

import (
	"io"
	"os"
)

func setupConsoleUTF8() {}

func consoleWriters() (io.Writer, io.Writer) {
	return os.Stdout, os.Stderr
}
