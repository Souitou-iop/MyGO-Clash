package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/tools"
)

// The connectivity page leads with four tiles, each the gist of one check:
// where the traffic leaves, how many sites open, which services work and
// the speed. A press on a tile shows its details below, the sites by
// category.

const (
	ctyExit = iota
	ctySites
	ctyUnlock
	ctySpeed
)

// siteKindOf rates a site's round. A site's test is a whole HTTP request
// through the proxy, so it takes longer than a node's delay test, and the
// node's bounds would call nearly every site slow.
func siteKindOf(d int) delayKind {
	switch {
	case d < 0:
		return delayNone
	case d == 0:
		return delayBad
	case d < 400:
		return delayGood
	case d < 1000:
		return delayOK
	}
	return delayBad
}

func siteColorOf(p *palette, d int) ui.Color {
	switch siteKindOf(d) {
	case delayGood:
		return p.Success
	case delayOK:
		return p.Warning
	case delayBad:
		return p.Danger
	}
	return p.Faint
}

func (m *mainUI) connectivityView(c *ui.Context) {
	s, a := &m.cty, m.a
	running := m.state.Core.Status == coremgr.StatusRunning
	m.ctyInit()
	m.ctyFollow()
	// Where checks go out now.
	if key := (homeKey{m.runtime, m.selection, running, m.st.Clash.Mode}); key != s.exitAt && running {
		s.exitAt = key
		mode, win := m.st.Clash.Mode, m.window()
		go func() {
			v, err := (Proxies{a}).View(context.Background())
			if err == nil {
				win.Update(func() {
					s.exit = exitNode(v, mode)
					if mode == "direct" {
						s.exit = "DIRECT"
					}
				})
			}
		}()
	}
	m.ctyStart(running)
	busy := s.sitesBusy || len(s.checking) > 0 || slices.ContainsFunc(exitViews, func(v string) bool { return s.ips[v] != nil && s.ips[v].loading })
	m.header(c, locales.NavConnectivity(), func() {
		if m.button(c, "all", locales.UnlockCheckAll(), icPlay, "primary").Disabled(busy || !running).Clicked() {
			m.loadExitIPs()
			m.testCtySites()
			m.checkUnlock(nil)
		}
	})
	ui.Scroll(c.Key("cty")).TrackScroll(m.keepScroll("cty")).Grow(1).Padding(4, 22, 22).Gap(14).Children(func() {
		if !running {
			m.banner(c, "warning", locales.CommonCoreNotRunning(), func() {
				if m.button(c, "start", locales.CoreStart(), icRotateCw, "").Clicked() {
					m.run(locales.CommonFailed(), func() error { return Core{a}.Restart(context.Background()) }, nil)
				}
			})
		}
		m.ctyTiles(c)
		dx := 16 * s.viewDir
		if c.Direction() == ui.RightToLeft {
			dx = -dx
		}
		detail := ui.Column(c.Key(fmt.Sprint("detail-", s.view))).Gap(10)
		detail.Transition(ui.ElementTransition{Enter: &ui.Motion{X: dx}, Duration: pageMotion, Ease: easeOut})
		detail.Children(func() {
			switch s.view {
			case ctyExit:
				m.exitDetail(c, running)
			case ctyUnlock:
				m.unlockDetail(c, running)
			case ctySpeed:
				m.speedDetail(c, running)
			default:
				m.sitesDetail(c, running)
			}
		})
	})
	m.sitesDialog(c, running)
	m.unlockDialog(c)
}

// ---- The tiles ----

