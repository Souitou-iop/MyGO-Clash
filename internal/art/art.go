// Package art draws the app's icons: a guitar pick, after the band MyGo is
// named for. Shapes are signed distance fields, so every size is drawn
// anti-aliased from the same description.
package art

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// pick is the signed distance from p (in units of the shape's radius,
// y pointing down) to the pick: a rounded triangle, pointing down, whose
// sides bulge toward a circle.
func pick(x, y float64) float64 {
	const k = 1.7320508075688772 // √3
	r := 0.62
	// iq's equilateral triangle, mirrored to point down.
	px, py := math.Abs(x)-r, y+r/k-0.08
	if px+k*py > 0 {
		px, py = (px-k*py)/2, (-k*px-py)/2
	}
	px -= math.Max(-2*r, math.Min(px, 0))
	tri := -math.Hypot(px, py) * sign(py)
	tri -= 0.22 // rounded corners
	circle := math.Hypot(x, y-0.06) - 0.9
	return 0.62*tri + 0.38*circle
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
	Off         Variant = iota // the proxy is off: an outline
	SystemProxy                // the system proxy is on: filled
	Tun                        // TUN is on: filled, with a dot
)

// Tray draws the tray icon at size px. A template icon is black on
// transparent, which macOS tints for the menu bar; otherwise it is colored.
func Tray(px int, v Variant, template bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	scale := float64(px) / 2.3 // units of the shape per pixel
	var ink color.NRGBA
	switch {
	case template:
		ink = color.NRGBA{0, 0, 0, 255}
	case v == Off:
		ink = color.NRGBA{142, 142, 150, 255}
	case v == SystemProxy:
		ink = color.NRGBA{232, 93, 154, 255}
	default:
		ink = color.NRGBA{52, 179, 112, 255}
	}
	stroke := 0.13 * scale // pixels
	for j := 0; j < px; j++ {
		for i := 0; i < px; i++ {
			x := (float64(i)+0.5)/scale - 1.15
			y := (float64(j)+0.5)/scale - 1.15
			d := pick(x, y) * scale
			var a float64
			if v == Off {
				a = coverage(math.Abs(d) - stroke/2)
			} else {
				a = coverage(d)
				// A string across the pick, cut out.
				line := math.Abs(y+0.12) * scale
				a *= 1 - coverage(line-0.06*scale)*coverage(d+0.22*scale)
			}
			if v == Tun {
				dot := (math.Hypot(x-0.78, y-0.78) - 0.3) * scale
				ring := (math.Hypot(x-0.78, y-0.78) - 0.45) * scale
				a = a * clamp01(ring) // clear room around the dot
				a = math.Max(a, coverage(dot))
			}
			img.SetNRGBA(i, j, color.NRGBA{ink.R, ink.G, ink.B, uint8(a * float64(ink.A))})
		}
	}
	return encode(img)
}

// AppIcon draws the app's icon at size px: a pick on a rounded square of
// the brand's gradient.
func AppIcon(px int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	s := float64(px)
	top := [3]float64{255, 138, 189}
	bottom := [3]float64{124, 92, 255}
	for j := 0; j < px; j++ {
		for i := 0; i < px; i++ {
			fx, fy := (float64(i)+0.5)/s, (float64(j)+0.5)/s
			// A squircle, as macOS draws app icons, inset for the shadow.
			nx, ny := (fx-0.5)/0.41, (fy-0.5)/0.41
			sq := math.Pow(math.Pow(math.Abs(nx), 5)+math.Pow(math.Abs(ny), 5), 0.2) - 1
			bg := coverage(sq * 0.41 * s)
			if bg == 0 {
				continue
			}
			t := clamp01(fx*0.35 + fy*0.75)
			var c [3]float64
			for k := range c {
				c[k] = top[k] + (bottom[k]-top[k])*t
			}
			// The pick, white, with a soft shadow.
			px := (fx - 0.5) / 0.26
			py := (fy - 0.49) / 0.26
			d := pick(px, py) * 0.26 * s
			shadow := clamp01(0.5-(pick(px, py-0.08)*0.26*s)/(0.05*s)) * 0.2
			a := coverage(d)
			line := math.Abs(py+0.12)*0.26*s - 0.012*s
			a *= 1 - coverage(line)*coverage(d+0.06*s)
			for k := range c {
				c[k] = c[k]*(1-shadow) + 0
				c[k] = c[k]*(1-a) + 255*a
			}
			img.SetNRGBA(i, j, color.NRGBA{uint8(c[0]), uint8(c[1]), uint8(c[2]), uint8(255 * bg)})
		}
	}
	return encode(img)
}

func encode(img image.Image) []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
