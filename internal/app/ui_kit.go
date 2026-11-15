package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/mygo-clash/mygo-clash/internal/app/locales"
)

// The main window's own widgets, in the app's look.

// gutter gives a scroll container a strip of its own at its end, from
// DIPs wide, taken from the padding around it, for its scroll bar to lie
// in rather than over what scrolls; what scrolls keeps its width.
func gutter(e ui.Element, from float32) ui.Element { return e.MarginEnd(-from).PaddingEnd(from) }

// iconButton is a small button with an icon alone, named by its tip.
func (m *mainUI) iconButton(c *ui.Context, key any, icon *ui.SVG, tip string) ui.Element {
	p := m.pal
	b := ui.ButtonBase(c.Key(key)).Size(28, 28).Radius(8).AlignItems(ui.Center).Justify(ui.Center).Label(tip).Shrink(0)
	if b.Hovered() {
		b.Background(p.SurfaceHover)
	}
	pressable(b)
	hot := b.Hovered()
	b.Children(func() { m.icon(c, icon, 15, p.TextMuted, hot) })
	if tip != "" {
		b.Tooltip(tip)
	}
	return b
}

// spinButton is an iconButton that turns into a spinner while busy.
func (m *mainUI) spinButton(c *ui.Context, key any, icon *ui.SVG, tip string, busy bool) ui.Element {
	if !busy {
		return m.iconButton(c, key, icon, tip)
	}
	b := ui.ButtonBase(c.Key(key)).Size(28, 28).Radius(7).AlignItems(ui.Center).Justify(ui.Center).Label(tip).Shrink(0).Disabled(true)
	b.Children(func() { ui.Spinner(c) })
	return b
}

// button is a button with a label, and an icon before it if given.
// variant is "" (plain), "primary", "ghost" or "danger".
func (m *mainUI) button(c *ui.Context, key any, label string, icon *ui.SVG, variant string) ui.Element {
	p := m.pal
	b := ui.ButtonBase(c.Key(key)).Row().Height(30).Padding(0, 12).Gap(6).Radius(9).AlignItems(ui.Center).Justify(ui.Center).Shrink(0)
	// Buttons are filled chips that sit on a tile or the window alike; no
	// outlines.
	bg, fg := p.Surface3, p.Text
	switch variant {
	case "primary":
		bg, fg = p.Accent, p.AccentText
		if b.Hovered() {
			bg = p.AccentHover
		}
	case "ghost":
		bg, fg = ui.Transparent, p.Text2
		if b.Hovered() {
			bg = p.SurfaceHover
		}
	case "danger":
		bg, fg = p.DangerSoft, p.Danger
	default:
		if b.Hovered() {
			bg = p.SurfacePressed
		}
	}
	pressable(b.Background(bg))
	b.Children(func() {
		if icon != nil {
			m.icon(c, icon, 14, fg, b.Hovered())
		}
		ui.Text(c, label).FontSize(12.5).FontWeight(550).TextColor(fg).SingleLine()
	})
	return b
}

// card is a tile with a title, the icon of what it is about, and its
// actions at the end of the title's line. Tiles stand apart by the gap
// between them, filled a step above the window, with no outline.
func (m *mainUI) card(c *ui.Context, key any, title string, icon *ui.SVG, actions, body func()) ui.Element {
	p := m.pal
	card := ui.Column(c.Key(key)).Background(p.Card).Radius(tileRadius)
	if m.fillCard {
		card.Grow(1)
	}
	card.Children(func() {
		ui.Row(c).Height(46).PaddingStart(18).PaddingEnd(10).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
			if icon != nil {
				ui.Icon(c, icon).FontSize(15).TextColor(p.TextMuted)
			}
			ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(p.Text2).SingleLine().Grow(1)
			if actions != nil {
				actions()
			}
		})
		ui.Column(c).Padding(2, 18, 18).Gap(12).Grow(1).Children(body)
	})
	return card
}

// linkTo is a card's way to its page.
func (m *mainUI) linkTo(c *ui.Context, page, tab, label string) {
	p := m.pal
	b := ui.ButtonBase(c.Key("to-" + page)).Row().Height(26).PaddingStart(8).PaddingEnd(4).Gap(2).Radius(6).AlignItems(ui.Center).Shrink(0)
	fg := p.TextMuted
	if b.Hovered() {
		b.Background(p.SurfaceHover)
		fg = p.Text
	}
	pressable(b)
	b.Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(fg).SingleLine()
		ui.Icon(c, icChevronRight).FontSize(13).TextColor(fg)
	})
	if b.Clicked() {
		m.goTo(page, tab)
	}
}

// lead is the first line of a card: what the card is about, in a word,
// what is under it, and something at its end.
func (m *mainUI) lead(c *ui.Context, title, sub string, extra func()) {
	p := m.pal
	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(16).FontWeight(650).SingleLine()
			if sub != "" {
				ui.Text(c, sub).FontSize(12).TextColor(p.TextMuted).SingleLine()
			}
		})
		if extra != nil {
			extra()
		}
	})
}

