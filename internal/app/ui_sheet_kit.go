package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// The pieces the home page's sheets are made of, so that each reads the
// same way: what it is about, large, first; then how it went over time, as
// a trail; then the facts, side by side; then what can be done there. The
// parts rise in one after another as the sheet opens, and a few marks are
// the app's own: the compass's ring of ticks for a day, and its glint for
// "now".

// sheetSection is a part of a sheet, the i-th: it rises in a beat after
// the sheet has grown, after the parts before it.
func (m *mainUI) sheetSection(c *ui.Context, key string, i int, fn func()) ui.Element {
	sec := ui.Column(c.Key(key)).Gap(12).Shrink(0)
	if !c.Preferences().ReduceMotion {
		sec.Transition(ui.ElementTransition{
			Enter:     &ui.Motion{Y: 12},
			Colors:    true,
			Animation: &ui.Animation{Duration: 340 * time.Millisecond, Delay: 180*time.Millisecond + time.Duration(i)*45*time.Millisecond, Ease: easeOut},
		})
	}
	sec.Children(fn)
	return sec
}

// sheetSplit lays a sheet w wide out in two columns where it is wide
// enough, what it is about on the left and its details on the right; one
// above the other where it is not.
func (m *mainUI) sheetSplit(c *ui.Context, w float32, hero, detail func()) {
	if w >= 760 {
		ui.Row(c.Key("split")).Gap(18).AlignItems(ui.Start).Shrink(0).Children(func() {
			ui.Column(c.Key("hero")).Width(min(420, w*0.42)).Shrink(0).Gap(14).Children(hero)
			ui.Column(c.Key("detail")).Grow(1).Basis(0).MinWidth(0).Gap(14).Children(detail)
		})
		return
	}
	ui.Column(c.Key("split")).Gap(14).Shrink(0).Children(func() {
		hero()
		detail()
	})
}

// panel is a block of a sheet: a step of grey, rounded, padded.
func (m *mainUI) panel(c *ui.Context, key string, fn func()) ui.Element {
	return ui.Column(c.Key(key)).Padding(16, 18).Gap(10).Radius(16).Background(m.pal.Surface2).Children(fn)
}

// panelLabel is the small grey name of a panel's content.
func (m *mainUI) panelLabel(c *ui.Context, s string) {
	ui.Text(c, s).FontSize(12).FontWeight(600).TextColor(m.pal.TextMuted).SingleLine()
}

// figure is a number large, its unit beside it.
func (m *mainUI) figure(c *ui.Context, num, unit string, size float32, col ui.Color) {
	p := m.pal
	ui.Row(c).Gap(4).AlignItems(ui.Baseline).Shrink(0).Children(func() {
		t := ui.Text(c, num).FontSize(size).FontWeight(700).LetterSpacing(-0.6).FontFeatures("tnum").ContentTransition(numRoll).SingleLine()
		if col != (ui.Color{}) {
			t.TextColor(col)
		}
		if unit != "" {
			ui.Text(c, unit).FontSize(max(12, size*0.36)).TextColor(p.TextMuted).SingleLine()
		}
	})
}

// stat is a fact of a row of them: its name small, its value below.
func (m *mainUI) stat(c *ui.Context, key, label, value string) {
	p := m.pal
	ui.Column(c.Key(key)).Gap(3).MinWidth(0).Children(func() {
		ui.Text(c, label).FontSize(11.5).TextColor(p.TextMuted).SingleLine()
		ui.Text(c, value).FontSize(14).FontWeight(620).FontFeatures("tnum").SingleLine().ContentTransition(numRoll)
	})
}

