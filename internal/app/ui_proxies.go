package app

import (
	"cmp"
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
)

// proxiesPage is what the proxies page keeps between frames.
type proxiesPage struct {
	view    ProxiesView
	err     string
	loading bool
	loaded  homeKey
	has     bool

	search   string
	sort     string // "", delay or name
	hideDead bool
	hide     string // the pattern being typed
	hideSet  bool

	delays   map[string]map[string]int // by group, then by member
	testing  map[string]bool           // group/member, or the group
	pending  map[string]string         // the selection asked for, by group
	busy     map[string]string         // what a provider does, by provider
	focus    string                    // a group to bring into view, once
	flash    string
	flashAt  time.Time
	scroller ui.ScrollState
	dial     string // the group shown
}

// coreDown stands in for what a page shows while the core is not running:
// why, and a button to start it again.
func (m *mainUI) coreDown(c *ui.Context) {
	p, core := m.pal, m.state.Core
	starting, failed := core.Status == coremgr.StatusStarting, core.Status == coremgr.StatusError
	title, hint := locales.CommonCoreNotRunning(), locales.CoreStoppedHint()
	switch {
	case starting:
		title, hint = locales.HomeStateStarting(), locales.CoreStartingHint()
	case failed:
		title, hint = locales.HomeStateError(), cmp.Or(core.Error, locales.CoreErrorHint())
	}
	m.empty(c, title, func() {
		ui.Text(c, hint).FontSize(12.5).TextColor(p.TextMuted).TextAlign(ui.Center).MaxWidth(420)
		label := locales.CoreStart()
		if failed {
			label = locales.CoreRestart()
		}
		if starting || m.home.busy == "core" {
			ui.Spinner(c)
		} else if m.button(c, "start-core", label, icRotateCw, "primary").Clicked() {
			m.home.busy = "core"
			m.run(locales.CommonFailed(), func() error { return Core{m.a}.Restart(context.Background()) }, func(error) { m.home.busy = "" })
		}
	})
}

// banner is a line across a page, in the tone of what it says.
func (m *mainUI) banner(c *ui.Context, tone, text string, action func()) {
	p := m.pal
	bg, fg := p.AccentSofter, p.AccentFg
	switch tone {
	case "danger":
		bg, fg = p.DangerSoft, p.Danger
	case "warning":
		bg, fg = p.WarningSoft, p.Warning
	}
	ui.Row(c.Key("banner-"+tone+text)).Padding(10, 12).Gap(10).Radius(8).Background(bg).AlignItems(ui.Center).Transition(noticeIn).Children(func() {
		ui.Text(c, text).FontSize(12.5).TextColor(fg).Grow(1).Basis(0)
		if action != nil {
			action()
		}
	})
}

// modeSwitch is the mode's segments, in the header of the pages that
// depend on it.
func (m *mainUI) modeSwitch(c *ui.Context) {
	modes := []string{"rule", "global", "direct"}
	if i := m.segmented(c, "mode", slices.Index(modes, m.st.Clash.Mode), locales.ModeRule(), locales.ModeGlobal(), locales.ModeDirect()); i >= 0 {
		m.st.Clash.Mode = modes[i] // at once; the settings confirm
		m.run(locales.CommonFailed(), func() error { return Core{m.a}.SetMode(context.Background(), modes[i]) }, nil)
	}
}

