//go:build !windows

package app

// fullScreenApp: a floating window keeps out of full-screen apps by
// itself elsewhere.
func fullScreenApp() bool { return false }
