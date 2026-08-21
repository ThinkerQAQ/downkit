//go:build windows

package downkit

import (
	"io"
	"os"
	"sync"
	"syscall"
	"unsafe"
)

const utf8CodePage = 65001

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	procSetConsoleCP       = kernel32.NewProc("SetConsoleCP")
	procGetConsoleMode     = kernel32.NewProc("GetConsoleMode")
	procWriteConsoleW      = kernel32.NewProc("WriteConsoleW")
	procGetConsoleFont     = kernel32.NewProc("GetCurrentConsoleFontEx")
	procSetConsoleFont     = kernel32.NewProc("SetCurrentConsoleFontEx")
)

type coord struct {
	x int16
	y int16
}

type consoleFontInfoEx struct {
	size       uint32
	font       uint32
	fontSize   coord
	fontFamily uint32
	fontWeight uint32
	faceName   [32]uint16
}

type windowsConsoleWriter struct {
	handle   syscall.Handle
	fallback io.Writer
	mu       sync.Mutex
}

func setupConsoleUTF8() {
	procSetConsoleOutputCP.Call(utf8CodePage)
	procSetConsoleCP.Call(utf8CodePage)
	setConsoleFont(syscall.Stdout, "NSimSun")
}

func consoleWriters() (io.Writer, io.Writer) {
	return newWindowsConsoleWriter(syscall.Stdout, os.Stdout), newWindowsConsoleWriter(syscall.Stderr, os.Stderr)
}

func newWindowsConsoleWriter(handle syscall.Handle, fallback io.Writer) io.Writer {
	var mode uint32
	result, _, _ := procGetConsoleMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if result == 0 {
		return fallback
	}
	return &windowsConsoleWriter{handle: handle, fallback: fallback}
}

func (w *windowsConsoleWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	text, err := syscall.UTF16FromString(string(p))
	if err != nil {
		return w.fallback.Write(p)
	}
	if len(text) <= 1 {
		return len(p), nil
	}
	var written uint32
	result, _, callErr := procWriteConsoleW.Call(
		uintptr(w.handle),
		uintptr(unsafe.Pointer(&text[0])),
		uintptr(len(text)-1),
		uintptr(unsafe.Pointer(&written)),
		0,
	)
	if result == 0 {
		if callErr != syscall.Errno(0) {
			return 0, callErr
		}
		return w.fallback.Write(p)
	}
	return len(p), nil
}

func setConsoleFont(handle syscall.Handle, face string) {
	var info consoleFontInfoEx
	info.size = uint32(unsafe.Sizeof(info))
	result, _, _ := procGetConsoleFont.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&info)))
	if result == 0 {
		return
	}
	name, err := syscall.UTF16FromString(face)
	if err != nil {
		return
	}
	for i := range info.faceName {
		info.faceName[i] = 0
	}
	copy(info.faceName[:], name)
	procSetConsoleFont.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&info)))
}
