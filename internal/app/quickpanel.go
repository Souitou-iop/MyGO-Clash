package app

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
)

// quickPanel is a small window of native UI under the tray icon: the
// mode, the switches, the proxies of a group and the traffic, at a glance.
// It draws without a web view, so it opens at once and works in
// lightweight mode, when the main window is closed.
type quickPanel struct {
	a *App

	mu   sync.Mutex
	win  *mygo.Window
	stop context.CancelFunc

	// What the view shows, changed on the main thread (Window.Update).
	st       config.Settings
	state    AppState
	ts       coreapi.TailscaleStatus
	groups   []ProxyGroup
	group    string
	traffic  coreapi.Traffic
	testing  bool
	selected int // the mode's segment
	sysOn    bool
	tunOn    bool

	history  []int64 // the download speeds of the last seconds, for the graph
	query    string  // filters the proxies of the group
	profile  string  // the name of the current profile
	profiles ProfilesView
	height   int       // the content's height, fitted to what shows
	hiddenAt time.Time // when the panel hid, to tell a click on the icon that closed it
}

const (
	panelW        = 340
	panelMinH     = 330
	panelMaxH     = 640
	panelRowH     = 32
	panelMaxRows  = 9
	panelSearchAt = 8 // members from which a search field shows
	historyLen    = 30
)

func newQuickPanel(a *App) *quickPanel { return &quickPanel{a: a} }

func (p *quickPanel) window() *mygo.Window {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.win != nil && !p.win.IsDestroyed() {
		return p.win
	}
	return nil
}

// toggle shows the panel, or hides it.
func (p *quickPanel) toggle() {
	if w := p.window(); w != nil && w.IsVisible() {
		p.hide()
		return
	}
	// A click on the icon blurs the open panel, which hides it, before the
	// click itself comes: that one must not open it again.
	p.mu.Lock()
	justHid := time.Since(p.hiddenAt) < 300*time.Millisecond
	p.mu.Unlock()
	if justHid {
		return
	}
	p.show()
}

func (p *quickPanel) show() {
	win := p.window()
	if win == nil {
		opts := mygo.WindowOptions{
			Title: p.a.name, Width: panelW, Height: p.contentHeight(), Hidden: true,
			Frameless: true, AlwaysOnTop: true, SkipTaskbar: true,
			DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
			Content: ui.View(p.view),
		}
		if runtime.GOOS == "darwin" {
			opts.Transparent, opts.Vibrancy = true, mygo.VibrancyPopover
		}
		win = mygo.NewWindow(opts)
		win.OnBlur(p.hide)
		p.mu.Lock()
		p.win = win
		p.mu.Unlock()
	}
	p.load(win)
	p.place(win)
	win.Show()
	win.Focus()
	p.live(win)
}

func (p *quickPanel) hide() {
	p.mu.Lock()
	if p.stop != nil {
		p.stop()
		p.stop = nil
	}
	p.hiddenAt = time.Now()
	win := p.win
	p.mu.Unlock()
	if win != nil && !win.IsDestroyed() {
		win.Hide()
	}
}

// place puts the panel under the tray icon (above it, when the taskbar is
// at the bottom), on the screen.
func (p *quickPanel) place(win *mygo.Window) {
	var x, y int
	placed := false
	if p.a.tray != nil && p.a.tray.tray != nil {
		if b := p.a.tray.tray.Bounds(); b.Width > 0 {
			x = b.X + b.Width/2 - panelW/2
			d := mygo.Screen.DisplayNearestPoint(mygo.Point{X: b.X, Y: b.Y})
			if b.Y < d.Bounds.Y+d.Bounds.Height/2 {
				y = b.Y + b.Height + 6
			} else {
				y = b.Y - p.contentHeight() - 6
			}
			placed = true
		}
	}
	d := mygo.Screen.PrimaryDisplay()
	if placed {
		d = mygo.Screen.DisplayNearestPoint(mygo.Point{X: x, Y: y})
	}
	wa := d.WorkArea
	if !placed {
		x, y = wa.X+wa.Width-panelW-12, wa.Y+12
		if runtime.GOOS == "windows" {
			y = wa.Y + wa.Height - p.contentHeight() - 12
		}
	}
	x = max(wa.X+8, min(x, wa.X+wa.Width-panelW-8))
	y = max(wa.Y+8, min(y, wa.Y+wa.Height-p.contentHeight()-8))
	win.SetPosition(x, y)
}

