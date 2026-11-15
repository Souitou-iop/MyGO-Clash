package app

import (
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
)

// The motion of the main window, as the pages had it: the curves, a press
// that gives under the pointer, dialogs that pop in and out, panels that
// fold, and delays that land.

var (
	// easeStd folds, reflows and slides: panels, the sidebar, a group's
	// chevron.
	easeStd = ui.CubicBezier(0.3, 0.7, 0.2, 1)
	// easeOut brings things in: a page rising, a dialog popping, a delay
	// landing.
	easeOut = ui.CubicBezier(0.2, 0.8, 0.3, 1)
	// easeBack overshoots a little: a bar growing, a node picked.
	easeBack = ui.CubicBezier(0.2, 0.8, 0.3, 1.2)

	// hoverFade fades the colors and shadows of what the pointer is over,
	// and nothing else, so that a button does not drift as the layout
	// moves.
	hoverFade = ui.ElementTransition{Colors: true, Shadows: true, Duration: 120 * time.Millisecond}
	// pageIn rises a page in as it comes; labelIn fades a label in as the
	// sidebar opens, once it has room for it.
	pageIn  = ui.ElementTransition{Enter: &ui.Motion{Y: 6}, Colors: true, Duration: 200 * time.Millisecond, Ease: easeOut}
	labelIn = ui.ElementTransition{Enter: &ui.Motion{}, Colors: true, Animation: &ui.Animation{Duration: 200 * time.Millisecond, Delay: 100 * time.Millisecond}}
	// tabIn rises a settings tab in: its scroll, which each tab has anew.
	tabIn = ui.ElementTransition{Enter: &ui.Motion{Y: 4}, Colors: true, Duration: 200 * time.Millisecond}
	// reflow glides the cards of a grid to their places, and grows the
	// new ones in.
	reflow = ui.ElementTransition{Position: true, Size: true, Animation: &ui.Animation{Duration: 320 * time.Millisecond, Ease: easeStd}, Enter: &ui.Motion{Scale: 0.97}}
	// tileMotion fades a node's colors and glides it as its group is
	// sorted again.
	tileMotion = ui.ElementTransition{Position: true, Colors: true, Shadows: true, Animation: &ui.Animation{Duration: 320 * time.Millisecond, Ease: easeStd}}
	// numRoll rolls the digits of a number that change.
	numRoll = ui.ContentTransition{Numeric: true}
	// noticeIn grows a notice in where it shows, and folds it away as it
	// goes: a banner, a line of what went wrong.
	noticeIn = ui.ElementTransition{Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true}, Colors: true, Animation: &ui.Animation{Duration: 220 * time.Millisecond, Ease: easeStd}}
	// checkIn pops a check mark in as its item is chosen.
	checkIn = ui.ElementTransition{Enter: &ui.Motion{Scale: 0.4}, Exit: &ui.Motion{Scale: 0.4}, Animation: &ui.Animation{Spring: ui.SpringOptions{Duration: 350 * time.Millisecond, Bounce: 0.35}}}
	// pressSpring scales a button pressed and let go.
	pressSpring = ui.Animation{Spring: ui.SpringOptions{Stiffness: 600, Damping: 32}}
)

// pressable gives b the feel of a button: its colors fade as the pointer
// comes and goes, and it gives under a press.
func pressable(b ui.Element) ui.Element {
	s := float32(1)
	if b.Pressed() {
		s = 0.97
	}
	return b.AnimatePose(ui.Pose{Scale: s}, pressSpring).Transition(hoverFade)
}

// enterPose poses e at from in the first frame it is built and at its
// place after, so that it comes in from there (AnimatePose shows the first
// pose at once).
func enterPose(c *ui.Context, e ui.Element, from ui.Pose, a ui.Animation) {
	entered := ui.Local(e, "entered", func() bool { return false })
	p := ui.Pose{}
	if !*entered {
		p, *entered = from, true
		c.Invalidate()
	}
	e.AnimatePose(p, a)
}

// modalState is where a dialog of the main window is in its coming and
// going.
type modalState struct {
	frame   uint64    // the last frame that built it
	shown   bool      // open in that frame
	closing time.Time // when it closed, while it pops out
}

const modalOut = 140 * time.Millisecond

