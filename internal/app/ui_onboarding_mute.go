package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
)

// onbSoundButton is the guide's sound, on and off, at the top of the
// window: a speaker drawn here so that it can move. While the guide
// sounds its two waves go out from it in turn, softly; muted, it gives a
// little as the waves fold back into it and a cross draws itself in,
// stroke by stroke; let sound again, the cross undraws and the waves come
// back out, one after the other. It reports a click.
func (m *mainUI) onbSoundButton(c *ui.Context, muted, rm bool) ui.Element {
	p := m.pal
	tip := locales.OnbMute()
	if muted {
		tip = locales.OnbUnmute()
	}
	b := ui.ButtonBase(c.Key("sound")).Size(30, 30).Radius(9).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Label(tip).Tooltip(tip)
	b.AccessSelected(muted)
	hot := b.Hovered()
	if hot {
		b.Background(p.SurfaceHover)
	}
	s := float32(1)
	if b.Pressed() {
		s = 0.92
	}
	b.AnimatePose(ui.Pose{Scale: s}, pressSpring).Transition(hoverFade)
	// k goes from 0, sounding, to 1, muted: the waves fold away in its
	// first half, the cross draws in its second, and back the other way.
	k := b.AnimateTo("muted", b2f(muted), ui.Animation{Duration: 460 * time.Millisecond, Ease: ui.CubicBezier(0.4, 0, 0.2, 1)})
	// A little give as it changes, the speaker pressed back and let go.
	flips := ui.Local(b, "flips", func() uint64 { return 0 })
	was := ui.Local(b, "was", func() bool { return muted })
	if *was != muted {
		*flips++
		*was = muted
	}
	give := float32(0)
	if *flips > 0 {
		give = b.Keyframes("give", []ui.Keyframe{{Value: 0}, {At: 90 * time.Millisecond, Value: 1, Ease: easeOut}, {At: 420 * time.Millisecond, Value: 0, Ease: ui.CubicBezier(0.3, 1.6, 0.5, 1)}}, ui.KeyframeOptions{Run: *flips})
	}
	// The waves breathe while it sounds.
	var breath float32
	if !muted && !rm {
		breath = b.Loop("breath", 1800*time.Millisecond, ui.Linear)
	}
	col := p.TextMuted
	if hot {
		col = p.Text2
	}
	b.Children(func() {
		ui.Box(c).Size(18, 18).Draw(func(pt *ui.Painter, r ui.Rect) {
			u := r.W / 24 // drawn on Lucide's grid of 24
			at := func(x, y float32) (float32, float32) { return r.X + x*u, r.Y + y*u }
			w := 1.9 * u
			// The speaker, giving.
			sp := 1 - 0.12*give
			pt.Transform(ui.Scaling(sp, sp), r.X+7*u, r.Y+12*u, func() {
				body := &ui.Path{}
				body.MoveTo(at(3, 9.5))
				body.LineTo(at(6.6, 9.5))
				body.LineTo(at(11, 5.2))
				body.LineTo(at(11, 18.8))
				body.LineTo(at(6.6, 14.5))
				body.LineTo(at(3, 14.5))
				body.Close().Join(ui.RoundJoin)
				pt.StrokePath(body, w, col)
			})
			// The waves: folding back into the speaker as k goes to 0.5.
			out := 1 - oClamp(k/0.5)
			for i, wave := range [][3][2]float32{{{15.5, 9}, {17.6, 12}, {15.5, 15}}, {{18.6, 5.8}, {23, 12}, {18.6, 18.2}}} {
				// Each a little after the other: the near one last to go, first
				// back.
				e := oClamp(out*1.6 - float32(i)*0.6)
				if e <= 0.01 {
					continue
				}
				// Breathing: a soft swell going out, the near wave first.
				pulse := float32(0)
				if !muted && !rm {
					ph := float64(breath) - float64(i)*0.18
					pulse = float32(math.Max(0, math.Sin(2*math.Pi*ph)))
				}
				dx := (1 - e) * -3.5 // folding in toward the speaker
				a := 0.55 + 0.45*e - 0.25*(1-pulse)*(1-oClamp(k*4))
				path := &ui.Path{}
				path.MoveTo(at(wave[0][0]+dx, wave[0][1]))
				path.QuadTo(r.X+(wave[1][0]+dx+pulse*0.8)*u, r.Y+wave[1][1]*u, r.X+(wave[2][0]+dx)*u, r.Y+wave[2][1]*u)
				// It draws out from its middle, and back into it.
				path.Trim(0.5-0.5*e, 0.5+0.5*e).Cap(ui.RoundCap)
				pt.StrokePath(path, w, col.Alpha(a))
			}
			// The cross, drawing in stroke by stroke as k goes from 0.5 to 1.
			x := oClamp((k - 0.45) / 0.55)
			for i, l := range [][4]float32{{16, 9, 22, 15}, {22, 9, 16, 15}} {
				e := oClamp(x*2 - float32(i))
				if e <= 0.01 {
					continue
				}
				line := (&ui.Path{}).MoveTo(at(l[0], l[1])).LineTo(at(l[2], l[3])).Cap(ui.RoundCap)
				pt.StrokePath(line.Trim(0, easeOut(e)), w, col)
			}
		})
	})
	return b
}
