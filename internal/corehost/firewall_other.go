//go:build !linux

package corehost

// tunTrust is for firewalld, on Linux.
type tunTrust struct{}

func (*tunTrust) sync()    {}
func (*tunTrust) release() {}