func (m *mainUI) proxiesView(c *ui.Context) {
	s, p := &m.proxies, m.pal
	mode := m.st.Clash.Mode
	running := m.state.Core.Status == coremgr.StatusRunning
	reload := m.loadProxies()
	m.header(c, locales.NavProxies(), func() {
		if m.spinButton(c, "reload", icRefreshCw, locales.CommonRefresh(), s.loading).Clicked() {
			reload()
		}
		m.modeSwitch(c)
	})

	var groups []*ProxyGroup
	switch {
	case mode == "global" && s.view.Global != nil:
		groups = []*ProxyGroup{s.view.Global}
	case mode != "direct":
		for i := range s.view.Groups {
			if !s.view.Groups[i].Hidden {
				groups = append(groups, &s.view.Groups[i])
			}
		}
	}
	// A bad regular expression filters nothing, rather than breaking the page.
	var hide *regexp.Regexp
	if v := strings.TrimSpace(m.st.UI.ProxyHide); v != "" {
		hide, _ = regexp.Compile("(?i)" + v)
	}
	// A group another page pointed to, such as the target of a rule: show
	// it, and light its tile up for a moment.
	if s.focus != "" && slices.ContainsFunc(groups, func(g *ProxyGroup) bool { return g.Name == s.focus }) {
		s.search, s.dial = "", s.focus
		s.flash, s.flashAt, s.focus = s.focus, c.Now(), ""
	}
	if s.flash != "" {
		if c.Now().Sub(s.flashAt) > 1400*time.Millisecond {
			s.flash = ""
		} else {
			c.AnimationFrame()
		}
	}
	var sel *ProxyGroup
	for _, g := range groups {
		if g.Name == s.dial {
			sel = g
		}
	}
	if sel == nil && len(groups) > 0 {
		sel = groups[0]
	}

	state := "ready"
	switch {
	case !running:
		state = "down"
	case !s.has && s.err == "":
		state = "loading"
	case s.err != "" && !s.has:
		state = "error"
	case mode == "direct":
		state = "direct"
	case len(groups) == 0:
		state = "none"
	}
	// The page crossfades between its states: loading to its groups, a
	// mode with groups to direct and back.
	m.swapFill(c, "proxies-state", state, func() {
		if state != "ready" {
			ui.Scroll(c.Key("proxies")).Grow(1).Padding(0, 22, 22).Gap(12).Children(func() {
				switch state {
				case "down":
					m.coreDown(c)
				case "loading":
					ui.Column(c).Height(200).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Spinner(c) })
				case "error":
					m.banner(c, "danger", s.err, nil)
				case "direct":
					m.banner(c, "info", locales.ProxiesDirectMode(), nil)
				default:
					m.empty(c, locales.ProxiesNoGroups(), func() {
						ui.Text(c, locales.ProxiesNoGroupsHint()).FontSize(12.5).TextColor(p.TextMuted)
					})
				}
			})
			return
		}
		m.proxiesReady(c, groups, sel, hide, reload)
	})
}

// proxiesReady is the proxies page with groups to show.
func (m *mainUI) proxiesReady(c *ui.Context, groups []*ProxyGroup, sel *ProxyGroup, hide *regexp.Regexp, reload func()) {
	s, p := &m.proxies, m.pal
	// The groups in a column of tiles, the one shown on the tile beside
	// them; on a narrow window the groups make a row above it.
	// The width is the box's around them, which stays: building the row
	// and then the column under one key would leave the row there too,
	// empty, taking half the height.
	wrap := ui.Column(c.Key("px")).Grow(1).MinHeight(0).Padding(0, 22, 22)
	narrow := wrap.LayoutBounds().W > 0 && wrap.LayoutBounds().W < 760
	wrap.Children(func() {
		var body ui.Element
		if narrow {
			body = ui.Column(c.Key("narrow")).Grow(1).MinHeight(0).Gap(12)
		} else {
			body = ui.Row(c.Key("wide")).Grow(1).MinHeight(0).Gap(12).AlignItems(ui.Stretch)
		}
		body.Children(func() {
			if len(groups) > 1 {
				list := ui.Scroll(c.Key("groups")).TrackScroll(m.keepScroll("groups")).Gap(8).Shrink(0)
				if narrow {
					list.Row().Height(78).ScrollbarAxes(true, false)
				} else {
					list.Width(236)
				}
				list.Children(func() {
					for _, g := range groups {
						t := m.groupTile(c, g, g == sel)
						if narrow {
							t.Width(190).Shrink(0)
						}
						if t.Clicked() && g != sel {
							s.dial = g.Name
						}
					}
				})
			}
			ui.Column(c.Key("main")).Grow(1).Basis(0).MinWidth(0).MinHeight(0).Background(p.Card).Radius(tileRadius).Clip().Children(func() {
				ui.Scroll(c.Key("nodes")).TrackScroll(m.keepScroll("nodes-"+sel.Name)).Grow(1).Padding(18, 20, 20).Gap(16).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
						ui.SearchField(c.Key("search"), &s.search).Placeholder(locales.ProxiesSearch()).Width(200)
						sorts := []string{locales.ProxiesSortDefault(), locales.ProxiesSortDelay(), locales.ProxiesSortName()}
						cur := sorts[max(0, slices.Index([]string{"", "delay", "name"}, s.sort))]
						if ui.Select(c.Key("sort"), &cur, sorts).Width(130).Changed() {
							s.sort = []string{"", "delay", "name"}[slices.Index(sorts, cur)]
						}
						variant := "ghost"
						if s.hideDead {
							variant = "primary"
						}
						if m.button(c, "dead", locales.ProxiesHideDead(), icEyeOff, variant).Tooltip(locales.ProxiesHideDeadTip()).Clicked() {
							s.hideDead = !s.hideDead
						}
						hideIn := ui.TextInput(c.Key("hide"), &s.hide).Placeholder(locales.ProxiesHidePh()).Width(150).Tooltip(locales.ProxiesHideTip())
						if v := strings.TrimSpace(s.hide); v != m.st.UI.ProxyHide && (hideIn.Submitted() || !hideIn.Focused()) {
							m.st.UI.ProxyHide = v
							m.patch(map[string]any{"ui": map[string]any{"proxyHide": v}})
						}
					})
					// The group's nodes come in anew as another group is shown.
					ui.Column(c.Key("g-" + sel.Name)).Gap(14).Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 8}, Duration: 260 * time.Millisecond, Ease: easeOut}).Children(func() {
						m.proxyGroup(c, sel, hide, reload)
					})
				})
			})
		})
	})
}

