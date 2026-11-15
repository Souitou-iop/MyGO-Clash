package app

import (
	"cmp"
	"context"
	"fmt"
	"hash/fnv"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/profiles"
	"github.com/mygo-clash/mygo-clash/internal/tools"
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
	if key == "exit" || key == "routes" {
		return g // the other routes are in the exit's sheet
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
	scroll := func(fn func()) { gutter(ui.Scroll(c.Key("scroll")).Grow(1).Gap(14), 16).Children(fn) }
	switch key {
	case "compass", "mode":
		scroll(func() { m.takeoverSheet(c, w) })
	case "speed":
		scroll(func() { m.speedSheet(c) })
	case "apps":
		m.appsSheet(c)
	case "today":
		scroll(func() { m.todaySheet(c) })
	case "card:profile":
		scroll(func() { m.profileSheet(c, w) })
	case "card:tailscale":
		scroll(func() { m.tailnetSheet(c, w) })
	case "card:ip":
		// The page it grows into opens at the addresses.
		m.ctyInit()
		m.cty.view = ctyExit
		scroll(func() { m.ipSheet(c, running) })
	case "card:test":
		m.ctyInit()
		m.ctyStart(running)
		m.cty.view = ctySites
		scroll(func() { m.sitesDetail(c, running) })
	case "card:core":
		scroll(func() { m.machineSheet(c, w, false) })
	case "card:system":
		scroll(func() { m.machineSheet(c, w, true) })
	default:
		if g := m.sheetGroup(key); g != nil {
			scroll(func() { m.groupSheet(c, g, key == "exit" || key == "routes", w) })
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

// takeoverSheet is the switch that takes traffic over, large, in the ring
// of today's hours, lit where it took traffic over; what is true now; the
// two ways of taking over, each with its switch and what goes through it;
// and the three modes, each drawn as what it does to traffic.
func (m *mainUI) takeoverSheet(c *ui.Context, w float32) {
	m.sheetSplit(c, w, func() {
		m.sheetSection(c, "dial", 0, func() { m.takeoverHero(c) })
	}, func() {
		m.sheetSection(c, "ways", 1, func() { m.takeoverWays(c) })
		m.sheetSection(c, "modes", 2, func() { m.modeCards(c) })
	})
}

// takeoverHero is the dial in the ring of today, and what is true now.
func (m *mainUI) takeoverHero(c *ui.Context) {
	p, s := m.pal, m.state
	running := s.Core.Status == coremgr.StatusRunning
	on := running && (s.SystemProxy || s.Tun)
	now := time.Now()
	lit := takeoverOn(m.sheetHistory().Takeover, now)
	nowTick := (now.Hour()*60 + now.Minute()) / 20
	m.panel(c, "hero", func() {
		ui.Column(c).AlignItems(ui.Center).Gap(14).Padding(6, 0, 4).Children(func() {
			ui.Box(c.Key("ring")).Size(232, 232).Shrink(0).Children(func() {
				m.tickRing(c, "ticks", 232, func(i int) bool { return lit[i] || (on && i == nowTick) }, nowTick).Absolute().Left(0).Top(0)
				ui.Box(c.Key("dial-slot")).Absolute().Left(0).Top(0).Right(0).Bottom(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
					m.takeoverDial(c, 168)
				})
			})
			state, col := locales.HomeStateOff(), p.Text
			switch {
			case s.Core.Status == coremgr.StatusStarting:
				state, col = locales.HomeStateStarting(), p.Warning
			case s.Core.Status == coremgr.StatusError:
				state, col = locales.HomeStateError(), p.Danger
			case !running:
				state, col = locales.HomeStateStopped(), p.Warning
			case s.Tun:
				state = locales.HomeStateTun()
			case on:
				state = locales.HomeStateOn()
			}
			ui.Box(c).Height(32).Shrink(0).Children(func() {
				ui.Text(c.Key("state-"+state), state).FontSize(22).FontWeight(650).TextColor(col).SingleLine().
					Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 10}, Exit: &ui.Motion{Y: -8}, Duration: 320 * time.Millisecond, Ease: easeOut})
			})
			ui.Row(c).Gap(24).Children(func() {
				m.stat(c, "core", locales.HomeCore(), strings.TrimSpace("mihomo "+s.Core.CoreVersion))
				m.stat(c, "conns", locales.HomeConnections(), fmt.Sprint(m.conns))
				m.stat(c, "mem", locales.HomeMemory(), fmtBytes(m.memory))
			})
		})
	})
}

// takeoverWays are the two ways of taking traffic over, side by side, each
// with its switch, what it does now and how many connections go through
// it.
func (m *mainUI) takeoverWays(c *ui.Context) {
	h, p, a, s, st := &m.home, m.pal, m.a, m.state, m.st
	running := s.Core.Status == coremgr.StatusRunning
	set := func(path string, on bool) {
		h.busy = "takeover"
		m.run(locales.CommonFailed(), func() error {
			_, err := (SettingsService{a}).Patch(context.Background(), patchPath(path, on))
			return err
		}, func(error) { h.busy = "" })
	}
	viaTun, viaPort := 0, 0
	for _, cn := range m.connList {
		if strings.EqualFold(cn.Metadata.Type, "tun") {
			viaTun++
		} else {
			viaPort++
		}
	}
	way := func(key, title, desc string, descCol ui.Color, active bool, conns int, control func()) {
		bg := p.Surface2
		if active {
			bg = p.AccentSoft
		}
		box := ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(220).Padding(16, 18).Gap(8).Radius(16)
		box.Background(box.AnimateColor("bg", bg, ui.Animation{Duration: 200 * time.Millisecond}))
		box.Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				t := ui.Text(c, title).FontSize(16).FontWeight(650).SingleLine().Grow(1)
				if active {
					t.TextColor(p.AccentFg)
				}
				control()
			})
			ui.Text(c, desc).FontSize(12.5).TextColor(descCol).Wrap()
			if active {
				ui.Text(c.Key("n"), locales.HomeConns(fmt.Sprint(conns))).FontSize(12.5).FontWeight(600).TextColor(p.Text2).FontFeatures("tnum").ContentTransition(numRoll).Transition(revealIn)
			}
		})
	}
	ui.Row(c).Gap(12).Wrap().Children(func() {
		sysDesc, sysCol := locales.HomeSysproxyOff(), p.TextMuted
		switch {
		case s.SystemProxyError != "":
			sysDesc, sysCol = s.SystemProxyError, p.Danger
		case s.SystemProxy:
			sysDesc = fmt.Sprintf("%s:%d", st.SystemProxy.Host, s.MixedPort)
		}
		way("sys", locales.SettingsSystemProxy(), sysDesc, sysCol, s.SystemProxy, viaPort, func() {
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
		way("tun", "TUN", tunDesc, p.TextMuted, s.Tun, viaTun, func() {
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
	})
}

// modeCards are the three modes, each drawn as what it does to traffic:
// the rules split it, global sends it all one way, direct goes past. The
// one in use is in the accent's soft fill; a press uses another.
func (m *mainUI) modeCards(c *ui.Context) {
	p, a := m.pal, m.a
	mode := m.st.Clash.Mode
	m.panelLabel(c, locales.HomeMode())
	ui.Row(c).Gap(10).Wrap().Children(func() {
		for _, md := range []string{"rule", "global", "direct"} {
			on := md == mode
			b := ui.ButtonBase(c.Key(md)).Column().Grow(1).Basis(0).MinWidth(150).Padding(14, 16).Gap(8).Radius(16).AlignItems(ui.Stretch).Justify(ui.Start).Label(m.T("mode." + md))
			bg := p.Surface2
			switch {
			case on:
				bg = p.AccentSoft
			case b.Hovered():
				bg = p.Surface3
			}
			kp := b.AnimateTo("press", map[bool]float32{true: 0.97, false: 1}[b.Pressed()], pressSpring)
			b.Scale(kp, kp).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 160 * time.Millisecond}))
			lit := b.AnimateTo("lit", b2f(on), ui.Animation{Duration: 260 * time.Millisecond, Ease: easeStd})
			b.Children(func() {
				ui.Box(c.Key("pic")).Height(44).Draw(func(pt *ui.Painter, r ui.Rect) { m.drawMode(pt, r, md, lit) })
				t := ui.Text(c, m.T("mode."+md)).FontSize(15).FontWeight(650).SingleLine()
				if on {
					t.TextColor(p.AccentFg)
				}
				ui.Text(c, m.T("mode."+md+"Desc")).FontSize(12).TextColor(p.TextMuted).Wrap()
			})
			if b.Clicked() && !on {
				m.st.Clash.Mode = md // at once; the settings confirm
				m.run(locales.CommonFailed(), func() error { return Core{a}.SetMode(context.Background(), md) }, nil)
			}
		}
	})
}

// drawMode draws what a mode does to traffic, in r: lines from the left
// to a compass in the middle, and on to the right; lit, in the accent.
func (m *mainUI) drawMode(pt *ui.Painter, r ui.Rect, mode string, lit float32) {
	p := m.pal
	col := p.TextMuted.Mix(p.Accent, lit)
	faint := p.Border.Mix(p.Accent.Alpha(0.5), lit)
	cx, cy := r.X+r.W*0.42, r.Y+r.H/2
	x0, x1 := r.X+4, r.X+r.W-6
	ys := []float32{r.Y + 8, cy, r.Y + r.H - 8}
	stroke := func(path *ui.Path, c ui.Color) {
		pt.StrokePath(path.Cap(ui.RoundCap).Join(ui.RoundJoin), 1.6, c)
	}
	switch mode {
	case "rule":
		// Many in, each to its own way out.
		for _, y := range ys {
			stroke((&ui.Path{}).MoveTo(x0, y).CubeTo(cx-14, y, cx-14, cy, cx-7, cy), faint)
		}
		for _, y := range []float32{r.Y + 8, r.Y + r.H - 8} {
			stroke((&ui.Path{}).MoveTo(cx+7, cy).CubeTo(cx+20, cy, cx+20, y, x1, y), col)
		}
	case "global":
		// Many in, all one way out.
		for _, y := range ys {
			stroke((&ui.Path{}).MoveTo(x0, y).CubeTo(cx-14, y, cx-14, cy, cx-7, cy), faint)
		}
		stroke((&ui.Path{}).MoveTo(cx+7, cy).LineTo(x1, cy), col)
	case "direct":
		// Straight past, the compass aside.
		for _, y := range []float32{r.Y + 10, r.Y + r.H - 10} {
			stroke((&ui.Path{}).MoveTo(x0, y).LineTo(x1, y), col)
		}
	}
	look := monoLook(p.TextMuted.Mix(p.AccentFg, lit), 0)
	if mode == "direct" {
		look = monoLook(p.Faint, -30)
	}
	drawCompass(pt, cx, cy, 22, look)
}

