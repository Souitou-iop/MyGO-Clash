package cloudsync

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// Vault encryption: what the server stores is encrypted on the device,
// and the server learns neither the contents nor the names of the files.
//
//	passphrase ──Argon2id(salt)──▶ KEK ──unwraps──▶ data key (random, 256-bit)
//	data key ──HKDF──▶ encryption key, naming key
//
// Every object is gzip-compressed, then sealed with XChaCha20-Poly1305 under
// the encryption key, with its name as associated data. Objects are named
// by the HMAC-SHA256 of their plaintext under the naming key, so equal
// content dedups while the server cannot test guesses. The passphrase only
// wraps the data key: changing it rewrites vault.json alone.
//
// A vault can also be created plain, when its owner chooses so: objects are
// only compressed, and named by their SHA-256. The choice belongs to the
// vault, and every device that joins it follows it.

// ErrPassphrase is returned when the passphrase does not open the vault.
var ErrPassphrase = errors.New("the passphrase does not open the vault on the server")

// VaultMeta is vault.json, the only file stored in the clear.
type VaultMeta struct {
	Format     string    `json:"format"` // "mygo-clash-vault", or "mygo-clash-plain"
	Version    int       `json:"version"`
	KDF        KDFParams `json:"kdf,omitzero"`
	WrappedKey []byte    `json:"wrappedKey,omitempty"` // nonce | sealed data key
	KeyID      string    `json:"keyId"`                // identifies the data key, not secret
	Created    time.Time `json:"created"`
}

// KDFParams are the Argon2id parameters.
type KDFParams struct {
	Alg     string `json:"alg"`
	Salt    []byte `json:"salt"`
	Time    uint32 `json:"time"`
	Memory  uint32 `json:"memory"` // KiB
	Threads uint8  `json:"threads"`
}

const (
	vaultFormat  = "mygo-clash-vault"
	plainFormat  = "mygo-clash-plain"
	objectMagic  = "MGCO"
	plainMagic   = "MGCP"
	objectFormat = 1
	maxPlain     = 128 << 20
)

// DefaultKDF is OWASP's recommendation for Argon2id: 64 MiB, 3 passes.
func DefaultKDF() KDFParams {
	return KDFParams{Alg: "argon2id", Salt: randBytes(16), Time: 3, Memory: 64 * 1024, Threads: 4}
}

func randBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}

func (k KDFParams) derive(passphrase string) ([]byte, error) {
	if k.Alg != "argon2id" {
		return nil, fmt.Errorf("unknown key derivation %q", k.Alg)
	}
	if k.Memory < 8*1024 || k.Time < 1 || k.Threads < 1 || len(k.Salt) < 16 || k.Memory > 4*1024*1024 || k.Time > 64 {
		return nil, errors.New("the vault's key derivation parameters are unsafe")
	}
	return argon2.IDKey([]byte(passphrase), k.Salt, k.Time, k.Memory, k.Threads, chacha20poly1305.KeySize), nil
}

// Keys are the keys of an opened vault; a plain vault's have none.
type Keys struct {
	ID    string
	plain bool
	data  []byte
	enc   []byte
	name  []byte
}

// Plain reports whether the vault is not encrypted.
func (k *Keys) Plain() bool { return k.plain }

// Plain reports whether the vault is not encrypted.
func (m VaultMeta) Plain() bool { return m.Format == plainFormat }

func keysFrom(data []byte) *Keys {
	k := &Keys{data: data, enc: subkey(data, "mygo-clash/sync/enc/v1"), name: subkey(data, "mygo-clash/sync/name/v1")}
	id := sha256.Sum256(subkey(data, "mygo-clash/sync/id/v1"))
	k.ID = hex.EncodeToString(id[:8])
	return k
}

func subkey(secret []byte, info string) []byte {
	out := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, secret, nil, []byte(info)), out); err != nil {
		panic(err)
	}
	return out
}

func wrapKey(kek, data []byte) []byte {
	aead, _ := chacha20poly1305.NewX(kek)
	nonce := randBytes(aead.NonceSize())
	return aead.Seal(nonce, nonce, data, []byte("mygo-clash/vault-key/v1"))
}

func unwrapKey(kek, wrapped []byte) ([]byte, error) {
	aead, _ := chacha20poly1305.NewX(kek)
	if len(wrapped) < aead.NonceSize() {
		return nil, ErrPassphrase
	}
	data, err := aead.Open(nil, wrapped[:aead.NonceSize()], wrapped[aead.NonceSize():], []byte("mygo-clash/vault-key/v1"))
	if err != nil {
		return nil, ErrPassphrase
	}
	return data, nil
}

// NewVault creates a vault with a new random data key.
func NewVault(passphrase string) (VaultMeta, *Keys, error) {
	if len(passphrase) < 8 {
		return VaultMeta{}, nil, errors.New("the passphrase needs at least 8 characters")
	}
	kdf := DefaultKDF()
	kek, err := kdf.derive(passphrase)
	if err != nil {
		return VaultMeta{}, nil, err
	}
	data := randBytes(32)
	keys := keysFrom(data)
	meta := VaultMeta{Format: vaultFormat, Version: 1, KDF: kdf, WrappedKey: wrapKey(kek, data), KeyID: keys.ID, Created: time.Now().UTC()}
	return meta, keys, nil
}

// NewPlainVault creates a vault that is not encrypted.
func NewPlainVault() (VaultMeta, *Keys) {
	id := hex.EncodeToString(randBytes(8))
	return VaultMeta{Format: plainFormat, Version: 1, KeyID: id, Created: time.Now().UTC()}, &Keys{ID: id, plain: true}
}

