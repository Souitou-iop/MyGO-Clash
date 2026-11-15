//go:build !darwin && !linux && !windows

package app

import "errors"

func ifCounters() (map[string]ifBytes, error) { return nil, errors.New("no interface counters") }