// speedSheet is the speed now and at its peak; the last two minutes as
// one curve, down above the middle and up below it, the pointer reading
// any second of it; and what each route brings now, over its last forty
// seconds.
func (m *mainUI) speedSheet(c *ui.Context) {
	d, p := &m.dash, m.pal
	var peak int64
	for i := range m.down {
		peak = max(peak, m.down[i], m.up[i])
	}
	m.sheetSection(c, "now", 0, func() {
		ui.Row(c).Gap(28).AlignItems(ui.End).Wrap().Children(func() {
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
			ui.Row(c).Gap(24).Children(func() {
				m.stat(c, "peak", strings.TrimSpace(locales.HomePeak("")), fmtRate(peak))
				m.stat(c, "conns", locales.HomeConnections(), fmt.Sprint(m.conns))
				m.stat(c, "today", locales.StatsToday(), fmtBytes(d.today))
			})
		})
	})
	m.sheetSection(c, "curve", 1, func() {
		ui.Column(c).Height(200).Padding(14).Radius(16).Background(p.Surface2).Children(func() { m.mirrorCurve(c) })
	})
	if len(m.routeHist) > 0 {
		m.sheetSection(c, "routes", 2, func() { m.routeRates(c) })
	}
}

// mirrorCurve is the last two minutes, the speed down above the middle and
// up below it; the pointer over it reads that second.
func (m *mainUI) mirrorCurve(c *ui.Context) {
	p := m.pal
	box := ui.Box(c.Key("mirror")).Grow(1)
	px, _, over := box.PointerPosition()
	box.Draw(func(pt *ui.Painter, r ui.Rect) {
		n := len(m.down)
		if n < 2 {
			return
		}
		var peak int64 = 1024
		for i := range n {
			peak = max(peak, m.down[i], m.up[i])
		}
		mid := r.Y + r.H/2
		step := r.W / float32(n-1)
		y := func(v int64, sign float32) float32 { return mid - sign*(r.H/2-4)*float32(v)/float32(peak) }
		area := func(v []int64, sign float32, col ui.Color) {
			var line, fill ui.Path
			fill.MoveTo(r.X, mid)
			for i := range v {
				x := r.X + float32(i)*step
				if i == 0 {
					line.MoveTo(x, y(v[i], sign))
				} else {
					line.LineTo(x, y(v[i], sign))
				}
				fill.LineTo(x, y(v[i], sign))
			}
			fill.LineTo(r.X+r.W, mid).Close()
			pt.FillPath(&fill, col.Alpha(0.16))
			pt.StrokePath(line.Join(ui.RoundJoin), 1.5, col)
		}
		pt.Line(r.X, mid, r.X+r.W, mid, 1, p.Border)
		area(m.down, 1, p.Accent)
		area(m.up, -1, p.Accent2)
		drawSparkle(pt, r.X+r.W-2, y(m.down[n-1], 1), 4.5, p.Spark)
		if over && px >= 0 && px <= r.W {
			i := int(math.Round(float64(px / step)))
			i = max(0, min(n-1, i))
			x := r.X + float32(i)*step
			pt.Line(x, r.Y, x, r.Y+r.H, 1, p.TextMuted.Alpha(0.6))
			pt.FillPath((&ui.Path{}).Circle(x, y(m.down[i], 1), 3.5), p.Accent)
			pt.FillPath((&ui.Path{}).Circle(x, y(m.up[i], -1), 3.5), p.Accent2)
			label := fmt.Sprintf("-%ds   ↓ %s   ↑ %s", n-1-i, fmtRate(m.down[i]), fmtRate(m.up[i]))
			tx := max(r.X, min(x+10, r.X+r.W-200))
			pt.Fill(ui.Rect{X: tx - 8, Y: r.Y, W: 200, H: 24}, p.Card, 7)
			pt.Text(tx, r.Y+16, label, 12, p.Text2)
		}
	})
}

// routeRates are the routes moving now, the busiest first: each with its
// flag, its last forty seconds and its speed now.
func (m *mainUI) routeRates(c *ui.Context) {
	p := m.pal
	type route struct {
		name string
		hist []int64
	}
	var rs []route
	for name, h := range m.routeHist {
		rs = append(rs, route{name, h})
	}
	last := func(r route) int64 {
		if len(r.hist) == 0 {
			return 0
		}
		return r.hist[len(r.hist)-1]
	}
	sum := func(r route) (t int64) {
		for _, v := range r.hist {
			t += v
		}
		return
	}
	slices.SortFunc(rs, func(a, b route) int { return cmp.Or(cmp.Compare(sum(b), sum(a)), strings.Compare(a.name, b.name)) })
	m.panelLabel(c, locales.HomeByRoute())
	m.rows(c, "route-rates", func() {
		for _, r := range rs[:min(8, len(rs))] {
			ui.Row(c.Key(r.name)).Padding(10, 14).Gap(12).AlignItems(ui.Center).Transition(listRow).Children(func() {
				ui.Row(c).Width(150).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
					if cc := flagOf(r.name); cc != "" {
						m.flagSmall(c, cc)
					}
					ui.Text(c, stripFlag(r.name)).FontSize(13.5).FontWeight(600).SingleLine()
				})
				vals := make([]float64, len(r.hist))
				for i, v := range r.hist {
					vals[i] = float64(v) + 0.001 // standing still is no failure
				}
				m.trail(c, "trail", vals, 0, 26, p.Accent)
				ui.Text(c, fmtRate(last(r))).FontSize(13).TextColor(p.TextMuted).FontFeatures("tnum").Width(84).TextAlign(ui.End).Shrink(0).ContentTransition(numRoll)
			})
		}
	})
}

// groupSheet is a group, w wide: where its traffic leaves, large, with the
// chain that leads there, how fast and how steady it has been, and where
// it comes out; the other routes, each to open in its place; and its
// nodes as keys, as listed, by delay or by region.
func (m *mainUI) groupSheet(c *ui.Context, g *ProxyGroup, hero bool, w float32) {
	h, d := &m.home, &m.dash
	now := g.Now
	if hero && h.pending != "" {
		now = h.pending
	}
	_, byName := m.heroGroup()
	m.sheetSplit(c, w, func() {
		m.sheetSection(c, "exit", 0, func() { m.exitPanel(c, g, hero, now, byName) })
		m.sheetSection(c, "routes", 1, func() { m.otherRoutes(c, g, byName) })
	}, func() {
		m.sheetSection(c, "nodes", 2, func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				m.panelLabel(c, locales.ProfilesNodes(fmt.Sprint(len(g.All))))
				ui.Spacer(c)
				if i := m.segmented(c, "order", d.nodeOrder, locales.HomeOrderListed(), locales.HomeOrderDelay(), locales.HomeOrderRegion()); i >= 0 {
					d.nodeOrder = i
				}
			})
			m.nodeKeys(c, g, hero, now, w)
		})
	})
}

// exitPanel is where a group's traffic leaves: the node, large, its flag
// and the chain that leads to it; its delay and its last tests; and, for
// the group the page leads with, where it comes out.
func (m *mainUI) exitPanel(c *ui.Context, g *ProxyGroup, hero bool, now string, byName map[string]*ProxyGroup) {
	h, p := &m.home, m.pal
	path := proxyChain(g, byName, now)
	exit := path[len(path)-1]
	var item *ProxyItem
	if last := byName[path[max(0, len(path)-2)]]; last != nil {
		for i := range last.All {
			if last.All[i].Name == exit {
				item = &last.All[i]
			}
		}
	}
	delay := -1
	if item != nil {
		delay = item.Delay
	}
	if v, ok := h.delays[exit]; ok {
		delay = v
	}
	m.panel(c, "exit-panel", func() {
		ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
			if cc := flagOf(exit); cc != "" {
				m.flag(c, cc, false)
			} else {
				ui.Icon(c, icLayers).FontSize(20).TextColor(p.TextMuted)
			}
			ui.Text(c.Key("n-"+exit), stripFlag(exit)).FontSize(26).FontWeight(700).SingleLine().Shrink(1).
				Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 8}, Duration: 300 * time.Millisecond, Ease: easeOut})
		})
		if len(path) > 1 {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
				for i, n := range path {
					if i > 0 {
						ui.Icon(c, icChevronRight).FontSize(12).TextColor(p.Faint)
					}
					col := p.TextMuted
					if i == len(path)-1 {
						col = p.Text2
					}
					ui.Text(c, stripFlag(n)).FontSize(12.5).TextColor(col).SingleLine()
				}
			})
		}
		ui.Row(c).Gap(14).AlignItems(ui.End).Children(func() {
			switch {
			case delay > 0:
				m.figure(c, fmt.Sprint(delay), "ms", 40, m.delayColor(delay))
			case delay == 0:
				m.figure(c, locales.CommonTimeout(), "", 30, p.Danger)
			}
			if item != nil && len(item.History) >= 2 {
				m.trail(c, "trail", historyVals(item.History), 0, 44, p.Accent)
			}
		})
		// What it is, and what it carries besides TCP.
		ui.Row(c).Gap(6).Wrap().Children(func() {
			if item != nil && item.Type != "" {
				m.badge(c, item.Type, "")
			}
			if item != nil {
				for _, f := range []struct {
					on   bool
					name string
				}{{item.UDP, "UDP"}, {item.XUDP, "XUDP"}, {item.TFO, "TFO"}, {item.MPTCP, "MPTCP"}} {
					if f.on {
						m.badge(c, f.name, "info")
					}
				}
			}
		})
		if hero && h.ip != nil && exit != "DIRECT" {
			ip := h.ip
			ui.Box(c).Height(1).Background(p.Border).Margin(4, 0, 0)
			ui.Row(c.Key("ip")).Gap(12).AlignItems(ui.Center).Children(func() {
				if ip.CountryCode != "" {
					m.flagMasked(c, ip.CountryCode, !m.reveal())
				}
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, m.ctyMask(strings.TrimSpace(ip.City+" "+ip.Country))).FontSize(14).FontWeight(620).SingleLine().ContentTransition(maskRoll(m.reveal()))
					ui.Text(c, m.ctyMask(strings.TrimSpace(ip.IP+"  "+cmp.Or(ip.ASN, ip.ISP)))).FontSize(12).Font("ui-monospace").TextColor(p.TextMuted).SingleLine().ContentTransition(maskRoll(m.reveal()))
				})
				if m.spinButton(c, "reload", icRefreshCw, locales.CommonRefresh(), h.ipLoading).Clicked() {
					m.loadIP()
				}
			})
		}
	})
}

