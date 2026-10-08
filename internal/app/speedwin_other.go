//go:build !windows

package app

import "github.com/egoist/mygo"

// fullScreenApp: a floating window keeps out of full-screen apps by
// itself elsewhere.
func fullScreenApp() bool { return false }

// toolWindow: SkipTaskbar does on Linux, and the Dock shows apps, not
// windows.
func toolWindow(*mygo.Window) {}

// keepOnTop: the window manager keeps a window that floats over the others
// there.
func keepOnTop(*mygo.Window) {}
