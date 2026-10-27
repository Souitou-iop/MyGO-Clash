package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	// The service is named after the app's paths.Slug: the installer
	// must find it under the same name.
	if got := slug("MyGO-Clash"); got != "mygo-clash" {
		t.Errorf("slug = %q", got)
	}
	if got := slug("MyGO-Clash Dev"); got != "mygo-clash-dev" {
		t.Errorf("slug = %q", got)
	}
}

func TestNumericVersion(t *testing.T) {
	for in, want := range map[string]string{
		"0.2.0-beta":   "0.2.0.0",
		"1.2.3":        "1.2.3.0",
		"v10.4":        "10.4.0.0",
		"3.0.1+build7": "3.0.1.0",
	} {
		got, err := numericVersion(in)
		if err != nil || got != want {
			t.Errorf("numericVersion(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := numericVersion("beta"); err == nil {
		t.Error("a version without a number was accepted")
	}
}

func TestInstallerName(t *testing.T) {
	c := config{Slug: "mygo-clash", Version: "0.2.15-beta"}
	if got := installerName(&c, "arm64"); got != "mygo-clash-setup-0.2.15-beta-arm64.exe" {
		t.Errorf("installerName = %q", got)
	}
}

func TestExcluded(t *testing.T) {
	for name, want := range map[string]bool{
		"MyGO-Clash.exe":                        false,
		"icon.png":                              false,
		"MyGO-Clash Setup 0.2.0-beta.exe":       true,
		"mygo-clash-setup-0.2.0-beta-amd64.exe": true,
		"update-windows-amd64.json":             true,
		"mygo-clash-0.2.0-windows.tar.gz":       true,
		"a-to-b-windows-amd64.delta":            true,
		"install.sh":                            true,
		"install.ps1":                           true,
		"SHA256SUMS":                            true,
	} {
		if got := excluded(name); got != want {
			t.Errorf("excluded(%q) = %v", name, got)
		}
	}
}

func TestUpdateChecksum(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "windows-amd64")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	installer := filepath.Join(dir, "mygo-clash-setup-0.2.15-beta-amd64.exe")
	content := []byte("custom installer")
	if err := os.WriteFile(installer, content, 0o644); err != nil {
		t.Fatal(err)
	}
	sums := filepath.Join(root, "SHA256SUMS")
	old := strings.Repeat("0", 64) + "  mygo-clash-setup-0.2.15-beta-amd64.exe\n" +
		strings.Repeat("1", 64) + "  another.exe\n"
	if err := os.WriteFile(sums, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := updateChecksum(root, installer); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(sums)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	want := hex.EncodeToString(sum[:]) + "  " + filepath.Base(installer)
	if !strings.Contains(string(b), want) {
		t.Errorf("SHA256SUMS does not contain %q:\n%s", want, b)
	}
	if !strings.Contains(string(b), strings.Repeat("1", 64)+"  another.exe") {
		t.Errorf("SHA256SUMS lost another file:\n%s", b)
	}
}

func TestImages(t *testing.T) {
	icon := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			icon.SetNRGBA(x, y, color.NRGBA{200, 30, 30, 255})
		}
	}

	ico := makeICO(icon)
	var head [3]uint16
	if err := binary.Read(bytes.NewReader(ico), binary.LittleEndian, &head); err != nil {
		t.Fatal(err)
	}
	if head != [3]uint16{0, 1, 7} {
		t.Errorf("icon header = %v", head)
	}

	for _, c := range []struct {
		name string
		bmp  []byte
		w, h int
	}{
		{"header", encodeBMP(headerImage(icon)), 150, 57},
		{"welcome", encodeBMP(welcomeImage(icon)), 164, 314},
	} {
		if string(c.bmp[:2]) != "BM" {
			t.Errorf("%s: not a bitmap", c.name)
		}
		row := (c.w*3 + 3) &^ 3
		if want := 54 + row*c.h; len(c.bmp) != want {
			t.Errorf("%s: %d bytes, want %d", c.name, len(c.bmp), want)
		}
	}

	// Look at them: WINSETUP_DUMP=dir writes the files there.
	if dir := os.Getenv("WINSETUP_DUMP"); dir != "" {
		real, err := readPNG(filepath.Join("..", "..", "resources", "icon.png"))
		if err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "installer.ico"), makeICO(real), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "header.bmp"), encodeBMP(headerImage(real)), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "welcome.bmp"), encodeBMP(welcomeImage(real)), 0o644)
	}
}