// loadProxies loads the proxies, the groups and the providers, as the
// runtime or the selections change; it returns the way to load them again.
func (m *mainUI) loadProxies() func() {
	s, a := &m.proxies, m.a
	if s.delays == nil {
		s.delays, s.testing, s.pending, s.busy = map[string]map[string]int{}, map[string]bool{}, map[string]string{}, map[string]string{}
	}
	if !s.hideSet {
		s.hideSet, s.hide = true, m.st.UI.ProxyHide
	}
	running := m.state.Core.Status == coremgr.StatusRunning
	reload := func() { s.loaded = homeKey{} }
	if key := (homeKey{m.runtime + 1, m.selection, running, ""}); key != s.loaded && !s.loading && running {
		s.loaded, s.loading = key, true
		win := m.window()
		go func() {
			ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
			defer cancel()
			v, err := (Proxies{a}).View(ctx)
			win.Update(func() {
				s.loading = false
				if err != nil {
					s.err = err.Error()
					return
				}
				s.view, s.err, s.has = v, "", true
				clear(s.pending)
			})
		}()
	}
	return reload
}

// providersView is the proxy providers, a tab of the proxies.
func (m *mainUI) providersView(c *ui.Context) {
	s := &m.proxies
	reload := m.loadProxies()
	running := m.state.Core.Status == coremgr.StatusRunning
	m.header(c, locales.ProxiesProviders(), func() {
		if m.spinButton(c, "reload", icRefreshCw, locales.CommonRefresh(), s.loading).Clicked() {
			reload()
		}
	})
	ui.Scroll(c.Key("providers")).TrackScroll(m.keepScroll("providers")).Grow(1).Padding(0, 22, 22).Gap(12).Children(func() {
		switch {
		case !running:
			m.coreDown(c)
		case !s.has && s.err == "":
			ui.Column(c).Height(200).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Spinner(c) })
		case len(s.view.Providers) == 0:
			m.empty(c, locales.ProxiesProviders(), nil)
		default:
			m.providersPanel(c, reload)
		}
	})
}

// chainsView is the proxy chains, a tab of the proxies.
func (m *mainUI) chainsView(c *ui.Context) {
	m.header(c, m.T("settings.chain"), nil)
	ui.Scroll(c.Key("chains")).TrackScroll(m.keepScroll("chains")).Grow(1).Padding(0, 22, 22).Children(func() {
		formColumn(c).Children(func() {
			m.section(c, "chain", locales.SettingsChain(), func() { m.chainsEditor(c) })
		})
	})
}

