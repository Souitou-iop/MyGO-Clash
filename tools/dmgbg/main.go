// Command dmgbg draws the background of the macOS disk image window: a calm
// solid field, a brand-blue arrow from the app to the Applications folder and
// a one-line hint. It writes background.png (660x400) and background@2x.png
// (1320x800); dmgbuild combines the pair into a Retina-aware TIFF.
//
//	go run -C tools/dmgbg . ../../packaging/macos
//
// The drawing is plain image/draw plus 4x supersampling, so the output is
// reproducible. The hint is set in Go Regular (BSD-3-Clause, golang.org/x/image).
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The window is 660x400 points. These are shared with dmgbuild-settings.py:
// the icons sit at (180, 170) and (480, 170).
const (
	width, height = 660, 400
	iconY         = 170
)

var (
	paper = color.RGBA{0xF4, 0xF8, 0xFB, 0xFF} // light, cool white; reads in both Finder appearances
	blue  = color.RGBA{0x0B, 0x88, 0xBB, 0xFF} // MyGO blue
	muted = color.RGBA{0x4F, 0x6B, 0x7A, 0xFF} // hint text, 5.3:1 on paper
)

func main() {
	out := "."
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	for _, s := range []struct {
		scale int
		name  string
	}{{1, "background.png"}, {2, "background@2x.png"}} {
		path := filepath.Join(out, s.name)
		f, err := os.Create(path)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, render(s.scale)); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("wrote", path)
	}
}

func render(scale int) *image.RGBA {
	const ss = 4 // supersampling for the arrow's edges
	k := scale * ss
	big := image.NewRGBA(image.Rect(0, 0, width*k, height*k))
	draw.Draw(big, big.Bounds(), image.NewUniform(paper), image.Point{}, draw.Src)

	// A block arrow centred between the two icons.
	cx, cy := 330.0, float64(iconY)
	poly := [][2]float64{
		{cx - 34, cy - 7}, {cx + 8, cy - 7}, {cx + 8, cy - 22},
		{cx + 38, cy}, {cx + 8, cy + 22}, {cx + 8, cy + 7}, {cx - 34, cy + 7},
	}
	fillPolygon(big, poly, float64(k), blue)

	img := image.NewRGBA(image.Rect(0, 0, width*scale, height*scale))
	downsample(img, big, ss)

	drawHint(img, "Drag MyGO-Clash onto Applications to install", float64(scale))
	return img
}

// fillPolygon fills a simple polygon (points in window points) with an
// even-odd scanline test at the pixel centres of dst, which is k pixels per
// point.
func fillPolygon(dst *image.RGBA, pts [][2]float64, k float64, c color.RGBA) {
	b := dst.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		fy := (float64(y) + 0.5) / k
		var xs []float64
		for i := range pts {
			a, d := pts[i], pts[(i+1)%len(pts)]
			if (a[1] <= fy) != (d[1] <= fy) {
				xs = append(xs, a[0]+(fy-a[1])/(d[1]-a[1])*(d[0]-a[0]))
			}
		}
		if len(xs) < 2 {
			continue
		}
		if xs[0] > xs[1] {
			xs[0], xs[1] = xs[1], xs[0]
		}
		for x := int(xs[0]*k + 0.5); x < int(xs[len(xs)-1]*k+0.5) && x < b.Max.X; x++ {
			if x >= b.Min.X {
				dst.SetRGBA(x, y, c)
			}
		}
	}
}

func downsample(dst, src *image.RGBA, f int) {
	for y := 0; y < dst.Bounds().Dy(); y++ {
		for x := 0; x < dst.Bounds().Dx(); x++ {
			var r, g, b, n int
			for dy := range f {
				for dx := range f {
					p := src.RGBAAt(x*f+dx, y*f+dy)
					r, g, b, n = r+int(p.R), g+int(p.G), b+int(p.B), n+1
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n), uint8(g / n), uint8(b / n), 0xFF})
		}
	}
}

func drawHint(dst *image.RGBA, text string, scale float64) {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		log.Fatal(err)
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: 14 * scale, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		log.Fatal(err)
	}
	defer face.Close()
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(muted), Face: face}
	w := d.MeasureString(text)
	d.Dot = fixed.Point26_6{X: (fixed.I(int(width*scale)) - w) / 2, Y: fixed.I(int(330 * scale))}
	d.DrawString(text)
}
