//go:build !windows

package coremgr

import "os/exec"

func hideWindow(*exec.Cmd) {}
