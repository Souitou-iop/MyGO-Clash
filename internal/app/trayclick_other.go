//go:build !windows

package app

import "time"

// doubleClickTime is unused off Windows: a click on the menu bar icon
// opens its menu, which takes the second click.
func doubleClickTime() time.Duration { return 0 }