// modal is ui.Modal with the pages' motion: the window dims in, and the
// dialog pops in from a little below; it pops out as it closes, while its
// caller keeps calling modal (those that stop calling it once closed lose
// only the way out).
func (m *mainUI) modal(c *ui.Context, key any, open *bool, fn func()) ui.Element {
	if m.modals == nil {
		m.modals = map[any]*modalState{}
	}
	st := m.modals[key]
	if st == nil || st.frame+1 < m.frame {
		st = &modalState{}
		m.modals[key] = st
	}
	st.frame = m.frame
	closing := false
	switch {
	case *open:
		st.shown, st.closing = true, time.Time{}
	case st.shown:
		st.shown, st.closing = false, c.Now()
		closing = true
	case !st.closing.IsZero() && c.Now().Sub(st.closing) < modalOut:
		closing = true
	default:
		return ui.Element{}
	}
	shown := true
	ptr := open
	if closing {
		ptr = &shown
		c.After(modalOut - c.Now().Sub(st.closing))
	}
	p := m.pal
	w, h := c.Size()
	return ui.DialogBase(c.Key(key), ptr, func(back, panel ui.Element) {
		target := float32(1)
		// The backdrop fades from clear: its first target is where it
		// starts.
		if first := ui.Local(back, "first", func() bool { return true }); *first || closing {
			*first, target = false, 0
			c.Invalidate()
		}
		fade := back.AnimateTo("fade", target, ui.Animation{Duration: modalOut, NonMotion: true})
		back.Background(ui.RGBA(0, 0, 0, 0.4*fade))
		panel.Padding(20).Gap(12).Radius(10).Background(p.Theme.Background).
			MaxWidth(w-40).MaxHeight(h-40).
			Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.3))
		if closing {
			back.IgnoreInput(true)
			panel.AnimatePose(ui.Pose{Y: 4, Scale: 0.985, Fade: 1}, ui.Animation{Duration: modalOut, Ease: ui.EaseIn})
		} else {
			enterPose(c, panel, ui.Pose{Y: 6, Scale: 0.985, Fade: 1}, ui.Animation{Duration: 180 * time.Millisecond, Ease: easeOut})
		}
		fn()
	})
}

// collapseState is where a folding panel is in its going.
type collapseState struct {
	frame   uint64
	open    bool
	closing time.Time
}

const collapseDur = 260 * time.Millisecond

// collapse shows fn's elements while open, growing in and folding away
// as open changes, with the content fading, and the elements after it
// following: a row of settings that a switch turns on.
func (m *mainUI) collapse(c *ui.Context, key string, open bool, fn func()) {
	m.fold(c, key, open, false, fn)
}

// collapseInline is a collapse along a row: what a switch shows beside
// itself, as the minutes it waits, growing in from nothing wide.
func (m *mainUI) collapseInline(c *ui.Context, key string, open bool, fn func()) {
	m.fold(c, key, open, true, fn)
}

func (m *mainUI) fold(c *ui.Context, key string, open, inline bool, fn func()) {
	if m.collapses == nil {
		m.collapses = map[string]*collapseState{}
	}
	st := m.collapses[key]
	fresh := st == nil || st.frame+1 < m.frame
	if fresh {
		st = &collapseState{open: open}
		m.collapses[key] = st
	}
	st.frame = m.frame
	switch {
	case open:
		st.closing = time.Time{}
	case st.open:
		st.closing = c.Now()
	}
	st.open = open
	if !open && (st.closing.IsZero() || c.Now().Sub(st.closing) >= collapseDur) {
		return
	}
	a := ui.Animation{Duration: collapseDur, Ease: easeStd}
	var box ui.Element
	if inline {
		box = ui.Row(c.Key("collapse-" + key)).Gap(8).AlignItems(ui.Center).Clip().Shrink(0)
		box.Transition(ui.ElementTransition{Enter: &ui.Motion{Collapse: true}, Animation: &a, Width: true})
	} else {
		box = ui.Column(c.Key("collapse-" + key)).Gap(12).Clip()
		box.Transition(ui.ElementTransition{Enter: &ui.Motion{Collapse: true}, Animation: &a, Height: true})
	}
	if !open {
		// Folding: the panel shrinks to nothing as its content fades, and
		// goes once it is there.
		c.After(collapseDur - c.Now().Sub(st.closing))
		if inline {
			box.Width(0)
		} else {
			box.Height(0)
		}
		box.IgnoreInput(true)
	}
	box.Opacity(box.AnimateTo("fade", b2f(open), ui.Animation{Duration: collapseDur, NonMotion: true}))
	box.Children(fn)
}

