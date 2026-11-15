package app

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"math"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/usage"
)

// Every tile of the home page opens into a sheet of its details, which
// grows out of the tile, from its place and its size, over the page, and
// shrinks back into it as it closes: from where it came, back to where it
// came from. A sheet turned back on its way goes back from where it is.
// Esc, a press beside the sheet, or one on its title closes it.

// noteTile keeps where a tile that opens a sheet is, for its sheet to grow
// from and shrink back to.
func (m *mainUI) noteTile(key string, e ui.Element) {
	d := &m.dash
	if d.tiles == nil {
		d.tiles = map[string]ui.Rect{}
	}
	if r := e.LayoutBounds(); r.W > 0 {
		d.tiles[key] = r
	}
}

// openSheet opens the sheet of the tile key. While another closes, the
// press lands on what covers the page, so a sheet only ever grows from the
// tile it shrinks back to.
func (m *mainUI) openSheet(key string) {
	d := &m.dash
	if d.sheetKey != "" && d.sheetKey != key {
		return
	}
	d.sheet, d.sheetKey = key, key
}

// tween is a value going from one number to another over a time, which a
// change on its way turns back from where it is.
type tween struct {
	from, to float32
	start    time.Time
	dur      time.Duration
}

// at is the value at now.
func (t *tween) at(now time.Time) float32 {
	if t.dur <= 0 || now.Sub(t.start) >= t.dur {
		return t.to
	}
	e := float32(now.Sub(t.start)) / float32(t.dur)
	if e < 0 {
		e = 0
	}
	return t.from + (t.to-t.from)*sheetEase(e)
}

// moving reports whether it is on its way at now.
func (t *tween) moving(now time.Time) bool { return t.dur > 0 && now.Sub(t.start) < t.dur }

// set sends it to v over d, from where it is at now.
func (t *tween) set(now time.Time, v float32, d time.Duration) {
	t.from, t.to, t.start, t.dur = t.at(now), v, now, d
}

// hold puts it at v, still.
func (t *tween) hold(v float32) { *t = tween{from: v, to: v} }

var sheetEase = ui.CubicBezier(0.6, 0, 0.3, 1)

// sheetDurations are how long a sheet takes to open or close, and to grow
// on into its page or back: less motion fades them, quickly.
func sheetDurations(c *ui.Context) (open, full time.Duration) {
	if c.Preferences().ReduceMotion {
		return 140 * time.Millisecond, 140 * time.Millisecond
	}
	return 420 * time.Millisecond, 260 * time.Millisecond
}

// sheetMotions moves the sheet on, frame by frame: where it is open, and
// grown into its page. A sheet grown all the way gives way to the page;
// one back from it shrinks to its sheet, then into its tile.
func (m *mainUI) sheetMotions(c *ui.Context) (open, full float32) {
	d := &m.dash
	now := c.Now()
	od, fd := sheetDurations(c)
	if d.backStart {
		d.backStart = false
		d.full.set(now, 0, fd)
	}
	if want := b2f(d.sheet != ""); d.open.to != want {
		d.open.set(now, want, od)
	}
	if d.backing && !d.full.moving(now) {
		d.backing, d.sheet = false, ""
		d.open.set(now, 0, od)
	}
	// The home page that the page grown into replaces goes as it was last
	// drawn, under the sheet grown over it; it is put away the next time
	// it shows.
	if d.leftFull {
		d.leftFull, d.sheet, d.sheetKey = false, "", ""
		d.open.hold(0)
		d.full.hold(0)
	}
	if d.toPlace != "" && !d.full.moving(now) && d.full.to == 1 {
		place, tab, tile := d.toPlace, d.toTab, d.sheetKey
		d.toPlace, d.toTab, d.leftFull = "", "", true
		m.show(place, tab, tile, false)
		return 1, 1
	}
	if d.growAfter && d.open.to == 1 && !d.open.moving(now) {
		d.growAfter = false
		m.growIntoPage(c, d.sheetKey)
	}
	if d.open.moving(now) || d.full.moving(now) {
		c.AnimationFrame()
	}
	return d.open.at(now), d.full.at(now)
}

// sheetBack brings the sheet of tile back from the page it grew into:
// over the home page at first, shrinking to the sheet, then into the tile.
func (m *mainUI) sheetBack(tile string) {
	d := &m.dash
	d.sheet, d.sheetKey, d.leftFull = tile, tile, false
	d.open.hold(1)
	d.full.hold(1)
	d.backing, d.backStart = true, true
}

