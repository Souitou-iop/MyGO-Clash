package app

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"
)

// The navigation's places: the home page, the proxies, the profiles, what
// goes on (connections, traffic and logs), connectivity, Tailscale, and
// at its foot the rules and the settings. The pages that
// were places of their own live on as the tabs of theirs, under the names
// links, notices, the settings' layout and the debug API know them by.

// places are the places of the navigation, from top to bottom.
var places = []string{"home", "proxies", "profiles", "activity", "connectivity", "tailscale", "rules", "settings"}

// footPlaces stand at the navigation's foot, apart from the others.
var footPlaces = []string{"rules", "settings"}

// placeTabs are the tabs of the places that have them, left to right.
var placeTabs = map[string][]string{
	"proxies":  {"groups", "providers", "chains"},
	"activity": {"connections", "stats", "logs"},
}

var placeIcons = map[string]*ui.SVG{
	"home": icLayoutDashboard, "proxies": icGlobe2, "profiles": icFileStack, "activity": icActivity,
	"connectivity": icRadar, "tailscale": icWaypoints, "rules": icListFilter, "settings": icSettings,
}

// pageMotion is how long a page or a tab takes to come in and go.
const pageMotion = 220 * time.Millisecond

// navEntry is a place left, to go back to: its tab, and the tile of the
// home page whose sheet grew into the place left for, if one did.
type navEntry struct {
	place, tab, tile string
}

// navState is where the navigation is, and where it was.
type navState struct {
	sub     map[string]string // the tab each place shows
	hist    []navEntry
	dir     float32 // 1: the place shown came from further down; -1 up
	tabDir  float32 // 1: the tab shown came from further along; -1 back
	instant bool    // the place shown took the place of a sheet that grew into it
	ahead   string  // the place the navigation's selection went to, before the page
	scrolls map[string]*ui.ScrollState
	tabsW   float32 // the width of the tabs over a page's header
	inTabs  bool    // a page shown as a tab is being built
}

// placeOf is the place and the tab of a page, as links name it.
func placeOf(page string) (place, tab string) {
	for pl, tabs := range placeTabs {
		if slices.Contains(tabs, page) {
			return pl, page
		}
	}
	if page == "proxies" {
		return "proxies", ""
	}
	if slices.Contains(places, page) {
		return page, ""
	}
	return "", ""
}

// placePages are the pages a place shows, as the settings' layout lists
// the navigation: the place itself, or the pages that are its tabs.
func placePages(place string) []string {
	switch place {
	case "activity":
		return placeTabs[place]
	}
	return []string{place}
}

// navPages are the pages the user keeps in the navigation, as its layout
// setting lists them.
func (m *mainUI) navPages() []string {
	if nav := m.st.UI.Nav; len(nav) > 0 {
		return nav
	}
	return pages
}

// navPlaces are the places the navigation shows, in the order of the
// layout setting, with the rules and the settings at its foot.
func (m *mainUI) navPlaces() (main, foot []string) {
	var all []string
	for _, pg := range m.navPages() {
		if pl, _ := placeOf(pg); pl != "" && !slices.Contains(all, pl) {
			all = append(all, pl)
		}
	}
	if !slices.Contains(all, "settings") {
		all = append(all, "settings")
	}
	for _, pl := range all {
		if slices.Contains(footPlaces, pl) {
			continue
		}
		main = append(main, pl)
	}
	for _, pl := range footPlaces {
		if slices.Contains(all, pl) {
			foot = append(foot, pl)
		}
	}
	return main, foot
}

