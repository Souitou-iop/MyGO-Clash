package secure

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

type fileKeyring struct{ dir string }

// FileKeyring keeps secrets in files of dir readable only by the user. It
// is the keyring of last resort, for systems without one.
func FileKeyring(dir string) Keyring { return fileKeyring{dir: dir} }

func (k fileKeyring) path(account string) string {
	return filepath.Join(k.dir, ".keyring-"+account)
}

func (k fileKeyring) Get(account string) ([]byte, error) {
	data, err := os.ReadFile(k.path(account))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(string(data)))
}

func (k fileKeyring) Set(account string, secret []byte) error {
	return WriteFileAtomic(k.path(account), []byte(base64.StdEncoding.EncodeToString(secret)), 0o600)
}

func (k fileKeyring) Delete(account string) error {
	err := os.Remove(k.path(account))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (fileKeyring) Name() string { return "file" }
func (fileKeyring) Secure() bool { return false }

// forceFile makes the app keep its master key in a file, for tests and
// headless machines: MYGO_CLASH_KEYRING=file.
func forceFile() bool { return os.Getenv("MYGO_CLASH_KEYRING") == "file" }
