package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
)

// Choosing the embedded node while it is logged in but not enabled: until
// its first status comes the page knows nothing of it, and must not put up
// a "starting" card for the status that follows to take away, which pulls
// the settings under it up and down again.
func TestTailscaleSwitchToIdleNodeShowsNoStartingCard(t *testing.T) {
	m := &mainUI{page: "tailscale"}
	m.state.Core.Status = coremgr.StatusRunning
	m.st.Tailscale.Mode = "off"
	m.tsPage.edited, m.tsPage.sysMode, m.tsPage.groupsAt = true, "off", m.runtime+1 // no groups to fetch
	m.ts = coreapi.TailscaleStatus{Source: "off", Peers: []coreapi.TailscalePeer{}}
	view := func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(false, false, "", "")
		ui.Column(c).Fill().Children(func() { m.tailscaleView(c) })
	}
	tt := ui.NewTester(view, 900, 700)
	tt.Frame()
	tt.Advance(time.Second)

	// The embedded mode is chosen; its status is on its way.
	m.st.Tailscale.Mode = "embedded"
	m.tsPage.sysMode = "embedded"
	tt.Frame()
	var shown []int
	for i := 0; i < 40; i++ {
		tt.Advance(16 * time.Millisecond)
		if i == 20 { // the node answers: logged in, not running
			m.ts = coreapi.TailscaleStatus{Source: "embedded", BackendState: "Stopped", Peers: []coreapi.TailscalePeer{}}
			tt.Frame()
		}
		if tt.HasText(locales.TsStarting()) {
			shown = append(shown, i)
		}
	}
	tt.Advance(time.Second)
	if len(shown) > 0 {
		t.Fatalf("the starting card showed in frames %v, for a node that is only stopped", shown)
	}
}

// The same for the installed client, whose status takes longer to come:
// the card of a missing app must not stand in for it.
func TestTailscaleSwitchToSystemShowsNoMissingCard(t *testing.T) {
	m := &mainUI{page: "tailscale"}
	m.state.Core.Status = coremgr.StatusRunning
	m.st.Tailscale.Mode = "off"
	m.tsPage.edited, m.tsPage.sysMode, m.tsPage.groupsAt = true, "off", m.runtime+1
	m.ts = coreapi.TailscaleStatus{Source: "off", Peers: []coreapi.TailscalePeer{}}
	view := func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(false, false, "", "")
		ui.Column(c).Fill().Children(func() { m.tailscaleView(c) })
	}
	tt := ui.NewTester(view, 900, 700)
	tt.Frame()
	tt.Advance(time.Second)
	m.st.Tailscale.Mode = "system"
	m.tsPage.sysMode = "system"
	tt.Frame()
	for i := 0; i < 40; i++ {
		tt.Advance(16 * time.Millisecond)
		if i == 20 {
			m.ts = coreapi.TailscaleStatus{Source: "system", Available: true, BackendState: "Running", Peers: []coreapi.TailscalePeer{}}
			tt.Frame()
		}
		if tt.HasText(locales.TsNotDetected()) {
			t.Fatalf("the card of a missing app showed in frame %d, for an app that is there", i)
		}
	}
}