// kv is a list of names and their values, a line each.
func (m *mainUI) kv(c *ui.Context, rows func()) {
	ui.Column(c).Gap(7).Children(rows)
}

// kvRow is a line of kv whose value fn builds.
func (m *mainUI) kvRow(c *ui.Context, name string, fn func()) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).MinHeight(18).Children(func() {
		ui.Text(c, name).FontSize(12.5).TextColor(m.pal.TextMuted).SingleLine().Shrink(0)
		ui.Spacer(c)
		fn()
	})
}

// kvText is a line of kv whose value is text.
func (m *mainUI) kvText(c *ui.Context, name, value string) ui.Element {
	var v ui.Element
	m.kvRow(c, name, func() {
		v = ui.Text(c, value).FontSize(12.5).TextColor(m.pal.Text2).SingleLine().FontFeatures("tnum").ContentTransition(numRoll)
	})
	return v
}

// delayText shows a delay as the rest of the app does: untested (-1) as a
// dash, failed (0) as a timeout, and a spinner while it is measured.
func (m *mainUI) delayText(c *ui.Context, d int, loading bool) {
	m.delayTextOf(c, d, loading, delayKindOf)
}

// siteDelayText is a site's delay, rated as sites are (siteKindOf).
func (m *mainUI) siteDelayText(c *ui.Context, d int, loading bool) {
	m.delayTextOf(c, d, loading, siteKindOf)
}

func (m *mainUI) delayTextOf(c *ui.Context, d int, loading bool, kindOf func(int) delayKind) {
	p := m.pal
	box := ui.Row(c).Shrink(0).AlignItems(ui.Center)
	freshDelay(box, d, loading)
	if loading {
		box.Children(func() { ui.Spinner(c) })
		return
	}
	s, col := "—", p.Faint
	switch kindOf(d) {
	case delayGood:
		col = p.Success
	case delayOK:
		col = p.Warning
	case delayBad:
		col = p.Danger
	}
	if d > 0 {
		s = fmt.Sprintf("%d ms", d)
	} else if d == 0 {
		s = locales.CommonTimeout()
	}
	box.Children(func() { ui.Text(c, s).FontSize(12).TextColor(col).FontFeatures("tnum").SingleLine().Shrink(0) })
}

// badge is a short text in a pill, in the tone of what it says: success,
// warning, danger, or none.
func (m *mainUI) badge(c *ui.Context, text, tone string) {
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
	ui.Row(c).Height(20).Padding(0, 8).Radius(10).Background(bg).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, text).FontSize(11.5).FontWeight(600).TextColor(fg).SingleLine()
	})
}

// segmented is a row of segments choosing one; it returns the one clicked,
// or -1.
func (m *mainUI) segmented(c *ui.Context, key any, selected int, labels ...string) int {
	return m.segmentedBase(c, key, selected, false, labels...)
}

// segmentedOf is a segmented control that sets *selected, as wide as its
// place with segments of equal width when fill; it reports a change.
func (m *mainUI) segmentedOf(c *ui.Context, key any, selected *int, fill bool, labels ...string) bool {
	if i := m.segmentedBase(c, key, *selected, fill, labels...); i >= 0 {
		*selected = i
		return true
	}
	return false
}

func (m *mainUI) segmentedBase(c *ui.Context, key any, selected int, fill bool, labels ...string) int {
	p, clicked := m.pal, -1
	track := ui.Row(c.Key(key)).Gap(2).Padding(2).Radius(8).Background(p.Surface3).Shrink(0)
	if fill {
		track.FillWidth()
	}
	thumb := segThumb{track.ID()}
	track.Children(func() {
		for i, label := range labels {
			seg := ui.ButtonBase(c.Key(i)).Height(26).Padding(0, 12).Radius(6).AlignItems(ui.Center).Justify(ui.Center)
			if fill {
				seg.Grow(1).Basis(0)
			}
			if i != selected && seg.Hovered() {
				seg.Background(p.SurfaceHover)
			}
			seg.Transition(hoverFade)
			seg.Children(func() {
				// The selected segment's card slides from the one before.
				if i == selected {
					ui.Box(c.Key("thumb")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Radius(6).
						Background(p.Card).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.12)).
						Transition(ui.ElementTransition{Match: thumb, Animation: &segSlide})
				}
				txt := ui.Text(c, label).FontSize(12.5).SingleLine().ZIndex(1)
				col := p.TextMuted
				if i == selected {
					txt.FontWeight(600)
					col = p.Text
				}
				txt.TextColor(txt.AnimateColor("fg", col, ui.Animation{Duration: 200 * time.Millisecond}))
			})
			if seg.Clicked() && i != selected {
				clicked = i
			}
		}
	})
	return clicked
}

// empty fills a page or a card that has nothing to show.
func (m *mainUI) empty(c *ui.Context, title string, body func()) {
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(10).Padding(24).Children(func() {
		ui.Text(c, title).FontSize(13).TextColor(m.pal.TextMuted).TextAlign(ui.Center)
		if body != nil {
			body()
		}
	})
}

