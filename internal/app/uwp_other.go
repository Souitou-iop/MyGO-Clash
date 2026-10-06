//go:build !windows

package app

import "errors"

// UWPLoopback is a Windows matter.
func UWPLoopback() error { return errors.New("only Windows has Store apps") }
