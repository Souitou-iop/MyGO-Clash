//go:build !darwin && !windows && !linux

package sysproxy

import "errors"

// DefaultBypass is what skips the proxy.
var DefaultBypass = []string{"localhost", "127.0.0.1"}

var errUnsupported = errors.New("setting the system proxy is not supported here")

func setManual(Proxy) error { return errUnsupported }
func setPAC(string) error   { return errUnsupported }
func setDirect() error      { return errUnsupported }
func get() (Proxy, error)   { return Proxy{}, errUnsupported }
