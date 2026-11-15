package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// What a segmented control, a row of tabs or a change of state swaps in
// below it: the content of the new choice comes in from the side of the
// segment it came from as the old one slides away the other way, fading,
// and the room they take glides from the one height to the other. A
// change of state with no side to it (loading to loaded, empty to full)
// crossfades in place with a small rise.

const (
	paneDur      = 280 * time.Millisecond
	paneExitDur  = 120 * time.Millisecond
	paneShiftDIP = 20 // DIPs a pane slides by
)

// paneIn is how a pane comes in.
var paneIn = ui.Animation{Duration: paneDur, Delay: 80 * time.Millisecond, Ease: easeOut}

// paneState is which pane a switch showed, and which way it last went.
type paneState struct {
	state any
	order int
	dir   float32
	at    time.Time // when it last changed
	gen   int       // the changes so far, which key the panes
}

// paneShift follows the switch held by host: how far the pane coming in
// starts across (0 for a swap in place), mirrored for right-to-left;
// whether the state changed in this frame, and whether the panes are
// switching (changed, or still on their way); and the key of the pane
// shown, new at each change, so that a pane shown again a moment after it
// went comes in anew rather than as the element it was.
type paneShift struct {
	dx                 float32
	changed, switching bool
	key                [2]any
}

func paneShiftOf(c *ui.Context, host ui.Element, state any, order int, sided bool) paneShift {
	st := ui.Local(host, "pane", func() paneState { return paneState{state: state, order: order, dir: 1} })
	changed := st.state != state
	if changed {
		st.dir = 1
		if order < st.order {
			st.dir = -1
		}
		st.state, st.order, st.at = state, order, c.Now()
		st.gen++
	}
	switching := changed || (!st.at.IsZero() && c.Now().Sub(st.at) < paneIn.Delay+paneDur+40*time.Millisecond)
	if switching {
		c.AnimationFrame()
	}
	ps := paneShift{changed: changed, switching: switching, key: [2]any{state, st.gen}}
	if sided {
		ps.dx = paneShiftDIP * st.dir
		if c.Direction() == ui.RightToLeft {
			ps.dx = -ps.dx
		}
	}
	return ps
}

func (m *mainUI) paneMotion(dx float32) ui.ElementTransition {
	enter := &ui.Motion{X: dx}
	exit := &ui.Motion{X: -dx}
	if dx == 0 {
		enter, exit = &ui.Motion{Y: 4}, &ui.Motion{}
	}
	// The old pane is quick to go, and the new waits a beat for it, so
	// that the two are seldom seen through each other.
	return ui.ElementTransition{Enter: enter, Exit: exit, Colors: true, Animation: &paneIn, ExitDuration: paneExitDur, ExitEase: ui.EaseOut, ExitDelay: -1}
}

// switchPane shows the index-th pane of a segmented control or tabs,
// built by fn into a column gap apart, flowing in its parent: a scroll, a
// dialog. Its height glides as the panes change.
func (m *mainUI) switchPane(c *ui.Context, key any, index int, gap float32, fn func()) ui.Element {
	return m.pane(c, key, index, index, true, false, gap, fn)
}

// swapPane is a switchPane for a change of state with no side to it, as
// loading to loaded or empty to full: the panes crossfade in place.
func (m *mainUI) swapPane(c *ui.Context, key any, state string, gap float32, fn func()) ui.Element {
	return m.pane(c, key, state, 0, false, false, gap, fn)
}

// switchFill is a switchPane that fills the rest of a column, as the
// tile of a page under its tabs; fn's children may Grow in it.
func (m *mainUI) switchFill(c *ui.Context, key any, index int, fn func()) ui.Element {
	return m.pane(c, key, index, index, true, true, 0, fn)
}

// swapFill is a swapPane that fills the rest of a column.
func (m *mainUI) swapFill(c *ui.Context, key any, state string, fn func()) ui.Element {
	return m.pane(c, key, state, 0, false, true, 0, fn)
}

func (m *mainUI) pane(c *ui.Context, key, state any, order int, sided, fill bool, gap float32, fn func()) ui.Element {
	var host ui.Element
	if fill {
		host = ui.Box(c.Key(key)).Grow(1).Basis(0).MinHeight(0)
	} else {
		host = ui.Column(c.Key(key))
	}
	ps := paneShiftOf(c, host, state, order, sided)
	mo := m.paneMotion(ps.dx)
	host.ChildExit(*mo.Exit)
	var paneH float32
	host.Children(func() {
		var pane ui.Element
		if fill {
			pane = ui.Column(c.Key(ps.key)).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		} else {
			pane = ui.Column(c.Key(ps.key)).Gap(gap).Shrink(0)
		}
		if !fill {
			paneH = pane.LayoutBounds().H
		}
		pane.Transition(mo).Children(fn)
	})
	if !fill {
		m.glideHeight(c, host, ps.changed, paneH)
		if ps.switching {
			host.Clip()
		}
	}
	return host
}

