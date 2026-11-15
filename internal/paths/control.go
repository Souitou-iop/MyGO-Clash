package paths

import (
	"os"
	"path/filepath"
)

// Names are the names the app runs under: the release's and the
// development build's, whose data directories are apart.
var Names = []string{"MyGO-Clash", "MyGO-Clash Dev"}

// DataDir returns the data directory of the app of that name, as MyGo
// lays it out (its PathUserData), without starting MyGo.
func DataDir(name string) (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// ControlSocket returns where the app of that data directory and slug
// serves its control socket: in the data directory, which only the user
// can enter, or in the core socket's directory when that path is too long
// for a socket.
func ControlSocket(data, slug string) (string, error) {
	p := filepath.Join(data, "control.sock")
	if len(p) <= maxSocketPath {
		return p, nil
	}
	dir, err := runtimeDir(slug)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "control.sock"), nil
}

// ControlToken returns the file of the control socket's token.
func ControlToken(data string) string { return filepath.Join(data, "control.token") }