func (m *mainUI) ctyTiles(c *ui.Context) {
	s, p := &m.cty, m.pal
	tile := func(i int, title string, extra func(), body func()) {
		on := s.view == i
		t := ui.ButtonBase(c.Key(i)).Column().Grow(1).Basis(0).MinWidth(0).Height(150).Padding(16, 16, 14).Gap(0).Radius(tileRadius).
			AlignItems(ui.Stretch).Justify(ui.Start).Clip().Label(title)
		bg := p.Card
		switch {
		case on:
			bg = p.AccentSofter
		case t.Hovered():
			bg = p.Surface2
		}
		t.Background(t.AnimateColor("bg", bg, ui.Animation{Duration: 180 * time.Millisecond}))
		k := t.AnimateTo("press", map[bool]float32{true: 0.985, false: 1}[t.Pressed()], pressSpring)
		t.Scale(k, k)
		t.Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).MinHeight(20).Children(func() {
				col := p.TextMuted
				if on {
					col = p.AccentFg
				}
				ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(col).SingleLine().Grow(1)
				if extra != nil {
					extra()
				}
			})
			body()
		})
		if t.Clicked() && !on {
			s.viewDir = 1
			if i < s.view {
				s.viewDir = -1
			}
			s.view = i
		}
	}
	big := func(value, of string) {
		ui.Row(c).Gap(4).AlignItems(ui.Baseline).Padding(8, 0, 0).Children(func() {
			ui.Text(c, value).FontSize(30).FontWeight(700).FontFeatures("tnum").ContentTransition(numRoll)
			if of != "" {
				ui.Text(c, of).FontSize(15).FontWeight(600).TextColor(p.TextMuted).FontFeatures("tnum")
			}
		})
	}
	// Four abreast, or two by two when the page is narrow.
	grid := ui.Grid(c.Key("tiles")).Gap(12)
	cols := 4
	if w := grid.LayoutBounds().W; w > 0 && w < 760 {
		cols = 2
	}
	grid.Columns(cols).Children(func() {
		// Where the traffic leaves.
		tile(ctyExit, locales.ConnectivityIp(), func() { m.splitBadge(c) }, func() {
			home, abroad := s.ips["domestic"], s.ips["global"]
			switch {
			case abroad == nil || (abroad.loading && abroad.info == nil):
				ui.Column(c).Padding(12, 0, 0).Gap(8).Children(func() {
					m.skeleton(c, 22).Width(120)
					m.skeleton(c, 14).Width(150)
				})
			case abroad.info == nil:
				ui.Text(c, cmp.Or(abroad.err, "—")).FontSize(12).TextColor(p.Faint).MaxLines(3).Padding(12, 0, 0)
			default:
				info := abroad.info
				ui.Row(c).Gap(10).AlignItems(ui.Center).Padding(12, 0, 0).Children(func() {
					m.flag(c, info.CountryCode, !m.reveal())
					ui.Text(c, m.ctyMask(cmp.Or(info.Country, "—"))).ContentTransition(maskRoll(m.reveal())).FontSize(20).FontWeight(700).SingleLine().Shrink(1)
				})
				ui.Text(c, m.ctyMask(info.IP)).ContentTransition(maskRoll(m.reveal())).FontSize(12.5).Font("ui-monospace").TextColor(p.Text2).SingleLine().Padding(6, 0, 0)
			}
			ui.Spacer(c)
			if home != nil && home.info != nil {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					m.flagMasked(c, home.info.CountryCode, !m.reveal())
					ui.Text(c, locales.ConnectivityGroupDomestic()+" "+m.ctyMask(cmp.Or(home.info.ISP, home.info.Country))).FontSize(12).TextColor(p.TextMuted).SingleLine().Shrink(1)
				})
			}
		})
		// How many sites open, with every round as a cell.
		ok, done := 0, 0
		for _, st := range s.sites {
			if r := s.rounds[st.ID]; len(r) > 0 {
				done++
				if bestRound(r) > 0 {
					ok++
				}
			}
		}
		tile(ctySites, locales.ConnectivitySites(), func() { m.siteVerdict(c) }, func() {
			v := "—"
			if done > 0 {
				v = fmt.Sprint(ok)
			}
			big(v, fmt.Sprintf("/%d", len(s.sites)))
			ui.Spacer(c)
			m.siteMosaic(c)
		})
		// Which services work.
		yes, part, no, checked := 0, 0, 0, 0
		for _, u := range s.unlocks {
			r, has := s.results[u.ID]
			if !has || s.checking[u.ID] {
				continue
			}
			checked++
			switch r.Status {
			case "yes":
				yes++
			case "partial":
				part++
			case "no":
				no++
			}
		}
		tile(ctyUnlock, locales.ConnectivityUnlock(), func() {
			if len(s.checking) > 0 {
				ui.Spinner(c)
				return
			}
			if no > 0 {
				ui.Text(c, fmt.Sprintf("✕ %d", no)).FontSize(12).FontWeight(600).TextColor(p.Danger)
			}
			if part > 0 {
				ui.Text(c, fmt.Sprintf("◐ %d", part)).FontSize(12).FontWeight(600).TextColor(p.Warning)
			}
		}, func() {
			v := "—"
			if checked > 0 {
				v = fmt.Sprint(yes + part)
			}
			big(v, fmt.Sprintf("/%d", len(s.unlocks)))
			ui.Spacer(c)
			// The logos share the tile's width, 16 to 20 points each; those
			// that do not fit are counted at the end.
			row := ui.Row(c.Key("logos")).Gap(6).AlignItems(ui.Center)
			size, shown := float32(20), len(s.unlocks)
			if w, n := row.LayoutBounds().W, float32(len(s.unlocks)); w > 0 && n > 0 {
				size = min(20, max(16, (w-6*(n-1))/n))
				if fit := int((w + 6) / (size + 6)); fit < len(s.unlocks) {
					shown = max(1, fit-1)
				}
			}
			row.Children(func() {
				for i, u := range s.unlocks {
					if i == shown {
						ui.Text(c.Key("more"), fmt.Sprintf("+%d", len(s.unlocks)-shown)).FontSize(11.5).FontWeight(600).TextColor(p.TextMuted).FontFeatures("tnum").SingleLine()
						break
					}
					state := "pending"
					if r, has := s.results[u.ID]; has {
						state = r.Status
					}
					if s.checking[u.ID] {
						state = "checking"
					}
					box := ui.Box(c.Key(u.ID)).Size(size, size).Shrink(0).Tooltip(u.Name + " · " + m.T("unlock."+state))
					box.Children(func() {
						logo := ui.Box(c).Size(size, size)
						if state == "no" || state == "failed" {
							logo.Opacity(0.35)
						}
						logo.Children(func() { m.logo(c, cmp.Or(u.Site, u.ID), u.Name, size) })
						dot := p.BorderStrong
						switch state {
						case "yes":
							dot = p.Success
						case "partial":
							dot = p.Warning
						case "no":
							dot = p.Danger
						}
						d := ui.Box(c).Absolute().Right(-2).Bottom(-2).Size(8, 8).Radius(4).Border(2, p.Card)
						d.Background(d.AnimateColor("c", dot, ui.Animation{Duration: 200 * time.Millisecond}))
						if state == "checking" {
							d.Opacity(1 - 0.55*d.Loop("pulse", time.Second, ui.Bounce(ui.EaseInOut)))
						}
					})
				}
			})
		})
		// The speed, from the last test.
		tile(ctySpeed, locales.SpeedTitle(), func() {
			if s.speedBusy {
				ui.Spinner(c)
			}
		}, func() {
			r := s.speed
			down, up, lat := "—", "—", "—"
			if r != nil && r.Error == "" {
				if r.Download > 0 {
					down = mbps(r.Download)
				}
				if r.Upload > 0 {
					up = mbps(r.Upload)
				}
				if r.Latency > 0 {
					lat = fmt.Sprintf("%.0f", r.Latency)
				}
			}
			big(down, "Mbps")
			ui.Spacer(c)
			ui.Row(c).Gap(14).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Gap(3).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icArrowUp).FontSize(12).TextColor(p.Accent2)
					ui.Text(c, up).FontSize(12.5).FontWeight(600).TextColor(p.Text2).FontFeatures("tnum").ContentTransition(numRoll)
				})
				ui.Text(c, lat+" ms").FontSize(12.5).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll)
			})
		})
	})
}

func (m *mainUI) ctyMask(v string) string {
	if m.reveal() || v == "" || v == "—" {
		return v
	}
	return strings.Repeat("•", min(12, max(4, len([]rune(v)))))
}

// splitBadge says whether sites at home and abroad see different
// addresses: whether the rules split them.
func (m *mainUI) splitBadge(c *ui.Context) {
	home, abroad := m.cty.ips["domestic"], m.cty.ips["global"]
	if home == nil || abroad == nil || home.info == nil || abroad.info == nil {
		return
	}
	if home.info.IP != abroad.info.IP {
		m.badge(c, locales.ConnectivitySplit(), "success")
	} else {
		m.badge(c, locales.ConnectivityNotSplit(), "")
	}
}