// badgeMotion is how a badge of state changes: its colors fade, its width
// glides to its new words, which crossfade, and it pops in and out.
var badgeMotion = ui.ElementTransition{
	Enter: &ui.Motion{Scale: 0.8}, Exit: &ui.Motion{Scale: 0.8},
	Colors: true, Width: true, Duration: 220 * time.Millisecond, Ease: easeOut,
}

// stateBadge is a badge that says the state of something, which changes
// in place: a connection's, a service's. Use badge for those that do not
// change, as the rows of a list.
func (m *mainUI) stateBadge(c *ui.Context, key any, text, tone string) ui.Element {
	p := m.pal
	bg, fg := p.Surface3, p.Text2
	switch tone {
	case "success":
		bg, fg = p.SuccessSoft, p.Success
	case "warning":
		bg, fg = p.WarningSoft, p.Warning
	case "danger":
		bg, fg = p.DangerSoft, p.Danger
	case "info":
		bg, fg = p.AccentSoft, p.AccentFg
	}
	b := ui.Row(c.Key(key)).Height(20).Padding(0, 8).Radius(10).Background(bg).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Clip().Transition(badgeMotion)
	b.Children(func() {
		t := ui.Text(c, text).FontSize(11.5).FontWeight(600).SingleLine().ContentTransition(ui.ContentTransition{})
		t.TextColor(t.AnimateColor("fg", fg, ui.Animation{Duration: 220 * time.Millisecond}))
	})
	return b
}

// iconSegments is a segmented control of icons: the selected one's card
// slides from the one before, as segmented's does, and the icons fade to
// their colors. It returns the one clicked, or -1.
func (m *mainUI) iconSegments(c *ui.Context, key any, selected int, icons ...*ui.SVG) int {
	p, clicked := m.pal, -1
	track := ui.Row(c.Key(key)).Gap(2).Padding(2).Radius(8).Background(p.Surface3).Shrink(0)
	thumb := segThumb{track.ID()}
	track.Children(func() {
		for i, icon := range icons {
			seg := ui.ButtonBase(c.Key(i)).Size(30, 26).Radius(6).AlignItems(ui.Center).Justify(ui.Center)
			pressable(seg)
			fg := p.TextMuted
			switch {
			case i == selected:
				fg = p.Text
			case seg.Hovered():
				seg.Background(p.SurfaceHover)
			}
			seg.Children(func() {
				if i == selected {
					ui.Box(c.Key("thumb")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Radius(6).
						Background(p.Card).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12)).
						Transition(ui.ElementTransition{Match: thumb, Animation: &segSlide})
				}
				ic := ui.Icon(c, icon).FontSize(14).ZIndex(1)
				ic.TextColor(ic.AnimateColor("fg", fg, ui.Animation{Duration: 200 * time.Millisecond}))
			})
			if seg.Clicked() && i != selected {
				clicked = i
			}
		}
	})
	return clicked
}

// listRow is how a row of a short list comes and goes as it is filtered
// or changes: it grows in and folds away, fading, the rows after it
// gliding to their places.
var listRow = ui.ElementTransition{
	Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true},
	Position: true, Colors: true, Duration: 240 * time.Millisecond, ExitDuration: 200 * time.Millisecond, Ease: easeStd,
}

// nodeMotion is tileMotion for the nodes of a grid that are filtered in
// and out: they grow in and shrink away, fading, as the rest glide.
var nodeMotion = func() ui.ElementTransition {
	t := tileMotion
	t.Enter, t.Exit = &ui.Motion{Scale: 0.94}, &ui.Motion{Scale: 0.94}
	t.ExitDuration = 160 * time.Millisecond
	return t
}()

// reflowOut is reflow for the cards of a grid that also go: a card
// removed shrinks away, fading, as the rest glide to close up.
var reflowOut = func() ui.ElementTransition {
	t := reflow
	t.Exit = &ui.Motion{Scale: 0.96}
	t.ExitDuration = 180 * time.Millisecond
	return t
}()

// busyMotion is how a control and the spinner standing in for it while
// it works turn into each other.
var busyMotion = ui.ElementTransition{
	Enter: &ui.Motion{Scale: 0.6}, Exit: &ui.Motion{Scale: 0.6},
	Duration: 200 * time.Millisecond, ExitDuration: 140 * time.Millisecond, Ease: easeOut,
}

// busySwap shows what fn builds (a button, say), or a spinner in its
// place while busy: the two turn into each other where they stand, the
// room between them gliding, rather than the row jumping.
func (m *mainUI) busySwap(c *ui.Context, key any, busy bool, fn func()) ui.Element {
	host := ui.Row(c.Key(key)).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).
		Transition(ui.ElementTransition{Width: true, Duration: 220 * time.Millisecond, Ease: easeStd})
	host.Children(func() {
		ui.Row(c.Key(busy)).AlignItems(ui.Center).Justify(ui.Center).Transition(busyMotion).Children(func() {
			if busy {
				ui.Spinner(c)
				return
			}
			fn()
		})
	})
	return host
}