// sheetPlace is the page a sheet grows into: its place and its tab.
func (m *mainUI) sheetPlace(key string) (place, tab string) {
	switch key {
	case "compass", "card:system":
		return "settings", ""
	case "exit", "routes":
		return "proxies", "groups"
	case "mode":
		return "rules", ""
	case "speed", "today":
		return "activity", "stats"
	case "apps":
		return "activity", "connections"
	case "card:profile":
		return "profiles", ""
	case "card:tailscale":
		return "tailscale", ""
	case "card:ip":
		return "connectivity", ""
	case "card:test":
		return "connectivity", ""
	case "card:core":
		return "activity", "logs"
	}
	if strings.HasPrefix(key, "group:") {
		return "proxies", "groups"
	}
	return "home", ""
}

// growIntoPage grows the open sheet on into the page of its tile, the
// navigation's selection going there at once.
func (m *mainUI) growIntoPage(c *ui.Context, key string) {
	d := &m.dash
	place, tab := m.sheetPlace(key)
	if place == "home" || d.toPlace != "" {
		return
	}
	switch key {
	case "compass":
		m.tab = "network"
	case "card:system":
		m.tab = "advanced"
	}
	if g := m.sheetGroup(key); g != nil {
		m.proxies.dial = g.Name
	}
	_, fd := sheetDurations(c)
	d.toPlace, d.toTab = place, tab
	d.full.set(c.Now(), 1, fd)
	m.nav.ahead = place
}

// homeSheet draws the open sheet over the stage sb, k of the way from its
// tile to its place, and f of the way on from there to the whole stage.
func (m *mainUI) homeSheet(c *ui.Context, sb ui.Rect, k, f float32) {
	d, p := &m.dash, m.pal
	key := d.sheetKey
	tile, ok := d.tiles[key]
	bar := c.TitleBar()
	from, to := sheetEnds(sb, tile, ok, c.Preferences().ReduceMotion, max(bar.Height, 44)+8)
	// Beside the sheet, the page dims; a press there closes the sheet, or,
	// on the tile it shrinks back to, opens it again.
	scrim := ui.ButtonBase(c.Key("scrim")).Absolute().Left(0).Top(0).Width(sb.W).Height(sb.H).Label(locales.CommonClose())
	scrim.Background(p.Theme.Background.Alpha(0.62 * k)).Cursor(ui.CursorDefault)
	if scrim.Clicked() && d.toPlace == "" && !d.backing {
		if d.sheet != "" {
			d.sheet = ""
		} else if x, y, ok := scrim.PointerPosition(); ok && from.Contains(x, y) {
			d.sheet = key
		}
	}
	r := sheetFrame(sheetFrame(from, to, k), ui.Rect{W: sb.W, H: sb.H}, f)
	// Grown into its page, the sheet becomes the window's ground, square.
	sheet := ui.Column(c.Key("sheet-"+key)).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H).
		Background(p.Card.Mix(p.Theme.Background, f)).Radius(tileRadius*(1-f)).Clip().Shadow(0, 12, 34, 0, ui.RGBA(0, 0, 0, 0.16*k*(1-f)))
	if c.Preferences().ReduceMotion {
		sheet.Opacity(k)
	}
	// Its content keeps the size it has open, and shows as the sheet nears
	// it, so that nothing reflows on the way; it fades as the sheet grows
	// on into its page.
	fade := float32(math.Max(0, math.Min(1, float64((k-0.45)/0.55)))) * float32(math.Max(0, 1-float64(f)*1.6))
	busy := d.toPlace != "" || d.backing
	sheet.Children(func() {
		in := ui.Column(c.Key("in")).Width(to.W).Height(to.H).Padding(20, 22, 22).Gap(16).Opacity(fade).IgnoreInput(busy)
		in.Children(func() {
			ui.Row(c.Key("head")).Gap(10).AlignItems(ui.Center).Shrink(0).Children(func() {
				// The title is the tile pressed again: it closes the sheet.
				title := ui.ButtonBase(c.Key("title")).Row().Grow(1).Basis(0).MinWidth(0).Height(36).AlignItems(ui.Center).Justify(ui.Start).Label(locales.CommonClose())
				title.Children(func() { ui.Text(c, m.sheetTitle(key)).FontSize(24).FontWeight(680).SingleLine() })
				if title.Clicked() {
					d.sheet = ""
				}
				m.sheetActions(c, key)
				if pl, _ := m.sheetPlace(key); pl != "home" {
					if m.bigChip(c, "full", locales.HomeFullPage(), icSquareArrowOutUpRight, false, false).Clicked() {
						m.growIntoPage(c, key)
					}
				}
				if m.iconButton(c, "close", icX, locales.CommonClose()).Size(36, 36).Clicked() {
					d.sheet = ""
				}
			})
			ui.Column(c.Key("body")).Grow(1).Basis(0).MinHeight(0).Gap(14).Children(func() { m.sheetBody(c, key, to.W-44) })
		})
	})
}

