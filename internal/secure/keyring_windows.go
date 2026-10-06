package secure

import (
	"errors"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

// DefaultKeyring returns secrets protected with DPAPI, which only the user
// can decrypt, in files of the data directory.
func DefaultKeyring(service, dataDir string) Keyring {
	if forceFile() {
		return FileKeyring(dataDir)
	}
	return dpapi{dir: dataDir, service: service}
}

type dpapi struct{ dir, service string }

func (k dpapi) path(account string) string { return filepath.Join(k.dir, ".keyring-"+account+".dpapi") }

func blob(b []byte) *windows.DataBlob {
	if len(b) == 0 {
		return &windows.DataBlob{}
	}
	return &windows.DataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func (k dpapi) entropy() *windows.DataBlob { return blob([]byte("mygo-clash/" + k.service)) }

func (k dpapi) Get(account string) ([]byte, error) {
	data, err := os.ReadFile(k.path(account))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var out windows.DataBlob
	if err := windows.CryptUnprotectData(blob(data), nil, k.entropy(), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return nil, err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), nil
}

func (k dpapi) Set(account string, secret []byte) error {
	var out windows.DataBlob
	name, _ := windows.UTF16PtrFromString(k.service)
	if err := windows.CryptProtectData(blob(secret), name, k.entropy(), 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &out); err != nil {
		return err
	}
	defer windows.LocalFree(windows.Handle(unsafe.Pointer(out.Data)))
	return WriteFileAtomic(k.path(account), append([]byte(nil), unsafe.Slice(out.Data, out.Size)...), 0o600)
}

func (k dpapi) Delete(account string) error {
	err := os.Remove(k.path(account))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (dpapi) Name() string { return "dpapi" }
func (dpapi) Secure() bool { return true }