// load reads the app's state into the panel's.
func (p *quickPanel) load(win *mygo.Window) {
	st, state, ts, pv := p.a.settings.Get(), p.a.snapshot(), p.a.ts.current(), p.a.profilesView()
	win.Update(func() {
		p.st, p.state, p.ts = st, state, ts
		p.selected = slices.Index([]string{"rule", "global", "direct"}, st.Clash.Mode)
		p.sysOn, p.tunOn = st.SystemProxy.Enabled, st.Tun.Enabled && state.TunAvailable
		p.profiles, p.profile = pv, state.ProfileName
		p.fit(win)
	})
}

// invalidate redraws the panel with the app's state, when it shows.
func (p *quickPanel) invalidate() {
	if p == nil {
		return
	}
	if w := p.window(); w != nil && w.IsVisible() {
		p.load(w)
	}
}

// live follows the traffic and the groups while the panel shows.
func (p *quickPanel) live(win *mygo.Window) {
	p.mu.Lock()
	if p.stop != nil {
		p.stop()
	}
	ctx, cancel := context.WithCancel(p.a.ctx)
	p.stop = cancel
	p.mu.Unlock()
	go func() {
		ch, unsub := p.a.traffic.Subscribe()
		defer unsub()
		for {
			select {
			case <-ctx.Done():
				return
			case t, ok := <-ch:
				if !ok {
					return
				}
				win.Update(func() {
					p.traffic = t
					p.history = append(p.history, t.Down)
					if len(p.history) > historyLen {
						p.history = p.history[len(p.history)-historyLen:]
					}
				})
			}
		}
	}()
	go func() {
		for {
			p.refreshGroups(ctx, win)
			select {
			case <-ctx.Done():
				return
			case <-time.After(3 * time.Second):
			}
		}
	}()
}

func (p *quickPanel) refreshGroups(ctx context.Context, win *mygo.Window) {
	if p.a.core.Client() == nil {
		return
	}
	rctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	v, err := Proxies{p.a}.View(rctx)
	if err != nil {
		return
	}
	var groups []ProxyGroup
	if v.Mode == "global" && v.Global != nil {
		groups = append(groups, *v.Global)
	}
	for _, g := range v.Groups {
		if !g.Hidden {
			groups = append(groups, g)
		}
	}
	win.Update(func() {
		p.groups = groups
		if !slices.ContainsFunc(groups, func(g ProxyGroup) bool { return g.Name == p.group }) {
			p.group = ""
			for _, g := range groups {
				if g.Type == "Selector" {
					p.group = g.Name
					break
				}
			}
		}
		p.fit(win)
	})
}

// delayColor colors a delay as the web UI does: untested (-1) muted,
// failed (0) as danger.
func delayColor(t *ui.Theme, d int) ui.Color {
	switch delayKindOf(d) {
	case delayGood:
		return t.Success
	case delayOK:
		return t.Warning
	case delayBad:
		return t.Danger
	}
	return t.TextMuted
}

// panelThemes caches panelTheme's light, dark and pure black themes.
var panelThemes [3]*ui.Theme

// panelTheme is mygo/ui's theme in the app's colors, as the web UI has
// them: MyGO's night blues, Tomori's blue for the accent, the members'
// hues for the states. oled turns the dark one pure black.
func panelTheme(dark, oled bool) *ui.Theme {
	i := 0
	if dark {
		i = 1
		if oled {
			i = 2
		}
	}
	if panelThemes[i] != nil {
		return panelThemes[i]
	}
	var t *ui.Theme
	if dark {
		t = ui.DarkTheme()
		t.Background, t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#1c1c1f"), ui.Hex("#26262b"), ui.Hex("#303036"), ui.Hex("#3a3a41")
		t.Border, t.Text, t.TextMuted = ui.Hex("#36363d"), ui.Hex("#ececef"), ui.Hex("#8e8e97")
		t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = ui.Hex("#46acdb"), ui.Hex("#5db8e2"), ui.Hex("#3698c6"), ui.Hex("#04131b")
		t.Danger, t.Warning, t.Success = ui.Hex("#ff6b61"), ui.Hex("#f0c766"), ui.Hex("#5cc985")
		t.Selection, t.Focus = ui.RGBA(70, 172, 219, 0.22), ui.RGBA(70, 172, 219, 0.55)
		if oled {
			t.Background, t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#000000"), ui.Hex("#101012"), ui.Hex("#19191c"), ui.Hex("#222226")
			t.Border = ui.Hex("#232327")
		}
	} else {
		t = ui.LightTheme()
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#f1f1f3"), ui.Hex("#e8e8eb"), ui.Hex("#dddde1")
		t.Border, t.Text, t.TextMuted = ui.Hex("#dcdce1"), ui.Hex("#18181b"), ui.Hex("#6c6c75")
		t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = ui.Hex("#0b88bb"), ui.Hex("#0a79a7"), ui.Hex("#086a93"), ui.Hex("#ffffff")
		t.Danger, t.Warning, t.Success = ui.Hex("#d1333f"), ui.Hex("#b26b00"), ui.Hex("#1f9254")
		t.Selection, t.Focus = ui.RGBA(11, 136, 187, 0.16), ui.RGBA(11, 136, 187, 0.5)
	}
	t.Radius = 7
	panelThemes[i] = t
	return t
}