// otherRoutes are the groups but this one, each where it leaves: a press
// shows its sheet in this one's place.
func (m *mainUI) otherRoutes(c *ui.Context, g *ProxyGroup, byName map[string]*ProxyGroup) {
	h, p, d := &m.home, m.pal, &m.dash
	hero, _ := m.heroGroup()
	var gs []*ProxyGroup
	for i := range h.view.Groups {
		o := &h.view.Groups[i]
		if o.Name != g.Name && !o.Hidden && (o.Type == "Selector" || o.Type == "URLTest" || o.Type == "Fallback") {
			gs = append(gs, o)
		}
	}
	if len(gs) == 0 {
		return
	}
	m.panelLabel(c, locales.HomeOtherRoutes())
	m.rows(c, "route-rows", func() {
		for _, o := range gs {
			path := proxyChain(o, byName, o.Now)
			exit := path[len(path)-1]
			b := ui.ButtonBase(c.Key(o.Name)).Row().Padding(10, 14).Gap(10).AlignItems(ui.Center).Justify(ui.Start).Label(o.Name)
			if b.Hovered() {
				b.Background(p.Surface3)
			}
			b.Transition(hoverFade)
			b.Children(func() {
				ui.Text(c, o.Name).FontSize(13.5).FontWeight(600).SingleLine().Grow(1).Basis(0).MinWidth(0)
				if cc := flagOf(exit); cc != "" {
					m.flagSmall(c, cc)
				}
				ui.Text(c, stripFlag(exit)).FontSize(13).TextColor(p.TextMuted).SingleLine()
				ui.Icon(c, icChevronRight).FontSize(13).TextColor(p.Faint)
			})
			if b.Clicked() {
				key := "group:" + o.Name
				if hero != nil && o.Name == hero.Name {
					key = "exit"
				}
				d.sheet, d.sheetKey = key, key
			}
		}
	})
}

// nodeKeys are a group's nodes as keys, in the order chosen: by region,
// under each country's flag, name and count.
func (m *mainUI) nodeKeys(c *ui.Context, g *ProxyGroup, hero bool, now string, w float32) {
	h, d := &m.home, &m.dash
	if w >= 760 {
		w = w - min(420, w*0.42) - 18
	}
	cols := max(2, int((w+10)/(176+10)))
	delayOf := func(it *ProxyItem) int {
		if v, ok := h.delays[it.Name]; ok {
			return v
		}
		return it.Delay
	}
	items := make([]*ProxyItem, len(g.All))
	for i := range g.All {
		items[i] = &g.All[i]
	}
	if d.nodeOrder == 1 {
		rank := func(it *ProxyItem) int {
			if v := delayOf(it); v > 0 {
				return v
			}
			return math.MaxInt32
		}
		slices.SortStableFunc(items, func(a, b *ProxyItem) int { return rank(a) - rank(b) })
	}
	grid := func(key string, its []*ProxyItem) {
		ui.Grid(c.Key(key)).Gap(10).Columns(cols).Shrink(0).Children(func() {
			for _, it := range its {
				tile, _ := m.nodeTile(c, it.Name, it, delayOf(it), it.Name == now, h.testing)
				tile.Transition(reflow)
				if tile.Clicked() && it.Name != now && g.Type == "Selector" {
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
	if d.nodeOrder != 2 {
		grid("all", items)
		return
	}
	var order []string
	by := map[string][]*ProxyItem{}
	for _, it := range items {
		cc := flagOf(it.Name)
		if _, ok := by[cc]; !ok {
			order = append(order, cc)
		}
		by[cc] = append(by[cc], it)
	}
	for _, cc := range order {
		ui.Column(c.Key("region-" + cc)).Gap(8).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if cc != "" {
					m.flagSmall(c, cc)
					m.panelLabel(c, m.countryName(cc))
				} else {
					m.panelLabel(c, locales.ConnectivityCatOther())
				}
				ui.Text(c, fmt.Sprint(len(by[cc]))).FontSize(12).TextColor(m.pal.Faint).FontFeatures("tnum")
			})
			grid("grid", by[cc])
		})
	}
}

// delayColor is a delay's color: good, fair or slow.
func (m *mainUI) delayColor(d int) ui.Color {
	p := m.pal
	switch delayKindOf(d) {
	case delayGood:
		return p.Success
	case delayOK:
		return p.Warning
	case delayBad:
		return p.Danger
	}
	return p.Faint
}

// appsSheet is who moves traffic now: each app with the routes its
// connections take, how many it has and its share of the speed now; and
// the connections that just closed, so that it says something when
// nothing is open. The page it grows into lists every connection.
func (m *mainUI) appsSheet(c *ui.Context) {
	d, a := &m.dash, m.a
	if time.Since(d.closedAt) > 5*time.Second {
		d.closedAt = time.Now()
		win := m.window()
		go func() {
			page, err := (Stats{a}).Closed(context.Background(), usage.ClosedQuery{Limit: 8})
			if err == nil {
				win.Update(func() { d.closed = page.Items })
			}
		}()
	}
	gutter(ui.Scroll(c.Key("scroll")).Grow(1).Gap(14), 16).Children(func() {
		if len(m.connList) > 0 {
			m.sheetSection(c, "apps", 0, func() { m.appRoutes(c) })
		}
		if len(d.closed) > 0 {
			m.sheetSection(c, "closed", 1, func() { m.justClosed(c) })
		}
		if len(m.connList) == 0 && len(d.closed) == 0 {
			m.empty(c, locales.ConnEmpty(), nil)
		}
	})
}

// appRoutes are the apps with open connections, the busiest first.
func (m *mainUI) appRoutes(c *ui.Context) {
	p := m.pal
	type app struct {
		name   string
		rate   int64
		n      int
		routes []string
	}
	byName := map[string]*app{}
	var list []*app
	var total int64
	for _, cn := range m.connList {
		name := cmp.Or(cn.Metadata.Process, cn.Metadata.Host, cn.Metadata.DestinationIP)
		ap := byName[name]
		if ap == nil {
			ap = &app{name: name}
			byName[name] = ap
			list = append(list, ap)
		}
		ap.rate += cn.DownloadSpeed + cn.UploadSpeed
		ap.n++
		total += cn.DownloadSpeed + cn.UploadSpeed
		if len(cn.Chains) > 0 && !slices.Contains(ap.routes, cn.Chains[0]) {
			ap.routes = append(ap.routes, cn.Chains[0])
		}
	}
	slices.SortStableFunc(list, func(x, y *app) int {
		return cmp.Or(cmp.Compare(y.rate, x.rate), cmp.Compare(y.n, x.n), strings.Compare(x.name, y.name))
	})
	m.panelLabel(c, locales.HomeWhoUses())
	m.rows(c, "app-rows", func() {
		for _, ap := range list[:min(12, len(list))] {
			ui.Row(c.Key(ap.name)).Padding(10, 14).Gap(12).AlignItems(ui.Center).Transition(listRow).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(5).Children(func() {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, ap.name).FontSize(14).FontWeight(620).SingleLine().Shrink(1)
						ui.Text(c, locales.HomeConns(fmt.Sprint(ap.n))).FontSize(12).TextColor(p.Faint).SingleLine().Shrink(0)
					})
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						for i, r := range ap.routes {
							if i == 3 {
								ui.Text(c, fmt.Sprintf("+%d", len(ap.routes)-3)).FontSize(11.5).TextColor(p.Faint)
								break
							}
							ui.Row(c.Key(r)).Height(20).Padding(0, 7).Gap(5).Radius(6).Background(p.Surface3).AlignItems(ui.Center).Shrink(0).Children(func() {
								if cc := flagOf(r); cc != "" {
									m.flagSmall(c, cc)
								}
								ui.Text(c, stripFlag(r)).FontSize(11.5).TextColor(p.Text2).SingleLine()
							})
						}
					})
				})
				share := 0.0
				if total > 0 {
					share = float64(ap.rate) / float64(total)
				}
				ui.Box(c).Width(80).Shrink(0).Children(func() { m.meter(c, "share", share, p.Accent.Alpha(0.8)) })
				ui.Text(c, fmtRate(ap.rate)).FontSize(13).TextColor(p.TextMuted).FontFeatures("tnum").Width(84).TextAlign(ui.End).Shrink(0).ContentTransition(numRoll)
			})
		}
	})
}