// groupTile is a group in the column: its name, where it goes now and how
// far that is; the accent's soft fill when shown, and a glow of it for a
// moment when another page pointed to it.
func (m *mainUI) groupTile(c *ui.Context, g *ProxyGroup, on bool) ui.Element {
	s, p := &m.proxies, m.pal
	b := ui.ButtonBase(c.Key("gt-"+g.Name)).Column().Padding(13, 16).Gap(4).Radius(16).AlignItems(ui.Stretch).Shrink(0).Label(g.Name)
	bg := p.Card
	switch {
	case on:
		bg = p.AccentSoft
	case b.Hovered():
		bg = p.Surface2
	}
	if s.flash == g.Name {
		k := float32(1)
		if t := float32(c.Now().Sub(s.flashAt).Seconds()); t > 0.63 {
			k = 1 - ui.EaseOut(min(1, (t-0.63)/0.77))
		}
		bg = bg.Mix(p.Card.Mix(p.Accent, 0.3), k)
		b.ScrollIntoView(ui.ScrollIntoViewOptions{Animation: &ui.Smooth})
	}
	kp := b.AnimateTo("press", map[bool]float32{true: 0.98, false: 1}[b.Pressed()], pressSpring)
	b.Scale(kp, kp).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 160 * time.Millisecond}))
	now := g.Now
	if v := s.pending[g.Name]; v != "" {
		now = v
	}
	delay := -1
	for _, it := range g.All {
		if it.Name == now {
			delay = it.Delay
		}
	}
	if d, ok := s.delays[g.Name][now]; ok {
		delay = d
	}
	b.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			col := p.TextMuted
			if on {
				col = p.AccentFg
			}
			name := ui.Text(c, g.Name).FontSize(13.5).SingleLine().Grow(1).Basis(0)
			name.TextColor(name.AnimateColor("fg", col, ui.Animation{Duration: 160 * time.Millisecond}))
			switch {
			case delay > 0:
				ui.Text(c, fmt.Sprintf("%d ms", delay)).FontSize(12.5).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll)
			case delay == 0:
				ui.Text(c, locales.CommonTimeout()).FontSize(12.5).TextColor(p.Danger)
			}
		})
		ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
			if cc := flagOf(now); cc != "" {
				m.flagSmall(c, cc)
			}
			ui.Text(c.Key("n-"+now), cmp.Or(stripFlag(now), "—")).FontSize(16).FontWeight(650).SingleLine().Shrink(1).
				Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 8}, Exit: &ui.Motion{Y: -8}, Duration: 300 * time.Millisecond, ExitDuration: 160 * time.Millisecond, Ease: easeOut})
		})
	})
	return b
}

// segmentedIcons is segmented with icons for labels.
func (m *mainUI) segmentedIcons(c *ui.Context, key any, selected int, icons ...*ui.SVG) int {
	return m.iconSegments(c, key, selected, icons...)
}