// trail draws vals as a line, w by h (a w of 0 takes the width there is):
// 1.5 points of col, the last point the glint of "now"; a zero, a failure,
// a dash of the danger color at the top. It draws itself in as it comes.
func (m *mainUI) trail(c *ui.Context, key any, vals []float64, w, h float32, col ui.Color) ui.Element {
	p := m.pal
	box := ui.Box(c.Key(key)).Height(h).Shrink(0)
	if w > 0 {
		box.Width(w)
	} else {
		box.Grow(1).MinWidth(0)
	}
	in := float32(1)
	if !c.Preferences().ReduceMotion {
		in = box.Keyframes("in", []ui.Keyframe{{Value: 0}, {At: 700 * time.Millisecond, Value: 1, Ease: easeStd}}, ui.KeyframeOptions{Run: 1})
	}
	vs := vals
	return box.Draw(func(pt *ui.Painter, r ui.Rect) {
		if len(vs) == 0 {
			return
		}
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, v := range vs {
			if v > 0 {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
		}
		if math.IsInf(lo, 0) {
			lo, hi = 0, 1
		}
		if hi-lo < 1e-9 {
			lo, hi = lo-1, hi+1
		}
		pad := float32(4)
		step := (r.W - 2*pad) / float32(max(1, len(vs)-1))
		at := func(i int) (float32, float32) {
			x := r.X + pad + float32(i)*step
			y := r.Y + pad + (r.H-2*pad)*float32(1-(vs[i]-lo)/(hi-lo))
			return x, y
		}
		end := r.X + pad + (r.W-2*pad)*in
		pt.Clip(ui.Rect{X: r.X, Y: r.Y - 4, W: end - r.X + 6, H: r.H + 8}, 0, func() {
			var path ui.Path
			open := false
			for i, v := range vs {
				x, y := at(i)
				if v <= 0 {
					pt.Line(x-3, r.Y+1, x+3, r.Y+1, 2, p.Danger)
					open = false
					continue
				}
				if !open {
					path.MoveTo(x, y)
					open = true
				} else {
					path.LineTo(x, y)
				}
			}
			pt.StrokePath(path.Join(ui.RoundJoin).Cap(ui.RoundCap), 1.5, col)
		})
		if in >= 1 {
			for i := len(vs) - 1; i >= 0; i-- {
				if vs[i] > 0 {
					x, y := at(i)
					drawSparkle(pt, x, y, 4.5, p.Spark)
					break
				}
			}
		}
	})
}

// tickRing draws a day around a circle of size, as the compass's outer
// ring: 72 ticks, three an hour, those lit in the accent, the tick of now
// with the glint. lit(i) says whether the i-th third of an hour is lit.
func (m *mainUI) tickRing(c *ui.Context, key any, size float32, lit func(i int) bool, now int) ui.Element {
	p := m.pal
	box := ui.Box(c.Key(key)).Size(size, size).Shrink(0)
	in := float32(1)
	if !c.Preferences().ReduceMotion {
		in = box.Keyframes("in", []ui.Keyframe{{Value: 0}, {At: 900 * time.Millisecond, Value: 1, Ease: easeStd}}, ui.KeyframeOptions{Run: 1})
	}
	return box.Draw(func(pt *ui.Painter, r ui.Rect) {
		cx, cy := r.X+r.W/2, r.Y+r.H/2
		outer := r.W/2 - 2
		for i := range 72 {
			if float32(i) > in*72 {
				break
			}
			a := float64(i)/72*2*math.Pi - math.Pi/2
			long := i%3 == 0
			inner := outer - 6
			if long {
				inner = outer - 10
			}
			col := p.Border
			if lit(i) {
				col = p.Accent
			}
			cs, sn := float32(math.Cos(a)), float32(math.Sin(a))
			w := float32(1.5)
			if long {
				w = 2
			}
			pt.Line(cx+cs*inner, cy+sn*inner, cx+cs*outer, cy+sn*outer, w, col)
		}
		if now >= 0 && in >= 1 {
			a := float64(now)/72*2*math.Pi - math.Pi/2
			drawSparkle(pt, cx+float32(math.Cos(a))*(outer+1), cy+float32(math.Sin(a))*(outer+1), 6, p.Spark)
		}
	})
}

// ghost holds the place of what has no history yet: the same shape, in a
// step of grey.
func (m *mainUI) ghost(c *ui.Context, w, h float32) ui.Element {
	b := ui.Box(c).Height(h).Radius(6).Background(m.pal.Surface3)
	if w > 0 {
		b.Width(w)
	} else {
		b.Grow(1)
	}
	return b
}