// errorLine is the last line of a card, what went wrong.
func (m *mainUI) errorLine(c *ui.Context, s string) {
	p := m.pal
	ui.Row(c.Key("error-line"+s)).Padding(6, 9).Radius(6).Background(p.DangerSoft).Transition(noticeIn).Children(func() {
		ui.Text(c, s).FontSize(12).TextColor(p.Danger).SingleLine().Tooltip(s)
	})
}

// trafficGraph draws the last n seconds of the traffic: the download as
// an area in the accent, the upload as a line of what goes up. It slides
// between samples rather than stepping.
func (m *mainUI) trafficGraph(c *ui.Context, n int, height float32, minimal bool) ui.Element {
	p := m.pal
	box := ui.Box(c).Height(height)
	if height == 0 {
		box.Grow(1).MinHeight(60)
	}
	return box.Draw(func(pt *ui.Painter, r ui.Rect) {
		up, down := m.up[len(m.up)-n:], m.down[len(m.down)-n:]
		var peak int64 = 1024
		for i := range up {
			peak = max(peak, up[i], down[i])
		}
		// The newest sample comes in from the right over a second.
		frac := float32(pt.Now().Sub(m.sampleAt).Seconds())
		if frac < 1 && frac >= 0 {
			pt.AnimationFrame()
		} else {
			frac = 1
		}
		step := r.W / float32(n-2)
		at := func(v []int64, i int) (float32, float32) {
			return r.X + (float32(i)-frac)*step, r.Y + r.H - 1 - (r.H-3)*float32(v[i])/float32(peak)
		}
		pt.Clip(r, 0, func() {
			if !minimal {
				for i := 1; i < 4; i++ {
					y := r.Y + r.H*float32(i)/4
					pt.Line(r.X, y, r.X+r.W, y, 1, p.Border.Alpha(0.6))
				}
			}
			line := func(v []int64, col ui.Color, fill bool) {
				var path, area ui.Path
				for i := range v {
					x, y := at(v, i)
					if i == 0 {
						path.MoveTo(x, y)
						area.MoveTo(x, r.Y+r.H).LineTo(x, y)
						continue
					}
					px, py := at(v, i-1)
					mx := (px + x) / 2
					path.CubeTo(mx, py, mx, y, x, y)
					area.CubeTo(mx, py, mx, y, x, y)
				}
				if fill {
					lx, _ := at(v, len(v)-1)
					area.LineTo(lx, r.Y+r.H).Close()
					pt.FillPath(&area, col.Alpha(0.14))
				}
				pt.StrokePath(&path, 1.5, col)
			}
			line(down, p.Accent, true)
			line(up, p.Accent2, false)
		})
	})
}

// segThumb is the identity of a segmented control's selected card, which
// slides from segment to segment.
type segThumb struct{ track any }

var segSlide = ui.Animation{Duration: 260 * time.Millisecond, Ease: ui.CubicBezier(0.3, 0.7, 0.3, 1)}

// tabs is a row of tabs setting *selected, the soft fill behind the
// selected one sliding from the one before; it reports a change.
func (m *mainUI) tabs(c *ui.Context, key any, selected *int, labels ...string) bool {
	p, changed := m.pal, false
	row := ui.Row(c.Key(key)).Gap(2).Shrink(0)
	line := segThumb{row.ID()}
	row.Children(func() {
		for i, label := range labels {
			b := ui.ButtonBase(c.Key(i)).Padding(6, 12).Radius(9).AlignItems(ui.Center).Label(label)
			on := i == *selected
			col := p.TextMuted
			switch {
			case on:
				col = p.Text
			case b.Hovered():
				col = p.Text2
				b.Background(p.SurfaceHover.Alpha(0.6))
			}
			b.Transition(hoverFade)
			b.Children(func() {
				if on {
					ui.Box(c.Key("line")).Absolute().Left(0).Right(0).Top(0).Bottom(0).Radius(9).Background(p.Surface3).
						Transition(ui.ElementTransition{Match: line, Animation: &segSlide})
				}
				t := ui.Text(c, label).FontSize(13.5).SingleLine().ZIndex(1)
				if on {
					t.FontWeight(600)
				}
				t.TextColor(t.AnimateColor("fg", col, ui.Animation{Duration: 200 * time.Millisecond}))
			})
			if b.Clicked() && !on {
				*selected, changed = i, true
			}
		}
	})
	return changed
}

// formWidth is the widest a column of settings grows: rows wider than
// this put a label and its control too far apart to read as one.
const formWidth = 860

// formColumn is a column of settings, as wide as the page up to formWidth
// and centred in it, so that a wide or full-screen window does not leave
// it against one side with the rest of the page empty.
func formColumn(c *ui.Context) ui.Element {
	return ui.Column(c).Gap(20).MaxWidth(formWidth).FillWidth().AlignSelf(ui.Center)
}