// tabsOf are the tabs a place shows: of activity, those whose
// pages the layout keeps.
func (m *mainUI) tabsOf(place string) []string {
	tabs := placeTabs[place]
	if place == "proxies" {
		return tabs
	}
	nav := m.navPages()
	var out []string
	for _, t := range tabs {
		if slices.Contains(nav, t) {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return tabs
	}
	return out
}

// subOf is the tab a place shows.
func (m *mainUI) subOf(place string) string {
	tabs := m.tabsOf(place)
	if t := m.nav.sub[place]; slices.Contains(tabs, t) {
		return t
	}
	if len(tabs) > 0 {
		return tabs[0]
	}
	return ""
}

// showing reports whether a page shows: its place, and its tab.
func (m *mainUI) showing(page string) bool {
	pl, tab := placeOf(page)
	return m.page == pl && (tab == "" || m.subOf(pl) == tab)
}

// navOrder is the order of the places down the navigation.
func (m *mainUI) navOrder() []string {
	main, foot := m.navPlaces()
	order := append(slices.Clone(main), foot...)
	for _, pl := range places {
		if !slices.Contains(order, pl) {
			order = append(order, pl)
		}
	}
	return order
}

// show goes to a place and a tab of it (none keeps the one it shows):
// the page comes in from the side of the navigation it lies on, the tab
// from the side of the tabs. Unless back, the place left is kept to go
// back to, with the tile whose sheet grew into the new place, if any.
func (m *mainUI) show(place, tab, tile string, back bool) {
	n := &m.nav
	if n.sub == nil {
		n.sub = map[string]string{}
	}
	if place == "" {
		return
	}
	cur := m.subOf(m.page)
	if tab == "" {
		tab = m.subOf(place)
	}
	if place == m.page && (tab == "" || tab == cur) {
		return
	}
	if !back && m.page != "" {
		n.hist = append(n.hist, navEntry{m.page, cur, tile})
		if len(n.hist) > 50 {
			n.hist = n.hist[len(n.hist)-50:]
		}
	}
	if place == m.page {
		tabs := m.tabsOf(place)
		n.tabDir = 1
		if slices.Index(tabs, tab) < slices.Index(tabs, cur) {
			n.tabDir = -1
		}
	} else {
		order := m.navOrder()
		n.dir = 1
		if slices.Index(order, place) < slices.Index(order, m.page) {
			n.dir = -1
		}
		n.tabDir = 0
	}
	if tab != "" {
		n.sub[place] = tab
	}
	n.instant, n.ahead = tile != "" && !back, ""
	m.page = place
}

// goBack goes back to the place left last, the way it came reversed; to
// the home page, into the tile whose sheet grew out of it.
func (m *mainUI) goBack() {
	n := &m.nav
	if len(n.hist) == 0 || m.dash.toPlace != "" {
		return
	}
	e := n.hist[len(n.hist)-1]
	n.hist = n.hist[:len(n.hist)-1]
	m.show(e.place, e.tab, "", true)
	if e.tile != "" && e.place == "home" {
		n.instant = true
		m.sheetBack(e.tile)
	}
}

// keepScroll is the scroll of a page's container known by key, kept while
// the page is away, so that it comes back where it was.
func (m *mainUI) keepScroll(key string) *ui.ScrollState {
	n := &m.nav
	if n.scrolls == nil {
		n.scrolls = map[string]*ui.ScrollState{}
	}
	s := n.scrolls[key]
	if s == nil {
		s = &ui.ScrollState{}
		n.scrolls[key] = s
	}
	return s
}

// navSelected is the place the navigation shows selected: where it went,
// ahead of the page while a sheet grows into it.
func (m *mainUI) navSelected() string {
	if m.nav.ahead != "" {
		return m.nav.ahead
	}
	return m.page
}

// tabbed is a place of tabs: the tab shown, which comes in from the side
// of the tabs it lies on, and over its header the tabs, which stay.
func (m *mainUI) tabbed(c *ui.Context, place string, views map[string]func()) {
	n := &m.nav
	tabs := m.tabsOf(place)
	sub := m.subOf(place)
	dx := 14 * n.tabDir
	if c.Direction() == ui.RightToLeft {
		dx = -dx
	}
	host := ui.Box(c.Key("tabs-" + place)).Grow(1).Basis(0).MinHeight(0).MinWidth(0)
	host.ChildExit(ui.Motion{X: -dx})
	host.Children(func() {
		page := ui.Column(c.Key("tab-" + sub)).Absolute().Left(0).Top(0).Right(0).Bottom(0)
		page.Transition(ui.ElementTransition{Enter: &ui.Motion{X: dx}, Exit: &ui.Motion{X: -dx}, Duration: pageMotion, Ease: easeOut})
		page.Children(func() {
			n.inTabs = len(tabs) > 1
			if fn := views[sub]; fn != nil {
				fn()
			}
			n.inTabs = false
		})
		if len(tabs) < 2 {
			n.tabsW = 0
			return
		}
		labels := make([]string, len(tabs))
		for i, t := range tabs {
			labels[i] = m.tabLabel(t)
		}
		bar := c.TitleBar()
		row := ui.Row(c.Key("tabbar")).Absolute().Top(0).Start(16).Height(max(bar.Height, 44) + 8).AlignItems(ui.Center)
		row.Children(func() {
			sel := slices.Index(tabs, sub)
			if m.tabs(c, "tabs", &sel, labels...) {
				m.show(place, tabs[sel], "", false)
			}
		})
		n.tabsW = row.LayoutBounds().W
	})
}

// tabLabel names a tab of a place.
func (m *mainUI) tabLabel(tab string) string {
	switch tab {
	case "groups":
		return m.T("proxies.groups")
	case "providers":
		return m.T("proxies.providers")
	case "chains":
		return m.T("settings.chain")
	}
	return m.T("nav." + tab)
}
