package app

import (
	"math"
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/config"
)

// The first-run guide of the main window: six short scenes the app's
// compass walks through, the one element they share. It is assembled from
// its facets as the guide opens for the first time, or flies out of the
// sidebar's mark when the settings play it again; each scene gives it a
// part (the hub traffic passes through, the needle that turns to the
// node picked, the mark of a small window, the arm of a radar), and as
// the guide ends it flies home to the sidebar. Its needle settles as a
// real one does, swinging past and back. The scenes are drawn from the
// time alone, so going back and forth replays nothing it should not.

const (
	onbSteps = 6
	onbGlide = 0.72 // seconds the compass takes between scenes
	onbLeave = 0.26 // seconds a scene takes to leave
)

// onboarding is where the guide is. Its fields are the view's.
type onboarding struct {
	open   bool // played from the settings
	shown  bool // built in the last frame
	replay bool // the compass came out of the sidebar, not assembled

	step, prev int
	dir        float32   // 1 forward, -1 back
	at, prevAt time.Time // when step and prev came
	openAt     time.Time
	from       onbPose // the compass as the step changed
	closing    time.Time
	brand      onbPose // the sidebar's mark

	// What input asked for, done in the next frame, which has its clock.
	pending int    // steps to go
	jump    int    // a step to go to, -1 for none
	finish  string // "done", "skip" or "profiles"

	swipe            float32 // a trackpad's horizontal scrolling gathered
	swipeAt, turnAt  time.Time
	mode             int // the traffic scene's mode picked, -1 while it shows both in turn
	modeAt           time.Time
	modeFrom, modeTo float32
}

// onbPose is the compass: its hub in window DIPs, its size, and its
// needle's turn in degrees from up.
type onbPose struct{ X, Y, Size, Turn float32 }

// replayOnboarding plays the guide again, from the settings.
func (m *mainUI) replayOnboarding() {
	m.onb.open = true
}

// onbLayout is where the guide's parts go in a window of w×h: the stage
// the scenes draw on, and the top of the words under it. What the eye
// sees as one block, from the dial's top mark above the stage to the
// words' last line, is centered in the window above the line of the
// buttons, which weigh little.
func onbLayout(w, h float32) (stage ui.Rect, copyY float32) {
	sw := min(680, w-96)
	sh := min(max(h*0.42, 210), 330)
	over := sh * 0.2               // the dial and its marks above the stage
	block := over + sh + 66 + 72   // to the words' last line
	top := max(56, (h-40-block)/2) // its middle at the middle of the window but the buttons' last 40
	y := min(top+over, h-76-(block-over))
	stage = ui.Rect{X: (w - sw) / 2, Y: y, W: sw, H: sh}
	return stage, y + sh + 18 + 34 + 14
}

// onbBrand is the sidebar's mark, where the compass comes from and goes.
func (m *mainUI) onbBrand(c *ui.Context, w float32) onbPose {
	bar := c.TitleBar()
	x := float32(33)
	if m.st.UI.CollapseNav {
		x = max(68, bar.Left+9) / 2
	}
	if c.Direction() == ui.RightToLeft {
		x = w - x
	}
	return onbPose{X: x, Y: max(bar.Height, 44) + 20, Size: 34}
}

// ---- Time ----

func oClamp(x float32) float32 { return min(1, max(0, x)) }

func secsOf(d time.Duration) float32 { return float32(d.Seconds()) }

// onbSettle is how much of a swing the needle has left t seconds after it
// was let go: it passes its mark and comes back, less each time.
func onbSettle(t float32) float32 {
	if t <= 0 {
		return 1
	}
	return float32(math.Exp(float64(-t/0.3)) * math.Cos(2*math.Pi*1.25*float64(t)))
}

// wrap180 brings an angle to within half a turn of none.
func wrap180(a float32) float32 {
	a = float32(math.Mod(float64(a)+180, 360))
	if a < 0 {
		a += 360
	}
	return a - 180
}

// glideEase brings the compass to its place, quick to leave and slow to
// arrive.
var glideEase = ui.CubicBezier(0.22, 1, 0.36, 1)

// ---- The view ----