// justClosed are the connections that closed last: where to, by whom,
// through what, how much, and when.
func (m *mainUI) justClosed(c *ui.Context) {
	p := m.pal
	m.panelLabel(c, locales.HomeJustClosed())
	m.rows(c, "closed-rows", func() {
		for _, cl := range m.dash.closed {
			ui.Row(c.Key(cl.ID)).Padding(9, 14).Gap(10).AlignItems(ui.Center).Transition(listRow).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, cmp.Or(cl.Host, cl.DestinationIP)).FontSize(13).FontWeight(600).SingleLine()
					sub := cl.Process
					if len(cl.Chains) > 0 {
						sub = strings.TrimSpace(sub + " → " + stripFlag(cl.Chains[0]))
					}
					ui.Text(c, sub).FontSize(11.5).TextColor(p.TextMuted).SingleLine()
				})
				ui.Text(c, fmtBytes(cl.Up+cl.Down)).FontSize(12).TextColor(p.TextMuted).FontFeatures("tnum").SingleLine().Shrink(0)
				ui.Text(c, m.relTime(cl.End)).FontSize(11.5).TextColor(p.Faint).SingleLine().Width(64).TextAlign(ui.End).Shrink(0)
			})
		}
	})
}

// todaySheet is today's traffic, large, against yesterday's and beside
// the last seven days; its hours, one of which may be chosen; and by app,
// site, node and destination, all day or in the hour chosen. A name chosen
// opens the statistics narrowed to it.
func (m *mainUI) todaySheet(c *ui.Context) {
	d, a, p := &m.dash, m.a, m.pal
	if time.Since(d.reportAt) > 30*time.Second || d.reportHour != d.hour {
		d.reportAt, d.reportHour = time.Now(), d.hour
		hour, win := d.hour, m.window()
		go func() {
			q := usage.StatsQuery{Range: "today", Top: 8}
			if hour != "" {
				q.Where = map[string]string{usage.WhereHour: hour}
			}
			r, err := (Stats{a}).Query(context.Background(), q)
			if err == nil {
				win.Update(func() {
					if d.hour != hour {
						return
					}
					if hour == "" {
						d.allDay = &r
					}
					d.report = &r
				})
			}
		}()
	}
	r := d.report
	if r == nil {
		ui.Column(c).Height(200).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Spinner(c) })
		return
	}
	m.sheetSection(c, "sum", 0, func() {
		ui.Row(c).Gap(18).AlignItems(ui.End).Wrap().Children(func() {
			ui.Column(c).Gap(4).Children(func() {
				num, unit, _ := strings.Cut(fmtBytes(d.today), " ")
				m.figure(c, num, unit, 44, ui.Color{})
				ui.Row(c).Gap(14).Children(func() {
					if all := d.allDay; all != nil && all.Prev != nil && all.Prev.Up+all.Prev.Down > 0 {
						delta := float64(all.Up+all.Down)/float64(all.Prev.Up+all.Prev.Down) - 1
						col := p.TextMuted
						if delta > 0.3 {
							col = p.Warning
						}
						ui.Text(c, locales.HomeVsYesterday(fmt.Sprintf("%+.0f%%", delta*100))).FontSize(13).TextColor(col).SingleLine()
					}
					ui.Text(c, locales.HomeMonth(fmtBytes(d.month))).FontSize(13).TextColor(p.TextMuted).SingleLine()
				})
			})
			ui.Spacer(c)
			m.weekBars(c)
		})
	})
	hours := r.Hours
	if d.allDay != nil {
		hours = d.allDay.Hours // the hours of all day, which a choice does not reshape
	}
	m.sheetSection(c, "hours", 1, func() { m.hourBars(c, hours) })
	before := maps.Clone(m.stats.where)
	m.sheetSection(c, "ranks", 2, func() {
		ui.Row(c.Key("ranks")).Gap(12).AlignItems(ui.Stretch).Wrap().Children(func() {
			for _, x := range []struct {
				dim, title string
				items      []usage.StatsItem
			}{{"app", locales.StatsApps(), r.Apps}, {"site", locales.StatsSites(), r.Sites}, {"node", locales.StatsNodes(), r.Nodes}, {"country", locales.StatsCountries(), r.Countries}} {
				if len(x.items) == 0 {
					continue
				}
				ui.Column(c.Key(x.dim)).Grow(1).Basis(0).MinWidth(240).Children(func() { m.ranking(c, x.dim, x.title, x.items, d.hour != "") })
			}
		})
	})
	if !maps.Equal(before, m.stats.where) {
		d.sheet = ""
		m.goTo("stats", "")
	}
}

// weekBars are the last seven days, today's in the accent.
func (m *mainUI) weekBars(c *ui.Context) {
	d, p := &m.dash, m.pal
	week := slices.Clone(d.week)
	if len(week) == 0 {
		week = []int64{d.today}
	}
	var most int64 = 1
	for _, v := range week {
		most = max(most, v)
	}
	ui.Row(c.Key("week")).Height(64).Gap(6).AlignItems(ui.End).Shrink(0).Children(func() {
		for i, v := range week {
			col := p.Surface3
			if i == len(week)-1 {
				col = p.Accent
			}
			bar := ui.Box(c.Key(i)).Width(14).Radius(4).Background(col).Tooltip(fmtBytes(v))
			h := bar.AnimateTo("h", 6+58*float32(v)/float32(most), ui.Animation{Duration: 500 * time.Millisecond, Ease: easeStd})
			bar.Height(h)
		}
	})
}

// hourBars are today's hours, down and up in each: a press chooses one,
// which the rankings narrow to, and again lets it go.
func (m *mainUI) hourBars(c *ui.Context, hours []usage.StatsPoint) {
	d, p := &m.dash, m.pal
	var most int64 = 1
	for _, h := range hours {
		most = max(most, h.Up+h.Down)
	}
	ui.Row(c).Height(28).AlignItems(ui.Center).Children(func() {
		m.panelLabel(c, locales.StatsToday())
		ui.Spacer(c)
		if d.hour != "" {
			if m.bigChip(c, "all-day", locales.HomeAllDay(), icX, false, false).Clicked() {
				d.hour = ""
			}
		}
	})
	ui.Row(c.Key("hour-bars")).Height(124).Gap(3).AlignItems(ui.Stretch).Children(func() {
		for i, h := range hours {
			label := h.Label
			on := d.hour == label
			b := ui.ButtonBase(c.Key(label)).Column().Grow(1).Basis(0).Radius(5).Justify(ui.End).AlignItems(ui.Stretch).Label(label).Tooltip(label + ":00 · " + fmtBytes(h.Up+h.Down))
			switch {
			case on:
				b.Background(p.AccentSoft)
			case b.Hovered():
				b.Background(p.Surface2)
			}
			b.Transition(hoverFade)
			frac := float32(h.Up+h.Down) / float32(most)
			dim := float32(1)
			if d.hour != "" && !on {
				dim = 0.4
			}
			b.Children(func() {
				anim := ui.Animation{Duration: 420 * time.Millisecond, Delay: time.Duration(i) * 10 * time.Millisecond, Ease: easeStd}
				up := ui.Box(c.Key("u")).Radius(3).Background(p.Accent2.Alpha(0.85*dim)).Margin(0, 2)
				up.Height(up.AnimateTo("h", 96*frac*float32(h.Up)/float32(h.Up+h.Down+1), anim))
				down := ui.Box(c.Key("d")).Radius(3).Background(p.Accent.Alpha(0.85*dim)).Margin(1, 2, 0)
				down.Height(down.AnimateTo("h", 2+96*frac*float32(h.Down+1)/float32(h.Up+h.Down+1), anim))
				if i%6 == 0 {
					ui.Text(c, label).FontSize(10).TextColor(p.Faint).TextAlign(ui.Center).Margin(4, 0, 0)
				} else {
					ui.Box(c).Height(16)
				}
			})
			if b.Clicked() {
				if on {
					d.hour = ""
				} else {
					d.hour = label
				}
			}
		}
	})
}

// machineSheet is the core and this computer, w wide, the one the sheet
// opened for first: the core's state and how long it has run, the days
// it started on, what it holds now and over the last two minutes, and its
// latest warnings; this computer, the app's version and how it keeps
// itself (launch at login, secrets, sync).
func (m *mainUI) machineSheet(c *ui.Context, w float32, systemFirst bool) {
	m.followLogs()
	core := func(i int) {
		m.sheetSection(c, "core", i, func() { m.corePanel(c) })
		m.sheetSection(c, "days", i+1, func() { m.coreDays(c) })
	}
	system := func(i int) { m.sheetSection(c, "system", i, func() { m.systemPanel(c) }) }
	m.sheetSplit(c, w, func() {
		if systemFirst {
			system(0)
			core(1)
		} else {
			core(0)
			system(2)
		}
	}, func() {
		m.sheetSection(c, "trails", 3, func() {
			ui.Row(c).Gap(12).Wrap().Children(func() {
				mem := make([]float64, len(m.memHist))
				for i, v := range m.memHist {
					mem[i] = float64(max(v, 0)) + 0.001 // none yet is no failure
				}
				conns := make([]float64, len(m.connHist))
				for i, v := range m.connHist {
					conns[i] = float64(max(v, 0)) + 0.001 // nothing open is no failure
				}
				m.trailPanel(c, "mem", locales.HomeMemory(), fmtBytes(m.memory), mem)
				m.trailPanel(c, "conns", locales.HomeConnections(), fmt.Sprint(m.conns), conns)
			})
		})
		m.sheetSection(c, "warnings", 4, func() { m.recentWarnings(c) })
	})
}