// collapseRows is a collapse among the rows of a settings box, with the
// hairlines between its own rows.
func (m *mainUI) collapseRows(c *ui.Context, key string, open bool, fn func()) {
	m.collapse(c, key, open, func() { ui.Column(c).Dividers(1, m.pal.Border).Children(fn) })
}

// pickPose gives a node's tile its press, the fade of its colors, and a
// small pop as it becomes the one chosen.
func pickPose(c *ui.Context, tile ui.Element, active, list, settled bool) {
	was := ui.Local(tile, "active", func() bool { return active })
	switch {
	case active && !*was:
		s := float32(0.96)
		if list {
			s = 0.98
		}
		tile.AnimatePose(ui.Pose{Scale: s}, ui.Animation{Duration: time.Millisecond})
		c.Invalidate()
	case tile.Pressed():
		tile.AnimatePose(ui.Pose{Scale: 0.97}, pressSpring)
	default:
		tile.AnimatePose(ui.Pose{}, ui.Animation{Spring: ui.SpringOptions{Duration: 450 * time.Millisecond, Bounce: 0.35}})
	}
	*was = active
	if settled {
		tile.Transition(tileMotion)
	} else {
		tile.Transition(hoverFade)
	}
}

// freshState notices a delay landing.
type freshState struct {
	d       int
	loading bool
	run     uint64
}

// freshDelay plays the landing of a delay measured: the number drops into
// place from a little above, brighter and larger, as it is measured, or
// as it changes.
func freshDelay(e ui.Element, d int, loading bool) {
	st := ui.Local(e, "fresh", func() freshState { return freshState{d: d, loading: loading} })
	if !loading && (st.loading || st.d != d) {
		st.run++
	}
	st.d, st.loading = d, loading
	if st.run == 0 || loading {
		return
	}
	k := e.Keyframes("fresh", []ui.Keyframe{{Value: 0}, {At: 600 * time.Millisecond, Value: 1, Ease: easeOut}}, ui.KeyframeOptions{Run: st.run})
	s := 1 + 0.15*(1-k)
	e.Opacity(0.2+0.8*k).Translate(0, -4*(1-k)).Scale(s, s)
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// toasts shows the window's toasts in the app's look: each slides in from
// the side, and slides out and folds as it goes, the others closing up.
func (m *mainUI) toasts(c *ui.Context) {
	p := m.pal
	dir := float32(1)
	if c.Direction() == ui.RightToLeft {
		dir = -1
	}
	motion := ui.ElementTransition{
		Enter: &ui.Motion{X: 16 * dir}, Exit: &ui.Motion{X: 16 * dir, Collapse: true},
		Position: true, Duration: 200 * time.Millisecond, ExitDuration: 260 * time.Millisecond, Ease: easeOut,
	}
	ui.ToastViewportBase(c, func(viewport ui.Element, toasts []ui.Toast) {
		viewport.Padding(16).AlignItems(ui.End).Gap(8)
		for _, t := range toasts {
			toast := ui.ToastBase(c, t)
			icon, col := icInfo, p.AccentFg
			switch t.Type {
			case "success":
				icon, col = icCircleCheck, p.Success
			case "warning":
				icon, col = icAlertTriangle, p.Warning
			case "error":
				icon, col = icAlertTriangle, p.Danger
			}
			toast.Root.Row().Gap(10).Padding(10, 12).Width(340).Radius(14).Background(p.Card).
				AlignItems(ui.Start).Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18)).Transition(motion)
			toast.Root.Children(func() {
				ui.Icon(c, icon).FontSize(16).TextColor(col).Shrink(0)
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
					ui.Text(c, t.Title).FontSize(13).FontWeight(600).Wrap()
					if t.Description != "" {
						ui.Text(c, t.Description).FontSize(12).TextColor(p.TextMuted).Wrap()
					}
				})
				if t.Action != "" {
					act := toast.ActionButton().Height(26).Padding(0, 10).Radius(6).Background(p.Surface3).AlignItems(ui.Center).Shrink(0)
					pressable(act).Children(func() { ui.Text(c, t.Action).FontSize(12).FontWeight(600) })
				}
				cl := toast.CloseButton().Size(22, 22).Radius(6).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Label(locales.CommonClose())
				if cl.Hovered() {
					cl.Background(p.SurfaceHover)
				}
				cl.Transition(hoverFade)
				hot := cl.Hovered()
				cl.Children(func() { m.icon(c, icX, 13, p.TextMuted, hot) })
			})
		}
	})
}