// proxyGroup is the group shown: its name and what it is, the way to test
// it, and its nodes as keys.
func (m *mainUI) proxyGroup(c *ui.Context, g *ProxyGroup, hide *regexp.Regexp, reload func()) {
	s, p, a := &m.proxies, m.pal, m.a
	delays := s.delays[g.Name]
	delayOf := func(it *ProxyItem) int {
		if d, ok := delays[it.Name]; ok {
			return d
		}
		return it.Delay
	}
	now := g.Now
	if v := s.pending[g.Name]; v != "" {
		now = v
	}
	selectable := g.Type == "Selector" || g.Type == "URLTest" || g.Type == "Fallback"
	q := strings.ToLower(strings.TrimSpace(s.search))
	var items []*ProxyItem
	for i := range g.All {
		it := &g.All[i]
		if q != "" && !strings.Contains(strings.ToLower(it.Name), q) && !strings.Contains(strings.ToLower(it.Type), q) {
			continue
		}
		// Hiding the unavailable keeps the selected one, so the group still shows where it goes.
		if it.Name != g.Now {
			d, tested := delays[it.Name]
			if s.hideDead && ((tested && d == 0) || (!tested && !it.Alive)) {
				continue
			}
			if hide != nil && hide.MatchString(it.Name) {
				continue
			}
		}
		items = append(items, it)
	}
	switch s.sort {
	case "name":
		slices.SortStableFunc(items, func(x, y *ProxyItem) int { return strings.Compare(x.Name, y.Name) })
	case "delay":
		rank := func(it *ProxyItem) int {
			switch d := delayOf(it); {
			case d > 0:
				return d
			case d == 0:
				return math.MaxInt32
			}
			return math.MaxInt32 - 1
		}
		slices.SortStableFunc(items, func(x, y *ProxyItem) int { return rank(x) - rank(y) })
	}
	testAll := func() {
		s.testing[g.Name] = true
		group, url, members := g.Name, g.TestURL, g.All
		win := m.window()
		go func() {
			res, err := (Proxies{a}).GroupDelay(context.Background(), group, url)
			if err != nil {
				m.notice(Notice{Level: "error", Message: locales.ProxiesTestFailed(), Detail: err.Error()})
			}
			win.Update(func() {
				delete(s.testing, group)
				if err == nil {
					all := map[string]int{}
					for _, it := range members {
						all[it.Name] = res[it.Name]
					}
					s.delays[group] = all
				}
				reload()
			})
		}()
	}
	testOne := func(it *ProxyItem) {
		key := g.Name + "/" + it.Name
		if s.testing[key] {
			return
		}
		s.testing[key] = true
		group, name, url := g.Name, it.Name, g.TestURL
		win := m.window()
		go func() {
			// A node that does not answer gives 0, a timeout; an error is the app's.
			d, err := (Proxies{a}).Delay(context.Background(), name, url)
			if err != nil {
				m.notice(Notice{Level: "error", Message: locales.ProxiesTestFailed(), Detail: err.Error()})
			}
			win.Update(func() {
				delete(s.testing, key)
				if err == nil {
					if s.delays[group] == nil {
						s.delays[group] = map[string]int{}
					}
					s.delays[group][name] = d
				}
			})
		}()
	}
	choose := func(it *ProxyItem) {
		group, name := g.Name, it.Name
		s.pending[group] = name // at once; the core confirms
		m.run(locales.ProxiesSelectFailed(), func() error { return Proxies{a}.Select(context.Background(), group, name) }, func(err error) {
			if err != nil {
				delete(s.pending, group)
			}
			reload()
		})
	}

	ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Text(c, g.Name).FontSize(22).FontWeight(680).SingleLine().Shrink(1)
		m.badge(c, m.groupType(g.Type), "")
		if g.Fixed != "" {
			m.stateBadge(c, "fixed", locales.ProxiesFixed(), "info")
		}
		ui.Text(c, itoa(len(g.All))).FontSize(12.5).TextColor(p.Faint).FontFeatures("tnum").ContentTransition(numRoll)
		ui.Spacer(c)
		if g.Fixed != "" && m.iconButton(c, "unfix", icPinOff, locales.ProxiesUnfix()).Clicked() {
			group := g.Name
			m.run(locales.CommonFailed(), func() error { return Proxies{a}.Unfix(context.Background(), group) }, func(error) { reload() })
		}
		layouts := []string{"card", "list"}
		if i := m.segmentedIcons(c, "layout", max(0, slices.Index(layouts, m.st.UI.ProxyLayout)), icLayoutGrid, icList); i >= 0 {
			m.st.UI.ProxyLayout = layouts[i]
			m.patch(map[string]any{"ui": map[string]any{"proxyLayout": layouts[i]}})
		}
		if m.bigChip(c, "test", locales.ProxiesTestGroup(), icZap, false, s.testing[g.Name]).Clicked() && !s.testing[g.Name] {
			testAll()
		}
	})
	if len(items) == 0 {
		m.empty(c, locales.ConnEmpty(), nil)
		return
	}
	list := m.st.UI.ProxyLayout == "list"
	cols := func(w float32) int {
		switch {
		case list:
			return 1
		case m.st.UI.ProxyColumns > 0:
			return m.st.UI.ProxyColumns
		}
		return int((w + 10) / (196 + 10))
	}
	gap := float32(10)
	if list {
		gap = 4
	}
	m.flipGrid(c, "grid", gap, cols, &nodeMotion, len(items), func(i int) any { return items[i].Name }, func(i int, box ui.Element) {
		it := items[i]
		box.Children(func() {
			active := it.Name == now
			testing := s.testing[g.Name+"/"+it.Name] || s.testing[g.Name]
			var tile, test ui.Element
			if list {
				tile, test = m.nodeRow(c, it, delayOf(it), active, testing)
			} else {
				tile, test = m.nodeTile(c, "tile", it, delayOf(it), active, testing)
			}
			tile.Grow(1)
			switch {
			case test.Clicked():
				testOne(it)
			case tile.Clicked() && selectable && !active:
				choose(it)
			}
		})
	})
}