// trailPanel is a value now, large, over its last two minutes as a trail.
func (m *mainUI) trailPanel(c *ui.Context, key, label, now string, vals []float64) {
	p := m.pal
	ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(220).Padding(16, 18).Gap(8).Radius(16).Background(p.Surface2).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			m.panelLabel(c, label)
			ui.Spacer(c)
			ui.Text(c, locales.HomeWindow()).FontSize(11.5).TextColor(p.Faint).SingleLine()
		})
		num, unit, _ := strings.Cut(now, " ")
		m.figure(c, num, unit, 28, ui.Color{})
		if len(vals) >= 2 {
			m.trail(c, "trail", vals, 0, 46, p.Accent)
		} else {
			m.ghost(c, 0, 46)
		}
	})
}

// corePanel is the core: its version and how it runs, how long it has,
// and the ways to restart it and to read its logs.
func (m *mainUI) corePanel(c *ui.Context) {
	a, p, core := m.a, m.pal, m.state.Core
	tone := "warning"
	switch core.Status {
	case coremgr.StatusRunning:
		tone = "success"
	case coremgr.StatusError:
		tone = "danger"
	}
	m.panel(c, "core-panel", func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icCpu).FontSize(18).TextColor(p.TextMuted)
			ui.Text(c, strings.TrimSpace("mihomo "+cmp.Or(core.CoreVersion, m.info.CoreVersion))).FontSize(20).FontWeight(700).SingleLine().Grow(1).Basis(0).MinWidth(0)
			m.badge(c, m.T("core."+core.Status), tone)
		})
		ui.Row(c).Gap(6).Wrap().Children(func() {
			how := locales.CoreSidecar()
			if core.Mode == "service" {
				how = locales.CoreService()
			}
			m.badge(c, how, "")
			m.badge(c, m.T("mode."+m.st.Clash.Mode), "info")
			if core.PID != 0 {
				m.badge(c, fmt.Sprintf("PID %d", core.PID), "")
			}
		})
		uptime := "—"
		if !core.StartedAt.IsZero() {
			uptime = fmtDuration(time.Since(core.StartedAt))
			c.After(10 * time.Second)
		}
		ui.Column(c).Gap(2).Margin(4, 0, 0).Children(func() {
			ui.Text(c, locales.CoreUptime()).FontSize(12).TextColor(p.TextMuted)
			ui.Text(c, uptime).FontSize(30).FontWeight(700).LetterSpacing(-0.4).FontFeatures("tnum").ContentTransition(numRoll).SingleLine()
		})
		ui.Row(c).Gap(24).Wrap().Children(func() {
			m.stat(c, "applied", locales.HomeApplied(), m.relTime(m.state.AppliedAt))
			m.stat(c, "restarts", locales.HomeRestarts(), fmt.Sprint(core.Restarts))
		})
		if msg := cmp.Or(m.state.ConfigError, core.Error); msg != "" {
			m.errorLine(c, msg)
		}
		ui.Row(c).Gap(8).Margin(4, 0, 0).Children(func() {
			if m.button(c, "restart", locales.CoreRestart(), icRefreshCw, "").Clicked() {
				m.run(locales.CommonFailed(), func() error { return Core{a}.Restart(context.Background()) }, nil)
			}
			if m.button(c, "logs", locales.NavLogs(), icScrollText, "ghost").Clicked() {
				m.dash.sheet = ""
				m.goTo("logs", "")
			}
		})
	})
}

// coreDays are the last thirty days, a cell each: lit on the days the
// core started, the brighter the more often it did; today with the glint.
func (m *mainUI) coreDays(c *ui.Context) {
	p := m.pal
	starts := m.sheetHistory().CoreStarts
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var count [30]int
	for _, t := range starts {
		d := 0 // days before today
		if at := time.Unix(t, 0); at.Before(today) {
			d = int(math.Ceil(today.Sub(at).Hours() / 24))
		}
		if d >= 0 && d < 30 {
			count[29-d]++
		}
	}
	m.panel(c, "days-panel", func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			m.panelLabel(c, locales.StatsDays30())
			ui.Spacer(c)
			total := 0
			for _, n := range count {
				total += n
			}
			ui.Text(c, locales.HomeRestarts()+" "+fmt.Sprint(max(0, total-1))).FontSize(11.5).TextColor(p.Faint).SingleLine()
		})
		row := ui.Row(c.Key("cells")).Height(26).Gap(3)
		row.Children(func() {
			for i, n := range count {
				col := p.Surface3
				switch {
				case n == 1:
					col = p.Accent.Alpha(0.55)
				case n > 1:
					col = p.Warning
				}
				cell := ui.Box(c.Key(i)).Height(26).Grow(1).Basis(0).Radius(3).Background(col)
				if n > 0 {
					cell.Tooltip(today.AddDate(0, 0, i-29).Format("01-02") + " · " + fmt.Sprint(n))
				}
				if i == 29 {
					cell.Draw(func(pt *ui.Painter, r ui.Rect) { drawSparkle(pt, r.X+r.W/2, r.Y-5, 4, p.Spark) })
				}
				if !c.Preferences().ReduceMotion {
					cell.Transition(ui.ElementTransition{Enter: &ui.Motion{Scale: 0.4}, Animation: &ui.Animation{Duration: 240 * time.Millisecond, Delay: 260*time.Millisecond + time.Duration(i)*12*time.Millisecond, Ease: easeOut}})
				}
			}
		})
	})
}

// recentWarnings are the core's last warnings and errors, at most three.
func (m *mainUI) recentWarnings(c *ui.Context) {
	p := m.pal
	var last []coreapi.LogEvent
	for i := len(m.logs.logs) - 1; i >= 0 && len(last) < 3; i-- {
		if e := m.logs.logs[i]; e.Type == "warning" || e.Type == "error" {
			last = append(last, e)
		}
	}
	if len(last) == 0 {
		return
	}
	m.panel(c, "warn-panel", func() {
		m.panelLabel(c, locales.LogsWarning())
		for i, e := range last {
			ui.Row(c.Key(i)).Gap(8).AlignItems(ui.Start).Children(func() {
				col := p.Warning
				if e.Type == "error" {
					col = p.Danger
				}
				ui.Box(c).Size(6, 6).Radius(3).Background(col).Margin(6, 0, 0).Shrink(0)
				ui.Text(c, e.Payload).FontSize(12).Font("ui-monospace").TextColor(p.Text2).Wrap().MaxLines(2).Grow(1).Basis(0).MinWidth(0)
				ui.Text(c, m.relTime(e.Time)).FontSize(11).TextColor(p.Faint).SingleLine().Shrink(0)
			})
		}
	})
}

// systemPanel is this computer, and how the app keeps itself on it.
func (m *mainUI) systemPanel(c *ui.Context) {
	h, p, info := &m.home, m.pal, m.info
	if !h.autostartKnown {
		h.autostartKnown, h.autostart = true, mygo.App.OpenAtLogin()
	}
	m.panel(c, "system-panel", func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icMonitor).FontSize(18).TextColor(p.TextMuted)
			ui.Text(c, info.Hostname).FontSize(20).FontWeight(700).SingleLine().Grow(1).Basis(0).MinWidth(0)
		})
		ui.Text(c, info.OS+" / "+info.Arch).FontSize(13).TextColor(p.TextMuted).SingleLine()
		ui.Row(c).Gap(24).Wrap().Margin(4, 0, 0).Children(func() {
			m.stat(c, "version", locales.HomeVersion(), "v"+info.Version)
			if sy := m.sy; sy.Configured {
				m.stat(c, "sync", locales.SyncLast(), m.relTime(sy.LastSync))
				if sy.Devices > 0 {
					m.stat(c, "devices", locales.SyncTitle(), locales.StatsNDevices(fmt.Sprint(sy.Devices)))
				}
			}
		})
		if u := m.upd; u != nil && u.Ready != "" {
			m.badge(c, locales.UpdatesReady("v"+u.Ready), "info")
		}
		ui.Box(c).Height(1).Background(p.Border).Margin(2, 0)
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SettingsAutoLaunch()).FontSize(13.5).Grow(1)
			on := h.autostart
			if ui.Switch(c.Key("autostart"), &on).Label(locales.SettingsAutoLaunch()).Changed() {
				h.autostart = on
				m.run(locales.CommonFailed(), func() error { return mygo.App.SetOpenAtLogin(on) }, func(err error) {
					if err != nil {
						h.autostart = mygo.App.OpenAtLogin()
					}
				})
			}
		})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.HomeKeyring()).FontSize(13.5).Grow(1)
			tone := "warning"
			if info.KeyringSecure {
				tone = "success"
			}
			m.badge(c, info.Keyring, tone)
		})
	})
}

// sheetHistory is what the sheets show over days, read again at most
// every few seconds.
func (m *mainUI) sheetHistory() *uiCache {
	d := &m.dash
	if time.Since(d.histAt) > 10*time.Second {
		d.histAt, d.hist = time.Now(), m.a.loadUICache()
	}
	return &d.hist
}

// profileSheet is the subscription in use as a card of its own: what it
// is and when it updates; how much of it is used, and, from its usage day
// by day, when it would run out; where its nodes are; the version before
// to go back to; and the other profiles, to switch to.
func (m *mainUI) profileSheet(c *ui.Context, w float32) {
	ps := &m.profs
	if ps.busy == nil {
		ps.busy = map[string]string{}
	}
	if len(m.pv.Items) == 0 {
		m.empty(c, locales.HomeNoProfile(), nil)
		return
	}
	var cur *profiles.Profile
	for i := range m.pv.Items {
		if m.pv.Items[i].UID == m.pv.Current {
			cur = &m.pv.Items[i]
		}
	}
	if cur == nil {
		cur = &m.pv.Items[0]
	}
	m.sheetSplit(c, w, func() {
		m.sheetSection(c, "card", 0, func() { m.profileFace(c, cur) })
		if u := cur.Usage; u != nil && u.Total > 0 {
			m.sheetSection(c, "usage", 1, func() { m.profileUsage(c, cur) })
		}
	}, func() {
		m.sheetSection(c, "regions", 2, func() { m.nodeRegions(c) })
		if cur.Prev != nil {
			m.sheetSection(c, "prev", 3, func() { m.profilePrev(c, cur) })
		}
		if len(m.pv.Items) > 1 {
			m.sheetSection(c, "others", 4, func() { m.otherProfiles(c, cur.UID) })
		}
	})
}

