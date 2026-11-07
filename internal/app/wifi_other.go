//go:build !darwin && !windows

package app

import (
	"os/exec"
)

// wifiSSID returns the name of the Wi-Fi network the computer is on, or ""
// when it is on none, is on a wire, or NetworkManager cannot tell.
func wifiSSID() (string, error) {
	if _, err := exec.LookPath("nmcli"); err != nil {
		return "", nil // no NetworkManager: nothing to act on
	}
	out, err := exec.Command("nmcli", "-t", "-f", "active,ssid", "dev", "wifi", "list").Output()
	if err != nil {
		return "", nil
	}
	return parseNmcliWiFi(string(out)), nil
}
