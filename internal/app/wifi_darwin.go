package app

import (
	"os/exec"
)

// wifiSSID returns the name of the Wi-Fi network the computer is on, or ""
// when it is on none, is on a wire, or the system cannot tell.
func wifiSSID() (string, error) {
	out, err := exec.Command("networksetup", "-listallhardwareports").Output()
	if err != nil {
		return "", err
	}
	dev := wifiDeviceDarwin(string(out))
	if dev == "" {
		return "", nil // no Wi-Fi hardware
	}
	out, err = exec.Command("networksetup", "-getairportnetwork", dev).Output()
	if err != nil {
		return "", nil // not associated, or the tool refused
	}
	return parseAirportNetwork(string(out)), nil
}
