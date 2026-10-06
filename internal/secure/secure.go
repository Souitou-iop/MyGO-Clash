// Package secure keeps the app's data private at rest.
//
// A random 256-bit master key lives in the operating system's keyring (the
// Keychain on macOS, DPAPI on Windows, the Secret Service on Linux, or a
// file readable only by the user where there is none). It seals the files
// that hold credentials, subscription URLs and proxy passwords with
// XChaCha20-Poly1305, each bound to what it is (its label) so that sealed
// files cannot be swapped for one another.
package secure

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/chacha20poly1305"
)

var (
	magic = []byte("MGCS")
	// ErrNotSealed is returned when data has no seal, such as a file
	// written before encryption was on.
	ErrNotSealed = errors.New("data is not sealed")
	// ErrNotFound is returned by keyrings for missing entries.
	ErrNotFound = errors.New("secret not found")
)

const (
	formatVersion = 1
	keySize       = chacha20poly1305.KeySize
	headerSize    = 4 + 1 + chacha20poly1305.NonceSizeX
)

// Keyring stores small secrets with the operating system.
type Keyring interface {
	Get(account string) ([]byte, error)
	Set(account string, secret []byte) error
	Delete(account string) error
	// Name says where secrets are kept, for the app to show.
	Name() string
	// Secure reports whether the keyring is protected by the system rather
	// than file permissions alone.
	Secure() bool
}

// Sealer seals data with the master key.
type Sealer struct {
	key     []byte
	keyring Keyring
}

// Open returns the sealer of the app, creating the master key on first
// use. service names the app in the keyring; dataDir holds the key's file
// on systems without a keyring.
func Open(service, dataDir string) (*Sealer, error) {
	kr := DefaultKeyring(service, dataDir)
	key, err := kr.Get("master-key")
	if errors.Is(err, ErrNotFound) {
		key = make([]byte, keySize)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		if err := kr.Set("master-key", key); err != nil {
			// The system keyring failed: keep the key in a file instead.
			fkr := FileKeyring(dataDir)
			if ferr := fkr.Set("master-key", key); ferr != nil {
				return nil, fmt.Errorf("store the master key: %v; %v", err, ferr)
			}
			kr = fkr
		}
	} else if err != nil {
		// The keyring cannot be read (locked, absent): a file key written
		// by an earlier fallback may exist.
		fkr := FileKeyring(dataDir)
		if fk, ferr := fkr.Get("master-key"); ferr == nil {
			key, kr = fk, fkr
		} else {
			return nil, fmt.Errorf("read the master key from %s: %w", kr.Name(), err)
		}
	}
	return NewSealer(key, kr)
}

// NewSealer returns a sealer of a 32-byte key.
func NewSealer(key []byte, kr Keyring) (*Sealer, error) {
	if len(key) != keySize {
		return nil, fmt.Errorf("the master key has %d bytes, not %d", len(key), keySize)
	}
	if _, err := chacha20poly1305.NewX(key); err != nil {
		return nil, err
	}
	return &Sealer{key: bytes.Clone(key), keyring: kr}, nil
}

// Keyring returns where the master key is kept.
func (s *Sealer) Keyring() Keyring { return s.keyring }

func aad(label string) []byte { return []byte("mygo-clash/v1/" + label) }

// Seal encrypts data, binding it to label.
func (s *Sealer) Seal(data []byte, label string) []byte {
	aead, _ := chacha20poly1305.NewX(s.key)
	out := make([]byte, headerSize, headerSize+len(data)+aead.Overhead())
	copy(out, magic)
	out[4] = formatVersion
	nonce := out[5:headerSize]
	if _, err := rand.Read(nonce); err != nil {
		panic(err) // the system's randomness failed
	}
	return aead.Seal(out, nonce, data, aad(label))
}

// IsSealed reports whether data starts like sealed data.
func IsSealed(data []byte) bool { return len(data) >= headerSize && bytes.Equal(data[:4], magic) }

// Unseal decrypts data sealed with label.
func (s *Sealer) Unseal(data []byte, label string) ([]byte, error) {
	if !IsSealed(data) {
		return nil, ErrNotSealed
	}
	if data[4] != formatVersion {
		return nil, fmt.Errorf("unknown seal version %d", data[4])
	}
	aead, _ := chacha20poly1305.NewX(s.key)
	plain, err := aead.Open(nil, data[5:headerSize], data[headerSize:], aad(label))
	if err != nil {
		return nil, errors.New("the data cannot be decrypted: it was changed, or sealed with another key")
	}
	return plain, nil
}

// WriteFile seals data into a file, atomically and readable only by the
// user.
func (s *Sealer) WriteFile(path string, data []byte, label string) error {
	return WriteFileAtomic(path, s.Seal(data, label), 0o600)
}

// ReadFile reads a sealed file. A file that is not sealed, written by an
// older version or by hand, is returned as it is, with ErrNotSealed, so that
// callers can seal it.
func (s *Sealer) ReadFile(path, label string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if !IsSealed(data) {
		return data, ErrNotSealed
	}
	return s.Unseal(data, label)
}

// WriteFileAtomic writes a file through a temporary file and a rename, so
// that readers never see half of it.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path) // replaces path on Windows too
}

// Secrets is a small sealed store of named secrets: passwords, keys,
// tokens.
type Secrets struct {
	mu     sync.Mutex
	path   string
	sealer *Sealer
	values map[string]string
}

const secretsLabel = "secrets"

// OpenSecrets loads the secrets in path.
func OpenSecrets(path string, sealer *Sealer) (*Secrets, error) {
	s := &Secrets{path: path, sealer: sealer, values: map[string]string{}}
	data, err := sealer.ReadFile(path, secretsLabel)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return s, nil
	case errors.Is(err, ErrNotSealed):
		return nil, fmt.Errorf("%s is not sealed", path)
	case err != nil:
		return nil, err
	}
	if err := json.Unmarshal(data, &s.values); err != nil {
		return nil, err
	}
	return s, nil
}

// Get returns a secret, or "".
func (s *Secrets) Get(name string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[name]
}

// Has reports whether a secret is set.
func (s *Secrets) Has(name string) bool { return s.Get(name) != "" }

// Set sets a secret; "" deletes it.
func (s *Secrets) Set(name, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if value == "" {
		delete(s.values, name)
	} else {
		s.values[name] = value
	}
	data, err := json.Marshal(s.values)
	if err != nil {
		return err
	}
	return s.sealer.WriteFile(s.path, data, secretsLabel)
}

// RandomBytes returns n random bytes.
func RandomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		panic(err)
	}
	return b
}
