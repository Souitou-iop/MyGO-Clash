package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/config"
)

// The invitation to the guide for those who knew the app before it: they
// never see it play by itself (see config.Load), so the sidebar's mark
// asks instead, on a few launches. Its needle swings once as the window
// first shows, a dot stays by it, and a press plays the guide out of it.

// tourInvite is whether this launch invites; decided once, so that
// counting the launch does not end the invitation under the user's eyes.
type tourInvite struct {
	decided, on bool
	since       time.Time // when the invitation first showed
}

// tourInviting reports whether the mark invites to the guide, and counts
// the launch the first time it does.
func (m *mainUI) tourInviting(c *ui.Context) bool {
	t := &m.tour
	if !t.decided {
		if m.st.Theme == "" { // the settings are not in yet
			return false
		}
		t.decided = true
		t.on = m.st.UI.InvitesToTour()
		if t.on {
			t.since = c.Now()
			m.patch(map[string]any{"ui": map[string]any{"tourInvites": m.st.UI.TourInvites + 1}})
		}
	}
	return t.on && !m.onb.shown
}

// playTour plays the guide from the mark, which ends the invitation.
func (m *mainUI) playTour() {
	m.endTourInvites()
	m.replayOnboarding()
}

// endTourInvites keeps the mark from inviting again: the guide played.
// Only once the sidebar has weighed the invitation: before, nothing has
// counted the launches.
func (m *mainUI) endTourInvites() {
	m.tour.on = false
	if m.tour.decided && m.st.UI.TourInvites < config.TourInviteLaunches {
		m.patch(map[string]any{"ui": map[string]any{"tourInvites": config.TourInviteLaunches}})
	}
}

// Its swing: tourNudgeAt seconds after the window shows, the needle is
// pushed tourNudgeDeg aside and settles as a real one does.
const (
	tourNudgeAt  = 1.2
	tourNudgeDeg = 38
	tourNudgeFor = 3.2
)

// tourNudge is the needle's turn, in degrees, of the invitation's swing.
func (m *mainUI) tourNudge(c *ui.Context) float32 {
	if c.Preferences().ReduceMotion {
		return 0
	}
	t := secsOf(c.Now().Sub(m.tour.since)) - tourNudgeAt
	if t > tourNudgeFor {
		return 0
	}
	c.Invalidate()
	if t <= 0 {
		return 0
	}
	return tourNudgeDeg * float32(math.Exp(float64(-t/0.7))*math.Sin(2*math.Pi*0.9*float64(t)))
}

// tourTip is the mark's tip. The mark sits under the window's title bar,
// where on macOS the traffic lights are, so the tip opens below it, from
// its left edge, instead of above it as a tip does by default.
func (m *mainUI) tourTip(c *ui.Context, mark ui.Element) {
	mark.Description(locales.NavTourInvite())
	th := m.pal.Theme
	fill, ink := th.Inverse, th.InverseText
	if fill == ui.Transparent {
		fill = th.Text
	}
	if ink == ui.Transparent {
		ink = th.Background
	}
	ui.TooltipBase(c, mark, func(tip ui.Element) {
		tip.AttachTo(mark, ui.AnchorBottomLeft, ui.AnchorTopLeft).Margin(6, 0, 0, 0)
		tip.Padding(5, 10).Radius(6).Background(fill).TextColor(ink).FontSize(12.5).Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.2))
		tip.Children(func() { ui.Text(c, locales.NavTourInvite()).SingleLine() })
	})
}