// Open opens a vault with its passphrase; a plain vault needs none.
func (m VaultMeta) Open(passphrase string) (*Keys, error) {
	if m.Format == plainFormat && m.Version == 1 {
		if m.KeyID == "" {
			return nil, errors.New("vault.json is damaged: it has no identifier")
		}
		return &Keys{ID: m.KeyID, plain: true}, nil
	}
	if m.Format != vaultFormat || m.Version != 1 {
		return nil, fmt.Errorf("the vault on the server is of an unknown format (%s %d)", m.Format, m.Version)
	}
	kek, err := m.KDF.derive(passphrase)
	if err != nil {
		return nil, err
	}
	data, err := unwrapKey(kek, m.WrappedKey)
	if err != nil {
		return nil, err
	}
	keys := keysFrom(data)
	if keys.ID != m.KeyID {
		return nil, errors.New("the vault's key does not match its identifier")
	}
	return keys, nil
}

// Rewrap returns the vault's metadata under a new passphrase.
func (m VaultMeta) Rewrap(keys *Keys, passphrase string) (VaultMeta, error) {
	if m.Plain() || keys.plain {
		return m, ErrPlain
	}
	if len(passphrase) < 8 {
		return m, errors.New("the passphrase needs at least 8 characters")
	}
	kdf := DefaultKDF()
	kek, err := kdf.derive(passphrase)
	if err != nil {
		return m, err
	}
	m.KDF = kdf
	m.WrappedKey = wrapKey(kek, keys.data)
	return m, nil
}

// ErrPlain is returned for what only an encrypted vault has.
var ErrPlain = errors.New("the vault is not encrypted")

const rawPlain = "plain:"

// Raw returns the data key, for keeping it in the device's secret store;
// for a plain vault, its identifier.
func (k *Keys) Raw() []byte {
	if k.plain {
		return []byte(rawPlain + k.ID)
	}
	return bytes.Clone(k.data)
}

// KeysFromRaw restores keys kept with Raw.
func KeysFromRaw(raw []byte) (*Keys, error) {
	if id, ok := bytes.CutPrefix(raw, []byte(rawPlain)); ok && len(id) > 0 {
		return &Keys{ID: string(id), plain: true}, nil
	}
	if len(raw) != 32 {
		return nil, errors.New("invalid vault key")
	}
	return keysFrom(bytes.Clone(raw)), nil
}

// Hash names plaintext: the HMAC-SHA256 under the naming key, in hex.
func (k *Keys) Hash(plain []byte) string {
	if k.plain {
		sum := sha256.Sum256(plain)
		return hex.EncodeToString(sum[:])
	}
	mac := hmac.New(sha256.New, k.name)
	mac.Write(plain)
	return hex.EncodeToString(mac.Sum(nil))
}

// Seal compresses and encrypts plaintext stored under name.
func (k *Keys) Seal(plain []byte, name string) ([]byte, error) {
	var zipped bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&zipped, gzip.BestCompression)
	if _, err := zw.Write(plain); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	if k.plain {
		return append([]byte{plainMagic[0], plainMagic[1], plainMagic[2], plainMagic[3], objectFormat}, zipped.Bytes()...), nil
	}
	aead, _ := chacha20poly1305.NewX(k.enc)
	out := make([]byte, 0, 5+aead.NonceSize()+zipped.Len()+aead.Overhead())
	out = append(out, objectMagic...)
	out = append(out, objectFormat)
	nonce := randBytes(aead.NonceSize())
	out = append(out, nonce...)
	return aead.Seal(out, nonce, zipped.Bytes(), []byte("mygo-clash/object/v1/"+name)), nil
}

// Open decrypts what Seal made for name.
func (k *Keys) Open(sealed []byte, name string) ([]byte, error) {
	var zipped []byte
	if k.plain {
		if len(sealed) < 5 || string(sealed[:4]) != plainMagic {
			return nil, errors.New("not an object of this vault")
		}
		if sealed[4] != objectFormat {
			return nil, fmt.Errorf("unknown object format %d", sealed[4])
		}
		zipped = sealed[5:]
	} else {
		aead, _ := chacha20poly1305.NewX(k.enc)
		head := 5 + aead.NonceSize()
		if len(sealed) < head+aead.Overhead() || string(sealed[:4]) != objectMagic {
			return nil, errors.New("not an object of this vault")
		}
		if sealed[4] != objectFormat {
			return nil, fmt.Errorf("unknown object format %d", sealed[4])
		}
		var err error
		zipped, err = aead.Open(nil, sealed[5:head], sealed[head:], []byte("mygo-clash/object/v1/"+name))
		if err != nil {
			return nil, fmt.Errorf("the object %s was changed on the server, or belongs to another vault", name)
		}
	}
	zr, err := gzip.NewReader(bytes.NewReader(zipped))
	if err != nil {
		return nil, err
	}
	plain, err := io.ReadAll(io.LimitReader(zr, maxPlain+1))
	if err != nil {
		return nil, err
	}
	if len(plain) > maxPlain {
		return nil, errors.New("the object is too large")
	}
	return plain, nil
}

// SealJSON encodes and seals a value.
func (k *Keys) SealJSON(v any, name string) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return k.Seal(data, name)
}

// OpenJSON opens and decodes a value.
func (k *Keys) OpenJSON(sealed []byte, name string, v any) error {
	data, err := k.Open(sealed, name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
