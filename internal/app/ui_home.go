package app

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/tools"
)

// homePage is what the home page keeps between frames.
type homePage struct {
	// cardRows is how the cards below the board were laid out, whose
	// changes glide them.
	cardRows string
	cardsH   float32 // the cards' grid, as last laid out
	busy     string  // what the compass waits for: takeover, svc, core

	// The board's layout, and its tiles' boxes as last placed and as they
	// were when the layout last changed, which they glide from.
	boardLayout string
	boardAt     map[string]ui.Rect
	boardFrom   map[string]ui.Rect
	boardStart  time.Time
	boardFromH  float32
	pillsH      float32 // the exit tile's route pills, as last laid out

	// The proxies of a group.
	view        ProxiesView
	viewLoading bool
	loaded      homeKey
	group       string
	testing     bool
	delays      map[string]int
	pending     string // the selection asked for, until the core confirms
	nodeList    ui.ScrollState

	// Where the traffic leaves.
	ip        *tools.IPInfo
	ipErr     string
	ipLoading bool
	ipFor     int // the runtime it was looked up for, +1

	// Sites.
	sites       []tools.Site
	results     map[string]tools.SiteResult
	sitesBusy   int
	sitesTested bool

	autostart, autostartKnown bool
	retrying                  bool

	editing bool
	draft   []config.HomeCard
}

// homeKey is what the proxies card was loaded for.
type homeKey struct {
	runtime, selection int
	ready              bool
	mode               string
}

// ---- Layout ----

type cardSpan struct{ span, least int }

var homeSpans = map[string]cardSpan{
	"control": {12, 12}, "traffic": {8, 8}, "proxy": {4, 4}, "profile": {4, 4}, "ip": {4, 4},
	"tailscale": {4, 4}, "test": {8, 4}, "core": {4, 4}, "system": {4, 4},
}

func homeSpan(id string, width float32) cardSpan {
	s, ok := homeSpans[id]
	if !ok {
		s = cardSpan{4, 4}
	}
	switch {
	case width < 540:
		return cardSpan{12, 12}
	case width < 820:
		if s.span >= 8 {
			return cardSpan{12, 12}
		}
		return cardSpan{6, 6}
	}
	return s
}