// profileFace is what a profile is, and when it updates.
func (m *mainUI) profileFace(c *ui.Context, pr *profiles.Profile) {
	ps, p, a := &m.profs, m.pal, m.a
	uid := pr.UID
	m.panel(c, "profile-card", func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, pr.Name).FontSize(24).FontWeight(700).SingleLine().Grow(1).Basis(0).MinWidth(0)
			if pr.Home != "" {
				if m.iconButton(c, "home", icExternalLink, locales.ProfilesHome()).Clicked() {
					go mygo.Shell.OpenExternal(pr.Home)
				}
			}
			if pr.Type == "remote" {
				busy := ps.busy[uid] != ""
				if m.spinButton(c, "update", icRefreshCw, locales.ProfilesUpdate(), busy).Clicked() && !busy {
					ps.busy[uid] = "update"
					m.run(locales.ProfilesUpdateFailed(), func() error {
						_, err := (Profiles{a}).Update(context.Background(), uid, nil)
						return err
					}, func(error) { delete(ps.busy, uid) })
				}
			}
		})
		kind := locales.ProfilesLocal()
		if pr.Type == "remote" {
			kind = locales.ProfilesRemote()
		}
		ui.Text(c, kind+" · "+locales.ProfilesNodes(fmt.Sprint(pr.Proxies))).FontSize(13).TextColor(p.TextMuted).SingleLine()
		ui.Row(c).Gap(24).Wrap().Margin(4, 0, 0).Children(func() {
			m.stat(c, "updated", locales.HomeUpdated(), m.relTime(time.Unix(pr.Updated, 0)))
			if next := profileNext(pr); !next.IsZero() {
				m.stat(c, "next", locales.HomeNextUpdate(), m.relTime(next))
			}
			if u := pr.Usage; u != nil && u.Expire > 0 {
				m.stat(c, "expire", locales.HomeExpire(), time.Unix(u.Expire, 0).Format(time.DateOnly))
			}
		})
		if pr.LastError != "" {
			ui.Row(c).Gap(8).Padding(8, 10).Radius(10).Background(p.DangerSoft).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icXCircle).FontSize(14).TextColor(p.Danger).Shrink(0)
				ui.Text(c, pr.LastError).FontSize(12.5).TextColor(p.Danger).SingleLine().Tooltip(pr.LastError)
			})
		}
	})
}

// profileNext is when a remote profile next updates by itself; zero for
// one that does not.
func profileNext(pr *profiles.Profile) time.Time {
	if pr.Type != "remote" || pr.Option.NoAutoUpdate || pr.Option.UpdateInterval < 0 || pr.Updated <= 0 {
		return time.Time{}
	}
	every := cmp.Or(pr.Option.UpdateInterval, pr.Suggested)
	if every <= 0 {
		return time.Time{}
	}
	return time.Unix(pr.Updated, 0).Add(time.Duration(every) * time.Minute)
}

// profileUsage is how much of a subscription is used: the share, large;
// days left; and its usage day by day as a line, with where it heads at
// the pace of its last days, and the day it ends.
func (m *mainUI) profileUsage(c *ui.Context, pr *profiles.Profile) {
	p, u := m.pal, pr.Usage
	used := u.Upload + u.Download
	share := float64(used) / float64(u.Total)
	marks := m.sheetHistory().ProfileUse[pr.UID]
	runOut := usageRunOut(marks, u.Total)
	m.panel(c, "usage-panel", func() {
		ui.Row(c).Gap(10).AlignItems(ui.End).Children(func() {
			num, unit, _ := strings.Cut(fmtBytes(used), " ")
			m.figure(c, num, unit, 36, ui.Color{})
			ui.Text(c, "/ "+fmtBytes(u.Total)).FontSize(14).TextColor(p.TextMuted).SingleLine().Margin(0, 0, 6)
			ui.Spacer(c)
			col := p.Text2
			if share > 0.9 {
				col = p.Danger
			}
			ui.Text(c, fmt.Sprintf("%.0f%%", share*100)).FontSize(18).FontWeight(650).TextColor(col).FontFeatures("tnum").Margin(0, 0, 4).ContentTransition(numRoll)
		})
		meterCol := p.Accent
		if share > 0.9 {
			meterCol = p.Danger
		}
		m.meter(c, "meter", share, meterCol)
		ui.Row(c).Gap(16).Wrap().Children(func() {
			if u.Expire > 0 {
				days := int(math.Ceil(time.Until(time.Unix(u.Expire, 0)).Hours() / 24))
				ui.Text(c, locales.HomeDaysLeft(fmt.Sprint(max(0, days)))).FontSize(12.5).TextColor(p.TextMuted).SingleLine()
			}
			if !runOut.IsZero() {
				col := p.TextMuted
				if u.Expire > 0 && runOut.Before(time.Unix(u.Expire, 0)) {
					col = p.Warning
				}
				ui.Text(c, locales.HomeRunsOut(runOut.Format("01-02"))).FontSize(12.5).TextColor(col).SingleLine()
			}
		})
		if len(marks) >= 2 {
			m.burnChart(c, marks, u.Total, u.Expire, runOut)
		} else {
			ui.Row(c).Height(90).Gap(6).AlignItems(ui.End).Children(func() {
				for i := range 14 {
					m.ghost(c, 0, float32(18+i*4))
				}
			})
		}
	})
}

// usageRunOut is when a subscription would be used up at the pace of its
// last two weeks; zero when that is not known, or it is not used.
func usageRunOut(marks []useMark, total int64) time.Time {
	if len(marks) < 2 {
		return time.Time{}
	}
	first := marks[max(0, len(marks)-15)]
	last := marks[len(marks)-1]
	t0, err0 := time.ParseInLocation(time.DateOnly, first.Day, time.Local)
	t1, err1 := time.ParseInLocation(time.DateOnly, last.Day, time.Local)
	days := t1.Sub(t0).Hours() / 24
	if err0 != nil || err1 != nil || days < 1 || last.Used <= first.Used {
		return time.Time{}
	}
	perDay := float64(last.Used-first.Used) / days
	left := float64(total - last.Used)
	if left <= 0 {
		return t1
	}
	return t1.Add(time.Duration(left / perDay * 24 * float64(time.Hour)))
}

// meter is a share of a whole as a bar.
func (m *mainUI) meter(c *ui.Context, key string, share float64, col ui.Color) ui.Element {
	p := m.pal
	bar := ui.Box(c.Key(key)).Height(8).Radius(4).Background(p.Surface3).Clip()
	k := bar.AnimateTo("k", float32(math.Max(0, math.Min(1, share))), ui.Animation{Duration: 600 * time.Millisecond, Ease: easeStd})
	return bar.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * k, H: r.H}, col, 4)
	})
}

// burnChart draws a subscription's usage by day, its whole as a line at
// the top, the day it ends as a line down, and where its usage heads,
// dashed, to where it runs out; today is the glint.
func (m *mainUI) burnChart(c *ui.Context, marks []useMark, total, expire int64, runOut time.Time) {
	p := m.pal
	day0, err := time.ParseInLocation(time.DateOnly, marks[0].Day, time.Local)
	if err != nil {
		return
	}
	at := func(s string) float64 {
		t, _ := time.ParseInLocation(time.DateOnly, s, time.Local)
		return t.Sub(day0).Hours() / 24
	}
	last := at(marks[len(marks)-1].Day)
	end := last + 1
	if expire > 0 {
		end = math.Max(end, time.Unix(expire, 0).Sub(day0).Hours()/24)
	}
	if !runOut.IsZero() {
		end = math.Max(end, runOut.Sub(day0).Hours()/24)
	}
	end = math.Min(end, last+120)
	box := ui.Box(c.Key("burn")).Height(110).Margin(4, 0, 0)
	in := box.Keyframes("in", []ui.Keyframe{{Value: 0}, {At: 800 * time.Millisecond, Value: 1, Ease: easeStd}}, ui.KeyframeOptions{Run: 1})
	box.Draw(func(pt *ui.Painter, r ui.Rect) {
		x := func(d float64) float32 { return r.X + r.W*float32(d/end) }
		y := func(v int64) float32 { return r.Y + 6 + (r.H-12)*(1-float32(float64(v)/float64(total))) }
		var top ui.Path
		top.MoveTo(r.X, y(total)).LineTo(r.X+r.W, y(total))
		pt.StrokePath(top.Dash(0, 4, 4), 1, p.Border)
		if expire > 0 {
			ex := x(time.Unix(expire, 0).Sub(day0).Hours() / 24)
			if ex <= r.X+r.W {
				var v ui.Path
				v.MoveTo(ex, r.Y).LineTo(ex, r.Y+r.H)
				pt.StrokePath(v.Dash(0, 3, 4), 1, p.TextMuted)
			}
		}
		pt.Clip(ui.Rect{X: r.X - 2, Y: r.Y - 6, W: (r.W + 8) * in, H: r.H + 12}, 0, func() {
			var line ui.Path
			for i, mk := range marks {
				px, py := x(at(mk.Day)), y(mk.Used)
				if i == 0 {
					line.MoveTo(px, py)
				} else {
					line.LineTo(px, py)
				}
			}
			pt.StrokePath(line.Join(ui.RoundJoin).Cap(ui.RoundCap), 2, p.Accent)
			lx, ly := x(last), y(marks[len(marks)-1].Used)
			if !runOut.IsZero() {
				ox := x(math.Min(end, runOut.Sub(day0).Hours()/24))
				var ahead ui.Path
				ahead.MoveTo(lx, ly).LineTo(ox, y(total))
				pt.StrokePath(ahead.Dash(0, 4, 4), 1.5, p.Accent.Alpha(0.6))
			}
			if in >= 1 {
				drawSparkle(pt, lx, ly, 5, p.Spark)
			}
		})
	})
}

