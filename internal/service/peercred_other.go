//go:build !darwin && !linux && !windows

package service

import "errors"

func peerUID(int) (int, error) { return -1, errors.New("unsupported") }
