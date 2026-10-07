package app

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"

	"github.com/mygo-clash/mygo-clash/internal/art"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
)

// trayUI is the tray icon and its menu.
type trayUI struct {
	a    *App
	tray *mygo.Tray

	mu        sync.Mutex
	variant   art.Variant
	menuKey   string // the state the menu was built from
	menu      *mygo.Menu
	timer     *time.Timer
	speedStop func()
	lastClick time.Time
}

var template = runtime.GOOS == "darwin"

func newTrayUI(a *App) *trayUI {
	t := &trayUI{a: a, variant: -1}
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           art.Tray(trayPx(), art.Off, template),
		IconIsTemplate: template,
		ToolTip:        a.name,
	})
	if err != nil {
		log.Printf("tray: %v", err) // e.g. Linux without AppIndicator
		return t
	}
	t.tray = tray
	tray.OnClick(func() {
		// Windows: a double click opens the main window, whatever a
		// single click does; the first click has already done that.
		if t.doubleClick() {
			a.showMain()
			return
		}
		switch a.settings.Get().TrayClick {
		case "window":
			a.toggleMain()
		case "panel":
			a.panel.toggle()
		}
	})
	tray.OnRightClick(func() {
		if a.settings.Get().TrayClick != "menu" {
			t.popup()
		}
	})
	t.rebuild()
	t.speed(a.settings.Get().Tray.ShowSpeed)
	return t
}

// doubleClick reports whether this click is the second of a double click
// (Windows only).
func (t *trayUI) doubleClick() bool {
	d := doubleClickTime()
	if d == 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if now.Sub(t.lastClick) <= d {
		t.lastClick = time.Time{}
		return true
	}
	t.lastClick = now
	return false
}

func trayPx() int {
	if runtime.GOOS == "windows" {
		return 32
	}
	return 36
}

// popup shows the menu of a tray whose clicks do something else.
func (t *trayUI) popup() {
	t.mu.Lock()
	m := t.menu
	t.mu.Unlock()
	if t.tray == nil || m == nil {
		return
	}
	t.tray.SetMenu(m)
	t.tray.PopUpMenu()
	t.tray.SetMenu(nil)
}

// refresh updates the icon and the tooltip from the state.
func (t *trayUI) refresh() {
	if t == nil || t.tray == nil {
		return
	}
	s := t.a.snapshot()
	v := art.Off
	switch {
	case s.Tun:
		v = art.Tun
	case s.SystemProxy:
		v = art.SystemProxy
	}
	key := menuKey(s)
	t.mu.Lock()
	changed := v != t.variant
	t.variant = v
	stale := key != t.menuKey
	t.mu.Unlock()
	if stale {
		t.rebuild()
	}
	if changed {
		_ = t.tray.SetIcon(art.Tray(trayPx(), v, template), template)
	}
	tip := fmt.Sprintf("%s · %s", t.a.name, statusLabel(t.a, s.Core))
	if s.ProfileName != "" {
		tip += " · " + s.ProfileName
	}
	t.tray.SetToolTip(tip)
}

// menuKey is the part of the state the menu shows, outside the settings.
func menuKey(s AppState) string {
	return fmt.Sprint(s.Core.Status, s.TunAvailable, s.Service.Installed, s.Service.Outdated, s.ProfileName)
}

func statusLabel(a *App, c coremgr.State) string {
	switch c.Status {
	case coremgr.StatusRunning:
		return tr(a, "running")
	case coremgr.StatusStarting:
		return tr(a, "starting")
	case coremgr.StatusError:
		return tr(a, "error")
	}
	return tr(a, "stopped")
}

// rebuild builds the menu again, soon: changes come in bursts.
func (t *trayUI) rebuild() {
	if t == nil || t.tray == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.timer != nil {
		t.timer.Stop()
	}
	t.timer = time.AfterFunc(250*time.Millisecond, t.build)
}