// nodeRegions is where the nodes in use are: each country's flag and how
// many, the most first.
func (m *mainUI) nodeRegions(c *ui.Context) {
	p := m.pal
	count := map[string]int{}
	seen := map[string]bool{}
	for _, g := range m.home.view.Groups {
		for _, it := range g.All {
			if it.IsGroup || seen[it.Name] || it.Name == "DIRECT" || it.Name == "REJECT" {
				continue
			}
			seen[it.Name] = true
			count[flagOf(it.Name)]++
		}
	}
	if len(count) == 0 {
		return
	}
	ccs := slices.Collect(maps.Keys(count))
	slices.SortFunc(ccs, func(a, b string) int { return cmp.Or(count[b]-count[a], strings.Compare(a, b)) })
	m.panel(c, "regions-panel", func() {
		m.panelLabel(c, locales.HomeNodeRegions())
		most := count[ccs[0]]
		for _, cc := range ccs {
			ui.Row(c.Key("r-" + cc)).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Row(c).Width(132).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
					if cc != "" {
						m.flagSmall(c, cc)
						ui.Text(c, m.countryName(cc)).FontSize(13).SingleLine()
					} else {
						ui.Text(c, locales.ConnectivityCatOther()).FontSize(13).TextColor(p.TextMuted).SingleLine()
					}
				})
				m.meter(c, "bar", float64(count[cc])/float64(most), p.Accent.Alpha(0.75)).Grow(1).Basis(0)
				ui.Text(c, fmt.Sprint(count[cc])).FontSize(12.5).TextColor(p.TextMuted).FontFeatures("tnum").Width(28).TextAlign(ui.End).Shrink(0)
			})
		}
	})
}

// profilePrev is the version before the last update, to go back to.
func (m *mainUI) profilePrev(c *ui.Context, pr *profiles.Profile) {
	ps, p, a := &m.profs, m.pal, m.a
	uid, prev := pr.UID, pr.Prev
	when := time.Unix(prev.Updated, 0)
	m.panel(c, "prev-panel", func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icRotateCcw).FontSize(16).TextColor(p.TextMuted)
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
				ui.Text(c, locales.ProfilesRollback(when.Format("01-02 15:04"))).FontSize(13.5).FontWeight(600).SingleLine()
				d := prev.Proxies - pr.Proxies
				diff := fmt.Sprintf("%+d", d)
				ui.Text(c, locales.ProfilesNodes(fmt.Sprint(prev.Proxies))+"  ("+diff+")").FontSize(12.5).TextColor(p.TextMuted).SingleLine()
			})
			if ps.busy[uid] == "rollback" {
				ui.Spinner(c)
			} else if m.button(c, "rollback", locales.ProfilesRollbackOk(), nil, "").Clicked() {
				m.confirm(locales.ProfilesRollbackTitle(), locales.ProfilesRollbackMsg(fmt.Sprint(prev.Proxies)), locales.ProfilesRollbackOk(), func() {
					ps.busy[uid] = "rollback"
					m.run(locales.CommonFailed(), func() error {
						_, err := (Profiles{a}).Rollback(context.Background(), uid)
						return err
					}, func(error) { delete(ps.busy, uid) })
				})
			}
		})
	})
}

// otherProfiles are the profiles not in use, each with how much of it is
// used: a press switches to it.
func (m *mainUI) otherProfiles(c *ui.Context, current string) {
	ps, p, a := &m.profs, m.pal, m.a
	m.panelLabel(c, locales.HomeOtherProfiles())
	m.rows(c, "other-rows", func() {
		for i := range m.pv.Items {
			pr := &m.pv.Items[i]
			if pr.UID == current {
				continue
			}
			uid := pr.UID
			b := ui.ButtonBase(c.Key(uid)).Row().Padding(10, 14).Gap(12).AlignItems(ui.Center).Justify(ui.Start).Label(pr.Name)
			if b.Hovered() {
				b.Background(p.Surface3)
			}
			b.Transition(hoverFade)
			b.Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, pr.Name).FontSize(13.5).FontWeight(600).SingleLine()
					ui.Text(c, locales.ProfilesNodes(fmt.Sprint(pr.Proxies))).FontSize(12).TextColor(p.TextMuted).SingleLine()
				})
				if u := pr.Usage; u != nil && u.Total > 0 {
					ui.Box(c).Width(90).Shrink(0).Children(func() {
						m.meter(c, "use", float64(u.Upload+u.Download)/float64(u.Total), p.Accent.Alpha(0.75))
					})
				}
				if ps.busy[uid] != "" {
					ui.Spinner(c)
				}
			})
			if b.Clicked() && ps.busy[uid] == "" {
				ps.busy[uid] = "activate"
				m.run(locales.ProfilesActivateFailed(), func() error { return Profiles{a}.Activate(context.Background(), uid) }, func(error) { delete(ps.busy, uid) })
			}
		}
	})
}

// tailnetSheet is Tailscale, w wide: the way to sign in, as the page has
// it, where this device must; running, the tailnet as ripples round this
// device, each peer as far as its ping, the line to it whole where it is
// direct and dashed through a relay; this device, the exit node, and the
// peers.
func (m *mainUI) tailnetSheet(c *ui.Context, w float32) {
	s, st, mode := &m.tsPage, m.ts, m.st.Tailscale.Mode
	if s.pings == nil {
		s.pings, s.pinging = map[string]*coreapi.TailscalePingResult{}, map[string]bool{}
	}
	switch st.BackendState {
	case "NeedsLogin", "NeedsMachineAuth":
		m.sheetSection(c, "login", 0, func() { m.tsLogin(c, &st) })
		return
	case "Running":
	default:
		m.sheetSection(c, "card", 0, func() { m.tailscaleCard(c) })
		return
	}
	m.pingPeers(&st, mode)
	m.sheetSplit(c, w, func() {
		m.sheetSection(c, "ripple", 0, func() { m.panel(c, "ripple-panel", func() { m.tailnetRipple(c, &st) }) })
	}, func() {
		m.sheetSection(c, "self", 1, func() { m.tsSelf(c, &st, mode) })
		m.sheetSection(c, "exit", 2, func() { m.tsExitNode(c, &st, mode) })
	})
	m.sheetSection(c, "peers", 3, func() { m.tsPeers(c, &st, mode) })
}

// pingPeers pings the online peers, a few at a time, as the sheet opens
// and every minute while it shows: how far each is places it.
func (m *mainUI) pingPeers(st *coreapi.TailscaleStatus, mode string) {
	s, a := &m.tsPage, m.a
	if time.Since(s.pingedAt) < time.Minute || mode == "off" {
		return
	}
	s.pingedAt = time.Now()
	type job struct{ id, ip string }
	var jobs []job
	for _, pe := range st.Peers {
		if pe.Online && len(pe.TailscaleIPs) > 0 && !s.pinging[pe.ID] {
			jobs = append(jobs, job{pe.ID, pe.TailscaleIPs[0]})
			s.pinging[pe.ID] = true
		}
	}
	win := m.window()
	slots := make(chan struct{}, 4)
	for _, j := range jobs {
		go func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
			defer cancel()
			r, err := (Tailscale{a}).Ping(ctx, j.ip)
			if err != nil {
				r = coreapi.TailscalePingResult{Err: err.Error()}
			}
			win.Update(func() {
				delete(s.pinging, j.id)
				s.pings[j.id] = &r
			})
		}()
	}
}

// tailnetRipple draws the tailnet round this device: three rings, at 20, 60
// and 150 ms; each online peer as far out as its ping (on the outer ring
// until it answers), the line to it whole when direct, dashed when
// relayed; the exit node ringed in the accent.
func (m *mainUI) tailnetRipple(c *ui.Context, st *coreapi.TailscaleStatus) {
	s, p := &m.tsPage, m.pal
	type dot struct {
		name           string
		angle          float64 // where round it is, from its ID, so that it stays as others come and go
		r              float32 // 0 to 1 of the radius
		direct, exit   bool
		answered, fail bool
	}
	var dots []dot
	for _, pe := range st.Peers {
		if !pe.Online {
			continue
		}
		h := fnv.New32a()
		h.Write([]byte(pe.ID))
		d := dot{name: cmp.Or(pe.HostName, strings.Split(pe.DNSName, ".")[0]), angle: float64(h.Sum32()%3600) / 3600 * 2 * math.Pi, r: 0.97, direct: pe.CurAddr != "", exit: pe.ExitNode}
		if r := s.pings[pe.ID]; r != nil {
			d.answered = true
			if r.Err != "" {
				d.fail = true
			} else {
				d.direct = d.direct || r.Endpoint != ""
				// 20, 60 and 150 ms on the rings; between them, between.
				ms := r.LatencyMs
				switch {
				case ms <= 20:
					d.r = float32(0.32 * ms / 20)
				case ms <= 60:
					d.r = float32(0.32 + 0.3*(ms-20)/40)
				case ms <= 150:
					d.r = float32(0.62 + 0.3*(ms-60)/90)
				default:
					d.r = 0.97
				}
				d.r = max(d.r, 0.16)
			}
		}
		dots = append(dots, d)
	}
	ui.Column(c).AlignItems(ui.Center).Gap(10).Children(func() {
		box := ui.Box(c.Key("ripple")).Size(300, 300).Shrink(0)
		in := float32(1)
		if !c.Preferences().ReduceMotion {
			in = box.Keyframes("in", []ui.Keyframe{{Value: 0}, {At: 900 * time.Millisecond, Value: 1, Ease: easeStd}}, ui.KeyframeOptions{Run: 1})
		}
		box.Draw(func(pt *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			rad := r.W/2 - 18
			for _, k := range []float32{0.32, 0.62, 0.92} {
				pt.StrokePath((&ui.Path{}).Circle(cx, cy, rad*k*in).Dash(0, 2, 5), 1, p.Border)
			}
			for _, d := range dots {
				a := d.angle - math.Pi/2
				dr := rad * d.r * in
				x, y := cx+float32(math.Cos(a))*dr, cy+float32(math.Sin(a))*dr
				line := (&ui.Path{}).MoveTo(cx, cy).LineTo(x, y)
				col := p.Accent.Alpha(0.55)
				switch {
				case d.fail:
					col = p.Danger.Alpha(0.5)
				case !d.direct:
					line.Dash(0, 4, 4)
				}
				pt.StrokePath(line, 1.2, col)
				if d.exit {
					pt.FillPath((&ui.Path{}).Circle(x, y, 11), p.AccentSoft)
				}
				dc := p.Accent
				if !d.answered {
					dc = p.TextMuted
				} else if d.fail {
					dc = p.Danger
				}
				pt.FillPath((&ui.Path{}).Circle(x, y, 4.5), dc)
				if len(dots) <= 14 && in >= 1 {
					tx := x + 8
					if x < cx {
						tx = x - 8 - float32(len(d.name))*6.2
					}
					pt.Text(tx, y+4, d.name, 11, p.TextMuted)
				}
			}
			drawCompass(pt, cx, cy, 38, compassLook{})
		})
		ui.Row(c).Gap(14).Children(func() {
			for _, ms := range []string{"20", "60", "150"} {
				ui.Text(c, ms+" ms").FontSize(11).TextColor(p.Faint)
			}
		})
	})
}