// siteVerdict reads the results like MyIP does: whether sites at home and
// abroad open.
func (m *mainUI) siteVerdict(c *ui.Context) {
	s := &m.cty
	opens := func(group string) (bool, bool) {
		// The sites of a country's own and the user's say little about
		// whether sites abroad open: the verdict leaves them out.
		var of []tools.Site
		for _, st := range s.sites {
			if cat := tools.SiteCategory(st); cmp.Or(st.Group, "global") == group && cat != tools.CategoryRegion && cat != tools.CategoryOther {
				of = append(of, st)
			}
		}
		ok := 0
		if len(of) == 0 {
			return false, false
		}
		for _, st := range of {
			if len(s.rounds[st.ID]) == 0 {
				return false, false
			}
			if bestRound(s.rounds[st.ID]) > 0 {
				ok++
			}
		}
		return ok*2 >= len(of), true
	}
	home, k1 := opens("domestic")
	abroad, k2 := opens("global")
	if !k1 || !k2 {
		return
	}
	tone, key := "danger", "none"
	switch {
	case home && abroad:
		tone, key = "success", "ok"
	case home:
		key = "noGlobal"
	case abroad:
		tone, key = "warning", "noDomestic"
	}
	m.badge(c, m.T("connectivity.verdict."+key), tone)
}

// siteMosaic is every site's every round as a small cell, lit as the
// round lands, as the bars of the sites below are; with more sites than
// would fit so, every site is one cell, lit as its first round lands, in
// the color of its best.
func (m *mainUI) siteMosaic(c *ui.Context) {
	s, p := &m.cty, m.pal
	n := len(s.sites)
	if n == 0 {
		return
	}
	const height = 56
	wrap := ui.Box(c.Key("mosaic")).Height(height).Clip()
	w := wrap.LayoutBounds().W
	if w <= 0 {
		w = 200
	}
	fits := func(cells, tall int, cell, gap float32) (int, bool) {
		perRow := max(1, int((w+gap)/(cell+gap)))
		rows := (cells + perRow - 1) / perRow
		return perRow, float32(rows)*(float32(tall)*cell+float32(tall-1)*gap)+float32(rows-1)*gap*2 <= height
	}
	// Rounds when they fit at a readable size, else a cell a site.
	cell, gap, perRow, rounds := float32(7), float32(2), 1, true
	for _, size := range []float32{7, 6, 5, 4} {
		g := max(1, size/3.5)
		if pr, ok := fits(n, siteRounds, size, g*1.5); ok {
			cell, gap, perRow = size, g, pr
			goto drawn
		}
	}
	rounds = false
	for _, size := range []float32{7, 6, 5, 4, 3, 2} {
		g := max(1, size/3.5)
		cell, gap = size, g
		pr, ok := fits(n, 1, size, g)
		perRow = pr
		if ok {
			break
		}
	}
drawn:
	lit := func(b ui.Element, r []int, k int, landed bool) {
		if landed {
			v := b.Keyframes("in", []ui.Keyframe{{Value: 0}, {At: 360 * time.Millisecond, Value: 1, Ease: easeBack}},
				ui.KeyframeOptions{Run: s.sitesRun<<8 | uint64(k+1)})
			b.Scale(v, v)
		}
	}
	wrap.Children(func() {
		if !rounds {
			ui.Grid(c).Columns(perRow).Gap(gap).Children(func() {
				for _, st := range s.sites {
					r := s.rounds[st.ID]
					b := ui.Box(c.Key(st.ID)).Size(cell, cell).Radius(max(0.5, cell/3.5)).Tooltip(st.Name)
					col := p.Surface3
					switch best := bestRound(r); {
					case best > 0:
						col = siteColorOf(p, best)
					case best == 0:
						col = p.Danger.Alpha(0.45)
					case s.sitesBusy:
						b.Opacity(1 - 0.4*b.Loop("pulse", time.Second, ui.Bounce(ui.EaseInOut)))
					}
					lit(b, r, 0, len(r) > 0)
					b.Background(b.AnimateColor("bg", col, ui.Animation{Duration: 160 * time.Millisecond}))
				}
			})
			return
		}
		ui.Grid(c).Columns(perRow).Gap(gap * 1.5).Children(func() {
			for _, st := range s.sites {
				r := s.rounds[st.ID]
				ui.Column(c.Key(st.ID)).Gap(gap).Children(func() {
					for k := range siteRounds {
						b := ui.Box(c.Key(k)).Size(cell, cell).Radius(max(1, cell/3.5))
						col, landed := p.Surface3, false
						switch {
						case k < len(r) && r[k] > 0:
							col, landed = siteColorOf(p, r[k]), true
						case k < len(r):
							col, landed = p.Danger.Alpha(0.45), true
						case s.sitesBusy && k == len(r):
							col = p.BorderStrong
							b.Opacity(1 - 0.55*b.Loop("pulse", time.Second, ui.Bounce(ui.EaseInOut)))
						}
						lit(b, r, k, landed)
						b.Background(b.AnimateColor("bg", col, ui.Animation{Duration: 160 * time.Millisecond}))
					}
				})
			}
		})
	})
}

// ---- Sites ----

// siteBars are a site's rounds as bars, on one scale for every site: a
// bar grows from its floor as its round lands and sinks back as the sites
// are tested again.
func (m *mainUI) siteBars(c *ui.Context, r []int, slowest int, h float32) {
	s, p := &m.cty, m.pal
	ui.Row(c).Height(h).Gap(4).AlignItems(ui.End).Children(func() {
		for k := range siteRounds {
			bar := ui.Box(c.Key(k)).Grow(1).Basis(0).Radius(3).TransformOrigin(0.5, 1)
			target, col, landed := float32(30), p.Surface2, false
			if !p.Dark {
				col = p.Surface3
			}
			switch {
			case k < len(r) && r[k] > 0:
				target, col, landed = float32(30+70*math.Sqrt(float64(r[k])/float64(slowest))), siteColorOf(p, r[k]), true
				bar.Tooltip(locales.ConnectivityRound(fmt.Sprint(k+1), fmt.Sprintf("%d ms", r[k])))
			case k < len(r):
				col, landed = p.Danger.Alpha(0.45), true
				bar.Tooltip(locales.ConnectivityRound(fmt.Sprint(k+1), locales.ConnectivityUnreachable()))
			case s.sitesBusy && k == len(r):
				col = p.BorderStrong
				bar.Opacity(1 - 0.55*bar.Loop("pulse", time.Second, ui.Bounce(ui.EaseInOut)))
			}
			ht := bar.AnimateTo("h", target, ui.Animation{Duration: 250 * time.Millisecond, Ease: ui.CubicBezier(0.25, 0.1, 0.25, 1)})
			grow := float32(1)
			if landed {
				grow = bar.Keyframes("in", []ui.Keyframe{{Value: 0}, {At: 360 * time.Millisecond, Value: 1, Ease: easeBack}},
					ui.KeyframeOptions{Run: s.sitesRun<<8 | uint64(k+1)})
			}
			bar.HeightPercent(ht).Scale(1, grow).Background(bar.AnimateColor("bg", col, ui.Animation{Duration: 160 * time.Millisecond}))
		}
	})
}