// sheetEnds are where a sheet starts and ends, inside the stage sb: from
// its tile, where the tile was last laid out (if known), to the stage
// under its header (top high) less the page's margins. With less motion it starts where it ends, and only
// fades.
func sheetEnds(sb, tile ui.Rect, known, still bool, top float32) (from, to ui.Rect) {
	to = ui.Rect{X: 22, Y: top, W: max(0, sb.W-44), H: max(0, sb.H-22-top)}
	from = to
	if known && !still {
		from = ui.Rect{X: tile.X - sb.X, Y: tile.Y - sb.Y, W: tile.W, H: tile.H}
	}
	return from, to
}

// sheetFrame is the sheet k of the way from from to to: wherever k turns,
// the sheet goes on from where it is.
func sheetFrame(from, to ui.Rect, k float32) ui.Rect {
	lerp := func(a, b float32) float32 { return a + (b-a)*k }
	return ui.Rect{X: lerp(from.X, to.X), Y: lerp(from.Y, to.Y), W: lerp(from.W, to.W), H: lerp(from.H, to.H)}
}

// sheetTitle names the sheet of a tile.
func (m *mainUI) sheetTitle(key string) string {
	switch key {
	case "compass":
		return locales.HomeTakeover()
	case "exit", "routes":
		if g, _ := m.heroGroup(); g != nil && key == "exit" {
			return g.Name
		}
		return locales.HomeOtherRoutes()
	case "mode":
		return locales.HomeMode()
	case "speed":
		return locales.HomeSpeedNow()
	case "apps":
		return locales.HomeWhoUses()
	case "today":
		return locales.HomeTraffic()
	case "card:profile":
		return locales.HomeProfile()
	case "card:tailscale":
		return "Tailscale"
	case "card:ip":
		return locales.HomeCardIp()
	case "card:test":
		return locales.HomeSites()
	case "card:core":
		return locales.HomeCore()
	case "card:system":
		return locales.HomeSystem()
	}
	return strings.TrimPrefix(key, "group:")
}

// sheetGroup is the group a sheet shows the nodes of, if it does.
func (m *mainUI) sheetGroup(key string) *ProxyGroup {
	g, byName := m.heroGroup()
	if key == "exit" {
		return g
	}
	if name, ok := strings.CutPrefix(key, "group:"); ok {
		return byName[name]
	}
	return nil
}

// sheetActions are the buttons on a sheet's title line, before the way to
// its page and the one that closes it.
func (m *mainUI) sheetActions(c *ui.Context, key string) {
	h := &m.home
	if g := m.sheetGroup(key); g != nil {
		if m.bigChip(c, "test", locales.ProxiesTestGroup(), icZap, false, h.testing).Clicked() && !h.testing {
			m.homeTest(g, nil)
		}
	}
}

