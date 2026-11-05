package app

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
)

var (
	machineOnce sync.Once
	machineHash string
)

// machineID identifies the computer for sync, hashed so that the OS's own
// identifier never leaves it: an app installed again on the same computer
// takes the place of its old self instead of adding a device. "" when the
// OS gives none.
func machineID() string {
	machineOnce.Do(func() {
		id := strings.TrimSpace(rawMachineID())
		if id == "" {
			return
		}
		sum := sha256.Sum256([]byte("mygo-clash/machine/v1/" + strings.ToLower(id)))
		machineHash = hex.EncodeToString(sum[:12])
	})
	return machineHash
}