func (t *trayUI) build() {
	a := t.a
	st := a.settings.Get()
	s := a.snapshot()
	t.mu.Lock()
	t.menuKey = menuKey(s)
	t.mu.Unlock()
	item := func(key string, fn func()) *mygo.MenuItem {
		return &mygo.MenuItem{Label: tr(a, key), Click: func(*mygo.MenuItem, *mygo.Window) { fn() }}
	}
	var items []*mygo.MenuItem
	head := a.name + " — " + statusLabel(a, s.Core)
	items = append(items, &mygo.MenuItem{Label: head, Disabled: true})
	items = append(items, item("dashboard", a.showMain), item("quickPanel", func() { a.panel.toggle() }), mygo.Separator())

	modes := []*mygo.MenuItem{}
	for _, m := range []string{"rule", "global", "direct"} {
		mode := m
		modes = append(modes, &mygo.MenuItem{Label: tr(a, m), Type: mygo.MenuItemRadio, Checked: st.Clash.Mode == m,
			Click: func(*mygo.MenuItem, *mygo.Window) { go a.setMode(mode) }})
	}
	if st.Tray.InlineModes {
		items = append(items, modes...)
	} else {
		items = append(items, &mygo.MenuItem{Label: tr(a, "mode") + ": " + tr(a, st.Clash.Mode), Submenu: modes})
	}

	if st.Tray.Groups != "off" {
		if groups := t.groups(); len(groups) > 0 {
			if st.Tray.Groups == "inline" {
				items = append(items, mygo.Separator())
				items = append(items, groups...)
			} else {
				items = append(items, &mygo.MenuItem{Label: tr(a, "proxies"), Submenu: groups})
			}
		}
	}
	items = append(items, t.profiles())
	items = append(items, mygo.Separator())
	items = append(items, &mygo.MenuItem{Label: tr(a, "systemProxy"), Type: mygo.MenuItemCheckbox, Checked: st.SystemProxy.Enabled,
		Click: func(*mygo.MenuItem, *mygo.Window) { go a.toggleSystemProxy() }})
	items = append(items, &mygo.MenuItem{Label: tunLabel(a, s), Type: mygo.MenuItemCheckbox, Checked: st.Tun.Enabled && s.TunAvailable,
		Click: func(*mygo.MenuItem, *mygo.Window) { go a.toggleTun() }})
	if st.Tailscale.Mode != "off" {
		items = append(items, t.tailscale())
	}
	items = append(items, mygo.Separator())
	items = append(items, item("copyEnv", func() { mygo.Clipboard.WriteText(envCommand(a.settings.Get())) }))
	items = append(items, &mygo.MenuItem{Label: tr(a, "openDir"), Submenu: []*mygo.MenuItem{
		item("dataDir", func() { _ = mygo.Shell.OpenPath(a.dirs.Data) }),
		item("coreDir", func() { _ = mygo.Shell.OpenPath(a.dirs.Core) }),
		item("logsDir", func() { _ = mygo.Shell.OpenPath(a.dirs.Logs) }),
	}})
	items = append(items, &mygo.MenuItem{Label: tr(a, "more"), Submenu: []*mygo.MenuItem{
		item("restartCore", func() { go a.startCore(context.Background()) }),
		item("reapply", func() {
			go func() {
				if err := a.applyConfig(context.Background()); err != nil {
					a.notifyErr("profiles", tr(a, "applyFailed"), err)
				}
			}()
		}),
		item("updateGeo", func() {
			go func() {
				if c := a.core.Client(); c != nil {
					a.notifyErr("settings/clash", tr(a, "updateGeo"), c.UpdateGeo(context.Background()))
				}
			}()
		}),
		item("syncNow", func() {
			go func() {
				if _, err := a.syncer.run(context.Background()); err != nil {
					a.notifyErr("settings/sync", tr(a, "syncFailed"), err)
				}
			}()
		}),
		mygo.Separator(),
		item("lightweight", a.enterLightweight),
		item("checkUpdates", updater.CheckForUpdates),
		item("restartApp", func() { go mygo.App.Relaunch() }),
		item("settings", func() { a.navigate("settings") }),
	}})
	items = append(items, mygo.Separator(), item("quit", a.quit))

	menu := mygo.NewMenu(items)
	t.mu.Lock()
	t.menu = menu
	t.mu.Unlock()
	if st.TrayClick == "menu" {
		t.tray.SetMenu(menu)
	} else {
		t.tray.SetMenu(nil)
	}
	t.refresh()
}

// groups returns a submenu for each selector group, with its members.
func (t *trayUI) groups() []*mygo.MenuItem {
	a := t.a
	c := a.core.Client()
	if c == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	view, err := Proxies{a}.View(ctx)
	if err != nil {
		return nil
	}
	var out []*mygo.MenuItem
	for _, g := range view.Groups {
		if g.Type != "Selector" || g.Hidden {
			continue
		}
		group := g
		var members []*mygo.MenuItem
		for _, m := range g.All {
			member := m
			label := m.Name
			if m.Delay > 0 {
				label += fmt.Sprintf("   %d ms", m.Delay)
			}
			members = append(members, &mygo.MenuItem{Label: label, Type: mygo.MenuItemRadio, Checked: m.Name == g.Now,
				Click: func(*mygo.MenuItem, *mygo.Window) {
					go func() {
						if err := a.selectProxy(context.Background(), group.Name, member.Name); err != nil {
							a.notifyErr("proxies", group.Name, err)
						}
					}()
				}})
			if len(members) >= 80 {
				break // menus are not lists
			}
		}
		out = append(out, &mygo.MenuItem{Label: fmt.Sprintf("%s  ›  %s", g.Name, g.Now), Submenu: members})
	}
	return out
}

