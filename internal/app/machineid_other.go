//go:build !darwin && !windows

package app

import "os"

func rawMachineID() string {
	for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		if data, err := os.ReadFile(p); err == nil {
			return string(data)
		}
	}
	return ""
}