// laidOut reports whether a grid that sizes its cells from its own width
// had that width in the frame before too: until then its cells jump to
// their first places rather than gliding there from nowhere.
func laidOut(grid ui.Element, width float32) bool {
	had := ui.Local(grid, "laidOut", func() bool { return false })
	settled := *had
	*had = width > 0
	return settled
}

// The cells of a grid that gains or loses columns glide to their new
// places, also while the window is resized, which Transition leaves
// still: each starts where it was drawn, offset from where the layout
// will put it, and the offset fades out; the grid's height follows.

const flipDur = 320 * time.Millisecond

// flipGridState is a grid's columns and height as last laid out, and its
// height gliding.
type flipGridState struct {
	cols  int
	laid  bool    // it had its width in the frame before
	gridH float32 // the grid's own height in the frame before
	fromH float32
	start time.Time
}

// flipCellState is a cell's offset gliding out.
type flipCellState struct {
	fromX, fromY float32
	start        time.Time
	offX, offY   float32
}

// flipGrid lays n cells out in the columns colsFor gives its width, gap
// apart, each built into its box by cell and known by id. motion, if set,
// is the cells' Transition between column changes, as they are sorted,
// added or removed.
func (m *mainUI) flipGrid(c *ui.Context, key any, gap float32, colsFor func(width float32) int, motion *ui.ElementTransition, n int, id func(i int) any, cell func(i int, box ui.Element)) {
	// The box holding the grid has its width and place, and its height but
	// while that glides.
	wrap := ui.Column(c.Key(key))
	wb := wrap.LayoutBounds()
	cols := max(1, colsFor(wb.W))
	gs := ui.Local(wrap, "flip", func() flipGridState { return flipGridState{cols: cols} })
	now := c.Now()
	prev := gs.laid
	// Not as the grid first gets its width, which only places its cells.
	changed := gs.cols != cols && prev && wb.W > 0 && !c.Preferences().ReduceMotion
	gs.laid, gs.cols = wb.W > 0, cols
	settled := prev && !changed
	if changed {
		gs.fromH, gs.start = wb.H, now
	}
	if e := float32(now.Sub(gs.start)) / float32(flipDur); !gs.start.IsZero() && e < 1 {
		wrap.Height(gs.fromH + (gs.gridH-gs.fromH)*easeStd(e))
		c.AnimationFrame()
	}
	cellW := (wb.W - float32(cols-1)*gap) / float32(cols)
	var rowTop, rowH float32
	wrap.Children(func() {
		grid := ui.Grid(c.Key("grid")).Gap(gap).Columns(cols).Shrink(0)
		gs.gridH = grid.LayoutBounds().H
		grid.Children(func() {
			for i := range n {
				box := ui.Column(c.Key(id(i)))
				cs := ui.Local(box, "flip", func() flipCellState { return flipCellState{} })
				lb := box.LayoutBounds()
				if i%cols == 0 && i > 0 {
					rowTop += rowH + gap
					rowH = 0
				}
				if changed && lb.W > 0 {
					// From where it shows to where it goes.
					nx, ny := float32(i%cols)*(cellW+gap), rowTop
					cs.fromX = lb.X - wb.X + cs.offX - nx
					cs.fromY = lb.Y - wb.Y + cs.offY - ny
					cs.start = now
				}
				rowH = max(rowH, lb.H)
				cs.offX, cs.offY = 0, 0
				if e := float32(now.Sub(cs.start)) / float32(flipDur); !cs.start.IsZero() && e < 1 {
					k := 1 - easeStd(e)
					cs.offX, cs.offY = cs.fromX*k, cs.fromY*k
					box.Translate(cs.offX, cs.offY).ZIndex(1)
					c.AnimationFrame()
				} else if motion != nil && settled {
					box.Transition(*motion)
				}
				cell(i, box)
			}
		})
	})
}