func (t *trayUI) profiles() *mygo.MenuItem {
	a := t.a
	v := a.profilesView()
	var items []*mygo.MenuItem
	for _, p := range v.Items {
		uid := p.UID
		items = append(items, &mygo.MenuItem{Label: p.Name, Type: mygo.MenuItemRadio, Checked: p.UID == v.Current,
			Click: func(*mygo.MenuItem, *mygo.Window) {
				go func() {
					if err := (Profiles{a}).Activate(context.Background(), uid); err != nil {
						a.notifyErr("profiles", tr(a, "applyFailed"), err)
					}
				}()
			}})
	}
	if len(items) == 0 {
		items = append(items, &mygo.MenuItem{Label: tr(a, "noProfile"), Disabled: true})
	}
	label := tr(a, "profiles")
	if s := a.snapshot(); s.ProfileName != "" {
		label += ": " + s.ProfileName
	}
	return &mygo.MenuItem{Label: label, Submenu: items}
}

func (t *trayUI) tailscale() *mygo.MenuItem {
	a := t.a
	s := a.ts.current()
	var items []*mygo.MenuItem
	state := tr(a, "disconnected")
	if s.BackendState == "Running" {
		state = tr(a, "connected")
		if s.Self != nil && len(s.Self.TailscaleIPs) > 0 {
			state += " · " + s.Self.TailscaleIPs[0]
		}
	} else if s.BackendState == "NeedsLogin" {
		state = tr(a, "tsNeedsLogin")
	}
	items = append(items, &mygo.MenuItem{Label: state, Disabled: true})
	if s.BackendState == "Running" {
		exits := []*mygo.MenuItem{{Label: tr(a, "none"), Type: mygo.MenuItemRadio, Checked: s.ExitNodeID == "",
			Click: func(*mygo.MenuItem, *mygo.Window) { go t.exitNode("") }}}
		for _, p := range s.Peers {
			if !p.ExitNodeOption {
				continue
			}
			id, label := p.ID, p.HostName
			if !p.Online {
				label += " (offline)"
			}
			exits = append(exits, &mygo.MenuItem{Label: label, Type: mygo.MenuItemRadio, Checked: p.ID == s.ExitNodeID,
				Click: func(*mygo.MenuItem, *mygo.Window) { go t.exitNode(id) }})
		}
		items = append(items, &mygo.MenuItem{Label: tr(a, "exitNode"), Submenu: exits})
		if s.Self != nil && len(s.Self.TailscaleIPs) > 0 {
			ip := s.Self.TailscaleIPs[0]
			items = append(items, &mygo.MenuItem{Label: tr(a, "copyIP") + " (" + ip + ")", Click: func(*mygo.MenuItem, *mygo.Window) { mygo.Clipboard.WriteText(ip) }})
		}
	}
	items = append(items, mygo.Separator(), &mygo.MenuItem{Label: tr(a, "adminConsole"), Click: func(*mygo.MenuItem, *mygo.Window) { _ = (Tailscale{a}).OpenAdmin() }})
	// A submenu has no checkmark: its label carries the state instead.
	label := tr(a, "tailscale")
	switch s.BackendState {
	case "Running":
		label += ": " + tr(a, "connected")
	case "NeedsLogin":
		label += ": " + tr(a, "tsNeedsLogin")
	}
	return &mygo.MenuItem{Label: label, Submenu: items}
}

func (t *trayUI) exitNode(id string) {
	if err := (Tailscale{t.a}).SetExitNode(context.Background(), id); err != nil {
		t.a.notifyErr("tailscale", tr(t.a, "exitNode"), err)
	}
}

// speed shows the traffic next to the icon (macOS), or stops showing it.
func (t *trayUI) speed(on bool) {
	if t == nil || t.tray == nil || runtime.GOOS != "darwin" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !on {
		if t.speedStop != nil {
			t.speedStop()
			t.speedStop = nil
			t.tray.SetTitle("")
		}
		return
	}
	if t.speedStop != nil {
		return
	}
	ch, stop := t.a.traffic.Subscribe()
	t.speedStop = stop
	go func() {
		for tr := range ch {
			t.tray.SetTitle(fmt.Sprintf("↑%s ↓%s", shortRate(tr.Up), shortRate(tr.Down)))
		}
	}()
}

// shortRate formats bytes per second in at most 5 characters.
func shortRate(b int64) string {
	units := []string{"B", "K", "M", "G"}
	f := float64(b)
	i := 0
	for f >= 1000 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if f < 10 && i > 0 {
		return strings.TrimSuffix(fmt.Sprintf("%.1f", f), ".0") + units[i]
	}
	return fmt.Sprintf("%.0f%s", f, units[i])
}

// tunLabel names TUN mode in the tray and the quick panel, with what
// turning it on takes when the service is missing or outdated.
func tunLabel(a *App, s AppState) string {
	switch {
	case s.TunAvailable:
		return tr(a, "tun")
	case s.Service.Installed && s.Service.Outdated:
		return tr(a, "tunUpdate")
	}
	return tr(a, "tunNeeds")
}
