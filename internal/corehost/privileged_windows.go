//go:build windows

package corehost

import "golang.org/x/sys/windows"

func isPrivileged() bool { return windows.GetCurrentProcessToken().IsElevated() }
