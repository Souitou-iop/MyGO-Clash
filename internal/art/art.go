// Package art draws the tray icon after the app icon: the compass of
// MyGO!!!!!, a star of four long and four short points tilted in a ring.
// Shapes are signed distance fields, so every size is drawn anti-aliased
// from the same description.
package art

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// The compass, in units of the long points' length (y pointing down), from
// the band's logo. starQuarter is a quarter of the star, from the top point
// round toward the left one; turn makes the rest.
var (
	starQuarter = [][2]float64{{0.1822, -0.9833}, {-0.1449, -0.3382}, {-0.3269, -0.4757}, {-0.2638, -0.2565}}
	// facetQuarter is the light face of the top point, which the logo
	// draws white over the star.
	facetQuarter = [][2]float64{{0.1525, -0.8227}, {-0.1471, -0.2141}, {0, 0}}
	ringInner    = 0.735
	ringOuter    = 0.857
)

// The compass is turned 10.5° clockwise.
const tiltCos, tiltSin = 0.98325, 0.18224

// turn repeats a quarter of the compass round the other three quarters.
func turn(q [][2]float64) [][2]float64 {
	var out [][2]float64
	for k := 0; k < 4; k++ {
		for _, p := range q {
			x, y := p[0], p[1]
			for i := 0; i < k; i++ {
				x, y = y, -x
			}
			out = append(out, [2]float64{x, y})
		}
	}
	return out
}

var star = turn(starQuarter)

// facets are the light faces of the four long points.
var facets = func() (f [][][2]float64) {
	all := turn(facetQuarter)
	for i := 0; i < len(all); i += len(facetQuarter) {
		f = append(f, all[i:i+len(facetQuarter)])
	}
	return f
}()

// facet is the signed distance to the light faces of the long points.
func facet(x, y float64) float64 {
	d := math.Inf(1)
	for _, f := range facets {
		d = math.Min(d, polygon(x, y, f))
	}
	return d
}

// polygon is the signed distance to a closed polygon (iq's sdPolygon).
func polygon(px, py float64, v [][2]float64) float64 {
	d := (px-v[0][0])*(px-v[0][0]) + (py-v[0][1])*(py-v[0][1])
	s := 1.0
	for i, j := 0, len(v)-1; i < len(v); j, i = i, i+1 {
		ex, ey := v[j][0]-v[i][0], v[j][1]-v[i][1]
		wx, wy := px-v[i][0], py-v[i][1]
		t := clamp01((wx*ex + wy*ey) / (ex*ex + ey*ey))
		bx, by := wx-ex*t, wy-ey*t
		d = math.Min(d, bx*bx+by*by)
		c1, c2, c3 := py >= v[i][1], py < v[j][1], ex*wy > ey*wx
		if (c1 && c2 && c3) || (!c1 && !c2 && !c3) {
			s = -s
		}
	}
	return s * math.Sqrt(d)
}

func sign(v float64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// coverage turns a distance (in pixels) into the share of a pixel covered.
func coverage(d float64) float64 { return clamp01(0.5 - d) }

// Variant of the tray icon.
type Variant int

// Variants of the tray icon.
const (
	Off         Variant = iota // the proxy is off: the compass, faint
	SystemProxy                // the system proxy is on: the compass
	Tun                        // TUN is on: the compass, with a dot
)

// Tray draws the tray icon at size px. A template icon is black on
// transparent, which macOS tints for the menu bar, and faint when off;
// otherwise it is colored: gray when off, Anon's pink for the system proxy,
// the band's blue for TUN.
func Tray(px int, v Variant, template bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	scale := float64(px) / 2.08 // pixels per unit of the shape
	var ink color.NRGBA
	switch {
	case template && v == Off:
		ink = color.NRGBA{0, 0, 0, 115}
	case template:
		ink = color.NRGBA{0, 0, 0, 255}
	case v == Off:
		ink = color.NRGBA{140, 148, 166, 255}
	case v == SystemProxy:
		ink = color.NRGBA{238, 108, 138, 255}
	default:
		ink = color.NRGBA{58, 143, 200, 255}
	}
	// The light faces of the points need room to show.
	faces := px >= 30
	for j := 0; j < px; j++ {
		for i := 0; i < px; i++ {
			x := (float64(i)+0.5)/scale - 1.04
			y := (float64(j)+0.5)/scale - 1.04
			r := math.Hypot(x, y)
			d := polygon(x, y, star) * scale
			points := coverage(d)
			if faces {
				points *= 1 - coverage(facet(x, y)*scale+0.03*scale)
			}
			// The ring passes under the points, with a pixel's gap.
			ring := coverage(math.Max(ringInner-r, r-ringOuter)*scale) * clamp01(d-1)
			var dot float64
			if v == Tun {
				// The ring opens between the east and south points, for a
				// dot in the corner.
				u, w := x*tiltCos+y*tiltSin, y*tiltCos-x*tiltSin
				ring *= 1 - coverage(-math.Min(u, w)*scale)
				dot = coverage((math.Hypot(x-0.78, y-0.78) - 0.27) * scale)
			}
			a := math.Max(points, math.Max(ring, dot))
			img.SetNRGBA(i, j, color.NRGBA{ink.R, ink.G, ink.B, uint8(a * float64(ink.A))})
		}
	}
	return encode(img)
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
