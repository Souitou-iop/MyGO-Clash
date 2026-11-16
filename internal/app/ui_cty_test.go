package app

import (
	"slices"
	"testing"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/tools"
)

// The page's numbers are worked out once for the sites and rounds as they
// are, and again when either changes.
func TestDerivedFollowsTheRounds(t *testing.T) {
	m, _ := ctyBench(t, "web", "", false)
	s := &m.cty
	d := m.derived()
	if d.done != len(s.sites) || d.ok != len(s.sites)-(len(s.sites)+96)/97 {
		t.Errorf("done %d ok %d of %d sites", d.done, d.ok, len(s.sites))
	}
	if m.derived() != d {
		t.Errorf("worked out again for the same sites and rounds")
	}
	// A site's round landing, as the batch of them does.
	s.rounds["apple"] = []int{0, 0, 0, 0, 0}
	s.roundsVer++
	d2 := m.derived()
	if d2 == d || d2.ok != d.ok-1 {
		t.Errorf("rounds changed: ok %d, was %d", d2.ok, d.ok)
	}
	if d2.byCat["web"].failed != d.byCat["web"].failed+1 {
		t.Errorf("web failed %d, was %d", d2.byCat["web"].failed, d.byCat["web"].failed)
	}
	// The favorites and the sites themselves.
	s.favoriteOrder = []string{"google", "apple"}
	if d3 := m.derived(); d3 == d2 || d3.favorite.n != 2 {
		t.Errorf("favorites: %d", d3.favorite.n)
	}
	s.sites = slices.Clone(s.sites)[:100]
	if d4 := m.derived(); d4.done != 100 {
		t.Errorf("fewer sites: %d done", d4.done)
	}
	// The cats are the categories in the order the page shows them.
	if want := tools.SiteCategories(s.sites); !slices.Equal(m.derived().cats, want) {
		t.Errorf("cats = %v, want %v", m.derived().cats, want)
	}
}

func TestVerdictOfTheSites(t *testing.T) {
	m, _ := ctyBench(t, "web", "", false)
	s := &m.cty
	// A site of each group, of kinds the verdict counts.
	s.sites = []tools.Site{{ID: "google", Group: "global"}, {ID: "apple", Group: "domestic"}}
	open, shut := []int{50, 60, 70, 80, 90}, []int{0, 0, 0, 0, 0}
	for _, c := range []struct {
		home, abroad []int
		key, tone    string
	}{
		{open, open, "ok", "success"},
		{open, shut, "noGlobal", "danger"},
		{shut, open, "noDomestic", "warning"},
		{shut, shut, "none", "danger"},
		{nil, open, "", ""},
	} {
		s.rounds = map[string][]int{"apple": c.home, "google": c.abroad}
		if k, tone := m.siteVerdictOf(); k != c.key || tone != c.tone {
			t.Errorf("home %v abroad %v: %q %q, want %q %q", c.home, c.abroad, k, tone, c.key, c.tone)
		}
	}
}

// A category too large to build whole builds the cards in view and a
// margin, and the rest as plates; scrolled to, they are built whole.
func TestLargeCategoryBuildsWhatShows(t *testing.T) {
	m, tt := ctyBench(t, tools.CategoryRegion, "", false)
	var of []tools.Site
	for _, st := range m.cty.sites {
		if tools.SiteCategory(st) == tools.CategoryRegion {
			of = append(of, st)
		}
	}
	if len(of) <= lazyCards {
		t.Fatalf("region has only %d sites", len(of))
	}
	tt.Frame()
	tt.Advance(time.Second)
	if !tt.HasText(of[0].Name) {
		t.Errorf("first card %s is not built", of[0].Name)
	}
	last := of[len(of)-1].Name
	if tt.HasText(last) {
		t.Errorf("last card %s, far from view, is built", last)
	}
}