// sheetBody fills the sheet of a tile, w wide.
func (m *mainUI) sheetBody(c *ui.Context, key string, w float32) {
	running := m.state.Core.Status == coremgr.StatusRunning
	scroll := func(fn func()) { ui.Scroll(c.Key("scroll")).Grow(1).Gap(14).Children(fn) }
	switch key {
	case "compass":
		scroll(func() { m.takeoverSheet(c) })
	case "mode":
		scroll(func() { m.modeSheet(c) })
	case "speed":
		m.speedSheet(c)
	case "routes":
		scroll(func() { m.routesSheet(c) })
	case "apps":
		m.appsSheet(c)
	case "today":
		scroll(func() { m.todaySheet(c) })
	case "card:profile":
		scroll(func() { m.profileSheet(c) })
	case "card:tailscale":
		scroll(func() {
			// To look and to switch: this device, the exit node and the
			// others; the routing is the page's to set up.
			if m.ts.BackendState == "Running" {
				st, mode := m.ts, m.st.Tailscale.Mode
				ui.Row(c).Gap(12).AlignItems(ui.Stretch).Wrap().Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(320).Children(func() { m.tsSelf(c, &st, mode) })
					ui.Column(c).Grow(1).Basis(0).MinWidth(280).Children(func() { m.tsExitNode(c, &st, mode) })
				})
				m.tsPeers(c, &st, mode)
			} else {
				m.tailscaleCard(c)
			}
		})
	case "card:ip":
		// The page it grows into opens at the addresses.
		m.ctyInit()
		m.cty.view = ctyExit
		scroll(func() { m.exitDetail(c, running) })
	case "card:test":
		m.ctyInit()
		m.ctyStart(running)
		m.cty.view = ctySites
		scroll(func() { m.sitesDetail(c, running) })
	case "card:core":
		scroll(func() { m.coreSheet(c) })
	case "card:system":
		scroll(func() { m.systemCard(c) })
	default:
		if g := m.sheetGroup(key); g != nil {
			scroll(func() { m.groupSheet(c, g, key == "exit", w) })
		} else {
			m.empty(c, locales.HomeNoGroups(), nil)
		}
	}
}

// sheetDialogs are the dialogs what the sheets reuse from their pages may
// open, while those sheets show.
func (m *mainUI) sheetDialogs(c *ui.Context) {
	running := m.state.Core.Status == coremgr.StatusRunning
	switch m.dash.sheetKey {
	case "card:test":
		m.sitesDialog(c, running)
	case "apps":
		m.connDetail(c)
	}
}

// sheetRow is a line of a sheet: a title, what it says under it, and a
// control at its end, on a step of grey.
func (m *mainUI) sheetRow(c *ui.Context, key, title, desc string, descCol ui.Color, control func()) {
	p := m.pal
	ui.Row(c.Key(key)).Padding(16, 18).Gap(14).Radius(14).Background(p.Surface2).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, title).FontSize(16).FontWeight(650).SingleLine()
			if desc != "" {
				ui.Text(c, desc).FontSize(13).TextColor(descCol).Wrap()
			}
		})
		if control != nil {
			control()
		}
	})
}

// takeoverSheet is the two ways the app takes traffic over, each with its
// switch and what it does now, and the core under them.
func (m *mainUI) takeoverSheet(c *ui.Context) {
	h, p, a, s, st := &m.home, m.pal, m.a, m.state, m.st
	running := s.Core.Status == coremgr.StatusRunning
	set := func(path string, on bool) {
		h.busy = "takeover"
		m.run(locales.CommonFailed(), func() error {
			_, err := (SettingsService{a}).Patch(context.Background(), patchPath(path, on))
			return err
		}, func(error) { h.busy = "" })
	}
	sysDesc, sysCol := locales.HomeSysproxyOff(), p.TextMuted
	switch {
	case s.SystemProxyError != "":
		sysDesc, sysCol = s.SystemProxyError, p.Danger
	case s.SystemProxy:
		sysDesc = fmt.Sprintf("%s:%d", st.SystemProxy.Host, s.MixedPort)
	}
	m.sheetRow(c, "sys", locales.SettingsSystemProxy(), sysDesc, sysCol, func() {
		on := st.SystemProxy.Enabled
		if ui.Switch(c.Key("sw"), &on).Label(locales.SettingsSystemProxy()).Disabled(!running || h.busy != "").Changed() {
			set("systemProxy.enabled", on)
		}
	})
	tunDesc := locales.HomeTunNeedsService()
	switch {
	case s.Tun:
		tunDesc = locales.HomeTunOn() + " · " + st.Tun.Stack
	case s.TunAvailable:
		tunDesc = locales.HomeTunOff()
	case s.Service.Installed && s.Service.Outdated:
		tunDesc = locales.HomeTunServiceOutdated()
	}
	m.sheetRow(c, "tun", "TUN", tunDesc, p.TextMuted, func() {
		switch {
		case s.TunAvailable:
			on := st.Tun.Enabled
			if ui.Switch(c.Key("sw"), &on).Label("TUN").Disabled(!running || h.busy != "").Changed() {
				set("tun.enabled", on)
			}
		case s.Service.Supported:
			if h.busy == "svc" {
				ui.Spinner(c)
			} else if m.button(c, "svc", locales.ServiceInstall(), nil, "").Clicked() {
				h.busy = "svc"
				m.run("", func() error { a.installServiceForTun(); return nil }, func(error) { h.busy = "" })
			}
		}
	})
	core := s.Core
	desc, col := strings.TrimSpace("mihomo "+core.CoreVersion), p.TextMuted
	switch core.Status {
	case coremgr.StatusStarting:
		desc, col = locales.HomeStateStarting(), p.Warning
	case coremgr.StatusError:
		desc, col = cmp.Or(core.Error, locales.HomeStateError()), p.Danger
	case coremgr.StatusRunning:
	default:
		desc, col = locales.HomeStateStopped(), p.Warning
	}
	m.sheetRow(c, "core", locales.HomeCore(), desc, col, func() {
		label := locales.CoreRestart()
		if !running {
			label = locales.CoreStart()
		}
		if h.busy == "core" || core.Status == coremgr.StatusStarting {
			ui.Spinner(c)
		} else if m.button(c, "restart", label, icRefreshCw, "").Clicked() {
			h.busy = "core"
			m.run(locales.CommonFailed(), func() error { return Core{a}.Restart(context.Background()) }, func(error) { h.busy = "" })
		}
	})
}