// siteDelay is a site's fastest round, in the site's colors.
func (m *mainUI) siteDelay(c *ui.Context, r []int) {
	s, p := &m.cty, m.pal
	best := bestRound(r)
	switch {
	case s.sitesBusy && best < 0:
		ui.Spinner(c)
	case best == 0:
		ui.Text(c, locales.ConnectivityUnreachable()).FontSize(12).TextColor(p.Danger).SingleLine().Shrink(0)
	case best > 0:
		ui.Text(c, fmt.Sprintf("%d ms", best)).FontSize(12).FontWeight(600).TextColor(siteColorOf(p, best)).FontFeatures("tnum").SingleLine().Shrink(0).ContentTransition(numRoll)
	default:
		ui.Text(c, "—").FontSize(12).TextColor(p.Faint)
	}
}

func (m *mainUI) catLabel(cat string) string {
	switch cat {
	case tools.CategoryGeneral:
		return locales.ConnectivityCatGeneral()
	case tools.CategoryDomestic:
		return locales.ConnectivityGroupDomestic()
	case tools.CategoryOther:
		return locales.ConnectivityCatOther()
	case tools.CategoryRegion:
		return locales.ConnectivityCatRegion()
	}
	return m.T("connectivity.cat." + cat)
}

// siteTally is how the sites of a category stand: how many, how many
// would not open, and how many wait for their rounds.
type siteTally struct{ n, failed, pending int }