// current is the group the list shows, nil when there is none.
func (p *quickPanel) current() *ProxyGroup {
	for i := range p.groups {
		if p.groups[i].Name == p.group {
			return &p.groups[i]
		}
	}
	return nil
}

// members are the proxies of g that the search keeps.
func (p *quickPanel) members(g *ProxyGroup) []ProxyItem {
	if g == nil || p.query == "" {
		if g == nil {
			return nil
		}
		return g.All
	}
	q := strings.ToLower(p.query)
	var out []ProxyItem
	for _, m := range g.All {
		if strings.Contains(strings.ToLower(m.Name), q) {
			out = append(out, m)
		}
	}
	return out
}

// contentHeight is the height the panel has, or will have.
func (p *quickPanel) contentHeight() int {
	if p.height == 0 {
		return 520
	}
	return p.height
}

// fit sizes the panel to what it shows, as a popover is: the list is as
// long as the group, up to a limit. On the main thread.
func (p *quickPanel) fit(win *mygo.Window) {
	g := p.current()
	rows := min(max(len(p.members(g)), 3), panelMaxRows)
	h := 281 + rows*panelRowH
	if g != nil && len(g.All) > panelSearchAt {
		h += 40
	}
	if p.ts.Source == "embedded" || p.ts.Source == "system" {
		h += 18
	}
	h = min(max(h, panelMinH), panelMaxH)
	if h == p.height {
		return
	}
	p.height = h
	win.SetContentSize(panelW, h)
	if win.IsVisible() {
		p.place(win)
	}
}

