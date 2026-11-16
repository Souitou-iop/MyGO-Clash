package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/mygo-clash/mygo-clash/internal/config"
)

// The speed lines in the sidebar glide with the fold; none of them may
// jump on the way, least of all as the bar reaches the rail or leaves it.
func TestSidebarRatesGlideWhenFolding(t *testing.T) {
	m := &mainUI{}
	m.st.UI = config.Defaults().UI
	m.st.UI.TrafficGraph = true
	m.st.UI.CollapseNav = false
	m.up, m.down = make([]int64, trafficLen), make([]int64, trafficLen)
	view := func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(false, false, "", "")
		ui.Column(c).Fill().Children(func() { m.sidebar(c) })
	}
	tt := ui.NewTester(view, 400, 700)
	tt.Frame()
	tt.Advance(time.Second)
	// where reads the place of the upload line, which says "0B" folded and
	// "0 B/s" unfolded.
	where := func() ui.Rect {
		for _, s := range []string{fmtRate(0), shortRate(0)} {
			if r, ok := tt.Find(s); ok {
				return r
			}
		}
		t.Fatal("no rate line in the sidebar")
		return ui.Rect{}
	}
	for _, collapse := range []bool{true, false, true} {
		m.st.UI.CollapseNav = collapse
		tt.Frame()
		prev := where()
		for i := 0; i < 60; i++ { // a second of frames, the fold taking 380ms
			tt.Advance(16 * time.Millisecond)
			now := where()
			if dx := now.X - prev.X; dx > 14 || dx < -14 {
				t.Fatalf("collapse=%v, frame %d: the rate line jumped %v px across (x %v → %v)", collapse, i, dx, prev.X, now.X)
			}
			if dy := now.Y - prev.Y; dy > 14 || dy < -14 {
				t.Fatalf("collapse=%v, frame %d: the rate line jumped %v px down (y %v → %v)", collapse, i, dy, prev.Y, now.Y)
			}
			prev = now
		}
	}
}
