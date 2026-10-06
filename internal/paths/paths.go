// Package paths finds the app's directories and the address of the core's
// private socket.
package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Dirs are the app's directories.
type Dirs struct {
	Data     string // settings, secrets, profiles
	Profiles string
	Core     string // the sidecar core's home: caches, GeoIP data, providers
	Logs     string
	Backups  string
	Sync     string // the sync state
	Cache    string
}

// New lays the directories out under data, logs and cache, and creates
// them, readable only by the user.
func New(data, logs, cache string) (Dirs, error) {
	d := Dirs{
		Data:     data,
		Profiles: filepath.Join(data, "profiles"),
		Core:     filepath.Join(data, "core"),
		Logs:     logs,
		Backups:  filepath.Join(data, "backups"),
		Sync:     filepath.Join(data, "sync"),
		Cache:    cache,
	}
	for _, dir := range []string{d.Data, d.Profiles, d.Core, d.Logs, d.Backups, d.Sync, d.Cache} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return d, err
		}
		_ = os.Chmod(dir, 0o700)
	}
	return d, nil
}

// Slug turns the app's name into a name for files: "MyGO-Clash Dev" →
// "mygo-clash-dev".
func Slug(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case b.Len() > 0 && !strings.HasSuffix(b.String(), "-"):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

// maxSocketPath keeps socket paths within sun_path (104 bytes on macOS,
// 108 on Linux, with room for the file name).
const maxSocketPath = 100

// CoreSocket returns where the core of the app serves its API: a Unix
// socket in a directory only the user can enter, or a named pipe on
// Windows. user identifies the user on Windows (their SID).
func CoreSocket(slug, user string) (string, error) {
	if runtime.GOOS == "windows" {
		sum := sha256.Sum256([]byte(user))
		return `\\.\pipe\` + slug + "-" + hex.EncodeToString(sum[:6]), nil
	}
	dir, err := runtimeDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "core.sock"), nil
}

// runtimeDir returns a private directory for sockets: $XDG_RUNTIME_DIR on
// Linux, $TMPDIR on macOS (both per user), or a directory of /tmp that the
// user owns and only they can enter.
func runtimeDir(slug string) (string, error) {
	var candidates []string
	if x := os.Getenv("XDG_RUNTIME_DIR"); x != "" && runtime.GOOS == "linux" {
		candidates = append(candidates, filepath.Join(x, slug))
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, filepath.Join(os.TempDir(), slug))
	}
	candidates = append(candidates, filepath.Join("/tmp", fmt.Sprintf("%s-%d", slug, os.Getuid())))
	var lastErr error
	for _, dir := range candidates {
		if len(dir)+len("/core.sock") > maxSocketPath {
			lastErr = fmt.Errorf("%s is too long for a socket", dir)
			continue
		}
		if err := ensurePrivateDir(dir); err != nil {
			lastErr = err
			continue
		}
		return dir, nil
	}
	return "", lastErr
}

// ensurePrivateDir creates dir, or checks that the one there belongs to the
// user and that nobody else can enter it.
func ensurePrivateDir(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !os.IsExist(err) {
		return err
	}
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", dir)
	}
	if !ownedByMe(fi) {
		return fmt.Errorf("%s belongs to another user", dir)
	}
	if fi.Mode().Perm() != 0o700 {
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
