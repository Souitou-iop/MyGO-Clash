package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/config"
)

// The sidebar's mark, inviting, says so and plays the guide when pressed;
// not inviting, a press only spins it.
func TestTourInviteMark(t *testing.T) {
	for _, inviting := range []bool{true, false} {
		m := &mainUI{}
		m.st.Theme = "system"
		m.st.UI = config.Defaults().UI
		m.st.UI.OnboardingSeen = true
		// The launch is counted already: nothing is saved in the test.
		m.st.UI.TourInvites = config.TourInviteLaunches
		m.tour = tourInvite{decided: true, on: inviting}
		tt := ui.NewTester(func(c *ui.Context) {
			m.frame++
			m.pal = mainPalette(false, false, "", "")
			ui.Column(c).Fill().Children(func() { m.sidebar(c) })
		}, 400, 700)
		tt.Advance(100 * time.Millisecond)
		mark, ok := tt.Access("MyGO-Clash")
		if !ok {
			t.Fatal("no mark")
		}
		if got := mark.Description == locales.NavTourInvite(); got != inviting {
			t.Errorf("inviting %v: the mark's tip is %q", inviting, mark.Description)
		}
		b := mark.Bounds
		tt.ClickAt(b.X+b.W/2, b.Y+b.H/2)
		tt.Advance(50 * time.Millisecond)
		if m.onb.open != inviting {
			t.Errorf("inviting %v: a press opened the guide: %v", inviting, m.onb.open)
		}
		if inviting && m.tour.on {
			t.Error("the mark still invites once the guide played")
		}
	}
}