// modeSheet is the three modes, each with what it does, the one in use in
// the accent's soft fill.
func (m *mainUI) modeSheet(c *ui.Context) {
	p, a := m.pal, m.a
	mode := m.st.Clash.Mode
	for _, md := range []string{"rule", "global", "direct"} {
		on := md == mode
		b := ui.ButtonBase(c.Key(md)).Row().Padding(18, 20).Gap(14).Radius(14).AlignItems(ui.Center).Label(m.T("mode." + md))
		bg := p.Surface2
		switch {
		case on:
			bg = p.AccentSoft
		case b.Hovered():
			bg = p.Surface3
		}
		kp := b.AnimateTo("press", map[bool]float32{true: 0.985, false: 1}[b.Pressed()], pressSpring)
		b.Scale(kp, kp).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 160 * time.Millisecond}))
		b.Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
				t := ui.Text(c, m.T("mode."+md)).FontSize(20).FontWeight(650)
				if on {
					t.TextColor(p.AccentFg)
				}
				ui.Text(c, m.T("mode."+md+"Desc")).FontSize(13.5).TextColor(p.TextMuted).Wrap()
			})
			if on {
				ui.Icon(c.Key("check"), icCheck).FontSize(20).TextColor(p.AccentFg).Transition(checkIn)
			}
		})
		if b.Clicked() && !on {
			m.st.Clash.Mode = md // at once; the settings confirm
			m.run(locales.CommonFailed(), func() error { return Core{a}.SetMode(context.Background(), md) }, nil)
		}
	}
}

// speedSheet is the speed now and at its peak, and the last two minutes,
// large.
func (m *mainUI) speedSheet(c *ui.Context) {
	d, p := &m.dash, m.pal
	var peak int64
	for i := range m.down {
		peak = max(peak, m.down[i], m.up[i])
	}
	ui.Row(c).Gap(28).AlignItems(ui.End).Wrap().Shrink(0).Children(func() {
		big := func(icon *ui.SVG, col ui.Color, v int64) {
			num, unit, _ := strings.Cut(fmtRate(v), " ")
			ui.Row(c).Gap(6).AlignItems(ui.Baseline).Children(func() {
				ui.Icon(c, icon).FontSize(22).TextColor(col)
				ui.Text(c, num).FontSize(44).FontWeight(680).LetterSpacing(-0.6).FontFeatures("tnum").ContentTransition(numRoll)
				ui.Text(c, unit).FontSize(15).TextColor(p.TextMuted)
			})
		}
		big(icArrowDown, p.Accent, m.now.Down)
		big(icArrowUp, p.Accent2, m.now.Up)
		ui.Spacer(c)
		ui.Column(c).Gap(4).AlignItems(ui.End).Children(func() {
			for _, s := range []string{locales.HomePeak(fmtRate(peak)), locales.HomeConns(fmt.Sprint(m.conns)), locales.HomeToday(fmtBytes(d.today))} {
				ui.Text(c, s).FontSize(13.5).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll)
			}
		})
	})
	ui.Column(c).Grow(1).MinHeight(120).Padding(14).Radius(14).Background(p.Surface2).Children(func() {
		m.trafficGraph(c, trafficLen, 0, false)
	})
}

