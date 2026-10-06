//go:build !darwin && !windows && !linux

package secure

// DefaultKeyring returns the file keyring on systems without one.
func DefaultKeyring(service, dataDir string) Keyring { return FileKeyring(dataDir) }