// packHome lays the cards out in rows of twelve columns. Each card has
// the span it wants and the least it can live with; rows take cards while
// their least spans fit, then share the row out, so every row is full and
// the cards in it are of one height, whatever is shown and in whatever
// order.
func packHome(ids []string, width float32) [][]struct {
	id   string
	span int
} {
	var rows [][]string
	var row []string
	used := 0
	for _, id := range ids {
		least := homeSpan(id, width).least
		if len(row) > 0 && used+least > 12 {
			rows = append(rows, row)
			row, used = nil, 0
		}
		row = append(row, id)
		used += least
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	// A card alone at the end, which would rather share its row, takes the
	// card before it along.
	if n := len(rows); n >= 2 && len(rows[n-1]) == 1 && len(rows[n-2]) >= 2 && homeSpan(rows[n-1][0], width).span < 12 {
		prev := rows[n-2]
		moved := prev[len(prev)-1]
		if homeSpan(moved, width).least+homeSpan(rows[n-1][0], width).least <= 12 {
			rows[n-2] = prev[:len(prev)-1]
			rows[n-1] = []string{moved, rows[n-1][0]}
		}
	}
	out := make([][]struct {
		id   string
		span int
	}, len(rows))
	for ri, r := range rows {
		spans, total := make([]int, len(r)), 0
		for i, id := range r {
			spans[i] = homeSpan(id, width).span
			total += spans[i]
		}
		for total > 12 { // too wide: the most flexible card gives way first
			best := -1
			for i, id := range r {
				if slack := spans[i] - homeSpan(id, width).least; slack > 0 && (best < 0 || slack > spans[best]-homeSpan(r[best], width).least) {
					best = i
				}
			}
			if best < 0 {
				break
			}
			cut := min(total-12, spans[best]-homeSpan(r[best], width).least)
			spans[best] -= cut
			total -= cut
		}
		if total < 12 { // too narrow: shared out evenly, the rest to the widest
			each := (12 - total) / len(r)
			for i := range spans {
				spans[i] += each
			}
			total += each * len(r)
			spans[slices.Index(spans, slices.Max(spans))] += 12 - total
		}
		for i, id := range r {
			out[ri] = append(out[ri], struct {
				id   string
				span int
			}{id, spans[i]})
		}
	}
	return out
}

// ---- The page ----

// secondaryCards are the cards below the board, which the user chooses.
var secondaryCards = []string{"profile", "ip", "tailscale", "test", "core", "system"}

// customizeHome is the dialog that chooses the cards and their order.
func (m *mainUI) customizeHome(c *ui.Context) {
	h, p := &m.home, m.pal
	m.modal(c, "customize", &h.editing, func() {
		ui.Column(c).Width(340).Gap(8).Children(func() {
			ui.Text(c, locales.HomeCustomize()).FontSize(15).FontWeight(650)
			ui.Text(c, locales.HomeCustomizeHint()).FontSize(12).TextColor(p.TextMuted)
			for i := range h.draft {
				card := &h.draft[i]
				if !slices.Contains(secondaryCards, card.ID) {
					continue
				}
				ui.Row(c.Key(card.ID)).Padding(6, 8).Gap(8).Radius(12).Background(p.Surface2).AlignItems(ui.Center).Children(func() {
					ui.Switch(c, &card.Visible).Label(m.T("home.card." + card.ID))
					ui.Text(c, m.T("home.card."+card.ID)).Grow(1).SingleLine()
					if m.iconButton(c, "up", icArrowUp, "").Disabled(i == 0).Clicked() {
						h.draft[i], h.draft[i-1] = h.draft[i-1], h.draft[i]
					}
					if m.iconButton(c, "down", icArrowDown, "").Disabled(i == len(h.draft)-1).Clicked() {
						h.draft[i], h.draft[i+1] = h.draft[i+1], h.draft[i]
					}
				})
			}
			ui.Row(c).Gap(8).Justify(ui.End).PaddingY(6).Children(func() {
				if m.button(c, "cancel", locales.CommonCancel(), nil, "").Clicked() {
					h.editing = false
				}
				if m.button(c, "save", locales.CommonSave(), nil, "primary").Clicked() {
					m.st.UI.HomeCards = slices.Clone(h.draft)
					m.patch(map[string]any{"ui": map[string]any{"homeCards": h.draft}})
					h.editing = false
				}
			})
		})
	})
}

// ---- The proxy that carries traffic ----

// chain follows a group's selection down to the proxy that carries traffic.
func proxyChain(g *ProxyGroup, groups map[string]*ProxyGroup, now string) []string {
	out := []string{g.Name}
	for i := 0; now != "" && i < 8; i++ {
		out = append(out, now)
		next := groups[now]
		if next == nil || next.Now == "" {
			break
		}
		now = next.Now
	}
	return out
}

func (m *mainUI) groupType(t string) string {
	if slices.Contains([]string{"Selector", "URLTest", "Fallback", "LoadBalance", "Relay"}, t) {
		return m.T("proxies.type." + t)
	}
	return t
}

// ---- Profile ----

func (m *mainUI) profileCard(c *ui.Context) {
	h, a := &m.home, m.a
	_ = h
	link := func() { m.linkTo(c, "profiles", "", locales.HomeManage()) }
	var cur *struct{ i int }
	for i := range m.pv.Items {
		if m.pv.Items[i].UID == m.pv.Current {
			cur = &struct{ i int }{i}
		}
	}
	if cur == nil {
		m.card(c, "profile", locales.HomeProfile(), icFileStack, link, func() {
			m.empty(c, locales.HomeNoProfile(), func() {
				if m.button(c, "import", locales.HomeImportProfile(), nil, "primary").Clicked() {
					m.goTo("profiles", "")
				}
			})
		}).Grow(1)
		return
	}
	pr := m.pv.Items[cur.i]
	remote := pr.Type == "remote"
	var used, total int64
	if pr.Usage != nil {
		used, total = pr.Usage.Upload+pr.Usage.Download, pr.Usage.Total
	}
	pct := fmtPercent(used, total)
	m.card(c, "profile", locales.HomeProfile(), icFileStack, func() {
		if remote && m.spinButton(c, "update", icRefreshCw, locales.ProfilesUpdate(), h.busy == "profile").Clicked() {
			h.busy = "profile"
			uid := pr.UID
			m.run(locales.ProfilesUpdateFailed(), func() error { _, err := (Profiles{a}).Update(context.Background(), uid, nil); return err }, func(err error) {
				h.busy = ""
				if err == nil {
					m.notices = append(m.notices, Notice{Level: "success", Message: locales.ProfilesUpdated()})
				}
			})
		}
		link()
	}, func() {
		sub := locales.ProfilesLocal()
		if remote {
			sub = locales.ProfilesRemote() + " · " + locales.ProfilesNodes(fmt.Sprint(pr.Proxies))
		}
		m.lead(c, pr.Name, sub, func() {
			if total > 0 {
				ui.Text(c, fmt.Sprintf("%.0f%%", pct)).FontSize(18).FontWeight(650).FontFeatures("tnum").ContentTransition(numRoll).TextColor(m.pal.Text2)
			}
		})
		if total > 0 {
			m.progress(c, pct)
		}
		m.kv(c, func() {
			if remote {
				if total > 0 {
					m.kvText(c, locales.HomeUsed(), fmtBytes(used)+" / "+fmtBytes(total))
				}
				expire := locales.ProfilesNoExpiry()
				if pr.Usage != nil && pr.Usage.Expire != 0 {
					expire = fmtDate(pr.Usage.Expire)
				}
				m.kvText(c, locales.HomeExpire(), expire)
				m.kvText(c, locales.HomeUpdated(), m.relTime(time.Unix(pr.Updated, 0)))
				next := locales.CommonOff()
				if t, ok := m.pv.NextUpdates[pr.UID]; ok && !t.IsZero() {
					next = m.relTime(t)
				}
				m.kvText(c, locales.HomeNextUpdate(), next)
			} else {
				m.kvText(c, locales.HomeNodes(), fmt.Sprint(pr.Proxies))
				m.kvText(c, locales.HomeGroups(), fmt.Sprint(pr.Groups))
				m.kvText(c, locales.HomeUpdated(), m.relTime(time.Unix(pr.Updated, 0)))
			}
		})
		if pr.LastError != "" {
			m.errorLine(c, pr.LastError)
		}
	}).Grow(1)
}

// progress is a bar of how much of something is used, which warns as it
// fills.
func (m *mainUI) progress(c *ui.Context, pct float64) {
	p := m.pal
	col := p.Accent
	switch {
	case pct > 90:
		col = p.Danger
	case pct > 75:
		col = p.Warning
	}
	bar := ui.Box(c).Height(6).Radius(3).Background(p.Surface3)
	w := bar.Animate("pct", float32(pct), 400*time.Millisecond)
	col = bar.AnimateColor("col", col, ui.Animation{Duration: 300 * time.Millisecond})
	bar.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * w / 100, H: r.H}, col, 3)
	})
}

