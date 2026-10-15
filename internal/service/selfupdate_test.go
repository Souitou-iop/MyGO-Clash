//go:build !windows

package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// archiveOf makes an update archive whose executable is a script saying
// version, as the app's says its own.
func archiveOf(t *testing.T, version string) []byte {
	t.Helper()
	exe := "mygo-clash"
	if runtime.GOOS == "darwin" {
		exe = "MyGO-Clash.app/Contents/MacOS/MyGO-Clash"
	}
	script := []byte("#!/bin/sh\necho " + version + "\n")
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Name: "icon.png", Mode: 0o644, Size: 1, Typeflag: tar.TypeReg})
	_, _ = tw.Write([]byte{0})
	_ = tw.WriteHeader(&tar.Header{Name: exe, Mode: 0o755, Size: int64(len(script)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(script)
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func sign(key ed25519.PrivateKey, b []byte) string {
	sum := sha256.Sum256(b)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, sum[:]))
}

func TestSelfUpdate(t *testing.T) {
	pub, key, _ := ed25519.GenerateKey(nil)
	_, other, _ := ed25519.GenerateKey(nil)
	oldKey, oldVersion := UpdateKey, Version
	t.Cleanup(func() { UpdateKey, Version = oldKey, oldVersion })
	UpdateKey, Version = base64.StdEncoding.EncodeToString(pub), "0.2.7-beta"

	dir := t.TempDir()
	p := Paths{Bin: filepath.Join(dir, "service")}
	if err := os.WriteFile(p.Bin, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	newer := archiveOf(t, "0.2.8-beta")

	for name, tc := range map[string]struct {
		version, sig string
		archive      []byte
		want         string
	}{
		"unsigned":      {"0.2.8-beta", sign(other, newer), newer, "not signed"},
		"older":         {"0.2.6-beta", sign(key, newer), newer, "already"},
		"same":          {"0.2.7-beta", sign(key, newer), newer, "already"},
		"other version": {"0.2.9", sign(key, newer), newer, "not 0.2.9"},
		"bad version":   {"../x", sign(key, newer), newer, "invalid version"},
	} {
		err := stage(ctx, p, tc.version, tc.sig, bytes.NewReader(tc.archive))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if _, err := os.Stat(stagedPath(p)); err == nil {
			t.Errorf("%s: left a staged update", name)
		}
	}
	if err := apply(p); err == nil {
		t.Fatal("applied without a staged update")
	}

	if err := stage(ctx, p, "0.2.8-beta", sign(key, newer), bytes.NewReader(newer)); err != nil {
		t.Fatal(err)
	}
	if err := apply(p); err != nil {
		t.Fatal(err)
	}
	if v, err := executableVersion(ctx, p.Bin); err != nil || v != "0.2.8-beta" {
		t.Fatalf("installed version = %q, %v", v, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("left files next to the service: %v", entries)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"0.2.8-beta", "0.2.7-beta", 1},
		{"0.2.7", "0.2.7-beta", 1},
		{"0.2.10", "0.2.9", 1},
		{"v0.3.0", "0.3.0", 0},
		{"0.3.0-beta.2", "0.3.0-beta.10", -1},
	} {
		if got := compareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