// sitesDetail shows the sites of one category at a time, the categories in
// a column beside them, however many there are, each saying how many sites
// it has and marking one with a site that would not open; the country
// groups fold out under their category as it is chosen.
func (m *mainUI) sitesDetail(c *ui.Context, running bool) {
	m.ctyFollow() // shown in the home page's sheet too
	s, p := &m.cty, m.pal
	// 常用 leads, empty or not: the user's shortcuts, before the categories.
	cats := append([]string{tools.CategoryGeneral}, tools.SiteCategories(s.sites)...)
	if !slices.Contains(cats, s.cat) {
		s.cat = cats[0]
	}
	if !slices.Contains(s.regionOrder, s.region) && len(s.regionOrder) > 0 {
		s.region = s.regionOrder[0]
	}
	byCat, byRegion := map[string]*siteTally{}, map[string]*siteTally{}
	favorites := favoriteSet(s.favoriteOrder)
	favoriteTally := &siteTally{}
	slowest := 1
	var times []int
	for _, st := range s.sites {
		r := s.rounds[st.ID]
		count := func(m map[string]*siteTally, key string) {
			t := m[key]
			if t == nil {
				t = &siteTally{}
				m[key] = t
			}
			t.n++
			if len(r) > 0 && bestRound(r) == 0 {
				t.failed++
			}
			if s.sitesBusy && len(r) < siteRounds {
				t.pending++
			}
		}
		count(byCat, tools.SiteCategory(st))
		if favorites[st.ID] {
			favoriteTally.n++
			if len(r) > 0 && bestRound(r) == 0 {
				favoriteTally.failed++
			}
			if s.sitesBusy && len(r) < siteRounds {
				favoriteTally.pending++
			}
		}
		if reg := tools.SiteRegion(st); reg != "" {
			count(byRegion, reg)
		}
		for _, d := range r {
			slowest = max(slowest, d)
		}
		if b := bestRound(r); b > 0 {
			times = append(times, b)
		}
	}
	pick := func(cat, region string) {
		if cat == s.cat && (cat != tools.CategoryRegion || region == s.region) {
			return
		}
		s.cat = cat
		s.regionOpen = cat == tools.CategoryRegion
		if region != "" {
			s.region = region
		}
		if s.queue != nil {
			s.queue.prefer(s.cat, s.region)
		}
	}
	inRegion := s.cat == tools.CategoryRegion
	regionExpanded := inRegion && s.regionOpen
	ui.Row(c.Key("sites")).Gap(14).AlignItems(ui.Start).Children(func() {
		rail := ui.Column(c.Key("rail")).Width(188).Shrink(0).Padding(6).Gap(2).Radius(tileRadius).Background(p.Card)
		thumb := segThumb{rail.ID()}
		item := func(key, label string, t *siteTally, on, lit bool, indent float32, lead func(), click func()) {
			b := ui.ButtonBase(c.Key(key)).Row().Height(34).PaddingStart(10 + indent).PaddingEnd(10).Gap(8).Radius(9).AlignItems(ui.Center).Label(label)
			if !on && b.Hovered() {
				b.Background(p.Surface2)
			}
			b.Transition(hoverFade)
			b.Children(func() {
				// The soft fill slides from the item chosen before.
				if on {
					ui.Box(c.Key("thumb")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Radius(9).Background(p.AccentSoft).
						Transition(ui.ElementTransition{Match: thumb, Animation: &segSlide})
				}
				if lead != nil {
					lead()
				}
				col := p.Text2
				if on || lit {
					col = p.AccentFg
				}
				txt := ui.Text(c, label).FontSize(13).FontWeight(560).SingleLine().Grow(1).Basis(0).ZIndex(1)
				txt.TextColor(txt.AnimateColor("fg", col, ui.Animation{Duration: 200 * time.Millisecond}))
				if t == nil {
					return
				}
				switch {
				case t.failed > 0:
					ui.Box(c.Key("dot")).Size(6, 6).Radius(3).Background(p.Danger).ZIndex(1).Transition(checkIn).Tooltip(locales.ConnectivityUnreachable())
				case t.pending > 0:
					d := ui.Box(c.Key("wait")).Size(6, 6).Radius(3).Background(p.BorderStrong).ZIndex(1)
					d.Opacity(1 - 0.55*d.Loop("pulse", time.Second, ui.Bounce(ui.EaseInOut)))
				}
				ui.Text(c, fmt.Sprint(t.n)).FontSize(11.5).TextColor(p.Faint).FontFeatures("tnum").ZIndex(1).ContentTransition(numRoll)
			})
			if b.Clicked() {
				click()
			}
		}
		regionToggle := func() {
			if s.cat != tools.CategoryRegion {
				s.cat, s.regionOpen = tools.CategoryRegion, true
			} else {
				s.regionOpen = !s.regionOpen
			}
			if s.queue != nil {
				s.queue.prefer(s.cat, s.region)
			}
		}
		// The countries' rows stand in slots, each gliding to its own on a
		// spring. A row dragged (as soon as it moves, or once held) lifts
		// and follows the pointer, the rows it passes making way for it, a
		// slot up or down, with a little bounce; let go, it springs into the
		// slot it is over, as fast as it was thrown, and the order is kept.
		n := len(s.regionOrder)
		lifted := s.regionPress != "" && s.regionLift
		regionTo := func() int {
			return max(0, min(n-1, s.regionFrom+int(math.Round(float64(s.regionDragY/regionPitch)))))
		}
		commit := false
		regionItem := func(reg string, idx int) {
			code := tools.RegionCode(reg)
			row := ui.ButtonBase(c.Key("r-" + reg)).Row().Absolute().Left(0).Right(0).Height(regionRowH).
				PaddingStart(10).PaddingEnd(10).Gap(8).Radius(9).AlignItems(ui.Center).Label(m.countryName(code))
			// Held, wherever the pointer is: Pressed is false while it is
			// outside the row, which a quick drag outruns.
			_, dy, pressed := row.Dragged()
			now := c.Now()
			switch {
			case pressed && s.regionPress == "":
				s.regionPress, s.regionPressAt, s.regionFrom, s.regionDragY, s.regionLift = reg, now, idx, dy, false
			case pressed && s.regionPress == reg:
				s.regionDragY += dy
			}
			mine := s.regionPress == reg
			if mine && pressed && !s.regionLift {
				if abs32(s.regionDragY) >= 4 || now.Sub(s.regionPressAt) >= regionHold {
					s.regionLift, lifted = true, true
				} else {
					c.After(16 * time.Millisecond)
				}
			}
			dragged := mine && s.regionLift
			released := mine && !pressed
			var y float32
			switch {
			case dragged && !released:
				lo, hi := -float32(idx)*regionPitch, float32(n-1-idx)*regionPitch
				y = row.Jump("y", float32(idx)*regionPitch+rubberBand(s.regionDragY, lo, hi))
			case dragged:
				y = row.Spring("y", float32(regionTo())*regionPitch, regionSettle)
			default:
				slot := idx
				if lifted && !mine {
					from, to := s.regionFrom, regionTo()
					switch {
					case from < idx && idx <= to:
						slot--
					case to <= idx && idx < from:
						slot++
					}
				}
				y = row.Spring("y", float32(slot)*regionPitch, regionMakeWay)
			}
			row.Top(y)
			if released {
				commit = commit || dragged
				if !dragged {
					s.regionPress = ""
				}
			}
			// Lifted, it grows a little and casts a shadow, and stretches
			// along the way it is thrown, settling round as it slows.
			up := row.Spring("lift", b2f(dragged && !released), regionLiftSpring)
			v := abs32(row.Velocity("y"))
			squash := row.Spring("squash", min(v/6000, 0.05), regionLiftSpring)
			if up > 0.001 || squash > 0.001 {
				row.Scale(1+0.025*up-squash*0.5, 1+0.025*up+squash).ZIndex(10)
				row.Shadow(0, 4*up, 14*up, 0, ui.RGBA(0, 0, 0, 0.22*up))
			}
			cursor := ui.CursorGrab
			if dragged && !released {
				cursor = ui.CursorGrabbing
			}
			row.Cursor(cursor)
			switch {
			case dragged && !released:
				row.Background(p.Surface3)
			case s.regionOpen && s.region == reg:
				row.Background(p.AccentSoft)
			case !lifted && row.Hovered():
				row.Background(p.Surface2)
			}
			row.Transition(hoverFade)
			row.Children(func() {
				ui.Icon(c, icGripVertical).FontSize(11).TextColor(p.Faint).Cursor(cursor).Shrink(0)
				m.flagSmall(c, code)
				ui.Text(c, m.countryName(code)).FontSize(13).FontWeight(560).SingleLine().Grow(1).Basis(0).ZIndex(1)
				ui.Text(c, fmt.Sprint(byRegion[reg].n)).FontSize(11.5).TextColor(p.Faint).FontFeatures("tnum").ZIndex(1)
			})
			if row.Clicked() && !dragged {
				pick(tools.CategoryRegion, reg)
			}
		}
		rail.Children(func() {
			for _, cat := range cats {
				if cat != tools.CategoryRegion {
					t := byCat[cat]
					if cat == tools.CategoryGeneral {
						t = favoriteTally
					}
					item(cat, m.catLabel(cat), t, s.cat == cat, false, 0, nil, func() { pick(cat, "") })
					continue
				}
				item(cat, m.catLabel(cat), byCat[cat], inRegion, regionExpanded, 0, func() {
					icon := icChevronRight
					if s.regionOpen {
						icon = icChevronDown
					}
					ui.Icon(c, icon).FontSize(13).TextColor(p.AccentFg).Shrink(0)
				}, regionToggle)
				if !regionExpanded {
					s.regionPress, s.regionLift = "", false
					continue
				}
				fold := ui.Box(c.Key("regions")).Height(max(0, float32(n)*regionPitch-regionGap))
				if !lifted {
					fold.Clip() // a lifted row's shadow may pass its edges
				}
				fold.Transition(ui.ElementTransition{Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true}, Duration: 260 * time.Millisecond, Ease: easeOut})
				fold.Children(func() {
					for i, reg := range slices.Clone(s.regionOrder) {
						regionItem(reg, i)
					}
				})
				if commit {
					m.moveRegion(s.regionFrom, regionTo())
					s.regionPress, s.regionLift, s.regionDragY = "", false, 0
				}
			}
		})
		var of []tools.Site
		if s.cat == tools.CategoryGeneral {
			byID := map[string]tools.Site{}
			for _, st := range s.sites {
				byID[st.ID] = st
			}
			for _, id := range s.favoriteOrder {
				if st, ok := byID[id]; ok {
					of = append(of, st)
				}
			}
		} else {
			for _, st := range s.sites {
				if tools.SiteCategory(st) == s.cat && (!regionExpanded || tools.SiteRegion(st) == s.region) {
					of = append(of, st)
				}
			}
		}
		view := s.cat
		title := m.catLabel(s.cat)
		if regionExpanded {
			view, title = "region-"+s.region, m.countryName(tools.RegionCode(s.region))
		}
		ui.Column(c.Key("pane")).Grow(1).Basis(0).MinWidth(0).Gap(10).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).MinHeight(30).Children(func() {
				ui.Row(c.Key("title-" + view)).Gap(8).AlignItems(ui.Baseline).Grow(1).MinWidth(0).
					Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 6}, Duration: 260 * time.Millisecond, Ease: easeOut}).Children(func() {
					ui.Text(c, title).FontSize(15).FontWeight(650).SingleLine()
					ui.Text(c, fmt.Sprint(len(of))).FontSize(12.5).TextColor(p.Faint).FontFeatures("tnum")
				})
				if len(times) > 0 {
					slices.Sort(times)
					ui.Text(c, locales.ConnectivityMedian(fmt.Sprint(times[len(times)/2]))).FontSize(12).TextColor(p.Faint).SingleLine().Shrink(0).ContentTransition(numRoll)
				}
				if m.iconButton(c, "manage", icCirclePlus, locales.ConnectivityManage()).Clicked() {
					s.manage, s.tab, s.custErr = true, 0, ""
				}
				if m.spinButton(c, "retest", icRefreshCw, locales.CommonRetest(), s.sitesBusy).Disabled(!running).Clicked() {
					m.testCtySites()
				}
			})
			ui.Column(c.Key("cat-" + view)).Transition(revealIn).Children(func() {
				card := func(st tools.Site, box ui.Element) {
					m.siteCard(c, st, slowest, box)
				}
				switch {
				case s.cat == tools.CategoryGeneral && len(of) == 0:
					m.empty(c, locales.ConnectivityFavEmpty(), nil)
				case s.cat == tools.CategoryGeneral:
					m.favoriteGrid(c, of, card)
				default:
					m.cardGridOf(c, "sites-"+view, 180, len(of), func(i int) any { return of[i].ID }, func(i int) {
						card(of[i], ui.Column(c).Grow(1))
					})
				}
			})
		})
	})
}