// glideState follows an element known by id across frames, and parents:
// where the layout put it, and its offset gliding out.
type glideState struct {
	frame        uint64
	last         ui.Rect // its box as last laid out, in window DIPs
	fromX, fromY float32
	start        time.Time
	offX, offY   float32
}

// glide moves e, known by id, from where it showed to where the layout
// now puts it, when changed: to want, its box relative to origin as the
// caller expects the layout to place it this frame (the layout itself
// says only in the next). Use it for layouts that rearrange as the window
// resizes, which Transition leaves still.
func (m *mainUI) glide(c *ui.Context, id string, e ui.Element, origin ui.Point, want ui.Rect, changed bool) {
	if m.glides == nil {
		m.glides = map[string]*glideState{}
	}
	st := m.glides[id]
	if st == nil {
		st = &glideState{}
		m.glides[id] = st
	}
	if lb := e.LayoutBounds(); lb.W > 0 {
		st.last = lb
	}
	now := c.Now()
	if changed && st.last.W > 0 && st.frame+1 >= m.frame && !c.Preferences().ReduceMotion {
		st.fromX = st.last.X + st.offX - (origin.X + want.X)
		st.fromY = st.last.Y + st.offY - (origin.Y + want.Y)
		st.start = now
	}
	st.frame = m.frame
	st.offX, st.offY = 0, 0
	if t := float32(now.Sub(st.start)) / float32(flipDur); !st.start.IsZero() && t < 1 {
		k := 1 - easeStd(t)
		st.offX, st.offY = st.fromX*k, st.fromY*k
		e.Translate(st.offX, st.offY).ZIndex(1)
		c.AnimationFrame()
	}
}

// heightGlide glides e's height from what it was to what its content
// makes it, when changed, as the content rearranges.
type heightGlide struct {
	fromH, toH float32
	start      time.Time
}

func (m *mainUI) glideHeight(c *ui.Context, e ui.Element, changed bool, natural float32) {
	st := ui.Local(e, "hglide", func() heightGlide { return heightGlide{} })
	now := c.Now()
	if changed && !c.Preferences().ReduceMotion {
		if h := e.LayoutBounds().H; h > 0 {
			st.fromH, st.start = h, now
		}
	}
	if natural > 0 {
		st.toH = natural
	}
	if t := float32(now.Sub(st.start)) / float32(flipDur); !st.start.IsZero() && t < 1 && st.toH > 0 {
		e.Height(st.fromH + (st.toH-st.fromH)*easeStd(t))
		c.AnimationFrame()
	}
}

// skeleton holds the place of what is loading, breathing.
func (m *mainUI) skeleton(c *ui.Context, h float32) ui.Element {
	box := ui.Box(c).Height(h).Radius(6).Background(m.pal.Surface2)
	return box.Opacity(0.45 + 0.55*box.Loop("breathe", 1600*time.Millisecond, ui.Bounce(ui.EaseInOut)))
}

// revealIn brings content in as it arrives in place of its skeleton: it
// rises a little and fades in, the skeleton fading out.
var revealIn = ui.ElementTransition{Enter: &ui.Motion{Y: 3}, Exit: &ui.Motion{}, Colors: true, Duration: 300 * time.Millisecond, ExitDuration: 150 * time.Millisecond}

// maskRoll is how a value hidden for privacy turns into itself and back:
// its characters roll, up as it shows and down as it hides.
func maskRoll(shown bool) ui.ContentTransition {
	return ui.ContentTransition{Numeric: true, Down: !shown}
}

// unveil brings a flag in and out as the values beside it show and hide:
// it fades and grows, taking its room in its row as it comes.
var unveil = ui.ElementTransition{Enter: &ui.Motion{Scale: 0.6, Collapse: true}, Exit: &ui.Motion{Scale: 0.6, Collapse: true}, Duration: 240 * time.Millisecond, Ease: easeOut}

// flagFade fades a masked flag's picture in and out over its block.
var flagFade = ui.ElementTransition{Enter: &ui.Motion{Scale: 0.8}, Exit: &ui.Motion{Scale: 0.8}, Duration: 220 * time.Millisecond, Ease: easeOut}
