//go:build windows

package app

import (
	"os/exec"
	"syscall"
)

// hideConsole keeps the player, a console program, from opening a console.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
