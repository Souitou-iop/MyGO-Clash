package app

import (
	"syscall"
	"time"
)

var procGetDoubleClickTime = syscall.NewLazyDLL("user32.dll").NewProc("GetDoubleClickTime")

// doubleClickTime is how far apart two clicks of a double click may be,
// as the user set it.
func doubleClickTime() time.Duration {
	if ms, _, _ := procGetDoubleClickTime.Call(); ms > 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return 500 * time.Millisecond
}