// ---- Services ----

// unlockDetail shows the service checks by kind of service, each with
// what it found, where, and the proxy the rules sent it through, which
// may differ from one service to the next.
func (m *mainUI) unlockDetail(c *ui.Context, running bool) {
	s, p := &m.cty, m.pal
	looks := map[string]struct {
		icon *ui.SVG
		tone string
	}{"yes": {icCheckCircle2, "success"}, "partial": {icMinusCircle, "warning"}, "no": {icXCircle, "danger"}, "failed": {icHelpCircle, ""}, "pending": {icCircleDashed, ""}, "checking": {icLoader2, ""}}
	var at time.Time
	for _, u := range s.unlocks {
		if r, ok := s.results[u.ID]; ok {
			if t, err := time.Parse(time.RFC3339, r.At); err == nil && t.After(at) {
				at = t
			}
		}
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).MinHeight(30).Children(func() {
		when := ""
		if len(s.checking) == 0 && !at.IsZero() {
			when = m.relTime(at)
		}
		ui.Text(c, when).FontSize(12).TextColor(p.Faint).SingleLine().Grow(1)
		if m.iconButton(c, "choose", icSlidersHorizontal, locales.UnlockManage()).Clicked() {
			s.unlockMgr = true
		}
		if m.spinButton(c, "check", icRefreshCw, locales.UnlockCheckAll(), len(s.checking) > 0).Disabled(!running).Clicked() {
			m.checkUnlock(nil)
		}
	})
	for _, g := range tools.UnlockGroups {
		var items []tools.Unlock
		for _, u := range s.unlocks {
			if u.Group == g {
				items = append(items, u)
			}
		}
		if len(items) == 0 {
			continue
		}
		ui.Column(c.Key("group-"+g)).Gap(8).Transition(listRow).Children(func() {
			ui.Text(c, m.unlockGroupLabel(g)).FontSize(12).FontWeight(600).TextColor(p.TextMuted).PaddingStart(2)
			m.cardGridOf(c, "unlock-"+g, 250, len(items), func(i int) any { return items[i].ID }, func(i int) {
				it := items[i]
				r, has := s.results[it.ID]
				state := "pending"
				if has {
					state = r.Status
				}
				if s.checking[it.ID] {
					state = "checking"
				}
				l := looks[state]
				if l.icon == nil {
					l = looks["pending"]
				}
				row := ui.Row(c).Grow(1).MinHeight(58).Padding(10, 10, 10, 14).Gap(10).Radius(tileRadius).Background(p.Card).AlignItems(ui.Center)
				hot := row.Hovered()
				row.Children(func() {
					m.logo(c, cmp.Or(it.Site, it.ID), it.Name, 26)
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
						ui.Text(c, m.unlockName(it)).FontSize(13).FontWeight(600).SingleLine()
						// What it found besides, and the proxy it went through.
						var sub []string
						if has && state != "checking" {
							if d := unlockDetailText(r.Detail); d != "" {
								sub = append(sub, d)
							}
							if r.Via != "" {
								sub = append(sub, locales.UnlockVia(r.Via))
							}
						}
						if len(sub) > 0 {
							line := strings.Join(sub, " · ")
							ui.Text(c, line).FontSize(11.5).TextColor(p.TextMuted).SingleLine().Tooltip(line)
						}
					})
					if has && r.Region != "" && state != "checking" {
						ui.Row(c).Gap(5).AlignItems(ui.Center).Shrink(0).Children(func() {
							reg := strings.ToUpper(r.Region)
							if len(reg) == 3 && reg != "HKD" && reg != "USD" && reg != "EUR" && reg != "JPY" && reg != "CNY" {
								// Some checks name the region in three letters.
								if two := iso2(reg); two != "" {
									reg = two
								}
							}
							if len(reg) == 2 {
								m.flagMasked(c, reg, !m.reveal())
							}
							if !m.reveal() {
								reg = "••"
							}
							ui.Text(c, reg).FontSize(12.5).TextColor(p.TextMuted).ContentTransition(maskRoll(m.reveal()))
						})
					}
					bg, fg := p.Surface3, p.Text2
					switch l.tone {
					case "success":
						bg, fg = p.SuccessSoft, p.Success
					case "warning":
						bg, fg = p.WarningSoft, p.Warning
					case "danger":
						bg, fg = p.DangerSoft, p.Danger
					}
					badge := ui.Row(c.Key("badge")).Height(22).Padding(0, 8).Gap(5).Radius(11).Background(bg).AlignItems(ui.Center).Shrink(0)
					badge.Transition(ui.ElementTransition{Colors: true, Duration: 200 * time.Millisecond})
					badge.Children(func() {
						icon := ui.Icon(c, l.icon).FontSize(12).TextColor(fg)
						if state == "checking" {
							icon.Rotate(360 * badge.Loop("spin", time.Second, ui.Linear))
						}
						ui.Text(c, m.T("unlock."+state)).FontSize(11.5).FontWeight(600).TextColor(fg).SingleLine()
					})
					retest := m.iconButton(c, "retest", icRefreshCw, locales.CommonRetest()).Disabled(state == "checking" || !running)
					retest.Opacity(retest.AnimateTo("show", b2f(hot), ui.Animation{Duration: 120 * time.Millisecond}))
					if retest.Clicked() {
						m.checkUnlock([]string{it.ID})
					}
				})
			})
		})
	}
}

