package app

import (
	"unsafe"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"
)

var (
	procQueryNotificationState = windows.NewLazySystemDLL("shell32.dll").NewProc("SHQueryUserNotificationState")
	user32                     = windows.NewLazySystemDLL("user32.dll")
	procGetWindowLongPtr       = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr       = user32.NewProc("SetWindowLongPtrW")
	procSetWindowPos           = user32.NewProc("SetWindowPos")
)

const (
	gwlExStyle      = ^uintptr(19) // -20
	wsExToolWindow  = 0x00000080
	wsExAppWindow   = 0x00040000
	hwndTopmost     = ^uintptr(0) // -1
	swpNoSize       = 0x0001
	swpNoMove       = 0x0002
	swpNoZOrder     = 0x0004
	swpNoActivate   = 0x0010
	swpFrameChanged = 0x0020
)

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

// toolWindow makes a hidden window a tool window, which the taskbar and
// Alt+Tab leave out for good: SkipTaskbar only removes the button the
// taskbar made, and it comes back each time the window is shown again.
func toolWindow(win *mygo.Window) {
	hwnd := win.NativeHandle()
	if hwnd == 0 {
		return
	}
	ex, _, _ := procGetWindowLongPtr.Call(hwnd, gwlExStyle)
	procSetWindowLongPtr.Call(hwnd, gwlExStyle, ex&^wsExAppWindow|wsExToolWindow)
	procSetWindowPos.Call(hwnd, 0, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
}

// keepOnTop puts the window back over the other topmost ones, the taskbar
// above all, which comes to the front of them when it is clicked.
func keepOnTop(win *mygo.Window) {
	if hwnd := win.NativeHandle(); hwnd != 0 {
		procSetWindowPos.Call(hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
	}
}
