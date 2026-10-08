package app

import (
	"context"
	"fmt"
	"runtime"
	"slices"
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
}

const panelW, panelH = 340, 580

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
	p.show()
}

func (p *quickPanel) show() {
	win := p.window()
	if win == nil {
		opts := mygo.WindowOptions{
			Title: p.a.name, Width: panelW, Height: panelH, Hidden: true,
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
				y = b.Y - panelH - 6
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
			y = wa.Y + wa.Height - panelH - 12
		}
	}
	x = max(wa.X+8, min(x, wa.X+wa.Width-panelW-8))
	y = max(wa.Y+8, min(y, wa.Y+wa.Height-panelH-8))
	win.SetPosition(x, y)
}

// load reads the app's state into the panel's.
func (p *quickPanel) load(win *mygo.Window) {
	st, state, ts := p.a.settings.Get(), p.a.snapshot(), p.a.ts.current()
	win.Update(func() {
		p.st, p.state, p.ts = st, state, ts
		p.selected = slices.Index([]string{"rule", "global", "direct"}, st.Clash.Mode)
		p.sysOn, p.tunOn = st.SystemProxy.Enabled, st.Tun.Enabled && state.TunAvailable
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
				win.Update(func() { p.traffic = t })
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

// view builds the panel.
func (p *quickPanel) view(c *ui.Context) {
	a := p.a
	c.SetTheme(panelTheme(c.Theme().Dark, a.settings.Get().OLED))
	t := c.Theme()
	if runtime.GOOS == "darwin" {
		c.Root().Background(ui.Transparent)
	}
	ui.Column(c).Fill().Padding(14).Gap(12).Children(func() {
		// Header: status and speeds.
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
			ui.Text(c, a.name).Bold().FontSize(14)
			ui.Text(c, statusLabel(a, p.state.Core)).TextColor(t.TextMuted).FontSize(12)
			ui.Spacer(c)
			if ui.Button(c, tr(a, "dashboard")).Padding(3, 10).FontSize(12).Clicked() {
				p.hide()
				a.showMain()
			}
		})
		ui.Row(c).Gap(10).Children(func() {
			for _, s := range []struct {
				label string
				v     int64
			}{{tr(a, "upload"), p.traffic.Up}, {tr(a, "download"), p.traffic.Down}} {
				ui.Column(c).Grow(1).Padding(8, 10).Radius(8).Background(t.Surface).Children(func() {
					ui.Text(c, s.label).FontSize(11).TextColor(t.TextMuted)
					ui.Text(c, shortRate(s.v)+"/s").FontSize(18).Bold().FontFeatures("tnum")
				})
			}
		})

		// Mode and switches.
		if ui.Segmented(c, &p.selected, tr(a, "rule"), tr(a, "global"), tr(a, "direct")).Label(tr(a, "mode")).Changed() {
			mode := []string{"rule", "global", "direct"}[p.selected]
			go a.setMode("quick panel", mode)
		}
		ui.Column(c).Gap(8).Padding(8, 10).Radius(8).Background(t.Surface).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tr(a, "systemProxy")).Grow(1)
				if ui.Switch(c, &p.sysOn).Label(tr(a, "systemProxy")).Changed() {
					on := p.sysOn
					go func() {
						_, err := a.updateSettings(from(context.Background(), "quick panel"), func(s *config.Settings) { s.SystemProxy.Enabled = on })
						a.notifyErr("settings/network", tr(a, "sysproxyFailed"), err)
					}()
				}
			})
			ui.Divider(c)
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tunLabel(a, p.state)).Grow(1)
				sw := ui.Switch(c, &p.tunOn).Label(tr(a, "tun"))
				if sw.Changed() {
					on := p.tunOn
					if on && !p.state.TunAvailable {
						p.tunOn = false // on once the service is in
						go a.installServiceForTun()
					} else {
						go func() {
							_, _ = a.updateSettings(from(context.Background(), "quick panel"), func(s *config.Settings) { s.Tun.Enabled = on })
						}()
					}
				}
			})
		})

		// The proxies of a group.
		var names []string
		for _, g := range p.groups {
			names = append(names, g.Name)
		}
		var cur *ProxyGroup
		for i := range p.groups {
			if p.groups[i].Name == p.group {
				cur = &p.groups[i]
			}
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if len(names) > 0 {
				ui.Select(c, &p.group, names).Grow(1).Label(tr(a, "group"))
			} else {
				ui.Text(c, tr(a, "notRunning")).TextColor(t.TextMuted).Grow(1)
			}
			if cur != nil {
				label := tr(a, "testDelay")
				if p.testing {
					label = "…"
				}
				if ui.Button(c, label).Disabled(p.testing).Clicked() {
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
		ui.Scroll(c).Grow(1).Radius(8).Background(t.Surface).Children(func() {
			if cur == nil {
				return
			}
			selectable := cur.Type == "Selector"
			for _, m := range cur.All {
				member := m
				row := ui.ButtonBase(c).Key(m.Name).Row().Gap(8).Padding(7, 10).AlignItems(ui.Center).Disabled(!selectable)
				if m.Name == cur.Now {
					row.Background(t.Selection)
				} else if row.Hovered() && selectable {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					ui.Column(c).Grow(1).Gap(1).Children(func() {
						ui.Text(c, member.Name).SingleLine().FontSize(13)
						ui.Text(c, member.Type).FontSize(11).TextColor(t.TextMuted)
					})
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

		// Footer: the tailnet and the profile.
		ui.Column(c).Gap(4).Children(func() {
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
			profile := p.state.ProfileName
			if profile == "" {
				profile = tr(a, "noProfile")
			}
			ui.Row(c).AlignItems(ui.Center).Children(func() {
				ui.Text(c, tr(a, "profile")+": "+profile).FontSize(12).TextColor(t.TextMuted).SingleLine().Grow(1)
				if ui.Button(c, tr(a, "quit")).Padding(3, 10).FontSize(12).Clicked() {
					go a.quit()
				}
			})
		})
	})
}