// ---- Tailscale ----

func (m *mainUI) tailscaleCard(c *ui.Context) {
	p, ts := m.pal, m.ts
	mode := m.st.Tailscale.Mode
	if mode == "" {
		mode = "off"
	}
	m.card(c, "tailscale", "Tailscale", icWaypoints, func() { m.linkTo(c, "tailscale", "", locales.CommonOpen()) }, func() {
		if mode == "off" {
			m.lead(c, locales.TsStateNoState(), locales.TsOffHint(), func() {
				if m.button(c, "setup", locales.TsSetUp(), nil, "primary").Clicked() {
					m.goTo("tailscale", "")
				}
			})
			ui.Spacer(c)
			return
		}
		state := ts.BackendState
		if state == "" {
			state = "NoState"
		}
		dot := p.Faint
		switch state {
		case "Running":
			dot = p.Success
		case "NeedsLogin", "NeedsMachineAuth":
			dot = p.Warning
		}
		sub := ts.TailnetName
		if sub == "" && ts.User != nil {
			sub = ts.User.LoginName
		}
		if sub == "" {
			sub = m.T("ts.mode." + mode)
		}
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(8, 8).Radius(4).Background(dot).Shrink(0)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, m.T("ts.state."+state)).FontSize(16).FontWeight(650).SingleLine()
				ui.Text(c, sub).FontSize(12).TextColor(p.TextMuted).SingleLine()
			})
			if (state == "NeedsLogin" || state == "NeedsMachineAuth") && m.button(c, "signin", locales.HomeSignIn(), nil, "primary").Clicked() {
				m.goTo("tailscale", "")
			}
		})
		online, exit, addr := 0, locales.CommonNone(), "—"
		for _, peer := range ts.Peers {
			if peer.Online {
				online++
			}
			if peer.ID == ts.ExitNodeID && ts.ExitNodeID != "" {
				exit = peer.HostName
			}
		}
		if ts.Self != nil && len(ts.Self.TailscaleIPs) > 0 {
			addr = ts.Self.TailscaleIPs[0]
		}
		m.kv(c, func() {
			m.kvText(c, locales.TsAddress(), addr).Font("ui-monospace").Selectable()
			m.kvText(c, locales.TsPeers(), locales.TsPeersOnline(fmt.Sprint(online), fmt.Sprint(len(ts.Peers))))
			m.kvText(c, locales.TsExitNode(), exit)
		})
		ui.Spacer(c)
	}).Grow(1)
}

