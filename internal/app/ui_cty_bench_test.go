package app

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/tools"
)

// ctyBench is the connectivity page with every default site tested, shown
// at the category cat (and the region, in the region category): what the
// page builds every frame while sites are being tested.
func ctyBench(tb testing.TB, cat, region string, busy bool) (*mainUI, *ui.Tester) {
	m := &mainUI{}
	m.state.Core.Status = coremgr.StatusRunning
	s := &m.cty
	s.started = true
	s.exitAt = homeKey{ready: true}
	s.ips = map[string]*exitIP{}
	s.rounds, s.checking, s.results = map[string][]int{}, map[string]bool{}, map[string]unlockResult{}
	s.sites = tools.ConnectivitySites
	s.regionOrder = regionOrder(nil, tools.SiteRegions(s.sites))
	s.unlocks = tools.UnlockList(nil, nil)
	s.view, s.cat, s.region, s.regionOpen = ctySites, cat, region, region != ""
	s.sitesBusy = busy
	for i, st := range s.sites {
		r := make([]int, siteRounds)
		for k := range r {
			r[k] = 40 + (i*37+k*91)%900
		}
		if i%97 == 0 {
			r = []int{0, 0, 0, 0, 0}
		}
		s.rounds[st.ID] = r
	}
	tt := ui.NewTester(func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(false, false, "", "")
		ui.Column(c).Fill().Background(m.pal.Background).Children(func() { m.connectivityView(c) })
	}, 1400, 900)
	return m, tt
}

// BenchmarkCtyFrame is a frame of the connectivity page with the sites
// tested, by what is shown: a small category, the region category's
// 248 sites, and a country of it.
func BenchmarkCtyFrame(b *testing.B) {
	for _, c := range []struct{ name, cat, region string }{
		{"web", "web", ""},
		{"media", "media", ""},
		{"region-all", tools.CategoryRegion, ""},
		{"region-japan", tools.CategoryRegion, "japan"},
	} {
		for _, busy := range []bool{false, true} {
			b.Run(fmt.Sprintf("%s/busy=%v", c.name, busy), func(b *testing.B) {
				_, tt := ctyBench(b, c.cat, c.region, busy)
				tt.Frame()
				b.ReportAllocs()
				for b.Loop() {
					tt.Frame()
				}
			})
		}
	}
}

// BenchmarkCtyTally is the walk of the sites sitesDetail makes every frame
// for the counts beside the categories.
func BenchmarkCtyTally(b *testing.B) {
	m, _ := ctyBench(b, "web", "", true)
	b.ReportAllocs()
	for b.Loop() {
		_ = m.tallySites()
	}
}
