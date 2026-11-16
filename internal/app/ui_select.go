package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// selectPopup is how the list of a drop-down, or a menu, comes and goes: it grows from
// the edge of its trigger, a little smaller and nearer to it, and shrinks
// back as it closes, faster than it came. Flip makes it come up from the
// trigger where it opened above it.
var selectPopup = ui.ElementTransition{
	Duration:     170 * time.Millisecond,
	ExitDuration: 110 * time.Millisecond,
	Ease:         easeOut,
	ExitEase:     ui.EaseIn,
	Enter:        &ui.Motion{Scale: 0.96, Y: -6, Flip: true},
	Exit:         &ui.Motion{Scale: 0.96, Y: -6, Flip: true},
}

// menuOptions places a button's menu, at its end below it unless at says
// otherwise, and gives it the motion of a drop-down's list.
func menuOptions(c *ui.Context, place ...ui.Anchor) ui.PopoverMenuButtonOptions {
	o := ui.PopoverMenuButtonOptions{At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd}
	if len(place) == 2 {
		o.At, o.Self = place[0], place[1]
	}
	motion := selectPopup
	if c.Preferences().ReduceMotion {
		motion.Duration, motion.ExitDuration = -1, -1
	}
	o.Panel = func(panel ui.Element) { panel.AttachedOrigin().Transition(motion) }
	return o
}

// dropdown is ui.Select, a drop-down choosing one of options into
// *selected, whose list opens and closes with a motion. It looks as
// ui.Select does, with the theme's colors, and returns the trigger, which
// takes the size, the label and Changed as a select's does.
func dropdown(c *ui.Context, selected *string, options []string) ui.Element {
	t := c.Theme()
	sel := ui.SelectBase(c, selected)
	b := sel.Trigger
	b.Row().AlignItems(ui.Center).Justify(ui.SpaceBetween).MinWidth(t.Space(35)).
		Padding(t.Space(1.5), t.Space(3.5)).Gap(t.Space(1.5)).Radius(t.Radius).Border(1, t.Border).TextColor(t.Text)
	bg := t.Surface
	switch {
	case b.IsDisabled():
	case b.Pressed():
		bg = t.SurfacePressed
	case b.Hovered():
		bg = t.SurfaceHover
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 120 * time.Millisecond}))
	b.Children(func() {
		ui.Text(c, *selected).SingleLine()
		// The arrow turns over as the list opens.
		arrow := ui.Box(c).Size(t.Space(2.5), t.Space(2.5)).Shrink(0)
		arrow.Rotate(180 * arrow.AnimateTo("open", b2f(sel.Open()), ui.Animation{Duration: 170 * time.Millisecond, Ease: easeOut}))
		arrow.Draw(func(p *ui.Painter, r ui.Rect) {
			var path ui.Path
			path.MoveTo(r.X+r.W*0.1, r.Y+r.H*0.3).LineTo(r.X+r.W*0.5, r.Y+r.H*0.7).LineTo(r.X+r.W*0.9, r.Y+r.H*0.3)
			p.StrokePath(&path, 1.5, t.TextMuted)
		})
	})
	motion := selectPopup
	if c.Preferences().ReduceMotion {
		motion.Duration, motion.ExitDuration = -1, -1
	}
	sel.Popup(func(panel ui.Element) {
		panel.Margin(t.Space(1), 0, 0, 0).Padding(t.Space(1)).Radius(t.Radius+2).Background(t.Background).Border(1, t.Border)
		panel.Shadow(0, 6, 20, 0, ui.RGBA(0, 0, 0, 0.18))
		panel.AttachedOrigin().Transition(motion)
		list := func() {
			for _, opt := range options {
				item := sel.Item(opt).Padding(t.Space(1.5), t.Space(2.5)).Radius(t.Radius)
				switch {
				case item.Highlighted():
					item.Background(t.Accent).TextColor(t.AccentText)
				case opt == *selected:
					item.Background(t.Surface)
				}
				item.Children(func() { ui.Text(c, opt).SingleLine() })
			}
		}
		// A long list scrolls instead of running off the window.
		if len(options) > 10 {
			ui.Scroll(c).MaxHeight(340).Children(list)
			return
		}
		list()
	})
	return b
}
