package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"os"
	"path/filepath"
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

func TestExcluded(t *testing.T) {
	for name, want := range map[string]bool{
		"MyGO-Clash.exe":                  false,
		"icon.png":                        false,
		"MyGO-Clash Setup 0.2.0-beta.exe": true,
		"update-windows-amd64.json":       true,
		"mygo-clash-0.2.0-windows.tar.gz": true,
		"a-to-b-windows-amd64.delta":      true,
	} {
		if got := excluded(name); got != want {
			t.Errorf("excluded(%q) = %v", name, got)
		}
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