// ipSheet is where the traffic comes out, as three sets of eyes see it:
// sites at home, sites abroad and Cloudflare, side by side, what one sees
// other than abroad marked in the accent's soft fill, so that a split
// shows itself; how the address abroad rates; and which services take it.
func (m *mainUI) ipSheet(c *ui.Context, running bool) {
	s := &m.cty
	if running && !s.ipsAsked && m.coreSettled() {
		s.ipsAsked = true
		m.loadExitIPs()
	}
	abroad := (*tools.IPInfo)(nil)
	if e := s.ips["global"]; e != nil {
		abroad = e.info
	}
	m.sheetSection(c, "ids", 0, func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			m.splitBadge(c)
			ui.Spacer(c)
			loading := slices.ContainsFunc(exitViews, func(v string) bool { return s.ips[v] != nil && s.ips[v].loading })
			if m.spinButton(c, "reload", icRefreshCw, locales.CommonRefresh(), loading).Disabled(!running).Clicked() {
				m.loadExitIPs()
			}
		})
		ui.Row(c).Gap(12).Wrap().AlignItems(ui.Stretch).Children(func() {
			for _, v := range exitViews {
				m.identityCard(c, v, abroad)
			}
		})
	})
	if abroad != nil {
		if q := m.ipQualityOf(abroad.IP); q != nil {
			m.sheetSection(c, "quality", 1, func() { m.riskPanel(c, q) })
		}
	}
	if len(s.unlocks) > 0 {
		m.sheetSection(c, "unlock", 2, func() { m.unlockStrip(c) })
	}
}

// identityCard is the exit as one set of eyes sees it: its flag and place,
// its address, who runs it; what differs from abroad in the soft fill.
func (m *mainUI) identityCard(c *ui.Context, v string, abroad *tools.IPInfo) {
	s, p := &m.cty, m.pal
	e := s.ips[v]
	var in *tools.IPInfo
	if e != nil {
		in = e.info
	}
	ui.Column(c.Key(v)).Grow(1).Basis(0).MinWidth(180).Padding(16, 18).Gap(8).Radius(16).Background(p.Surface2).Children(func() {
		m.panelLabel(c, m.T("connectivity.view."+v))
		switch {
		case e == nil || (e.loading && in == nil):
			m.skeleton(c, 22).Width(120)
			m.skeleton(c, 14).Width(150)
			m.skeleton(c, 14).Width(100)
			return
		case in == nil:
			ui.Text(c, cmp.Or(e.err, "—")).FontSize(12.5).TextColor(p.Faint).Wrap()
			return
		}
		diff := func(same bool) ui.Color {
			if v != "global" && abroad != nil && !same {
				return p.AccentSoft
			}
			return ui.Color{}
		}
		field := func(key string, bg ui.Color, fn func()) {
			r := ui.Row(c.Key(key)).Gap(8).AlignItems(ui.Center).Padding(3, 6).Margin(0, -6).Radius(7)
			if bg != (ui.Color{}) {
				r.Background(bg)
			}
			r.Children(fn)
		}
		field("place", diff(abroad != nil && in.CountryCode == abroad.CountryCode), func() {
			if in.CountryCode != "" {
				m.flagMasked(c, in.CountryCode, !m.reveal())
			}
			ui.Text(c, m.ctyMask(cmp.Or(strings.TrimSpace(in.City+" "+in.Country), "—"))).FontSize(16).FontWeight(650).SingleLine().Shrink(1).ContentTransition(maskRoll(m.reveal()))
		})
		field("ip", diff(abroad != nil && in.IP == abroad.IP), func() {
			ui.Text(c, m.ctyMask(in.IP)).FontSize(12.5).Font("ui-monospace").TextColor(p.Text2).SingleLine().Shrink(1).ContentTransition(maskRoll(m.reveal()))
		})
		field("isp", diff(abroad != nil && in.ISP == abroad.ISP), func() {
			ui.Text(c, m.ctyMask(cmp.Or(in.ISP, in.ASN, "—"))).FontSize(12.5).TextColor(p.TextMuted).SingleLine().Shrink(1).ContentTransition(maskRoll(m.reveal()))
		})
		if in.Colo != "" {
			field("colo", ui.Color{}, func() {
				ui.Text(c, locales.ConnectivityColo()+" "+in.Colo).FontSize(12).TextColor(p.Faint).SingleLine()
			})
		}
	})
}

// riskPanel is how the address abroad rates: a dial of its risk, what kind
// of line it is, and what it is taken for.
func (m *mainUI) riskPanel(c *ui.Context, q *tools.IPQuality) {
	p := m.pal
	m.panel(c, "risk-panel", func() {
		ui.Row(c).Gap(18).AlignItems(ui.Center).Children(func() {
			if q.Risk >= 0 {
				col := p.Success
				switch {
				case q.Risk >= 66:
					col = p.Danger
				case q.Risk >= 33:
					col = p.Warning
				}
				g := ui.Box(c.Key("gauge")).Size(120, 70).Shrink(0)
				k := g.AnimateTo("k", float32(q.Risk)/100, ui.Animation{Duration: 800 * time.Millisecond, Ease: easeStd})
				g.Draw(func(pt *ui.Painter, r ui.Rect) {
					cx, cy, rad := r.X+r.W/2, r.Y+r.H-6, r.W/2-6
					arc := func(to float64, col ui.Color) {
						var path ui.Path
						for i := 0; i <= 48; i++ {
							a := math.Pi + math.Pi*to*float64(i)/48
							x, y := cx+rad*float32(math.Cos(a)), cy+rad*float32(math.Sin(a))
							if i == 0 {
								path.MoveTo(x, y)
							} else {
								path.LineTo(x, y)
							}
						}
						pt.StrokePath(path.Cap(ui.RoundCap), 8, col)
					}
					arc(1, p.Surface3)
					if k > 0.005 {
						arc(float64(k), col)
					}
				})
				ui.Column(c).Gap(2).Children(func() {
					m.figure(c, fmt.Sprint(q.Risk), "/ 100", 32, col)
					ui.Text(c, locales.IpqQuality()).FontSize(12).TextColor(p.TextMuted)
				})
			}
			ui.Spacer(c)
			ui.Column(c).Gap(6).AlignItems(ui.End).Children(func() { m.ipQualityView(c, q, !m.reveal()) })
		})
	})
}

// unlockStrip is the services checked through this exit, each with its
// logo and name, where it took the exit for, and what it found.
func (m *mainUI) unlockStrip(c *ui.Context) {
	s, p := &m.cty, m.pal
	m.panel(c, "unlock-panel", func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			m.panelLabel(c, locales.ConnectivityUnlock())
			ui.Spacer(c)
			m.linkTo(c, "connectivity", "", locales.NavConnectivity())
		})
		m.cardGridOf(c, "unlock-grid", 190, len(s.unlocks), func(i int) any { return s.unlocks[i].ID }, func(i int) {
			u := s.unlocks[i]
			r, has := s.results[u.ID]
			state := "pending"
			if has {
				state = r.Status
			}
			if s.checking[u.ID] {
				state = "checking"
			}
			col, fg := p.BorderStrong, p.TextMuted
			switch state {
			case "yes":
				col, fg = p.Success, p.Success
			case "partial":
				col, fg = p.Warning, p.Warning
			case "no":
				col, fg = p.Danger, p.Danger
			}
			ui.Row(c).Grow(1).Padding(9, 12).Gap(10).Radius(12).Background(p.Surface3).AlignItems(ui.Center).Children(func() {
				logo := ui.Box(c).Size(22, 22).Shrink(0)
				if state == "no" || state == "failed" {
					logo.Opacity(0.45)
				}
				logo.Children(func() { m.logo(c, cmp.Or(u.Site, u.ID), u.Name, 22) })
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(1).Children(func() {
					ui.Text(c, m.unlockName(u)).FontSize(13).FontWeight(600).SingleLine()
					ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(6, 6).Radius(3).Background(col).Shrink(0)
						ui.Text(c, m.T("unlock."+state)).FontSize(11.5).TextColor(fg).SingleLine()
					})
				})
				if has && r.Region != "" && state != "checking" {
					reg := strings.ToUpper(r.Region)
					if two := iso2(reg); len(reg) == 3 && two != "" {
						reg = two
					}
					if len(reg) == 2 {
						m.flagMasked(c, reg, !m.reveal())
					}
				}
			})
		})
	})
}