// ---- Sites ----

func (m *mainUI) testCard(c *ui.Context) {
	h, p, a := &m.home, m.pal, m.a
	if h.sites == nil {
		h.sites = (Tools{a}).Sites()
	}
	test := func() {
		h.results, h.sitesBusy = map[string]tools.SiteResult{}, len(h.sites)
		win, sites := m.window(), h.sites
		go a.testSites(a.ctx, sites, func(r tools.SiteResult) {
			win.Update(func() {
				h.results[r.ID] = r
				h.sitesBusy--
			})
		})
	}
	if !h.sitesTested {
		h.sitesTested = true
		test()
	}
	m.card(c, "test", locales.HomeSites(), icTimer, func() {
		if m.spinButton(c, "retest", icRefreshCw, locales.CommonRetest(), h.sitesBusy > 0).Clicked() {
			test()
		}
		m.linkTo(c, "connectivity", "", locales.NavConnectivity())
	}, func() {
		// Six sites: one row of six, rows of three or two, or a list.
		cols := func(width float32) int {
			switch {
			case width >= 840:
				return 6
			case width >= 480:
				return 3
			case width >= 250:
				// Two by three, so that a card of a third of the home page
				// is about as tall as those beside it.
				return 2
			}
			return 1
		}
		m.flipGrid(c, "sites", 8, cols, &reflow, len(h.sites), func(i int) any { return h.sites[i].ID }, func(i int, box ui.Element) {
			s := h.sites[i]
			box.Children(func() {
				r, done := h.results[s.ID]
				tile := ui.Row(c).Grow(1).Height(36).Padding(0, 10).Gap(8).Radius(8).Background(p.Surface2).AlignItems(ui.Center)
				tip := s.URL
				if done && r.Error != "" {
					tip = r.Error
				}
				tile.Tooltip(tip)
				tile.Children(func() {
					m.logo(c, s.ID, s.Name, 16)
					ui.Text(c, s.Name).FontSize(12.5).SingleLine().Grow(1).Basis(0)
					switch {
					case done && r.Error != "":
						m.siteDelayText(c, 0, false)
					case done:
						m.siteDelayText(c, r.DelayMs, false)
					case h.sitesBusy > 0:
						m.siteDelayText(c, -1, true)
					default:
						m.siteDelayText(c, -1, false)
					}
				})
			})
		})
		ui.Spacer(c)
	}).Grow(1)
}

// ---- The core ----

func (m *mainUI) coreCard(c *ui.Context) {
	a, core := m.a, m.state.Core
	tone := "warning"
	switch core.Status {
	case coremgr.StatusRunning:
		tone = "success"
	case coremgr.StatusError:
		tone = "danger"
	}
	m.card(c, "core", locales.HomeCore(), icCpu, func() {
		if m.iconButton(c, "restart", icRefreshCw, locales.CoreRestart()).Clicked() {
			m.run(locales.CommonFailed(), func() error { return Core{a}.Restart(context.Background()) }, nil)
		}
		m.linkTo(c, "logs", "", locales.NavLogs())
	}, func() {
		version := core.CoreVersion
		if version == "" {
			version = m.info.CoreVersion
		}
		sub := locales.CoreSidecar()
		if core.Mode == "service" {
			sub = locales.CoreService()
		}
		if core.PID != 0 {
			sub += fmt.Sprintf(" · PID %d", core.PID)
		}
		m.lead(c, strings.TrimSpace("mihomo "+version), sub, func() { m.badge(c, m.T("core."+core.Status), tone) })
		m.kv(c, func() {
			uptime := "—"
			if !core.StartedAt.IsZero() {
				uptime = fmtDuration(time.Since(core.StartedAt))
				c.After(10 * time.Second)
			}
			m.kvText(c, locales.CoreUptime(), uptime)
			m.kvText(c, locales.HomeApplied(), m.relTime(m.state.AppliedAt))
			m.kvText(c, locales.HomeRestarts(), fmt.Sprint(core.Restarts))
		})
		if msg := cmp.Or(m.state.ConfigError, core.Error); msg != "" {
			m.errorLine(c, msg)
		}
		ui.Spacer(c)
	}).Grow(1)
}

