package art

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

// TestTray draws every tray icon and checks the variants differ where they
// should. ART_PREVIEW=dir also writes them there, to look at.
func TestTray(t *testing.T) {
	out := os.Getenv("ART_PREVIEW")
	for _, px := range []int{18, 22, 32, 36, 44} {
		for v, name := range []string{"off", "sysproxy", "tun"} {
			for _, tmpl := range []bool{false, true} {
				b := Tray(px, Variant(v), tmpl)
				if _, err := png.Decode(bytes.NewReader(b)); err != nil {
					t.Fatal(err)
				}
				if out != "" {
					if err := os.WriteFile(filepath.Join(out, fmt.Sprintf("tray-%s-%d-%v.png", name, px, tmpl)), b, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
		}
	}
	const px = 44
	draw := func(v Variant) image.Image {
		img, err := png.Decode(bytes.NewReader(Tray(px, v, true)))
		if err != nil {
			t.Fatal(err)
		}
		return img
	}
	// alpha at a point in the shape's units.
	alpha := func(img image.Image, x, y float64) uint32 {
		s := float64(px) / 2.08
		_, _, _, a := img.At(int((x+1.04)*s), int((y+1.04)*s)).RGBA()
		return a >> 8
	}
	off, on, tun := draw(Off), draw(SystemProxy), draw(Tun)
	ring := (ringInner + ringOuter) / 2
	for _, c := range []struct {
		name     string
		img      image.Image
		x, y     float64
		min, max uint32
	}{
		{"ring, on", on, -ring * 0.7071, ring * 0.7071, 230, 255},
		{"a point, on", on, 0, -0.6, 230, 255},
		{"a point, off", off, 0, -0.6, 80, 150},
		{"between the points", on, 0.45, -0.45, 0, 20},
		{"dot, on", on, 0.78, 0.78, 0, 20},
		{"dot, tun", tun, 0.78, 0.78, 230, 255},
	} {
		if a := alpha(c.img, c.x, c.y); a < c.min || a > c.max {
			t.Errorf("%s: alpha %d, want %d to %d", c.name, a, c.min, c.max)
		}
	}
}
