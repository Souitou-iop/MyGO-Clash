package app

import (
	"math"

	"github.com/egoist/mygo/ui"
)

// How the compass lands back on the sidebar's mark as the guide ends: the
// band's five stars come out of it, go once round it as they close in,
// and go into it, while its needle turns a full turn, finding north
// again. The overlay stays over the window, letting it through, until the
// landing is done; the compass then lies exactly where the sidebar's own
// mark is, which takes over.

const (
	onbFly      = 0.62 // seconds the compass flies home; it lands then
	onbLandTail = 1.25 // seconds the landing plays after it lands
	onbClosing  = onbFly + onbLandTail
	onbStarsIn  = 1.0 // seconds the stars take to go round and in
)

// onbFlyEase is how it flies home: it leaves gently and glides in.
var onbFlyEase = ui.CubicBezier(0.5, 0, 0.2, 1)

// onbLandingDraw draws the compass flying home, t seconds after it left,
// and its landing at home, the sidebar's mark.
func (m *mainUI) onbLandingDraw(pt *ui.Painter, cp onbPose, t float32, home onbPose) {
	p := m.pal
	// The sidebar's own mark is under where it lands: its place waits for
	// it, empty, until the landing is done.
	pt.FillPath((&ui.Path{}).Circle(home.X, home.Y, home.Size*0.6), p.Theme.Background)
	τ := t - onbFly // since it landed
	if τ < 0 {
		drawCompass(pt, cp.X, cp.Y, cp.Size, compassLook{Ring: true, Turn: cp.Turn})
		return
	}
	x, y, size := home.X, home.Y, home.Size
	in := oClamp(τ / onbStarsIn)
	// A full turn as the stars go round, and the needle's settling.
	turn := 360 * ui.CubicBezier(0.4, 0, 0.2, 1)(in)
	if d := τ - onbStarsIn; d > 0 {
		turn = 360 + 14*float32(math.Sin(float64(d*14))*math.Exp(float64(-d/0.15)))
	}
	// As the stars go in, it swells a little, and its glints show.
	swell := float32(0)
	if d := τ - onbStarsIn; d > 0 {
		swell = 0.12 * float32(math.Sin(math.Pi*float64(oClamp(d/0.3))))
	}
	drawCompass(pt, x, y, size*(1+swell), compassLook{Ring: true, Turn: turn, Glints: swell * 6, Glint: p.Spark})
	if τ >= onbStarsIn {
		return
	}
	for i, hex := range mygoMarks {
		// Out, then round and in.
		out := easeOut(oClamp(τ / 0.22))
		r := size * (0.5 + 1.5*out) * (1 - float32(math.Pow(float64(in), 2.2)))
		deg := float64(-90+float32(i)*72) + 360*float64(ui.CubicBezier(0.3, 0, 0.6, 1)(in))
		sx := x + float32(math.Cos(deg*math.Pi/180))*r
		sy := y + float32(math.Sin(deg*math.Pi/180))*r
		star := (10 + 2.5*float32(math.Sin(float64(τ)*18+float64(i)))) * (1 - 0.55*in)
		drawSparkle(pt, sx, sy, star, ui.Hex(hex))
		// A short trail behind each.
		for k := 1; k <= 3; k++ {
			back := deg - float64(k)*9
			bx := x + float32(math.Cos(back*math.Pi/180))*r
			by := y + float32(math.Sin(back*math.Pi/180))*r
			pt.FillPath((&ui.Path{}).Circle(bx, by, 2.4*(1-float32(k)/4)), ui.Hex(hex).Alpha(0.55*(1-float32(k)/4)))
		}
	}
}