// routesSheet is every group that chooses, each as a key that opens its
// own sheet in place of this one.
func (m *mainUI) routesSheet(c *ui.Context) {
	h, p := &m.home, m.pal
	_, byName := m.heroGroup()
	for i := range h.view.Groups {
		g := &h.view.Groups[i]
		if g.Hidden || (g.Type != "Selector" && g.Type != "URLTest" && g.Type != "Fallback") {
			continue
		}
		path := proxyChain(g, byName, g.Now)
		exit := path[len(path)-1]
		m.sheetRow(c, g.Name, g.Name, stripFlag(exit)+"  ·  "+m.groupType(g.Type), p.TextMuted, func() {
			if m.button(c, "go", locales.HomeChangeNode(), nil, "").Clicked() {
				m.proxies.focus = g.Name
				m.dash.sheet = ""
				m.goTo("proxies", "")
			}
		})
	}
}

// groupSheet is a group's nodes as keys, w wide; for the group the page
// leads with, where its traffic really leaves first.
func (m *mainUI) groupSheet(c *ui.Context, g *ProxyGroup, hero bool, w float32) {
	h, d, p := &m.home, &m.dash, m.pal
	now := g.Now
	if hero && h.pending != "" {
		now = h.pending
	}
	if hero && h.ip != nil && now != "DIRECT" {
		ip := h.ip
		ui.Row(c.Key("ip")).Padding(14, 18).Gap(14).Radius(14).Background(p.Surface2).AlignItems(ui.Center).Shrink(0).Children(func() {
			if ip.CountryCode != "" {
				m.flag(c, ip.CountryCode, false)
			}
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
				addr, isp := ip.IP, ip.ISP
				if !m.reveal() {
					addr, isp = strings.Repeat("•", 10), ""
				}
				ui.Text(c, strings.TrimSpace(ip.City+" "+ip.Country)).FontSize(15).FontWeight(620).SingleLine()
				ui.Text(c, strings.TrimSpace(addr+"  "+isp)).FontSize(12.5).Font("ui-monospace").TextColor(p.TextMuted).SingleLine().Selectable().ContentTransition(maskRoll(m.reveal()))
			})
			m.badge(c, locales.HomeMeasured(), "success")
			if m.spinButton(c, "reload", icRefreshCw, locales.CommonRefresh(), h.ipLoading).Clicked() {
				m.loadIP()
			}
		})
	}
	selectable := g.Type == "Selector"
	cols := max(2, int((w+10)/(196+10)))
	ui.Grid(c.Key("grid")).Gap(10).Columns(cols).Shrink(0).Children(func() {
		for i := range g.All {
			it := &g.All[i]
			delay := it.Delay
			if v, ok := h.delays[it.Name]; ok {
				delay = v
			}
			tile, _ := m.nodeTile(c, it.Name, it, delay, it.Name == now, h.testing)
			if tile.Clicked() && it.Name != now && selectable {
				if hero {
					m.homeSelect(g.Name, it.Name)
				} else {
					m.selectIn(g.Name, it.Name)
				}
				d.sheet = ""
			}
		}
	})
}

// appsSheet is the connections now, as the connections page lists them.
func (m *mainUI) appsSheet(c *ui.Context) {
	if len(m.connList) == 0 {
		m.empty(c, locales.ConnEmpty(), nil)
		return
	}
	list := make([]*coreapi.Connection, len(m.connList))
	for i := range m.connList {
		list[i] = &m.connList[i]
	}
	ui.Column(c.Key("table")).Grow(1).MinHeight(0).MarginX(-22).Children(func() { m.connTable(c, list) })
}

