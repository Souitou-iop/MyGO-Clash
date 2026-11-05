package cloudsync

import (
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// Identity is a device's own key pair. Its public half is listed with the
// device in the manifest; when the vault's data key is replaced, the device
// that replaces it seals the new key to the public key of every device that
// stays, in vault.json, so that they follow without the passphrase. A device
// that was removed gets no copy.
type Identity struct {
	priv *ecdh.PrivateKey
}

// NewIdentity makes a key pair.
func NewIdentity() *Identity {
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	return &Identity{priv: k}
}

// IdentityFromRaw restores a key pair kept with Raw.
func IdentityFromRaw(raw []byte) (*Identity, error) {
	k, err := ecdh.X25519().NewPrivateKey(raw)
	if err != nil {
		return nil, err
	}
	return &Identity{priv: k}, nil
}

// Raw returns the private key, for the device's secret store.
func (i *Identity) Raw() []byte { return i.priv.Bytes() }

// Public returns the public key.
func (i *Identity) Public() []byte { return i.priv.PublicKey().Bytes() }

const memberInfo = "mygo-clash/member-key/v1"

func memberKey(shared, ephPub, pub []byte) []byte {
	salt := append(append([]byte{}, ephPub...), pub...)
	out := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(hkdf.New(sha256.New, shared, salt, []byte(memberInfo)), out); err != nil {
		panic(err)
	}
	return out
}

// SealTo encrypts data so that only the holder of pub's private key opens
// it: an ephemeral X25519 exchange, HKDF, XChaCha20-Poly1305.
func SealTo(pub, data []byte) ([]byte, error) {
	peer, err := ecdh.X25519().NewPublicKey(pub)
	if err != nil {
		return nil, err
	}
	eph, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	shared, err := eph.ECDH(peer)
	if err != nil {
		return nil, err
	}
	ephPub := eph.PublicKey().Bytes()
	aead, _ := chacha20poly1305.NewX(memberKey(shared, ephPub, pub))
	nonce := randBytes(aead.NonceSize())
	out := append(append([]byte{}, ephPub...), nonce...)
	return aead.Seal(out, nonce, data, []byte(memberInfo)), nil
}

// Open decrypts what SealTo sealed to this identity.
func (i *Identity) Open(sealed []byte) ([]byte, error) {
	aead, _ := chacha20poly1305.NewX(make([]byte, chacha20poly1305.KeySize))
	if len(sealed) < 32+aead.NonceSize()+aead.Overhead() {
		return nil, errors.New("the sealed key is too short")
	}
	ephPub, nonce, ct := sealed[:32], sealed[32:32+aead.NonceSize()], sealed[32+aead.NonceSize():]
	peer, err := ecdh.X25519().NewPublicKey(ephPub)
	if err != nil {
		return nil, err
	}
	shared, err := i.priv.ECDH(peer)
	if err != nil {
		return nil, err
	}
	aead, _ = chacha20poly1305.NewX(memberKey(shared, ephPub, i.Public()))
	data, err := aead.Open(nil, nonce, ct, []byte(memberInfo))
	if err != nil {
		return nil, errors.New("the sealed key is not for this device")
	}
	return data, nil
}
