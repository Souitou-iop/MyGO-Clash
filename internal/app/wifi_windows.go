package app

import (
	"os/exec"
	"strings"
)

// wifiSSID returns the name of the Wi-Fi network the computer is on, or ""
// when it is on none, is on a wire, or the system cannot tell.
func wifiSSID() (string, error) {
	out, err := exec.Command("netsh", "wlan", "show", "interfaces").Output()
	if err != nil {
		return "", nil // no wlan service: nothing to act on
	}
	return parseNetshInterfaces(string(out)), nil
}
