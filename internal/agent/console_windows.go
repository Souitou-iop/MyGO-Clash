//go:build windows

package agent

import (
	"os"
	"syscall"
)

// attachConsole gives the command line the console of the shell that ran
// it: the app is built for the GUI subsystem, which starts without one.
// Redirected output (a pipe, as MCP clients use, or a file) is kept.
func attachConsole() {
	if h, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE); err == nil && h != 0 && h != syscall.InvalidHandle {
		if _, err := syscall.GetFileType(h); err == nil {
			return
		}
	}
	const attachParentProcess = ^uint32(0) // ATTACH_PARENT_PROCESS
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")
	if r, _, _ := proc.Call(uintptr(attachParentProcess)); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
}