// view builds the panel.
func (p *quickPanel) view(c *ui.Context) {
	a := p.a
	c.SetTheme(panelTheme(c.Theme().Dark, a.settings.Get().OLED))
	t := c.Theme()
	if runtime.GOOS == "darwin" {
		c.Root().Background(ui.Transparent)
	}
	if c.Shortcut(0, ui.KeyEscape) {
		p.hide()
	}

	// iconButton is a small button with an icon, for the header and the
	// footer.
	iconButton := func(key string, icon *ui.SVG, tip string) bool {
		b := ui.ButtonBase(c.Key(key)).Size(28, 28).Radius(7).AlignItems(ui.Center).Justify(ui.Center).Label(tip)
		if b.Hovered() {
			b.Background(t.SurfaceHover)
		}
		b.Children(func() { ui.Icon(c, icon).FontSize(15).TextColor(t.TextMuted) })
		b.Tooltip(tip)
		return b.Clicked()
	}
	// tile is a switch drawn as a tile, in the accent color while on.
	tile := func(key string, icon *ui.SVG, label, sub string, on bool) bool {
		b := ui.ButtonBase(c.Key(key)).Row().Grow(1).Gap(9).Padding(9, 11).Radius(10).AlignItems(ui.Center)
		bg, fg, muted := t.Surface, t.Text, t.TextMuted
		switch {
		case on && b.Hovered():
			bg, fg, muted = t.AccentHover, t.AccentText, t.AccentText.Alpha(0.78)
		case on:
			bg, fg, muted = t.Accent, t.AccentText, t.AccentText.Alpha(0.78)
		case b.Hovered():
			bg = t.SurfaceHover
		}
		b.Background(bg)
		b.Children(func() {
			ui.Icon(c, icon).FontSize(17).TextColor(fg)
			ui.Column(c).Grow(1).Gap(1).Children(func() {
				ui.Text(c, label).FontSize(12.5).Bold().TextColor(fg).SingleLine()
				ui.Text(c, sub).FontSize(11).TextColor(muted).SingleLine()
			})
		})
		return b.Clicked()
	}

	onOff := func(on bool) string {
		if on {
			return tr(a, "on")
		}
		return tr(a, "off")
	}

	ui.Column(c).Fill().Padding(12).Gap(10).Children(func() {
		// Header: the core's state, and the way to the window and the settings.
		ui.Row(c).Gap(8).AlignItems(ui.Center).DragWindow().Children(func() {
			dot := t.TextMuted
			switch p.state.Core.Status {
			case coremgr.StatusRunning:
				dot = t.Success
			case coremgr.StatusError:
				dot = t.Danger
			case coremgr.StatusStarting:
				dot = t.Warning
			}
			ui.Box(c).Size(8, 8).Radius(4).Background(dot)
			ui.Text(c, a.name).Bold().FontSize(14).SingleLine()
			ui.Text(c, statusLabel(a, p.state.Core)).TextColor(t.TextMuted).FontSize(12).SingleLine()
			ui.Spacer(c)
			if iconButton("settings", iconSettings, tr(a, "settings")) {
				p.hide()
				a.navigate("settings")
			}
			if iconButton("dashboard", iconWindow, tr(a, "dashboard")) {
				p.hide()
				a.showMain()
			}
		})

		// Speeds, and the last seconds of the download.
		ui.Row(c).Gap(6).AlignItems(ui.Center).Padding(0, 2).Children(func() {
			ui.Icon(c, iconUp).FontSize(13).TextColor(t.Accent)
			ui.Text(c, shortRate(p.traffic.Up)+"/s").FontSize(13).Bold().FontFeatures("tnum").Width(64)
			ui.Icon(c, iconDown).FontSize(13).TextColor(t.Success)
			ui.Text(c, shortRate(p.traffic.Down)+"/s").FontSize(13).Bold().FontFeatures("tnum").Width(64)
			ui.Spacer(c)
			var peak int64 = 1
			for _, v := range p.history {
				peak = max(peak, v)
			}
			ui.Row(c).Gap(1.5).AlignItems(ui.End).Height(20).Children(func() {
				for i := 0; i < historyLen; i++ {
					var v int64
					if j := i - (historyLen - len(p.history)); j >= 0 {
						v = p.history[j]
					}
					h := float32(2)
					if v > 0 {
						h = max(2, 20*float32(v)/float32(peak))
					}
					ui.Box(c.Key(i)).Size(2.5, h).Radius(1).Background(t.Success.Alpha(0.35 + 0.65*float32(i)/historyLen))
				}
			})
		})

		// Mode: three segments of one width, across the panel.
		ui.Row(c).Gap(2).Padding(2).Radius(9).Background(t.Surface).Label(tr(a, "mode")).Children(func() {
			for i, m := range []string{"rule", "global", "direct"} {
				seg := ui.ButtonBase(c.Key(m)).Grow(1).Basis(0).Height(28).Radius(7).AlignItems(ui.Center).Justify(ui.Center)
				switch {
				case p.selected == i:
					seg.Background(t.Background).Border(1, t.Border)
				case seg.Hovered():
					seg.Background(t.SurfaceHover)
				}
				seg.Children(func() {
					txt := ui.Text(c, tr(a, m)).FontSize(13).SingleLine()
					if p.selected == i {
						txt.Bold()
					} else {
						txt.TextColor(t.TextMuted)
					}
				})
				if seg.Clicked() && p.selected != i {
					p.selected = i
					go a.setMode("quick panel", m)
				}
			}
		})

		// Switches.
		ui.Row(c).Gap(8).Children(func() {
			if tile("sys", iconGlobe, tr(a, "systemProxy"), onOff(p.sysOn), p.sysOn) {
				go a.toggleSystemProxy("quick panel")
			}
			tunSub := onOff(p.tunOn)
			if !p.state.TunAvailable {
				tunSub = tunLabel(a, p.state)
			}
			if tile("tun", iconShield, tr(a, "tun"), tunSub, p.tunOn) {
				if !p.tunOn && !p.state.TunAvailable {
					go a.installServiceForTun() // on once the service is in
				} else {
					go a.toggleTun("quick panel")
				}
			}
		})

		// The proxies of a group.
		var names []string
		for _, g := range p.groups {
			names = append(names, g.Name)
		}
		cur := p.current()
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if len(names) > 0 {
				if ui.Select(c, &p.group, names).Grow(1).Label(tr(a, "group")).Changed() {
					p.query = ""
					p.fit(p.window())
				}
			} else {
				ui.Text(c, tr(a, "notRunning")).TextColor(t.TextMuted).Grow(1)
			}
			if cur != nil {
				testing := p.testing
				b := ui.ButtonBase(c.Key("test")).Size(32, 32).Radius(7).AlignItems(ui.Center).Justify(ui.Center).Disabled(testing).Label(tr(a, "testDelay"))
				b.Background(t.Surface)
				if b.Hovered() && !testing {
					b.Background(t.SurfaceHover)
				}
				b.Children(func() {
					if testing {
						ui.Spinner(c)
					} else {
						ui.Icon(c, iconZap).FontSize(15).TextColor(t.Text)
					}
				})
				b.Tooltip(tr(a, "testDelay"))
				if b.Clicked() {
					p.testing = true
					group, testURL := cur.Name, cur.TestURL
					win := p.window()
					go func() {
						a.testGroup(group, testURL)
						if win != nil {
							ctx, cancel := context.WithTimeout(a.ctx, 10*time.Second)
							defer cancel()
							p.refreshGroups(ctx, win)
							win.Update(func() { p.testing = false })
						}
					}()
				}
			}
		})
		if cur != nil && len(cur.All) > panelSearchAt {
			if ui.SearchField(c, &p.query).Changed() {
				p.fit(p.window())
			}
		}
		ui.Scroll(c).Grow(1).Radius(10).Background(t.Surface).Padding(4, 0).Children(func() {
			if cur == nil {
				return
			}
			selectable := cur.Type == "Selector"
			for _, m := range p.members(cur) {
				member := m
				row := ui.ButtonBase(c.Key(m.Name)).Row().Gap(8).Padding(0, 10).Height(panelRowH).AlignItems(ui.Center).Disabled(!selectable)
				if m.Name == cur.Now {
					row.Background(t.Selection)
				} else if row.Hovered() && selectable {
					row.Background(t.SurfaceHover)
				}
				row.Tooltip(member.Type)
				row.Children(func() {
					box := ui.Box(c).Size(14, 14)
					if member.Name == cur.Now {
						ui.Icon(c, iconCheck).FontSize(14).TextColor(t.Accent)
					} else {
						box.Size(14, 14)
					}
					ui.Text(c, member.Name).SingleLine().FontSize(13).Grow(1)
					delay := "—"
					if member.Delay > 0 {
						delay = fmt.Sprintf("%d ms", member.Delay)
					} else if member.Delay == 0 {
						delay = tr(a, "timeout")
					}
					ui.Text(c, delay).FontSize(12).FontFeatures("tnum").TextColor(delayColor(t, member.Delay))
				})
				if row.Clicked() && selectable && member.Name != cur.Now {
					group := cur.Name
					cur.Now = member.Name // at once; the core confirms
					go func() { a.notifyErr("proxies", group, a.selectProxy(context.Background(), group, member.Name)) }()
				}
			}
		})

		// Footer: the tailnet, the profile, and the way out.
		ui.Column(c).Gap(6).Children(func() {
			if p.ts.Source == "embedded" || p.ts.Source == "system" {
				line := tr(a, "tailscale") + ": " + tr(a, "disconnected")
				if p.ts.BackendState == "Running" {
					line = tr(a, "tailscale") + ": " + tr(a, "connected")
					if p.ts.Self != nil && len(p.ts.Self.TailscaleIPs) > 0 {
						line += " · " + p.ts.Self.TailscaleIPs[0]
					}
				} else if p.ts.BackendState == "NeedsLogin" {
					line = tr(a, "tailscale") + ": " + tr(a, "tsNeedsLogin")
				}
				ui.Text(c, line).FontSize(12).TextColor(t.TextMuted).SingleLine()
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				var pnames []string
				uids := map[string]string{}
				for _, it := range p.profiles.Items {
					pnames = append(pnames, it.Name)
					uids[it.Name] = it.UID
				}
				if len(pnames) > 0 {
					if ui.Select(c, &p.profile, pnames).Grow(1).Label(tr(a, "profile")).Changed() {
						uid := uids[p.profile]
						go func() {
							if err := (Profiles{a}).Activate(context.Background(), uid); err != nil {
								a.notifyErr("profiles", tr(a, "applyFailed"), err)
							}
						}()
					}
				} else {
					ui.Text(c, tr(a, "noProfile")).FontSize(12).TextColor(t.TextMuted).Grow(1)
				}
				if iconButton("quit", iconPower, tr(a, "quit")) {
					go a.quit()
				}
			})
		})
	})
}
