//go:build !windows

package agent

import (
	"os/exec"
	"syscall"
)

// detach starts the app in a session of its own, so that it outlives the
// command (and the MCP client that started it).
func detach(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
