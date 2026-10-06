package app

import (
	"os/exec"
	"syscall"
)

// UWPLoopback exempts every Store app from the loopback restriction, so
// that they can reach the proxy. It runs as an administrator.
func UWPLoopback() error {
	script := `Get-ChildItem 'HKCU:\Software\Classes\Local Settings\Software\Microsoft\Windows\CurrentVersion\AppContainer\Mappings' | ForEach-Object { CheckNetIsolation.exe LoopbackExempt -a "-p=$($_.PSChildName)" | Out-Null }`
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd.Run()
}