// todaySheet is today's traffic by the hour, and by app, site and node; a
// name chosen opens the statistics narrowed to it.
func (m *mainUI) todaySheet(c *ui.Context) {
	d, a := &m.dash, m.a
	if time.Since(d.reportAt) > 30*time.Second {
		d.reportAt = time.Now()
		win := m.window()
		go func() {
			r, err := (Stats{a}).Query(context.Background(), usage.StatsQuery{Range: "today", Top: 8})
			if err == nil {
				win.Update(func() { d.report = &r })
			}
		}()
	}
	r := d.report
	if r == nil {
		ui.Column(c).Height(200).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Spinner(c) })
		return
	}
	ui.Column(c.Key("hours")).Shrink(0).Children(func() { m.statsBars(c, r.Hours, true) })
	before := maps.Clone(m.stats.where)
	ui.Row(c.Key("ranks")).Gap(12).AlignItems(ui.Stretch).Wrap().Children(func() {
		for _, x := range []struct {
			dim, title string
			items      []usage.StatsItem
		}{{"app", locales.StatsApps(), r.Apps}, {"site", locales.StatsSites(), r.Sites}, {"node", locales.StatsNodes(), r.Nodes}} {
			ui.Column(c.Key(x.dim)).Grow(1).Basis(0).MinWidth(240).Children(func() { m.ranking(c, x.dim, x.title, x.items, false) })
		}
	})
	if !maps.Equal(before, m.stats.where) {
		d.sheet = ""
		m.goTo("stats", "")
	}
}

// coreSheet is the core: its card, and what it carries now.
func (m *mainUI) coreSheet(c *ui.Context) {
	m.coreCard(c)
	m.card(c, "now", locales.HomeTraffic(), icActivity, nil, func() {
		m.kv(c, func() {
			m.kvText(c, locales.HomeMemory(), fmtBytes(m.memory))
			m.kvText(c, locales.HomeConnections(), fmt.Sprint(m.conns))
			m.kvText(c, locales.HomeMode(), m.T("mode."+m.st.Clash.Mode))
		})
	})
}

// profileSheet is the profiles to look at and to switch: the one in use,
// a press on another to use it, and the way to update a remote one. What
// edits them and their extensions is the profiles page's.
func (m *mainUI) profileSheet(c *ui.Context) {
	ps, p, a := &m.profs, m.pal, m.a
	if ps.busy == nil {
		ps.busy = map[string]string{}
	}
	if len(m.pv.Items) == 0 {
		m.empty(c, locales.HomeNoProfile(), nil)
		return
	}
	for i := range m.pv.Items {
		pr := &m.pv.Items[i]
		uid, current := pr.UID, pr.UID == m.pv.Current
		b := ui.ButtonBase(c.Key(uid)).Row().Padding(14, 18).Gap(14).Radius(14).AlignItems(ui.Center).Label(pr.Name)
		bg := p.Surface2
		switch {
		case current:
			bg = p.AccentSoft
		case b.Hovered():
			bg = p.Surface3
		}
		kp := b.AnimateTo("press", map[bool]float32{true: 0.985, false: 1}[b.Pressed()], pressSpring)
		b.Scale(kp, kp).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 160 * time.Millisecond}))
		b.Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(3).Children(func() {
				t := ui.Text(c, pr.Name).FontSize(16).FontWeight(650).SingleLine()
				if current {
					t.TextColor(p.AccentFg)
				}
				var facts []string
				if pr.Type == "remote" {
					facts = append(facts, locales.ProfilesRemote())
				} else {
					facts = append(facts, locales.ProfilesLocal())
				}
				if pr.Proxies > 0 {
					facts = append(facts, locales.ProfilesNodes(fmt.Sprint(pr.Proxies)))
				}
				if pr.Usage != nil && pr.Usage.Total > 0 {
					facts = append(facts, fmtBytes(pr.Usage.Upload+pr.Usage.Download)+" / "+fmtBytes(pr.Usage.Total))
				}
				ui.Text(c, strings.Join(facts, " · ")).FontSize(13).TextColor(p.TextMuted).SingleLine()
			})
			switch {
			case ps.busy[uid] != "":
				ui.Spinner(c)
			case current:
				ui.Icon(c.Key("check"), icCheck).FontSize(20).TextColor(p.AccentFg).Transition(checkIn)
			}
			if pr.Type == "remote" && ps.busy[uid] == "" {
				if m.iconButton(c, "update", icRefreshCw, locales.ProfilesUpdate()).Size(34, 34).Clicked() {
					ps.busy[uid] = "update"
					m.run(locales.ProfilesUpdateFailed(), func() error {
						_, err := (Profiles{a}).Update(context.Background(), uid, nil)
						return err
					}, func(error) { delete(ps.busy, uid) })
				}
			}
		})
		if b.Clicked() && !current && ps.busy[uid] == "" {
			ps.busy[uid] = "activate"
			m.run(locales.ProfilesActivateFailed(), func() error { return Profiles{a}.Activate(context.Background(), uid) }, func(error) { delete(ps.busy, uid) })
		}
	}
}
