package app

import (
	"slices"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/config"
)

func TestPlaceOf(t *testing.T) {
	for page, want := range map[string][2]string{
		"home": {"home", ""}, "proxies": {"proxies", ""}, "profiles": {"profiles", ""},
		"connections": {"activity", "connections"}, "stats": {"activity", "stats"}, "logs": {"activity", "logs"},
		"connectivity": {"connectivity", ""}, "tailscale": {"tailscale", ""},
		"providers": {"proxies", "providers"}, "rules": {"rules", ""}, "settings": {"settings", ""},
		"nowhere": {"", ""},
	} {
		if pl, tab := placeOf(page); pl != want[0] || tab != want[1] {
			t.Errorf("placeOf(%q) = %q, %q; want %q, %q", page, pl, tab, want[0], want[1])
		}
	}
}

func TestNavPlacesFromOldLayout(t *testing.T) {
	m := &mainUI{}
	// The layout as settings have long saved it, the user's order kept.
	m.st.UI.Nav = []string{"home", "logs", "proxies", "tailscale", "rules", "profiles", "settings"}
	main, foot := m.navPlaces()
	if want := []string{"home", "activity", "proxies", "tailscale", "profiles"}; !slices.Equal(main, want) {
		t.Errorf("main = %v, want %v", main, want)
	}
	if want := []string{"rules", "settings"}; !slices.Equal(foot, want) {
		t.Errorf("foot = %v, want %v", foot, want)
	}
	// Activity shows only the tabs the layout keeps.
	if got := m.tabsOf("activity"); !slices.Equal(got, []string{"logs"}) {
		t.Errorf("activity tabs = %v", got)
	}
	// Settings cannot be hidden.
	m.st.UI.Nav = []string{"home"}
	if _, foot := m.navPlaces(); !slices.Equal(foot, []string{"settings"}) {
		t.Errorf("foot without settings = %v", foot)
	}
	// The default layout gives all the places.
	m.st.UI.Nav = config.Defaults().UI.Nav
	main, foot = m.navPlaces()
	if len(main)+len(foot) != len(places) {
		t.Errorf("default layout: %v + %v", main, foot)
	}
}

func TestShowAndBack(t *testing.T) {
	m := &mainUI{page: "home"}
	m.goTo("logs", "")
	if m.page != "activity" || m.subOf("activity") != "logs" || m.nav.dir != 1 {
		t.Fatalf("to logs: page %q tab %q dir %v", m.page, m.subOf("activity"), m.nav.dir)
	}
	m.goTo("connections", "")
	if m.nav.tabDir != -1 {
		t.Errorf("connections lies before logs: tabDir %v", m.nav.tabDir)
	}
	m.goTo("settings", "sync")
	if m.page != "settings" || m.tab != "sync" {
		t.Errorf("settings/sync: %q %q", m.page, m.tab)
	}
	m.goBack()
	if m.page != "activity" || m.subOf("activity") != "connections" || m.nav.dir != -1 {
		t.Errorf("back: page %q tab %q dir %v", m.page, m.subOf("activity"), m.nav.dir)
	}
	m.goBack()
	m.goBack()
	if m.page != "home" || len(m.nav.hist) != 0 {
		t.Errorf("back to the start: page %q, %d left", m.page, len(m.nav.hist))
	}
	m.goBack() // nothing left: stays
	if m.page != "home" {
		t.Errorf("back past the start: %q", m.page)
	}
}

func TestBackIntoTile(t *testing.T) {
	m := &mainUI{page: "home"}
	m.show("activity", "stats", "speed", false)
	if !m.nav.instant {
		t.Error("a page a sheet grew into takes its place without moving")
	}
	m.goBack()
	if m.page != "home" || m.dash.sheetKey != "speed" || !m.dash.backing {
		t.Errorf("back: page %q sheet %q backing %v", m.page, m.dash.sheetKey, m.dash.backing)
	}
}