// onboardingView shows the guide over the window on the first run, and
// when the settings play it again.
func (m *mainUI) onboardingView(c *ui.Context) {
	o := &m.onb
	first := m.st.Theme != "" && !m.st.UI.OnboardingSeen // the settings loaded, and the guide not seen
	want := first || o.open
	w, h := c.Size()
	now := c.Now()
	rm := c.Preferences().ReduceMotion
	stage, copyY := onbLayout(w, h)
	if !o.shown {
		if !want || w <= 0 {
			return
		}
		*o = onboarding{open: o.open, shown: true, replay: !first, step: 0, prev: -1, jump: -1, mode: -1, dir: 1, at: now, openAt: now}
		o.from = m.onbScenePose(0, 0, stage, c.Direction() == ui.RightToLeft)
		if o.replay {
			o.from = m.onbBrand(c, w)
		}
		m.onbPrepare(rm)
		if o.replay {
			m.onbScore("fly", rm)
		} else {
			m.onbScore("build", rm)
		}
	}
	o.brand = m.onbBrand(c, w)
	closing := !o.closing.IsZero()
	if closing && secsOf(now.Sub(o.closing)) >= onbClosing {
		*o = onboarding{}
		return
	}
	// Input from the last frame.
	if !closing {
		switch {
		case o.finish != "":
			m.onbFinish(c, o.finish, stage)
		case !want:
			m.onbFinish(c, "skip", stage) // seen elsewhere meanwhile
		case o.jump >= 0:
			m.onbGo(c, o.jump, stage)
		case o.pending != 0:
			m.onbGo(c, o.step+o.pending, stage)
		}
		o.pending, o.jump, o.finish = 0, -1, ""
		closing = !o.closing.IsZero()
	}

	p := m.pal
	rtl := c.Direction() == ui.RightToLeft
	sd := o.dir // the side the next step comes from
	if rtl {
		sd = -sd
	}
	since := secsOf(now.Sub(o.openAt))
	out := float32(0) // how far the guide is gone
	if closing {
		out = oClamp(secsOf(now.Sub(o.closing)) / onbFly)
	}
	// The window behind: hidden from the first frame on the first run;
	// dimmed in as the settings play it again; uncovered as it ends.
	cover := float32(1)
	if o.replay {
		cover = oClamp(since / 0.35)
	}
	if closing {
		cover = min(cover, 1-oClamp((out-0.12)/0.62))
	}
	chrome := 1 - oClamp(out/0.25) // the words and the buttons

	ui.Overlay(c, func() {
		root := ui.Box(c.Key("onboarding")).Absolute().Left(0).Top(0).Width(w).Height(h)
		if closing {
			root.IgnoreInput(true)
		} else {
			// The guide itself holds the keyboard, so that the arrows and
			// Enter turn its pages rather than press a button of it; Tab
			// still reaches them.
			root.Modal().Label(locales.SettingsOnboarding()).Focusable().FocusRing(false).AutoFocus()
		}
		// The pointer leans the stage a little, as if it had depth.
		px, py, over := root.PointerPosition()
		var nx, ny float32
		if over && !rm && w > 0 && h > 0 {
			nx, ny = oClamp(px/w)*2-1, oClamp(py/h)*2-1
		}
		lean := ui.Animation{Duration: 900 * time.Millisecond, Ease: easeOut}
		nx, ny = root.AnimateTo("nx", nx, lean), root.AnimateTo("ny", ny, lean)
		// A trackpad's swipe pulls the stage before it turns the page.
		pull := float32(0)
		if now.Sub(o.swipeAt) < 180*time.Millisecond && !rm {
			pull = -max(-24, min(24, o.swipe*0.3))
		}
		pull = root.AnimateTo("pull", pull, ui.Animation{Spring: ui.SpringOptions{Duration: 380 * time.Millisecond, Bounce: 0.2}})
		if !rm && (since < 3 || closing) {
			c.AnimationFrame()
		}
		root.HandleInput(func(ev ui.InputEvent) bool { return m.onbScroll(ev, rtl) })

		root.Draw(func(pt *ui.Painter, r ui.Rect) {
			pt.Fill(r, p.Theme.Background.Alpha(cover), 0)
		})
		root.Children(func() {
			// Keys: the arrows, Enter, Escape; a swipe's back and forward.
			fwd, back := ui.KeyRight, ui.KeyLeft
			if rtl {
				fwd, back = back, fwd
			}
			next := func() {
				if o.step == onbSteps-1 {
					o.finish = "done"
				} else {
					o.pending = 1
				}
			}
			prev := func() { o.pending = -1 }
			c.OnWindowShortcut(0, fwd, func() { o.pending = 1 })
			c.OnWindowShortcut(0, back, prev)
			c.OnWindowShortcut(0, ui.KeyForward, func() { o.pending = 1 })
			c.OnWindowShortcut(0, ui.KeyBack, prev)
			c.OnWindowShortcut(0, ui.KeyEnter, next)
			c.OnWindowShortcut(0, ui.KeyEscape, func() { o.finish = "skip" })

			// The stage: the dial, the scenes, and the compass.
			st := ui.Box(c.Key("stage")).Absolute().Left(stage.X).Top(stage.Y).Size(stage.W, stage.H)
			st.Translate(pull, 0)
			st.Draw(func(pt *ui.Painter, r ui.Rect) {
				m.onbDraw(pt, r, onbFrame{now: pt.Now(), rm: rm, rtl: rtl, nx: nx, ny: ny, cover: cover, out: out, since: since})
				if !rm || pt.Now().Sub(o.at) < time.Second || closing {
					pt.AnimationFrame()
				}
			})

			// What a scene has besides its drawing.
			props := ui.Row(c.Key("props")).Absolute().Left(0).Width(w).Top(stage.Y + stage.H + 18).Height(34).Justify(ui.Center).AlignItems(ui.Center)
			props.Opacity(chrome).Translate(pull*0.6, 0).ChildExit(ui.Motion{X: -20 * sd})
			props.Children(func() {
				if o.step == 1 && since > 0.05 {
					row := ui.Row(c.Key("modes")).Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 8}, Exit: &ui.Motion{X: -20 * sd}, ExitDuration: 150 * time.Millisecond, ExitDelay: -1,
						Animation: &ui.Animation{Duration: 420 * time.Millisecond, Delay: 260 * time.Millisecond, Ease: easeOut}})
					row.Children(func() {
						t := secsOf(now.Sub(o.at))
						if o.mode < 0 {
							// The drawing changes mode in frames that only paint; this
							// is built again as it does.
							c.After(onbModeNext(t))
						}
						mode := 0
						if m.onbMode(t, now) >= 0.5 {
							mode = 1
						}
						if i := m.segmented(c, "mode", mode, locales.SettingsSystemProxy(), locales.SettingsTun()); i >= 0 {
							k := m.onbMode(t, now)
							o.mode, o.modeAt, o.modeFrom, o.modeTo = i, now, k, float32(i)
						}
					})
				}
			})

			// The words.
			cw := min(560, w-64)
			words := ui.Box(c.Key("copy")).Absolute().Left((w - cw) / 2).Top(copyY).Width(cw).Height(112)
			words.Opacity(chrome).Translate(pull*0.8, 0).ChildExit(ui.Motion{X: -24 * sd})
			words.Children(func() {
				if since < 0.05 {
					return // the next frame, once its box was there, brings them in
				}
				// The words that leave are gone before the new ones come, so that
				// the two never read over each other.
				delay := 170 * time.Millisecond
				if o.step == 0 && now.Sub(o.at) < 200*time.Millisecond {
					delay = 380 * time.Millisecond // after the compass
					if !o.replay {
						delay = 900 * time.Millisecond
					}
				}
				if rm {
					delay = 0
				}
				title, body := onbCopy(o.step)
				in := func(d time.Duration) ui.ElementTransition {
					return ui.ElementTransition{Enter: &ui.Motion{X: 32 * sd}, Animation: &ui.Animation{Duration: 560 * time.Millisecond, Delay: d, Ease: easeOut}}
				}
				col := ui.Column(c.Key(o.step)).Absolute().Left(0).Top(0).Width(cw).AlignItems(ui.Center).Gap(10)
				col.Transition(ui.ElementTransition{Exit: &ui.Motion{X: -24 * sd}, ExitDuration: 150 * time.Millisecond, ExitEase: ui.EaseIn})
				col.Children(func() {
					ui.Text(c, title).FontSize(26).FontWeight(700).TextAlign(ui.Center).Wrap().Transition(in(delay))
					ui.Text(c, body).FontSize(14.5).LineHeight(1.5).TextColor(p.TextMuted).TextAlign(ui.Center).Wrap().MaxWidth(480).Transition(in(delay + 80*time.Millisecond))
				})
			})

			// Skip, and the way to drag the window.
			bar := c.TitleBar()
			end := float32(20)
			if c.Direction() != ui.RightToLeft && bar.Right > 0 {
				end += bar.Right
			}
			top := ui.Row(c.Key("top")).Absolute().Left(0).Top(0).Width(w).Height(max(bar.Height, 44) + 8).PaddingEnd(end).Gap(4).AlignItems(ui.Center).Justify(ui.End).DragWindow()
			top.Opacity(chrome)
			top.Children(func() {
				// The guide's sound, on and off.
				muted := onbMuted()
				snd := m.onbSoundButton(c, muted, rm)
				enterPose(c, snd, ui.Pose{Y: -6, Fade: 1}, ui.Animation{Duration: 500 * time.Millisecond, Delay: m.onbChromeDelay(rm), Ease: easeOut})
				if snd.Clicked() {
					onbMute(!muted)
					if muted {
						m.onbScore(onbSceneScore(o.step), rm)
					}
				}
				last := o.step == onbSteps-1
				skip := m.onbButton(c, "skip", locales.OnbSkip(), "quiet")
				skip.Opacity(skip.AnimateTo("shown", b2f(!last), ui.Animation{Duration: 200 * time.Millisecond, NonMotion: true})).Disabled(last || closing).AccessHidden(last)
				enterPose(c, skip, ui.Pose{Y: -6, Fade: 1}, ui.Animation{Duration: 500 * time.Millisecond, Delay: m.onbChromeDelay(rm), Ease: easeOut})
				if skip.Clicked() {
					o.finish = "skip"
				}
			})

			// Back, where the guide is, and on.
			foot := ui.Row(c.Key("foot")).Absolute().Left(0).Width(w).Top(h-76).Height(76).Padding(0, 28).AlignItems(ui.Center)
			foot.Opacity(chrome)
			enterPose(c, foot, ui.Pose{Y: 10, Fade: 1}, ui.Animation{Duration: 560 * time.Millisecond, Delay: m.onbChromeDelay(rm), Ease: easeOut})
			foot.Children(func() {
				ui.Row(c).Grow(1).Basis(0).Children(func() {
					b := m.onbButton(c, "back", locales.CommonBack(), "")
					b.Opacity(b.AnimateTo("shown", b2f(o.step > 0), ui.Animation{Duration: 200 * time.Millisecond, NonMotion: true})).Disabled(o.step == 0 || closing).AccessHidden(o.step == 0)
					if b.Clicked() {
						o.pending = -1
					}
				})
				m.onbDots(c)
				ui.Row(c).Grow(1).Basis(0).Justify(ui.End).Gap(8).Children(func() {
					last := o.step == onbSteps-1
					if last && len(m.pv.Items) == 0 {
						b := m.onbButton(c, "profiles", locales.OnbAddProfile(), "")
						b.Transition(ui.ElementTransition{Enter: &ui.Motion{X: 12 * sd}, Duration: 300 * time.Millisecond, Ease: easeOut})
						if b.Clicked() {
							o.finish = "profiles"
						}
					}
					label := locales.CommonNext()
					if last {
						label = locales.OnbStart()
					}
					b := m.onbButton(c, "next", label, "primary")
					if b.Clicked() {
						next()
					}
				})
			})
		})
	})
}

// onbChromeDelay holds the buttons back as the guide opens, until the
// compass has come.
func (m *mainUI) onbChromeDelay(rm bool) time.Duration {
	switch {
	case rm:
		return 0
	case m.onb.replay:
		return 300 * time.Millisecond
	}
	return 1000 * time.Millisecond
}

// onbCopy is a step's title and its line.
func onbCopy(step int) (string, string) {
	switch step {
	case 1:
		return locales.OnbTrafficTitle(), locales.OnbTrafficBody()
	case 2:
		return locales.OnbNodeTitle(), locales.OnbNodeBody()
	case 3:
		return locales.OnbHomeTitle(), locales.OnbHomeBody()
	case 4:
		return locales.OnbCheckTitle(), locales.OnbCheckBody()
	case 5:
		return locales.OnbMoreTitle(), locales.OnbMoreBody()
	}
	return locales.OnbWelcomeTitle(), locales.OnbWelcomeBody()
}

// onbButton is one of the guide's buttons: "primary", "quiet" (smaller,
// for Skip) or "" (plain, no fill until the pointer is on it).
func (m *mainUI) onbButton(c *ui.Context, key, label, variant string) ui.Element {
	p := m.pal
	b := ui.ButtonBase(c.Key(key)).Row().Height(38).Padding(0, 18).Radius(12).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Label(label)
	bg, fg := ui.Transparent, p.Text2
	switch variant {
	case "primary":
		bg, fg = p.Accent, p.AccentText
		if b.Hovered() {
			bg = p.AccentHover
		}
		b.MinWidth(112)
	case "quiet":
		b.Height(30).Padding(0, 12).Radius(9)
		fg = p.TextMuted
		if b.Hovered() {
			bg, fg = p.SurfaceHover, p.Text2
		}
	default:
		if b.Hovered() {
			bg = p.SurfaceHover
		}
	}
	s := float32(1)
	if b.Pressed() {
		s = 0.97
	}
	b.Background(bg).AnimatePose(ui.Pose{Scale: s}, pressSpring)
	b.Transition(ui.ElementTransition{Colors: true, Shadows: true, Width: true, Animation: &ui.Animation{Duration: 280 * time.Millisecond, Ease: easeStd}})
	b.Children(func() {
		size, weight := float32(14), 600
		if variant == "quiet" {
			size, weight = 13, 550
		}
		// NoWrap rather than SingleLine: a weighted Arabic label measures
		// narrower than it draws, and SingleLine cuts it short.
		ui.Text(c, label).FontSize(size).FontWeight(weight).TextColor(fg).NoWrap()
	})
	return b
}

// onbDots is where the guide is: a dot a step, the current one drawn out
// long, the soft fill of those passed, each a way to its step.
func (m *mainUI) onbDots(c *ui.Context) {
	p, o := m.pal, &m.onb
	ui.Row(c.Key("dots")).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
		for i := range onbSteps {
			b := ui.ButtonBase(c.Key(i)).Height(24).Padding(0, 4).AlignItems(ui.Center).Justify(ui.Center).
				Label(locales.OnbStep(strconv.Itoa(i+1), strconv.Itoa(onbSteps)))
			col := p.Faint.Alpha(0.6)
			switch {
			case i == o.step:
				col = p.Accent
			case i < o.step:
				col = p.TextMuted.Alpha(0.7)
			case b.Hovered():
				col = p.TextMuted
			}
			width := float32(6)
			if i == o.step {
				width = 24
			}
			b.AccessSelected(i == o.step)
			b.Children(func() {
				dot := ui.Box(c).Height(6).Radius(3)
				dot.Width(dot.AnimateTo("w", width, ui.Animation{Spring: ui.SpringOptions{Duration: 420 * time.Millisecond, Bounce: 0.25}}))
				dot.Background(dot.AnimateColor("c", col, ui.Animation{Duration: 240 * time.Millisecond}))
			})
			if b.Clicked() && i != o.step {
				o.jump = i
			}
		}
	})
}

