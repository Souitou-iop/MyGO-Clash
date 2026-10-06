package secure

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSealRoundTripAndBinding(t *testing.T) {
	s, err := NewSealer(RandomBytes(32), nil)
	if err != nil {
		t.Fatal(err)
	}
	plain := []byte("proxies: [{name: hk, password: hunter2}]")
	sealed := s.Seal(plain, "profile/abc")
	if bytes.Contains(sealed, []byte("hunter2")) {
		t.Fatal("sealed data contains the plaintext")
	}
	got, err := s.Unseal(sealed, "profile/abc")
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("unseal: %q %v", got, err)
	}
	if _, err := s.Unseal(sealed, "profile/other"); err == nil {
		t.Fatal("a seal opened under another label")
	}
	sealed[len(sealed)-1] ^= 1
	if _, err := s.Unseal(sealed, "profile/abc"); err == nil {
		t.Fatal("tampered data opened")
	}
	other, _ := NewSealer(RandomBytes(32), nil)
	if _, err := other.Unseal(s.Seal(plain, "x"), "x"); err == nil {
		t.Fatal("another key opened the seal")
	}
	if _, err := s.Unseal(plain, "x"); !errors.Is(err, ErrNotSealed) {
		t.Fatalf("plain data: %v", err)
	}
}

func TestOpenWithFileKeyringKeepsKey(t *testing.T) {
	t.Setenv("MYGO_CLASH_KEYRING", "file")
	dir := t.TempDir()
	s1, err := Open("MyGO-Clash Test", dir)
	if err != nil {
		t.Fatal(err)
	}
	if s1.Keyring().Secure() {
		t.Fatal("the file keyring claims to be secure")
	}
	path := filepath.Join(dir, "secrets.bin")
	sec, err := OpenSecrets(path, s1)
	if err != nil {
		t.Fatal(err)
	}
	if err := sec.Set("webdav.password", "s3cret"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	s2, err := Open("MyGO-Clash Test", dir)
	if err != nil {
		t.Fatal(err)
	}
	sec2, err := OpenSecrets(path, s2)
	if err != nil {
		t.Fatal(err)
	}
	if sec2.Get("webdav.password") != "s3cret" {
		t.Fatal("the secret did not survive a reopen")
	}
	raw, _ := os.ReadFile(path)
	if bytes.Contains(raw, []byte("s3cret")) {
		t.Fatal("secrets file is readable")
	}
}
