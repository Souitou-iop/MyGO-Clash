//go:build !windows

package corehost

import "os"

func isPrivileged() bool { return os.Geteuid() == 0 }