// onbScroll turns a trackpad's sideways swipe into a step, pulling the
// stage along until it does.
func (m *mainUI) onbScroll(ev ui.InputEvent, rtl bool) bool {
	o := &m.onb
	if ev.Kind != ui.InputScroll || !o.closing.IsZero() {
		return false
	}
	if !ev.Precise || abs32(ev.DX) <= abs32(ev.DY) {
		return true // nothing behind the guide scrolls
	}
	if ev.Momentum || ev.Time.Sub(o.turnAt) < 450*time.Millisecond {
		o.swipe = 0
		return true
	}
	if ev.Time.Sub(o.swipeAt) > 240*time.Millisecond {
		o.swipe = 0
	}
	o.swipe += ev.DX
	o.swipeAt = ev.Time
	if abs32(o.swipe) > 64 {
		d := 1
		if (o.swipe < 0) != rtl {
			d = -1
		}
		o.pending, o.swipe, o.turnAt = d, 0, ev.Time
	}
	return true
}

func abs32(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// onbGo goes to step to.
func (m *mainUI) onbGo(c *ui.Context, to int, stage ui.Rect) {
	o := &m.onb
	to = min(onbSteps-1, max(0, to))
	if to == o.step {
		return
	}
	now := c.Now()
	o.from = m.onbPoseAt(now, stage, c.Direction() == ui.RightToLeft)
	o.prev, o.prevAt = o.step, o.at
	o.dir = 1
	if to < o.step {
		o.dir = -1
	}
	o.step, o.at = to, now
	if to == 1 {
		o.mode = -1
	}
	m.onbScore(onbSceneScore(to), c.Preferences().ReduceMotion)
	c.Invalidate()
}

// onbFinish ends the guide: how, "done", "skip" or "profiles", which goes
// on to add a subscription. The first run's guide is seen once ended.
func (m *mainUI) onbFinish(c *ui.Context, how string, stage ui.Rect) {
	o := &m.onb
	now := c.Now()
	o.from = m.onbPoseAt(now, stage, c.Direction() == ui.RightToLeft)
	o.closing, o.open = now, false
	if !m.st.UI.OnboardingSeen && m.st.Theme != "" {
		m.st.UI.OnboardingSeen = true // at once; the settings confirm
		m.patch(map[string]any{"ui": map[string]any{"onboardingSeen": true, "tourInvites": config.TourInviteLaunches}})
	} else {
		m.endTourInvites() // played from the settings
	}
	if how == "profiles" {
		m.goTo("profiles", "")
	}
	if how != "skip" {
		m.onbChime()
	} else {
		onbHush(150 * time.Millisecond)
	}
	c.Invalidate()
}

// ---- The compass ----

// onbPoseAt is the compass at now: gliding from where it was to its place
// in the scene, along an arc, its needle swinging into its new heading;
// flying home as the guide ends.
func (m *mainUI) onbPoseAt(now time.Time, stage ui.Rect, rtl bool) onbPose {
	o := &m.onb
	var to onbPose
	var k, swing float32
	if !o.closing.IsZero() {
		to = o.brand
		t := secsOf(now.Sub(o.closing)) / onbFly
		k, swing = onbFlyEase(oClamp(t)), secsOf(now.Sub(o.closing))
	} else {
		t := secsOf(now.Sub(o.at))
		to = m.onbScenePose(o.step, t, stage, rtl)
		k, swing = glideEase(oClamp(t/onbGlide)), t
	}
	pose := onbPose{
		X:    o.from.X + (to.X-o.from.X)*k,
		Y:    o.from.Y + (to.Y-o.from.Y)*k,
		Size: o.from.Size + (to.Size-o.from.Size)*k,
	}
	// An arc rather than a line: it bows to the side of its way.
	dx, dy := to.X-o.from.X, to.Y-o.from.Y
	bow := float32(math.Sin(math.Pi*float64(k))) * 0.12
	pose.X -= dy * bow
	pose.Y += dx * bow
	pose.Turn = to.Turn + wrap180(o.from.Turn-to.Turn)*onbSettle(swing)
	return pose
}

// onbScenePose is the compass's place in a scene, t seconds into it.
func (m *mainUI) onbScenePose(step int, t float32, r ui.Rect, rtl bool) onbPose {
	fx := func(f float32) float32 {
		if rtl {
			f = 1 - f
		}
		return r.X + f*r.W
	}
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	fw := float32(1)
	if rtl {
		fw = -1
	}
	switch step {
	case 1: // the hub traffic passes through, facing the world
		return onbPose{X: cx, Y: cy, Size: r.H * 0.3, Turn: 90 * fw}
	case 2: // its needle on the node picked
		return onbPose{X: fx(0.16), Y: cy, Size: r.H * 0.42, Turn: onbNodeTurn(t, r, rtl)}
	case 3: // the mark of the window
		mw := onbMiniWindow(r, rtl)
		return onbPose{X: mw.rail.X + mw.rail.W/2, Y: mw.rail.Y + mw.rail.W*0.62, Size: mw.rail.W * 0.72}
	case 4: // the source of the ripples, its needle on the fastest
		return onbPose{X: cx, Y: cy, Size: r.H * 0.28 * (1 + 0.05*onbPulseBeat(t)), Turn: onbRadarTurn(t, r) * fw}
	case 5:
		return onbPose{X: cx, Y: cy, Size: r.H * 0.34}
	}
	return onbPose{X: cx, Y: cy, Size: r.H * 0.72}
}

// drawCompassBuilt draws the compass coming together, t seconds after it
// began: its facets fly in from around it one after another, the wheel
// draws itself round, and the M lands last. From 0.95 s on it is whole.
func drawCompassBuilt(p *ui.Painter, cx, cy, size, turn, t float32) {
	k := size / compassBox
	at := func(rot float32) ui.Transform {
		return ui.Translation(-compassCX, -compassCY).Then(ui.Rotation(rot)).Then(ui.Scaling(k, k)).Then(ui.Translation(cx, cy))
	}
	ringBlue := ui.Hex("#0283c0")
	// The wheel: a stroke goes round, and the wheel fills in under it.
	trace := easeOut(oClamp((t - 0.05) / 0.55))
	fill := oClamp((t - 0.45) / 0.35)
	if trace > 0 && fill < 1 {
		rb := compass.ring.Bounds()
		rad := rb.W / 2 * 0.95
		ring := (&ui.Path{}).Circle(compassCX, compassCY, rad).Trim(0, trace)
		p.Transform(at(-90), 0, 0, func() { p.StrokePath(ring, rb.W*0.05, ringBlue.Alpha(1-fill)) })
	}
	if fill > 0 {
		s := 0.94 + 0.06*easeOut(fill)
		p.Transform(at(0).Then(ui.Translation(-cx, -cy)).Then(ui.Scaling(s, s)).Then(ui.Translation(cx, cy)), 0, 0, func() {
			p.FillPath(compass.ring, ringBlue.Alpha(fill))
			p.FillPath(compass.inlay, ui.Hex("#fafcfc").Alpha(fill))
		})
	}
	// The facets, each from the side it points to.
	for i, kite := range compass.kites {
		e := oClamp((t - float32(i)*0.05) / 0.5)
		if e <= 0 {
			continue
		}
		q := easeBack(e)
		b := kite.clip.Bounds()
		ux, uy := b.X+b.W/2-compassCX, b.Y+b.H/2-compassCY
		n := float32(math.Hypot(float64(ux), float64(uy)))
		if n > 0 {
			ux, uy = ux/n, uy/n
		}
		off := (1 - q) * 46
		spin := (1 - q) * 30
		if i%2 == 1 {
			spin = -spin
		}
		alpha := oClamp(e * 2.2)
		local := ui.Translation(-(b.X + b.W/2), -(b.Y + b.H/2)).Then(ui.Rotation(spin)).Then(ui.Translation(b.X+b.W/2+ux*off, b.Y+b.H/2+uy*off))
		p.Transform(local.Then(at(turn-compassTilt)), 0, 0, func() {
			p.ClipPath(kite.clip, func() {
				for _, f := range kite.facets {
					p.FillPath(f.path, f.color.Alpha(alpha))
				}
			})
		})
	}
	// The M lands, upright.
	if e := oClamp((t - 0.62) / 0.33); e > 0 {
		s := 1 + 0.35*(1-easeBack(e))
		p.Transform(at(0).Then(ui.Translation(-cx, -cy)).Then(ui.Scaling(s, s)).Then(ui.Translation(cx, cy)), 0, 0, func() {
			p.StrokePath(compass.m, 5.6, ui.Hex("#fafcfc").Alpha(e))
			p.FillPath(compass.m, ui.Hex("#026b9c").Alpha(e))
		})
	}
}

// ---- The stage ----

// onbFrame is what a frame of the stage draws with.
type onbFrame struct {
	now     time.Time
	rm, rtl bool
	nx, ny  float32 // the pointer, -1 to 1 across the window
	cover   float32 // how much the guide covers the window
	out     float32 // how far the guide is gone
	since   float32 // seconds since it opened
}

// onbScene is a scene drawing itself.
type onbScene struct {
	m       *mainUI
	pt      *ui.Painter
	r       ui.Rect
	t       float32 // seconds into the scene
	a       float32 // 1, and less as it leaves
	dx, dy  float32 // where it is moved: leaving, coming, the pointer
	rm, rtl bool
	cp      onbPose // the compass as drawn
	u       float32 // a DIP of a stage 300 high
	fw      float32 // 1, or -1 right to left
	d0      float32 // seconds before its parts come
}

// k is how far a part that comes at delay has come, in dur seconds.
func (s *onbScene) k(delay, dur float32) float32 {
	if s.rm {
		return 1
	}
	return oClamp((s.t - s.d0 - delay) / dur)
}

// f is how far a part has faded in: as k, but also with less motion,
// quickly and all at once.
func (s *onbScene) f(delay, dur float32) float32 {
	if s.rm {
		return oClamp(s.t/0.25) * s.a
	}
	return oClamp((s.t-s.d0-delay)/dur) * s.a
}

func (s *onbScene) x(f float32) float32 {
	if s.rtl {
		f = 1 - f
	}
	return s.r.X + f*s.r.W + s.dx
}

func (s *onbScene) y(f float32) float32 { return s.r.Y + f*s.r.H + s.dy }

func (s *onbScene) circle(x, y, r float32, col ui.Color) {
	if r > 0 {
		s.pt.FillPath((&ui.Path{}).Circle(x, y, r), col)
	}
}

// icon draws an icon popping in at k, centered at x, y.
func (s *onbScene) icon(ic *ui.SVG, x, y, size float32, col ui.Color, k float32) {
	if k <= 0.02 {
		return
	}
	r := ui.Rect{X: x - size/2, Y: y - size/2, W: size, H: size}
	col = col.Alpha(oClamp(k))
	if q := 0.6 + 0.4*k; q < 0.995 || q > 1.005 {
		// At its own size, scaled: see image.
		s.pt.Transform(ui.Scaling(q, q), x, y, func() { s.pt.Icon(ic, r, col) })
		return
	}
	s.pt.Icon(ic, r, col)
}

// image draws a picture growing in at k: pictures do not fade.
func (s *onbScene) image(src ui.ImageSource, r ui.Rect, k float32) {
	if src == nil || k <= 0.05 {
		return
	}
	q := min(k, 1.2)
	if q > 0.995 && q < 1.005 {
		s.pt.Image(src, r, ui.Contain)
		return
	}
	// At its own size, scaled by a transform: a picture drawn at another
	// size every frame of a pop is drawn anew every frame.
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	s.pt.Transform(ui.Scaling(q, q), cx, cy, func() { s.pt.Image(src, r, ui.Contain) })
}

// text draws a line with its start at x and its middle at y.
func (s *onbScene) text(x, y float32, str string, size float32, weight int, col ui.Color, center bool) {
	if col.A == 0 {
		return // a span with no color takes the theme's
	}
	span := ui.Span{Text: str, Size: size, Weight: weight, Color: col}
	w, h := s.pt.MeasureText(0, span)
	if center {
		x -= w / 2
	} else if s.rtl {
		x -= w
	}
	s.pt.RichText(x, y-h/2, 0, span)
}

// onbDraw draws the stage: the dial behind, the scene leaving and the one
// coming, and the compass over them.
func (m *mainUI) onbDraw(pt *ui.Painter, r ui.Rect, f onbFrame) {
	o, p := &m.onb, m.pal
	now := f.now
	t := secsOf(now.Sub(o.at))
	show := f.cover * (1 - oClamp(f.out/0.35)) // the scenes, as the guide opens and ends
	if o.replay && !f.rm {
		show *= oClamp((f.since - 0.2) / 0.35)
	}
	lx, ly := -f.nx*10, -f.ny*6 // the pointer's lean

	m.onbDial(pt, r, now, f, show, lx*0.4, ly*0.4)

	scene := func(step int, t, a, dx float32) {
		s := &onbScene{m: m, pt: pt, r: r, t: t, a: a * show, dx: dx + lx, dy: ly, rm: f.rm, rtl: f.rtl, u: r.H / 300, fw: 1}
		if f.rtl {
			s.fw = -1
		}
		if s.a <= 0.01 {
			return
		}
		s.cp = m.onbPoseAt(now, r, f.rtl)
		s.cp.X += lx
		s.cp.Y += ly
		if step == 0 && step == o.step && o.at.Equal(o.openAt) {
			s.d0 = 0.35
			if !o.replay {
				s.d0 = 1.0
			}
		}
		switch step {
		case 0:
			s.welcome()
		case 1:
			s.traffic()
		case 2:
			s.nodes()
		case 3:
			s.tiles()
		case 4:
			s.radar()
		case 5:
			s.more()
		}
	}
	sd := o.dir
	if f.rtl {
		sd = -sd
	}
	if o.prev >= 0 && t < onbLeave {
		e := oClamp(t / onbLeave)
		dx := float32(0)
		if !f.rm {
			dx = -sd * 28 * r.H / 300 * easeOut(e)
		}
		scene(o.prev, secsOf(now.Sub(o.prevAt)), 1-e, dx)
	}
	if t > onbLeave*0.5 || o.prev < 0 || f.rm {
		dx := float32(0)
		if !f.rm && o.prev >= 0 {
			dx = sd * 28 * r.H / 300 * (1 - easeOut(oClamp((t-onbLeave*0.5)/0.6)))
		}
		a := float32(1)
		if o.prev >= 0 {
			a = oClamp((t - onbLeave*0.5) / 0.3)
			if f.rm {
				a = oClamp(t / 0.25)
			}
		}
		scene(o.step, t, a, dx)
	}

	// The compass.
	cp := m.onbPoseAt(now, r, f.rtl)
	home := f.out
	cp.X += lx * (1 - home)
	cp.Y += ly * (1 - home)
	if f.rm && !o.closing.IsZero() {
		// Less motion: no flight home; it fades where it is.
		if a := 1 - oClamp(f.out/0.5); a > 0.01 {
			drawCompass(pt, o.from.X, o.from.Y, o.from.Size, compassLook{Ring: true, Turn: o.from.Turn, Alpha: a})
		}
		return
	}
	if f.rm && o.prev >= 0 && t < 0.3 {
		// Less motion: it fades from its old place to its new one.
		k := oClamp(t / 0.3)
		to := m.onbScenePose(o.step, t, r, f.rtl)
		if a := 1 - k; a > 0.01 {
			drawCompass(pt, o.from.X, o.from.Y, o.from.Size, compassLook{Ring: true, Turn: o.from.Turn, Alpha: a})
		}
		if k > 0.01 {
			drawCompass(pt, to.X+lx, to.Y+ly, to.Size, compassLook{Ring: true, Turn: to.Turn, Alpha: k})
		}
		return
	}
	if f.rm {
		cp = m.onbScenePose(o.step, t, r, f.rtl)
		if !o.closing.IsZero() {
			cp = o.from
		}
	}
	if !o.closing.IsZero() {
		m.onbLandingDraw(pt, cp, secsOf(now.Sub(o.closing)), o.brand)
		return
	}
	since := secsOf(now.Sub(o.openAt))
	if !o.replay && !f.rm && since < 0.95 {
		drawCompassBuilt(pt, cp.X, cp.Y, cp.Size, cp.Turn, since)
		return
	}
	// Its glints grow once it is whole, twinkle while it rests, and go as
	// it flies home, where the sidebar's mark has none.
	glints := float32(0)
	if o.step == 0 || !o.closing.IsZero() || o.prev == 0 && t < 0.4 {
		g0 := float32(1.0)
		if o.replay {
			g0 = 0.45
		}
		glints = easeBack(oClamp((since - g0) / 0.45))
		if !f.rm {
			glints *= 1 + 0.12*float32(math.Sin(float64(since)*2.6))
		}
		if o.step != 0 && o.prev == 0 {
			glints *= 1 - oClamp(t/0.4)
		}
		if !o.closing.IsZero() {
			glints *= 1 - oClamp(f.out/0.4)
		}
	}
	if f.rm && o.step == 0 {
		glints = 1
	}
	drawCompass(pt, cp.X, cp.Y, cp.Size, compassLook{Ring: true, Turn: cp.Turn, Glints: glints, Glint: p.Spark})
}

// onbDial draws the bearings behind the stage: a ring of ticks that draws
// itself round as the guide opens and turns a mark on with each step, the
// step's own mark coming to the top.
func (m *mainUI) onbDial(pt *ui.Painter, r ui.Rect, now time.Time, f onbFrame, a, lx, ly float32) {
	o, p := &m.onb, m.pal
	if a <= 0.01 {
		return
	}
	cx, cy := r.X+r.W/2+lx, r.Y+r.H/2+ly
	rad := r.H * 0.66
	t := secsOf(now.Sub(o.at))
	// The dial turns a mark a step, the needle's way: past, and back.
	const per = 22 // degrees between the steps' marks
	turn := -float32(o.step) * per
	if !f.rm && o.prev >= 0 {
		turn += float32(o.step-o.prev) * per * onbSettle(t*0.8+0.0001) * (1 - oClamp(t/2))
	}
	drawn := float32(1)
	if !f.rm {
		drawn = easeOut(oClamp((f.since - 0.15) / 1.1))
	}
	tick := p.Faint
	if p.Dark {
		tick = p.BorderStrong
	}
	// The ticks as two paths, short and long: Painter.Line draws only
	// along the axes.
	const n = 120
	var short, long ui.Path
	for i := range n {
		at := float32(i) / n
		if at > drawn {
			break
		}
		deg := at*360 + turn
		path, l := &short, r.H*0.022
		if i%10 == 0 {
			path, l = &long, r.H*0.045
		}
		s, c := math.Sincos(float64(deg) * math.Pi / 180)
		path.MoveTo(cx+float32(s)*rad, cy-float32(c)*rad).LineTo(cx+float32(s)*(rad-l), cy-float32(c)*(rad-l))
	}
	pt.StrokePath(&short, 1, tick.Alpha(0.55*a))
	pt.StrokePath(&long, 1, tick.Alpha(0.9*a))
	// The steps' marks, the one shown in the accent at the top.
	for i := range onbSteps {
		deg := float32(i)*per + turn
		if f.rtl {
			deg = -float32(i)*per - turn
		}
		s, c := math.Sincos(float64(deg) * math.Pi / 180)
		x, y := cx+float32(s)*(rad+r.H*0.035), cy-float32(c)*(rad+r.H*0.035)
		col, size := tick.Alpha(0.9*a*drawn), r.H*0.008
		if i == o.step {
			col, size = p.Accent.Alpha(a*drawn), r.H*0.014
		} else if i < o.step {
			col = p.TextMuted.Alpha(0.6 * a * drawn)
		}
		pt.FillPath((&ui.Path{}).Circle(x, y, size), col)
	}
}

// ---- The scenes ----

// mygoMarks are the five of MyGO!!!!!, in the colors the app's accents
// name after them.
var mygoMarks = []string{"#4fa3d1", "#f07891", "#4fbf6b", "#e8b84a", "#7d7bc0"}

// welcome: the compass alone, the band's five marks circling it.
func (s *onbScene) welcome() {
	cp, u := s.cp, s.u
	for i, hex := range mygoMarks {
		k := s.k(0.1+float32(i)*0.08, 0.5)
		if k <= 0 {
			continue
		}
		q := easeBack(k)
		spin := s.t * 7
		if s.rm {
			spin = 0
		}
		deg := float64(-90+float32(i)*72+spin) * math.Pi / 180
		rad := cp.Size * (0.38 + 0.26*q)
		x, y := cp.X+float32(math.Cos(deg))*rad, cp.Y+float32(math.Sin(deg))*rad
		size := 7 * u * q
		if !s.rm {
			size *= 1 + 0.22*float32(math.Sin(float64(s.t)*2.2+float64(i)*1.3))
		}
		drawSparkle(s.pt, x, y, size, ui.Hex(hex).Alpha(s.f(0.1+float32(i)*0.08, 0.3)))
	}
}

const onbModeStart, onbModePeriod = 1.6, 3.2

// onbModeNext is how long until the traffic scene, t seconds in, has
// shown its next mode.
func onbModeNext(t float32) time.Duration {
	next := float32(onbModeStart + 0.35)
	if t >= onbModeStart {
		n := int((t-onbModeStart)/onbModePeriod) + 1
		next = onbModeStart + float32(n)*onbModePeriod + 0.35
	}
	return time.Duration((next - t) * float32(time.Second))
}

// onbMode is the traffic scene's mode, 0 the system proxy and 1 TUN: as
// picked, or each in turn.
func (m *mainUI) onbMode(t float32, now time.Time) float32 {
	o := &m.onb
	if o.mode >= 0 {
		e := easeStd(oClamp(secsOf(now.Sub(o.modeAt)) / 0.6))
		return o.modeFrom + (o.modeTo-o.modeFrom)*e
	}
	const start, period = onbModeStart, onbModePeriod
	if t < start {
		return 0
	}
	n := int((t - start) / period)
	in := (t - start) - float32(n)*period
	from, to := float32(n%2), float32((n+1)%2)
	return from + (to-from)*easeStd(oClamp(in/0.6))
}

func oLerp(a, b, k float32) float32 { return a + (b-a)*k }

// cubicAt is the point t along a cubic curve.
func cubicAt(p [4][2]float32, t float32) (float32, float32) {
	u := 1 - t
	a, b, c, d := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return a*p[0][0] + b*p[1][0] + c*p[2][0] + d*p[3][0], a*p[0][1] + b*p[1][1] + c*p[2][1] + d*p[3][1]
}

func cubicPath(p [4][2]float32) *ui.Path {
	return (&ui.Path{}).MoveTo(p[0][0], p[0][1]).CubeTo(p[1][0], p[1][1], p[2][0], p[2][1], p[3][0], p[3][1])
}

// traffic: three apps send their traffic through the compass to the
// world. With the system proxy the terminal goes its own way, around; TUN
// bends its line into the compass too. What has passed the compass takes
// its color.
func (s *onbScene) traffic() {
	m, p, u, fw, cp := s.m, s.m.pal, s.u, s.fw, s.cp
	box := s.r.H * 0.17
	ax := s.x(0.1)
	ys := []float32{s.y(0.2), s.y(0.5), s.y(0.8)}
	icons := []*ui.SVG{icGlobe, icAppWindow, icCode2}
	gx, gy, gr := s.x(0.9), s.y(0.5), s.r.H*0.12
	mode := m.onbMode(s.t, s.pt.Now())
	if s.rm {
		mode = float32(math.Round(float64(mode)))
	}
	edge := cp.Size * 0.44
	// The lines.
	routes := make([][2][4][2]float32, 3) // in to the hub, and on to the world
	line := p.BorderStrong
	for i, y := range ys {
		start := [2]float32{ax + fw*(box/2+6*u), y}
		hubIn := [4][2]float32{start, {start[0] + fw*s.r.W*0.16, y}, {cp.X - fw*s.r.W*0.14, cp.Y}, {cp.X - fw*edge, cp.Y}}
		routes[i][0] = hubIn
		routes[i][1] = [4][2]float32{{cp.X + fw*edge, cp.Y}, {cp.X + fw*s.r.W*0.14, cp.Y}, {gx - fw*s.r.W*0.12, gy}, {gx - fw*(gr+5*u), gy}}
		if i == 2 {
			// Around the compass, straight to the world: the terminal with the
			// system proxy.
			around := [4][2]float32{start, {start[0] + fw*s.r.W*0.3, y + s.r.H*0.06}, {gx - fw*s.r.W*0.22, gy + s.r.H*0.26}, {gx - fw*gr*0.7, gy + gr*0.7}}
			for j := range 4 {
				routes[i][0][j][0] = oLerp(around[j][0], hubIn[j][0], mode)
				routes[i][0][j][1] = oLerp(around[j][1], hubIn[j][1], mode)
			}
		}
		k := easeOut(s.k(0.3+float32(i)*0.06, 0.6))
		if k <= 0 {
			continue
		}
		path := cubicPath(routes[i][0]).Trim(0, k)
		if i == 2 && mode < 0.5 {
			path.Dash(-s.t*14*u, 4*u, 5*u)
			s.pt.StrokePath(path, 1.6*u, p.Faint.Alpha(s.a))
		} else {
			s.pt.StrokePath(path, 1.6*u, line.Alpha(s.a))
		}
	}
	if k := easeOut(s.k(0.55, 0.6)); k > 0 {
		s.pt.StrokePath(cubicPath(routes[0][1]).Trim(0, k), 2*u, p.Accent.Alpha(0.55*s.a))
	}
	// The packets.
	if pa := s.f(1.0, 0.4); pa > 0 {
		for i := range 3 {
			for j := range 3 {
				ph := float32(math.Mod(float64(s.t*0.36+float32(j)/3+float32(i)*0.21), 1))
				if s.rm {
					ph = 0.25 + float32(j)*0.3
				}
				var x, y float32
				col := p.TextMuted
				bypass := i == 2 && mode < 0.5
				switch {
				case i == 2 && mode > 0.15 && mode < 0.85:
					continue // its line is bending
				case bypass:
					x, y = cubicAt(routes[i][0], ph)
					col = p.Faint
				case ph < 0.5:
					x, y = cubicAt(routes[i][0], ph/0.5)
				default:
					x, y = cubicAt(routes[i][1], (ph-0.5)/0.5)
					col = p.Accent
				}
				s.circle(x, y, 3.2*u, col.Alpha(pa))
			}
		}
	}
	// The apps, and the world.
	for i, y := range ys {
		k := s.k(0.08+float32(i)*0.07, 0.5)
		if k <= 0 {
			continue
		}
		q := easeBack(k)
		b := box * q
		s.pt.Fill(ui.Rect{X: ax - b/2, Y: y - b/2, W: b, H: b}, p.Card.Alpha(s.a), b*0.28)
		s.icon(icons[i], ax, y, box*0.46, p.Text2.Alpha(s.a), q)
	}
	if k := s.k(0.5, 0.5); k > 0 {
		q := easeBack(k)
		s.circle(gx, gy, gr*q, p.AccentSoft.Alpha(s.a))
		s.icon(icGlobe2, gx, gy, gr*1.05, p.AccentFg.Alpha(s.a), q)
	}
}

// The node scene: six nodes measured, the fastest picked, each time
// round with other delays.
var (
	onbFlags  = []string{"jp", "sg", "us", "de", "gb", "kr"}
	onbDelays = [][]int{{68, 112, 187, 154, 231, 95}, {104, 59, 176, 143, 198, 121}, {91, 133, 162, 0, 207, 48}}
)

const (
	onbNodeStart  = 0.45 // seconds before the first round
	onbNodeRound  = 4.4  // seconds a round
	onbNodePicked = 1.35 // seconds into a round when the fastest is picked
)

// onbNodeTiles are the nodes' tiles in the stage, and the compass's hub.
func onbNodeTiles(r ui.Rect, rtl bool) []ui.Rect {
	x0, x1 := r.X+r.W*0.36, r.X+r.W*0.97
	gap := r.H * 0.045
	tw := (x1 - x0 - 2*gap) / 3
	th := r.H * 0.3
	y0 := r.Y + r.H/2 - th - gap/2
	tiles := make([]ui.Rect, 6)
	for i := range tiles {
		x := x0 + float32(i%3)*(tw+gap)
		if rtl {
			x = r.X + r.W - (x - r.X) - tw
		}
		tiles[i] = ui.Rect{X: x, Y: y0 + float32(i/3)*(th+gap), W: tw, H: th}
	}
	return tiles
}

// onbNodeRoundAt is the round of the node scene t seconds in, and how far
// into it; -1 before the first.
func onbNodeRoundAt(t float32) (int, float32) {
	if t < onbNodeStart {
		return -1, 0
	}
	n := int((t - onbNodeStart) / onbNodeRound)
	return n, t - onbNodeStart - float32(n)*onbNodeRound
}

func onbWinner(round int) int {
	if round < 0 {
		return -1
	}
	d := onbDelays[round%len(onbDelays)]
	best := -1
	for i, v := range d {
		if v > 0 && (best < 0 || v < d[best]) {
			best = i
		}
	}
	return best
}

// onbNodeTurn is the needle in the node scene: on the node picked, and
// swinging to the next one picked.
func onbNodeTurn(t float32, r ui.Rect, rtl bool) float32 {
	tiles := onbNodeTiles(r, rtl)
	hx := r.X + r.W*0.16
	if rtl {
		hx = r.X + r.W*0.84
	}
	hy := r.Y + r.H/2
	angle := func(i int) float32 {
		if i < 0 {
			return 0
		}
		b := tiles[i]
		return float32(math.Atan2(float64(b.X+b.W/2-hx), float64(hy-(b.Y+b.H/2))) * 180 / math.Pi)
	}
	n, in := onbNodeRoundAt(t)
	if n < 0 {
		return 0
	}
	if in < onbNodePicked {
		return angle(onbWinner(n - 1))
	}
	to := angle(onbWinner(n))
	return to + wrap180(angle(onbWinner(n-1))-to)*onbSettle(in-onbNodePicked)
}

func (s *onbScene) nodes() {
	p, u := s.m.pal, s.u
	tiles := onbNodeTiles(s.r, s.rtl)
	for i := range tiles {
		tiles[i].X += s.dx
		tiles[i].Y += s.dy
	}
	n, in := onbNodeRoundAt(s.t)
	if s.rm {
		n, in = 0, onbNodeRound-0.5
	}
	win, was := onbWinner(n), onbWinner(n-1)
	picked := in >= onbNodePicked
	// The fill of the one picked, sliding from the one before.
	sel := -1
	var selR ui.Rect
	switch {
	case picked:
		sel = win
		e := easeStd(oClamp((in - onbNodePicked) / 0.45))
		if was < 0 || s.rm {
			selR = tiles[win]
		} else {
			a, b := tiles[was], tiles[win]
			selR = ui.Rect{X: oLerp(a.X, b.X, e), Y: oLerp(a.Y, b.Y, e), W: b.W, H: b.H}
		}
	case was >= 0:
		sel, selR = was, tiles[was]
	}
	for i, b := range tiles {
		k := s.k(0.12+float32(i)*0.05, 0.5)
		if k <= 0 {
			continue
		}
		q := easeBack(k)
		lift := (1 - q) * 14 * u
		tile := ui.Rect{X: b.X, Y: b.Y + lift, W: b.W, H: b.H}
		s.pt.Fill(tile, p.Card.Alpha(s.a*oClamp(k*2)), 12*u)
		if i == 0 && sel >= 0 {
			s.pt.Fill(ui.Rect{X: selR.X, Y: selR.Y + lift, W: selR.W, H: selR.H}, p.AccentSoft.Alpha(s.a), 12*u)
		}
		pad := 11 * u
		fx := tile.X + pad
		if s.rtl {
			fx = tile.X + tile.W - pad - 24*u
		}
		s.image(svgImage("assets/flags/"+onbFlags[i]+".svg"), ui.Rect{X: fx, Y: tile.Y + pad, W: 24 * u, H: 18 * u}, q*oClamp(s.a*2))
		bw := tile.W * 0.36
		bx := fx + 24*u + 8*u
		if s.rtl {
			bx = fx - 8*u - bw
		}
		s.pt.Fill(ui.Rect{X: bx, Y: tile.Y + pad + 5*u, W: bw, H: 8 * u}, p.Surface3.Alpha(s.a*oClamp(k*2)), 4*u)
		// Its delay, landing once measured.
		ty := tile.Y + tile.H - pad - 8*u
		tx := tile.X + pad
		if s.rtl {
			tx = tile.X + tile.W - pad
		}
		land := 0.35 + float32(i)*0.1
		if n < 0 || in < land {
			// Measuring.
			spin := s.t * 400
			arc := (&ui.Path{}).Circle(0, 0, 5*u).Trim(0, 0.7)
			cx := tx + s.fw*6*u
			s.pt.Transform(ui.Rotation(spin).Then(ui.Translation(cx, ty)), 0, 0, func() {
				s.pt.StrokePath(arc, 1.5*u, p.TextMuted.Alpha(s.a*oClamp(k*2)))
			})
		} else {
			d := onbDelays[n%len(onbDelays)][i]
			e := oClamp((in - land) / 0.5)
			if s.rm {
				e = 1
			}
			str, col := strconv.Itoa(d)+" ms", p.Success
			switch {
			case d == 0:
				str, col = locales.CommonTimeout(), p.Danger
			case d >= 200:
				col = p.Danger
			case d >= 120:
				col = p.Warning
			}
			drop := (1 - easeOut(e)) * -5 * u
			s.text(tx, ty+drop, str, 12*u*(1+0.15*(1-e)), 600, col.Alpha(s.a*(0.2+0.8*e)), false)
		}
		// The check on the one picked.
		if i == sel && picked {
			e := oClamp((in - onbNodePicked - 0.15) / 0.4)
			if s.rm {
				e = 1
			}
			if e > 0 {
				q := easeBack(e)
				cx := tile.X + tile.W - pad - 7*u
				if s.rtl {
					cx = tile.X + pad + 7*u
				}
				cy := tile.Y + pad + 8*u
				s.circle(cx, cy, 8*u*q, p.Accent.Alpha(s.a))
				s.icon(icCheck, cx, cy, 11*u, p.AccentText.Alpha(s.a), q)
			}
		}
	}
	// The bearing from the needle to the one picked.
	if picked && win >= 0 {
		e := easeOut(oClamp((in - onbNodePicked - 0.2) / 0.5))
		if s.rm {
			e = 1
		}
		b := tiles[win]
		tx, ty := b.X+b.W/2, b.Y+b.H/2
		dx, dy := tx-s.cp.X, ty-s.cp.Y
		dist := float32(math.Hypot(float64(dx), float64(dy)))
		if dist > 0 {
			from := s.cp.Size * 0.56
			to := dist - min(b.W, b.H)*0.62
			if to > from {
				path := (&ui.Path{}).MoveTo(s.cp.X+dx/dist*from, s.cp.Y+dy/dist*from).LineTo(s.cp.X+dx/dist*to, s.cp.Y+dy/dist*to)
				path.Dash(0, 2*u, 5*u).Trim(0, e)
				s.pt.StrokePath(path, 1.6*u, p.Accent.Alpha(0.7*s.a))
			}
		}
	}
}

// onbMini is the small window of the tiles scene: its frame, its rail,
// where its tiles go, and the tiles.
type onbMini struct {
	frame, rail, content ui.Rect
	tiles                []ui.Rect
}

func onbMiniWindow(r ui.Rect, rtl bool) onbMini {
	fh := r.H * 0.9
	fwd := min(r.W*0.8, fh*1.7)
	f := ui.Rect{X: r.X + (r.W-fwd)/2, Y: r.Y + (r.H-fh)/2, W: fwd, H: fh}
	railW := fwd * 0.1
	mw := onbMini{frame: f, rail: ui.Rect{X: f.X, Y: f.Y + fh*0.08, W: railW, H: fh * 0.9}}
	pad := fh * 0.05
	mw.content = ui.Rect{X: f.X + railW + pad*0.4, Y: f.Y + fh*0.12, W: fwd - railW - pad*1.4, H: fh*0.88 - pad}
	if rtl {
		mw.rail.X = f.X + f.W - railW
		mw.content.X = f.X + pad
	}
	c, g := mw.content, pad*0.7
	cw, ch := (c.W-2*g)/3, (c.H-g)/2
	cell := func(col, row, span int) ui.Rect {
		x := c.X + float32(col)*(cw+g)
		w := cw*float32(span) + g*float32(span-1)
		if rtl {
			x = c.X + c.W - (x - c.X) - w
		}
		return ui.Rect{X: x, Y: c.Y + float32(row)*(ch+g), W: w, H: ch}
	}
	mw.tiles = []ui.Rect{cell(0, 0, 2), cell(2, 0, 1), cell(0, 1, 1), cell(1, 1, 1), cell(2, 1, 1)}
	return mw
}

// tiles: a small window of the app; a pointer opens a tile, which grows
// to fill the window and goes back to its place, one tile after another.
func (s *onbScene) tiles() {
	p, u := s.m.pal, s.u
	mw := onbMiniWindow(s.r, s.rtl)
	move := func(r ui.Rect) ui.Rect { r.X += s.dx; r.Y += s.dy; return r }
	k := s.k(0, 0.55)
	q := easeOut(k)
	// The frame grows from a little smaller.
	sc := 0.94 + 0.06*q
	f := move(mw.frame)
	cx, cy := f.X+f.W/2, f.Y+f.H/2
	s.pt.Transform(ui.Translation(-cx, -cy).Then(ui.Scaling(sc, sc)).Then(ui.Translation(cx, cy)), 0, 0, func() {
		a := s.a * oClamp(k*1.6)
		s.pt.Fill(f, p.Sidebar.Alpha(a), 14*u)
		s.pt.Stroke(f, p.BorderStrong.Alpha(a), 14*u, 1)
		// Its window buttons, and the places of its rail.
		for i := range 3 {
			x := f.X + 12*u + float32(i)*9*u
			if s.rtl {
				x = f.X + f.W - 12*u - float32(i)*9*u
			}
			s.circle(x, f.Y+11*u, 2.6*u, p.Faint.Alpha(a))
		}
		rail := move(mw.rail)
		for i := range 5 {
			rk := easeOut(s.k(0.25+float32(i)*0.04, 0.4))
			y := rail.Y + rail.W*1.35 + float32(i)*rail.W*0.62
			sz := rail.W * 0.36 * rk
			col := p.Faint.Alpha(0.7 * s.a)
			if i == 0 {
				s.pt.Fill(ui.Rect{X: rail.X + rail.W*0.2, Y: y - rail.W*0.27, W: rail.W * 0.6, H: rail.W * 0.54}, p.AccentSoft.Alpha(s.a*rk), 6*u)
				col = p.AccentFg.Alpha(s.a)
			}
			s.pt.Fill(ui.Rect{X: rail.X + rail.W/2 - sz/2, Y: y - sz/2, W: sz, H: sz}, col.Alpha(rk), 3*u)
		}
		// The tiles, one opening.
		const start, period = 1.0, 4.6
		seq := []int{1, 3, 0}
		open, grow, cursorK := -1, float32(0), float32(1)
		var cur, prevAt [2]float32
		tiles := make([]ui.Rect, len(mw.tiles))
		for i, t := range mw.tiles {
			tiles[i] = move(t)
		}
		center := func(r ui.Rect) [2]float32 { return [2]float32{r.X + r.W*0.55, r.Y + r.H*0.6} }
		cont := move(mw.content)
		rest := [2]float32{cont.X + cont.W*0.92, cont.Y + cont.H*1.05}
		if s.rtl {
			rest[0] = cont.X + cont.W*0.08
		}
		pressed := false
		if s.rm {
			// Less motion: one tile open, still.
			open, grow, cursorK = 1, 1, 0
		} else if s.t >= start {
			n := int((s.t - start) / period)
			in := s.t - start - float32(n)*period
			open = seq[n%len(seq)]
			prevAt = rest
			if n > 0 {
				prevAt = center(tiles[seq[(n-1)%len(seq)]])
			}
			to := center(tiles[open])
			e := easeStd(oClamp(in / 0.6))
			cur = [2]float32{oLerp(prevAt[0], to[0], e), oLerp(prevAt[1], to[1], e) - float32(math.Sin(math.Pi*float64(e)))*12*u}
			pressed = in > 0.6 && in < 0.8
			switch {
			case in < 0.75:
				grow = 0
			case in < 1.35:
				grow = easeStd((in - 0.75) / 0.6)
			case in < 3.3:
				grow = 1
			case in < 3.9:
				grow = 1 - easeStd((in-3.3)/0.6)
			}
			cursorK = 1 - oClamp((in-0.85)/0.2) + oClamp((in-3.6)/0.3)
			cursorK = oClamp(cursorK)
		} else {
			cur = rest
			cursorK = oClamp((s.t - 0.6) / 0.3)
		}
		for i, t := range tiles {
			tk := easeBack(s.k(0.3+float32(i)*0.06, 0.5))
			if tk <= 0 {
				continue
			}
			a := s.a * oClamp(tk*2)
			if open >= 0 && i != open {
				a *= 1 - 0.55*grow
			}
			r := t
			if i == open && pressed {
				r = ui.Rect{X: t.X + t.W*0.02, Y: t.Y + t.H*0.02, W: t.W * 0.96, H: t.H * 0.96}
			}
			sz := 0.85 + 0.15*tk
			r = ui.Rect{X: r.X + r.W*(1-sz)/2, Y: r.Y + r.H*(1-sz)/2, W: r.W * sz, H: r.H * sz}
			s.pt.Fill(r, p.Card.Alpha(a), 9*u)
			s.miniTile(i, r, a, 0)
		}
		if open >= 0 && grow > 0 {
			t := tiles[open]
			r := ui.Rect{X: oLerp(t.X, cont.X, grow), Y: oLerp(t.Y, cont.Y, grow), W: oLerp(t.W, cont.W, grow), H: oLerp(t.H, cont.H, grow)}
			s.pt.Shadow(r, 9*u, 0, 6*u, 18*u, 0, ui.RGBA(0, 0, 0, 0.16*grow*s.a))
			s.pt.Fill(r, p.Card.Alpha(s.a), 9*u)
			s.miniTile(open, r, s.a, grow)
		}
		// The pointer.
		if cursorK > 0.01 && !s.rm {
			a := s.a * cursorK * oClamp((s.t-0.6)/0.3)
			fill := p.Text.Alpha(0.12 * a)
			if pressed {
				fill = p.Text.Alpha(0.3 * a)
			}
			s.circle(cur[0], cur[1], 7*u, fill)
			s.pt.StrokePath((&ui.Path{}).Circle(cur[0], cur[1], 7*u), 1.5*u, p.Text.Alpha(0.7*a))
		}
	})
}

// miniTile draws what a tile of the small window holds, more of it as it
// grows open.
func (s *onbScene) miniTile(i int, r ui.Rect, a, grow float32) {
	p, u := s.m.pal, s.u
	pad := 8 * u
	x := r.X + pad
	bar := func(y, w, h float32, col ui.Color) {
		bx := x
		if s.rtl {
			bx = r.X + r.W - pad - w
		}
		s.pt.Fill(ui.Rect{X: bx, Y: y, W: w, H: h}, col, h/2)
	}
	bar(r.Y+pad, min(r.W*0.3, 40*u), 5*u, p.Faint.Alpha(0.8*a))
	if i == 0 || i == 1 {
		// A line of traffic, drawing itself.
		k := float32(1)
		if grow > 0 {
			k = oClamp((grow - 0.3) / 0.7)
		}
		path := &ui.Path{}
		gx, gy, gw, gh := r.X+pad, r.Y+r.H*0.45, r.W-2*pad, r.H*0.4
		for j := 0; j <= 16; j++ {
			fx := float32(j) / 16
			v := 0.5 + 0.35*float32(math.Sin(float64(fx)*7+float64(i)*2+float64(s.t)*0.8*b2fNot(s.rm)))
			px, py := gx+fx*gw, gy+gh*(1-v)
			if j == 0 {
				path.MoveTo(px, py)
			} else {
				path.LineTo(px, py)
			}
		}
		s.pt.StrokePath(path.Trim(0, k), 1.5*u, p.Accent.Alpha(a))
	} else {
		bar(r.Y+r.H*0.5, min(r.W*0.5, 60*u), 9*u, p.Surface3.Alpha(a))
	}
	// Opened: the rows of its page come in one after another.
	if grow > 0.5 {
		for j := range 4 {
			e := oClamp((grow - 0.55 - float32(j)*0.08) / 0.3)
			if e <= 0 {
				continue
			}
			y := r.Y + r.H*0.18 + float32(j)*12*u
			if i == 0 || i == 1 {
				y = r.Y + r.H*0.9 - float32(4-j)*10*u
			}
			bar(y, r.W*(0.5-0.07*float32(j))*easeOut(e), 5*u, p.Surface3.Alpha(a*e))
		}
	}
}

func b2fNot(b bool) float64 {
	if b {
		return 0
	}
	return 1
}

// The connectivity scene: the compass sends out ripples, and the services
// around it answer as a ripple reaches them, each as far from it as it is
// slow; the nearest, the fastest, answers first, and the needle turns to
// it. The rings are the delays of 50, 100 and 150 ms.
var onbSites = []struct {
	id    string
	deg   float32
	delay int
}{
	// The slow, far ones to the sides, where the stage is wide.
	{"youtube", 32, 62}, {"netflix", 92, 86}, {"google", 145, 38},
	{"telegram", 205, 91}, {"discord", 268, 144}, {"spotify", 330, 73},
}

const (
	onbPulseStart  = 0.45 // seconds before the first ripple
	onbPulseFirst  = 2.4  // seconds the first ripple takes to go out
	onbPulseLater  = 2.2  // and those after it, fainter
	onbPulseEvery  = 3.0
	onbRippleReach = 0.5 // of the stage's height, where a ripple is gone
)

// onbDelayRadius is how far from the compass, in stage heights, a service
// of delay ms is.
func onbDelayRadius(delay int) float32 {
	return 0.18 + 0.0018*float32(delay) // 50 ms: 0.27, 150 ms: 0.45
}

// onbPulse is the k-th ripple: when it leaves, and how long it goes.
func onbPulse(k int) (at, dur float32) {
	if k == 0 {
		return onbPulseStart, onbPulseFirst
	}
	return onbPulseStart + float32(k)*onbPulseEvery, onbPulseLater
}

// onbRippleAt is how far, in stage heights, a ripple is f of its way out:
// quick to leave, slowing as it spreads, as a ripple on water.
func onbRippleAt(f float32) float32 { return onbRippleReach * (1 - (1-f)*(1-f)) }

// onbAnswer is when the k-th ripple reaches a service of delay ms.
func onbAnswer(k, delay int) float32 {
	at, dur := onbPulse(k)
	x := float64(onbDelayRadius(delay) / onbRippleReach)
	return at + dur*float32(1-math.Sqrt(1-x))
}

// onbPulseBeat is the compass's small swell as it sends a ripple out.
func onbPulseBeat(t float32) float32 {
	if t < onbPulseStart {
		return 0
	}
	in := float32(math.Mod(float64(t-onbPulseStart), onbPulseEvery))
	if in > 0.5 {
		return 0
	}
	return float32(math.Sin(math.Pi * float64(in/0.5)))
}

// onbFastest is the service that answers first.
func onbFastest() int {
	best := 0
	for i, s := range onbSites {
		if s.delay < onbSites[best].delay {
			best = i
		}
	}
	return best
}

// onbRadarTurn is the needle: up until the fastest answers, then on it,
// swinging into place.
func onbRadarTurn(t float32, r ui.Rect) float32 {
	f := onbFastest()
	at := onbAnswer(0, onbSites[f].delay)
	if t < at {
		return 0
	}
	to := onbSites[f].deg
	return to + wrap180(0-to)*onbSettle(t-at)
}

// radarAt is the point at deg from up, rad from the middle.
func (s *onbScene) radarAt(cx, cy, deg, rad float32) (float32, float32) {
	sn, cs := math.Sincos(float64(deg) * math.Pi / 180)
	return cx + float32(sn)*s.fw*rad, cy - float32(cs)*rad
}

func (s *onbScene) radar() {
	p, u, cp := s.m.pal, s.u, s.cp
	cx, cy := s.x(0.5), s.y(0.5)
	H := s.r.H
	// The delays' rings, drawing themselves round from the top.
	for i, ms := range []int{50, 100, 150} {
		k := easeOut(s.k(0.05+float32(i)*0.08, 0.8))
		if k <= 0 {
			continue
		}
		ring := (&ui.Path{}).Circle(cx, cy, H*onbDelayRadius(ms)).Trim(0, k)
		s.pt.Transform(ui.Rotation(-90), cx, cy, func() { s.pt.StrokePath(ring, 1, p.Border.Alpha(s.a)) })
	}
	// The ripples: a ring and a fainter one just behind it, thinning and
	// fading as they spread.
	if !s.rm {
		for k := 0; ; k++ {
			at, dur := onbPulse(k)
			if at > s.t {
				break
			}
			f := (s.t - at) / dur
			if f >= 1 {
				continue
			}
			strength := float32(1)
			if k > 0 {
				strength = 0.55
			}
			for j, lag := range []float32{0, 0.07} {
				g := f - lag
				if g <= 0 {
					continue
				}
				rad := max(cp.Size*0.5, H*onbRippleAt(g))
				a := s.a * strength * 0.6 * float32(math.Pow(float64(1-g), 1.6))
				w := (1.8 - 1.0*g) * u
				if j == 1 {
					a *= 0.45
					w *= 0.7
				}
				s.pt.StrokePath((&ui.Path{}).Circle(cx, cy, rad), w, p.Accent.Alpha(a))
			}
		}
	}
	// The services.
	plate := H * 0.072
	for i, site := range onbSites {
		x, y := s.radarAt(cx, cy, site.deg, H*onbDelayRadius(site.delay))
		answer := onbAnswer(0, site.delay)
		if s.rm {
			answer = 0
		}
		if s.t < answer {
			// Not yet reached: a point where it will answer.
			s.circle(x, y, 2.4*u, p.Faint.Alpha(s.f(0.2+float32(i)*0.04, 0.3)))
			continue
		}
		e := oClamp((s.t - answer) / 0.55)
		if s.rm {
			e = 1
		}
		q := easeBack(e)
		// The answer: a ring goes out from it each time a ripple reaches it,
		// and the first time a line flashes back to the compass.
		if !s.rm {
			for k := 0; ; k++ {
				at := onbAnswer(k, site.delay)
				if at > s.t {
					break
				}
				g := (s.t - at) / 0.8
				if g >= 1 {
					continue
				}
				strength := float32(1)
				if k > 0 {
					strength = 0.6
				}
				ra := s.a * strength * 0.55 * (1 - g) * (1 - g)
				s.pt.StrokePath((&ui.Path{}).Circle(x, y, plate*(1+1.1*easeOut(g))), 1.3*u, p.Accent.Alpha(ra))
			}
			if g := (s.t - answer) / 0.6; g < 1 {
				dx, dy := x-cx, y-cy
				d := float32(math.Hypot(float64(dx), float64(dy)))
				if d > 0 {
					from, to := cp.Size*0.55, d-plate*1.1
					line := (&ui.Path{}).MoveTo(cx+dx/d*from, cy+dy/d*from).LineTo(cx+dx/d*to, cy+dy/d*to)
					// It runs in from the service to the compass, and is gone.
					line.Trim(oClamp(g*1.4-0.4), oClamp(g*1.6))
					s.pt.StrokePath(line, 1.4*u, p.Accent.Alpha(s.a*0.7*(1-g)))
				}
			}
		}
		s.circle(x, y, plate*q, p.Card.Alpha(s.a))
		var src ui.ImageSource
		if sv := svgImage("assets/logos/" + site.id + ".svg"); sv != nil {
			src = sv
		}
		ls := plate * 1.15
		s.image(src, ui.Rect{X: x - ls/2, Y: y - ls/2, W: ls, H: ls}, q*oClamp(s.a*2))
		// Its delay counts up to what it took.
		col := p.Success
		if site.delay >= 120 {
			col = p.Warning
		}
		n := site.delay
		if c := easeOut(oClamp((s.t - answer - 0.05) / 0.5)); c < 1 && !s.rm {
			n = int(math.Round(float64(c) * float64(site.delay)))
		}
		s.text(x, y+plate+9*u, strconv.Itoa(n)+" ms", 10.5*u, 600, col.Alpha(s.a*oClamp(e*2)), true)
	}
}

// more: Tailscale's mesh of devices on one side, sync's devices and the
// cloud between them on the other.
func (s *onbScene) more() {
	p, u := s.m.pal, s.u
	// Tailscale.
	mx, my, mr := s.x(0.17), s.y(0.46), s.r.H*0.2
	nodes := [4][2]float32{{mx, my - mr}, {mx + mr*1.05, my}, {mx, my + mr}, {mx - mr*1.05, my}}
	icons := []*ui.SVG{icLaptop, icServer, icSmartphone, icMonitor}
	pairs := [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}, {0, 2}, {1, 3}}
	for j, pr := range pairs {
		k := easeOut(s.k(0.35+float32(j)*0.07, 0.5))
		if k <= 0 {
			continue
		}
		a, b := nodes[pr[0]], nodes[pr[1]]
		path := (&ui.Path{}).MoveTo(a[0], a[1]).LineTo(b[0], b[1]).Trim(0, k)
		s.pt.StrokePath(path, 1.4*u, p.BorderStrong.Alpha(s.a))
	}
	if pa := s.f(1.1, 0.4); pa > 0 {
		for j := range 2 {
			ph := float32(math.Mod(float64(s.t*0.5+float32(j)*0.5), 1))
			if s.rm {
				ph = 0.5
			}
			pr := pairs[(int(s.t*0.5+float32(j)*0.5)+j*3)%len(pairs)]
			a, b := nodes[pr[0]], nodes[pr[1]]
			s.circle(oLerp(a[0], b[0], ph), oLerp(a[1], b[1], ph), 3*u, p.Accent.Alpha(pa))
		}
	}
	for i, n := range nodes {
		k := s.k(0.1+float32(i)*0.07, 0.5)
		if k <= 0 {
			continue
		}
		q := easeBack(k)
		s.circle(n[0], n[1], s.r.H*0.07*q, p.Card.Alpha(s.a))
		s.icon(icons[i], n[0], n[1], s.r.H*0.07, p.Text2.Alpha(s.a), q)
	}
	s.text(mx, my+mr+s.r.H*0.15, "Tailscale", 12.5*u, 600, p.TextMuted.Alpha(s.f(0.5, 0.4)), true)

	// Sync.
	sx, sy := s.x(0.83), s.y(0.46)
	cloud := [2]float32{sx, sy - s.r.H*0.15}
	devs := [2][2]float32{{sx - s.r.H*0.13*s.fw, sy + s.r.H*0.16}, {sx + s.r.H*0.13*s.fw, sy + s.r.H*0.16}}
	for j, d := range devs {
		k := easeOut(s.k(0.45+float32(j)*0.1, 0.5))
		if k <= 0 {
			continue
		}
		// Up from one, down to the other.
		path := (&ui.Path{}).MoveTo(d[0], d[1]).QuadTo(d[0], cloud[1], cloud[0], cloud[1])
		off := s.t * 16 * u
		if j == 1 {
			off = -off
		}
		if s.rm {
			off = 0
		}
		path.Dash(off, 3*u, 4*u).Trim(0, k)
		s.pt.StrokePath(path, 1.5*u, p.Accent.Alpha(0.7*s.a))
	}
	if k := s.k(0.2, 0.5); k > 0 {
		q := easeBack(k)
		// The accent's soft fill, made solid on the ground, so that the
		// lines going into the cloud stop at its edge rather than show
		// through it.
		s.circle(cloud[0], cloud[1], s.r.H*0.09*q, p.Theme.Background.Mix(p.Accent, 0.2).Alpha(s.a))
		s.icon(icCloud, cloud[0], cloud[1], s.r.H*0.095, p.AccentFg.Alpha(s.a), q)
	}
	for j, d := range devs {
		k := s.k(0.3+float32(j)*0.08, 0.5)
		if k <= 0 {
			continue
		}
		q := easeBack(k)
		s.circle(d[0], d[1], s.r.H*0.065*q, p.Card.Alpha(s.a))
		ic := icLaptop
		if j == 1 {
			ic = icSmartphone
		}
		s.icon(ic, d[0], d[1], s.r.H*0.065, p.Text2.Alpha(s.a), q)
	}
	// The lock: what goes through the cloud is sealed.
	if k := s.k(0.95, 0.45); k > 0 {
		q := easeBack(k)
		lx, ly := cloud[0]+s.r.H*0.065*s.fw, cloud[1]+s.r.H*0.06
		s.circle(lx, ly, s.r.H*0.038*q, p.Card.Alpha(s.a))
		s.icon(icLock, lx, ly, s.r.H*0.045, p.Accent2.Alpha(s.a), q)
	}
	s.text(sx, my+mr+s.r.H*0.15, "WebDAV", 12.5*u, 600, p.TextMuted.Alpha(s.f(0.6, 0.4)), true)
}
