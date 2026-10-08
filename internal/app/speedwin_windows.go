package app

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procQueryNotificationState = windows.NewLazySystemDLL("shell32.dll").NewProc("SHQueryUserNotificationState")

// fullScreenApp reports whether a game, a video or a presentation runs
// full screen, as Windows tells notifications to keep out of the way.
func fullScreenApp() bool {
	var state uint32
	if procQueryNotificationState.Find() != nil {
		return false
	}
	if r, _, _ := procQueryNotificationState.Call(uintptr(unsafe.Pointer(&state))); r != 0 {
		return false
	}
	// QUNS_BUSY, QUNS_RUNNING_D3D_FULL_SCREEN, QUNS_PRESENTATION_MODE
	return state == 2 || state == 3 || state == 4
}