// iso2 turns the three-letter country codes some checks give into the two
// letters the flags go by.
func iso2(code string) string {
	return map[string]string{
		"JPN": "JP", "USA": "US", "HKG": "HK", "TWN": "TW", "SGP": "SG", "KOR": "KR", "GBR": "GB", "DEU": "DE",
		"FRA": "FR", "CAN": "CA", "AUS": "AU", "NLD": "NL", "IND": "IN", "BRA": "BR", "RUS": "RU", "CHN": "CN",
		"MYS": "MY", "THA": "TH", "VNM": "VN", "PHL": "PH", "IDN": "ID", "TUR": "TR", "ARE": "AE", "MAC": "MO",
	}[code]
}

// ---- Addresses ----

// exitDetail sets the addresses side by side, a value Cloudflare shares
// with the sites abroad said as such.
func (m *mainUI) exitDetail(c *ui.Context, running bool) {
	s, p := &m.cty, m.pal
	ui.Row(c).Gap(8).AlignItems(ui.Center).MinHeight(30).Children(func() {
		ui.Spacer(c)
		loading := slices.ContainsFunc(exitViews, func(v string) bool { return s.ips[v] != nil && s.ips[v].loading })
		if m.spinButton(c, "reload", icRefreshCw, locales.CommonRefresh(), loading).Disabled(!running).Clicked() {
			m.loadExitIPs()
		}
	})
	info := func(v string) *tools.IPInfo {
		if e := s.ips[v]; e != nil {
			return e.info
		}
		return nil
	}
	abroad := info("global")
	type row struct {
		name string
		cell func(v string, in *tools.IPInfo)
		same func(a, b *tools.IPInfo) bool
	}
	asn := func(in *tools.IPInfo) string {
		if in.ASN == "" {
			return "—"
		}
		return "AS" + strings.TrimPrefix(in.ASN, "AS")
	}
	rows := []row{
		{locales.ConnectivityPlace(), func(v string, in *tools.IPInfo) {
			ui.Row(c).Gap(8).AlignItems(ui.Center).MinWidth(0).Children(func() {
				m.flag(c, in.CountryCode, !m.reveal())
				place := in.Country
				if in.City != "" && in.City != in.Country {
					place += " · " + in.City
				}
				ui.Text(c, m.ctyMask(cmp.Or(place, "—"))).ContentTransition(maskRoll(m.reveal())).FontSize(13).FontWeight(600).SingleLine().Shrink(1)
			})
		}, func(a, b *tools.IPInfo) bool { return a.Country == b.Country && a.City == b.City }},
		{"IP", func(v string, in *tools.IPInfo) {
			t := ui.Text(c, m.ctyMask(in.IP)).ContentTransition(maskRoll(m.reveal())).FontSize(13).Font("ui-monospace").SingleLine().Shrink(1)
			if m.reveal() {
				t.Selectable()
			}
		}, func(a, b *tools.IPInfo) bool { return a.IP == b.IP }},
		{locales.HomeIsp(), func(v string, in *tools.IPInfo) {
			ui.Text(c, m.ctyMask(cmp.Or(in.ISP, "—"))).ContentTransition(maskRoll(m.reveal())).FontSize(13).SingleLine().Shrink(1)
		}, func(a, b *tools.IPInfo) bool { return a.ISP == b.ISP }},
		{"ASN", func(v string, in *tools.IPInfo) {
			ui.Text(c, m.ctyMask(asn(in))).ContentTransition(maskRoll(m.reveal())).FontSize(13).SingleLine()
		}, func(a, b *tools.IPInfo) bool { return asn(a) == asn(b) }},
		{locales.IpqQuality(), func(v string, in *tools.IPInfo) {
			m.ipQualityBadges(c, in.IP, !m.reveal())
		}, func(a, b *tools.IPInfo) bool { return a.IP == b.IP }},
		{locales.ConnectivityColo(), func(v string, in *tools.IPInfo) {
			ui.Text(c, m.ctyMask(cmp.Or(in.Colo, "—"))).ContentTransition(maskRoll(m.reveal())).FontSize(13).SingleLine()
		}, nil},
	}
	ui.Column(c).Padding(4, 18).Radius(tileRadius).Background(p.Card).Children(func() {
		line := func(key string, name func(), cells func(i int, v string)) {
			ui.Row(c.Key(key)).Gap(20).MinHeight(44).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Width(110).Shrink(0).Children(name)
				for i, v := range exitViews {
					ui.Row(c.Key(v)).Grow(1).Basis(0).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() { cells(i, v) })
				}
			})
		}
		line("head", func() {}, func(i int, v string) {
			ui.Text(c, m.T("connectivity.view."+v)).FontSize(12.5).FontWeight(650).SingleLine().Grow(1)
			if in := info(v); in != nil && in.Source != "" && in.Source != m.T("connectivity.view."+v) {
				ui.Text(c, in.Source).FontSize(11.5).TextColor(p.Faint).SingleLine()
			}
		})
		for ri, r := range rows {
			ui.Box(c.Key(fmt.Sprint("rule", ri))).Height(1).Background(p.Border)
			line(fmt.Sprint("row", ri), func() {
				ui.Text(c, r.name).FontSize(12.5).TextColor(p.TextMuted).SingleLine()
			}, func(i int, v string) {
				e := s.ips[v]
				in := info(v)
				ui.Row(c.Key(func() string {
					switch {
					case e == nil || (e.loading && in == nil):
						return "loading"
					case in == nil:
						return "err"
					}
					return "info"
				}())).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Transition(revealIn).Children(func() {
					switch {
					case e == nil || (e.loading && in == nil):
						m.skeleton(c, 14).Width(90)
					case in == nil:
						if ri == 0 {
							ui.Text(c, cmp.Or(e.err, "—")).FontSize(12).TextColor(p.Faint).SingleLine().Tooltip(e.err)
						} else {
							ui.Text(c, "—").FontSize(13).TextColor(p.Faint)
						}
					case v == "cloudflare" && abroad != nil && r.same != nil && r.same(in, abroad):
						ui.Text(c, locales.ConnectivitySameAsGlobal()).FontSize(12.5).TextColor(p.Faint).SingleLine()
					default:
						r.cell(v, in)
					}
				})
			})
		}
	})
}