// nodeRow is a node as a line of the list layout: its name, what it is,
// and how far it is, which tests it.
func (m *mainUI) nodeRow(c *ui.Context, it *ProxyItem, delay int, active, testing bool) (ui.Element, ui.Element) {
	p := m.pal
	b := ui.ButtonBase(c.Key("row")).Row().Height(46).Padding(0, 14).Gap(10).Radius(12).AlignItems(ui.Center).Tooltip(it.Name)
	bg := p.Card
	switch {
	case active:
		bg = p.AccentSoft
	case b.Hovered():
		bg = p.Surface2
	}
	b.Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 160 * time.Millisecond}))
	var test ui.Element
	b.Children(func() {
		if cc := flagOf(it.Name); cc != "" {
			m.flagSmall(c, cc)
		}
		name := ui.Text(c, stripFlag(it.Name)).FontSize(14.5).FontWeight(600).SingleLine().Shrink(1)
		if active {
			name.TextColor(p.AccentFg)
		}
		ui.Text(c, it.Type).FontSize(12.5).TextColor(p.TextMuted).SingleLine().Grow(1).Basis(0)
		test = ui.ButtonBase(c.Key("delay")).Row().Height(26).Padding(0, 8).Radius(8).AlignItems(ui.Center).Shrink(0)
		if test.Hovered() {
			test.Background(p.Surface3)
		}
		test.Transition(hoverFade)
		test.Children(func() { m.delayText(c, delay, testing) })
	})
	return b, test
}

// providersPanel lists the proxy providers of the configuration.
func (m *mainUI) providersPanel(c *ui.Context, reload func()) {
	s, p, a := &m.proxies, m.pal, m.a
	providers := s.view.Providers
	if len(providers) == 0 {
		return
	}
	act := func(name, kind, failure string, fn func() error) {
		s.busy[name] = kind
		m.run(failure, fn, func(error) {
			delete(s.busy, name)
			reload()
		})
	}
	ui.Row(c).Gap(6).AlignItems(ui.Center).PaddingY(8).Children(func() {
		ui.Icon(c, icServer).FontSize(13).TextColor(p.TextMuted)
		ui.Text(c, locales.ProxiesProviders()).FontSize(12).FontWeight(600).TextColor(p.TextMuted)
		ui.Spacer(c)
		if m.button(c, "update-all", locales.ProxiesUpdateAll(), icRefreshCw, "ghost").Clicked() {
			names := make([]string, len(providers))
			for i, pr := range providers {
				names[i] = pr.Name
				s.busy[pr.Name] = "update"
			}
			m.run(locales.ProxiesProviderUpdateFailed(), func() error {
				var first error
				for _, n := range names {
					if err := (Proxies{a}).UpdateProvider(context.Background(), n); err != nil && first == nil {
						first = err
					}
				}
				return first
			}, func(error) {
				clear(s.busy)
				reload()
			})
		}
	})
	m.flipGrid(c, "providers", 10, func(w float32) int { return int((w + 10) / (280 + 10)) }, &reflow, len(providers), func(i int) any { return providers[i].Name }, func(i int, box ui.Element) {
		box.Children(func() {
			pr := &providers[i]
			card := ui.Column(c).Grow(1).Background(p.Surface2).Radius(14).Padding(12, 14).Gap(8)
			card.Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
						ui.Text(c, pr.Name).FontWeight(620).SingleLine()
						updated := "—"
						if pr.UpdatedAt != nil {
							updated = m.relTime(*pr.UpdatedAt)
						}
						ui.Text(c, pr.VehicleType+" · "+locales.ProfilesNodes(fmt.Sprint(pr.Count))+" · "+updated).FontSize(12).TextColor(p.TextMuted).SingleLine()
					})
					name := pr.Name
					if m.spinButton(c, "check", icStethoscope, locales.ProxiesHealthcheck(), s.busy[name] == "check").Clicked() {
						act(name, "check", locales.CommonFailed(), func() error { return Proxies{a}.HealthcheckProvider(context.Background(), name) })
					}
					if m.spinButton(c, "update", icRefreshCw, locales.ProxiesUpdateProvider(), s.busy[name] == "update").Clicked() {
						act(name, "update", locales.ProxiesProviderUpdateFailed(), func() error { return Proxies{a}.UpdateProvider(context.Background(), name) })
					}
				})
				if info := pr.SubscriptionInfo; info != nil && info.Total > 0 {
					used := info.Upload + info.Download
					m.progress(c, fmtPercent(used, info.Total))
					ui.Row(c).Children(func() {
						ui.Text(c, fmtBytes(used)+" / "+fmtBytes(info.Total)).FontSize(11.5).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll)
						ui.Spacer(c)
						if info.Expire > 0 {
							ui.Text(c, locales.ProfilesExpires(fmtDate(info.Expire))).FontSize(11.5).TextColor(p.TextMuted)
						}
					})
				}
			})
		})
	})
}