// ---- This computer ----

func (m *mainUI) systemCard(c *ui.Context) {
	h, info := &m.home, m.info
	if !h.autostartKnown {
		h.autostartKnown, h.autostart = true, mygo.App.OpenAtLogin()
	}
	m.card(c, "system", locales.HomeSystem(), icMonitor, func() { m.linkTo(c, "settings", "advanced", locales.NavSettings()) }, func() {
		m.lead(c, info.Hostname, info.OS+" / "+info.Arch, nil)
		m.kv(c, func() {
			m.kvText(c, locales.HomeVersion(), "v"+info.Version)
			m.kvRow(c, locales.SettingsAutoLaunch(), func() {
				on := h.autostart
				if ui.Switch(c, &on).Label(locales.SettingsAutoLaunch()).Changed() {
					h.autostart = on
					m.run(locales.CommonFailed(), func() error { return mygo.App.SetOpenAtLogin(on) }, func(err error) {
						if err != nil {
							h.autostart = mygo.App.OpenAtLogin()
						}
					})
				}
			})
			m.kvRow(c, locales.HomeKeyring(), func() {
				tone := "warning"
				if info.KeyringSecure {
					tone = "success"
				}
				m.badge(c, info.Keyring, tone)
			})
		})
		ui.Spacer(c)
	}).Grow(1)
}

// ---- Where the traffic leaves ----

// ipCard is the gist of the connectivity page's addresses: the address
// sites abroad see, who runs it and how it rates, and whether sites at
// home see another, that is whether the rules split them.
func (m *mainUI) ipCard(c *ui.Context) {
	s, p := &m.cty, m.pal
	running := m.state.Core.Status == coremgr.StatusRunning
	m.ctyInit()
	if running && !s.ipsAsked && m.coreSettled() {
		s.ipsAsked = true
		m.loadExitIPs()
	}
	m.card(c, "ip", locales.HomeCardIp(), icMapPin, func() {
		m.linkTo(c, "connectivity", "", locales.NavConnectivity())
	}, func() {
		home, abroad := s.ips["domestic"], s.ips["global"]
		state := "info"
		switch {
		case abroad == nil || (abroad.loading && abroad.info == nil):
			state = "loading"
		case abroad.info == nil:
			state = "err"
		}
		ui.Column(c.Key(state)).Gap(12).Transition(revealIn).Children(func() {
			switch state {
			case "loading":
				m.skeleton(c, 22).Width(140)
				m.skeleton(c, 64)
			case "err":
				m.empty(c, cmp.Or(abroad.err, locales.CommonFailed()), nil)
			default:
				info := abroad.info
				ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
					m.flag(c, info.CountryCode, !m.reveal())
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
						place := info.Country
						if info.City != "" && info.City != info.Country {
							place += " · " + info.City
						}
						ui.Text(c, m.ctyMask(cmp.Or(place, "—"))).ContentTransition(maskRoll(m.reveal())).FontSize(16).FontWeight(650).SingleLine()
						ui.Text(c, m.ctyMask(info.IP)).ContentTransition(maskRoll(m.reveal())).FontSize(12).Font("ui-monospace").TextColor(p.TextMuted).SingleLine()
					})
					m.splitBadge(c)
				})
				m.kv(c, func() {
					m.kvText(c, locales.HomeIsp(), m.ctyMask(cmp.Or(info.ISP, "—")))
					m.ipQualityRows(c, info.IP, !m.reveal())
					if home != nil && home.info != nil {
						m.kvRow(c, locales.ConnectivityGroupDomestic(), func() {
							ui.Row(c).Gap(6).AlignItems(ui.Center).MinWidth(0).Children(func() {
								m.flagMasked(c, home.info.CountryCode, !m.reveal())
								ui.Text(c, m.ctyMask(cmp.Or(home.info.ISP, home.info.Country, home.info.IP))).ContentTransition(maskRoll(m.reveal())).FontSize(12.5).SingleLine().Shrink(1)
							})
						})
					}
				})
			}
		})
		ui.Spacer(c)
	})
}