// ---- Speed ----

func (m *mainUI) startSpeed() {
	s, a := &m.cty, m.a
	proxy := a.proxyURL()
	if proxy == "" {
		m.notices = append(m.notices, Notice{Level: "error", Message: locales.SpeedFailed(), Detail: tr(a, "noRuntime")})
		return
	}
	ctx, cancel := context.WithTimeout(a.ctx, 45*time.Second)
	s.speedBusy, s.speedStop = true, cancel
	via, win := s.exit, m.window()
	last := time.Time{}
	go func() {
		defer cancel()
		tools.SpeedTest(ctx, proxy, func(sm tools.SpeedSample) {
			// Every sample would be a lot of frames; a few a second do.
			if sm.Phase != "done" && time.Since(last) < 100*time.Millisecond && sm.Progress < 1 {
				return
			}
			last = time.Now()
			r := &speedResult{sm, time.Now(), via}
			if sm.Phase == "done" {
				a.saveUICache(func(c *uiCache) { c.Speed = r })
			}
			win.Update(func() { s.speed = r })
		})
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			m.notice(Notice{Level: "error", Message: locales.SpeedFailed(), Detail: ctx.Err().Error()})
		}
		win.Update(func() { s.speedBusy, s.speedStop = false, nil })
	}()
}

func (m *mainUI) speedDetail(c *ui.Context, running bool) {
	s, p := &m.cty, m.pal
	phase := "done"
	if s.speedBusy && s.speed != nil {
		phase = s.speed.Phase
	}
	r := s.speed
	ui.Row(c).Gap(8).AlignItems(ui.Center).MinHeight(30).Children(func() {
		var parts []string
		switch {
		case r == nil:
			parts = append(parts, locales.SpeedHint())
		case r.Error != "":
		default:
			if r.Via != "" {
				parts = append(parts, locales.UnlockVia(r.Via))
			}
			if r.Colo != "" {
				parts = append(parts, "Cloudflare "+r.Colo)
			}
			if !s.speedBusy && !r.At.IsZero() {
				parts = append(parts, m.relTime(r.At))
			}
			if r.Bytes > 0 {
				parts = append(parts, locales.SpeedUsed(fmt.Sprint(int(math.Round(float64(r.Bytes)/1048576)))))
			}
		}
		if r != nil && r.Error != "" {
			ui.Text(c, r.Error).FontSize(12).TextColor(p.Danger).SingleLine().Grow(1).Tooltip(r.Error)
		} else {
			ui.Text(c, strings.Join(parts, " · ")).FontSize(12).TextColor(p.Faint).SingleLine().Grow(1)
		}
		if s.speedBusy {
			if m.button(c, "stop", locales.SpeedStop(), icSquare, "").Clicked() && s.speedStop != nil {
				s.speedStop()
			}
			return
		}
		label := locales.SpeedStart()
		if r != nil {
			label = locales.SpeedAgain()
		}
		if m.button(c, "start", label, icGauge, "").Disabled(!running).Clicked() {
			m.startSpeed()
		}
	})
	lat, jitter, down, up, prog := "—", " ", "—", "—", 0.0
	if r != nil {
		if r.Latency > 0 {
			lat = fmt.Sprintf("%.0f", r.Latency)
		}
		if r.Jitter > 0 {
			jitter = locales.SpeedJitter(fmt.Sprintf("%.1f", r.Jitter))
		}
		if r.Download > 0 || phase == "download" {
			down = mbps(r.Download)
		}
		if r.Upload > 0 || phase == "upload" {
			up = mbps(r.Upload)
		}
		prog = r.Progress
	}
	metric := func(key, label string, icon *ui.SVG, value, unit, sub string) {
		live := phase == key
		ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(0).Padding(16, 18, 18).Gap(6).Radius(tileRadius).Background(p.Card).Children(func() {
			ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
				if icon != nil {
					ui.Icon(c, icon).FontSize(13).TextColor(p.TextMuted)
				}
				col := p.TextMuted
				if live {
					col = p.AccentFg
				}
				ui.Text(c, label).FontSize(12).TextColor(col)
			})
			ui.Row(c).Gap(5).AlignItems(ui.Baseline).Padding(4, 0, 0).Children(func() {
				ui.Text(c, value).FontSize(36).FontWeight(700).FontFeatures("tnum").ContentTransition(numRoll)
				ui.Text(c, unit).FontSize(13).TextColor(p.Faint)
			})
			track := ui.Box(c).Height(4).Radius(2).Background(p.Surface3)
			w := track.Animate("p", float32(prog), 200*time.Millisecond)
			track.Draw(func(pt *ui.Painter, rr ui.Rect) {
				if live {
					pt.Fill(ui.Rect{X: rr.X, Y: rr.Y, W: rr.W * w, H: rr.H}, p.Accent, 2)
				}
			})
			ui.Text(c, sub).FontSize(11.5).TextColor(p.Faint).FontFeatures("tnum").ContentTransition(numRoll)
		})
	}
	ui.Row(c).Gap(12).Children(func() {
		metric("latency", locales.SpeedLatency(), nil, lat, "ms", jitter)
		metric("download", locales.CommonDownload(), icArrowDown, down, "Mbps", " ")
		metric("upload", locales.CommonUpload(), icArrowUp, up, "Mbps", " ")
	})
}

// The rows of the countries in the rail, and how they move as one is
// dragged to another place.
const (
	regionRowH  = 34
	regionGap   = 2
	regionPitch = regionRowH + regionGap
	regionHold  = 350 * time.Millisecond // held this long, a row lifts unmoved
)

var (
	regionMakeWay    = ui.SpringOptions{Duration: 340 * time.Millisecond, Bounce: 0.32}
	regionSettle     = ui.SpringOptions{Duration: 420 * time.Millisecond, Bounce: 0.28}
	regionLiftSpring = ui.SpringOptions{Duration: 260 * time.Millisecond, Bounce: 0.2}
)

// rubberBand holds v within lo and hi, giving way less and less past
// them, as iOS holds content pulled past its edges.
func rubberBand(v, lo, hi float32) float32 {
	const k = 0.3
	switch {
	case v < lo:
		return lo - (lo-v)*k
	case v > hi:
		return hi + (v-hi)*k
	}
	return v
}
