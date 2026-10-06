package app

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"reflect"
	"slices"
	"time"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/service"
	"github.com/mygo-clash/mygo-clash/internal/sysproxy"
)

// changeSettings saves new settings and does what they change.
func (a *App) changeSettings(ctx context.Context, patch []byte) (config.Settings, error) {
	old, cur, err := a.settings.Patch(patch)
	if err != nil {
		return old, err
	}
	if err := a.settingsChanged(ctx, old, cur); err != nil {
		return cur, err
	}
	return cur, nil
}

// updateSettings changes settings from Go.
func (a *App) updateSettings(ctx context.Context, fn func(*config.Settings)) (config.Settings, error) {
	old, cur, err := a.settings.Update(fn)
	if err != nil {
		return old, err
	}
	return cur, a.settingsChanged(ctx, old, cur)
}

// settingsChanged applies the differences between two settings: as little
// as needed, a mode switch never reloads the configuration.
func (a *App) settingsChanged(ctx context.Context, old, cur config.Settings) error {
	_ = SettingsEvent.Broadcast(cur)
	changed := func(f func(config.Settings) any) bool { return !reflect.DeepEqual(f(old), f(cur)) }
	var errs []error

	if old.Theme != cur.Theme {
		a.applyTheme(cur)
	}
	if changed(func(s config.Settings) any { return s.Hotkeys }) {
		a.registerHotkeys(cur)
	}
	if changed(func(s config.Settings) any { return []any{s.Language, s.Tray, s.Tailscale.Mode} }) && a.tray != nil {
		go a.tray.rebuild()
	}
	if a.tray != nil {
		a.tray.speed(cur.Tray.ShowSpeed)
	}

	// The core: how it runs, then what it runs.
	needRestart := old.CoreMode != cur.CoreMode
	if cur.Tun.Enabled && !old.Tun.Enabled && !a.core.State().Privileged {
		svc := a.snapshot().Service
		if !svc.Usable() {
			// TUN needs the service: undo the switch and say why. The
			// pages, the tray and the hotkey install it first instead.
			_, _, _ = a.settings.Update(func(s *config.Settings) { s.Tun.Enabled = false })
			_ = SettingsEvent.Broadcast(a.settings.Get())
			if svc.Installed && svc.Outdated {
				a.notify(Notice{Level: "warning", Message: tr(a, "serviceOutdated"), Action: "repair-service", Page: "settings/network"})
				return errors.New(tr(a, "serviceOutdated"))
			}
			a.notify(Notice{Level: "warning", Message: tr(a, "tunNeedsService"), Action: "install-tun", Page: "settings/network"})
			return errors.New(tr(a, "tunNeedsService"))
		}
		needRestart = true // move the core to the service
	}
	if needRestart {
		go a.startCore(context.Background())
	} else {
		onlyMode := old.Clash.Mode != cur.Clash.Mode
		structural := changed(func(s config.Settings) any {
			c := s.Clash
			c.Mode = ""
			return []any{c, s.Tun, s.DNS, s.BuiltinEnhanced, s.Tailscale}
		})
		switch {
		case structural:
			if err := a.applyConfig(ctx); err != nil {
				errs = append(errs, err)
			}
		case onlyMode:
			if err := a.patchMode(ctx, cur.Clash.Mode); err != nil {
				errs = append(errs, err)
			}
		}
		if old.Clash.LogLevel != cur.Clash.LogLevel && a.logs.hub != nil {
			a.logs.hub.restart()
		}
	}
	if changed(func(s config.Settings) any { return []any{s.SystemProxy, s.Clash.MixedPort} }) {
		if err := a.applySystemProxy(cur); err != nil {
			errs = append(errs, err)
		}
	}
	if changed(func(s config.Settings) any { return s.Sync }) {
		a.syncer.restart()
	}
	if changed(func(s config.Settings) any { return s.Updates }) && a.updates != nil {
		a.updates.schedule()
	}
	if changed(func(s config.Settings) any { return s.Tailscale }) {
		a.ts.settingsChanged(old.Tailscale, cur.Tailscale)
	}
	if !reflect.DeepEqual(syncable(old), syncable(cur)) {
		a.syncer.changed()
	}
	return errors.Join(errs...)
}

// patchMode switches the mode of the running core without a reload.
func (a *App) patchMode(ctx context.Context, mode string) error {
	c := a.core.Client()
	if c == nil {
		return nil
	}
	if err := c.PatchGeneral(ctx, coreapi.GeneralPatch{Mode: mode}); err != nil {
		return err
	}
	if a.settings.Get().AutoCloseConnections {
		_ = c.CloseAllConnections(ctx)
	}
	a.updateState(func(s *AppState) { s.Mode = mode })
	return nil
}

// setMode switches the mode, from the tray, a shortcut or the panel.
func (a *App) setMode(mode string) {
	if _, err := a.updateSettings(context.Background(), func(s *config.Settings) { s.Clash.Mode = mode }); err != nil {
		a.notifyErr("", tr(a, "modeFailed"), err)
	}
}

// toggleSystemProxy turns the system proxy on or off.
func (a *App) toggleSystemProxy() {
	on := !a.settings.Get().SystemProxy.Enabled
	if _, err := a.updateSettings(context.Background(), func(s *config.Settings) { s.SystemProxy.Enabled = on }); err != nil {
		a.notifyErr("settings/network", tr(a, "sysproxyFailed"), err)
	}
}

// toggleTun turns TUN on or off.
func (a *App) toggleTun() {
	on := !a.settings.Get().Tun.Enabled
	if on && !a.snapshot().TunAvailable {
		a.installServiceForTun()
		return
	}
	_, _ = a.updateSettings(context.Background(), func(s *config.Settings) { s.Tun.Enabled = on })
}

// installService installs, repairs or updates the service, asking for an
// administrator's authorization, and moves the core to it. With enableTun
// it then turns TUN mode on: turning TUN on without the service installs
// it first, as Clash Verge does.
func (a *App) installService(ctx context.Context, enableTun bool) error {
	err := service.Elevate(ctx, tr(a, "servicePrompt"), "service", "install", "--name", a.slug, "--owner", service.CurrentOwner())
	if err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && !a.checkService(ctx).Usable() {
		time.Sleep(300 * time.Millisecond)
	}
	a.startCore(ctx)
	if !enableTun {
		return nil
	}
	if !a.snapshot().TunAvailable {
		return errors.New(tr(a, "tunNeedsService"))
	}
	_, err = a.updateSettings(ctx, func(s *config.Settings) { s.Tun.Enabled = true })
	return err
}

// installServiceForTun turns TUN on from the tray, the quick panel or a
// hotkey when the service is missing or outdated: the system's
// authorization dialog asks first, and declining it changes nothing.
func (a *App) installServiceForTun() {
	err := a.installService(context.Background(), true)
	if err != nil && !errors.Is(err, service.ErrCanceled) {
		a.notifyErr("settings/network", tr(a, "serviceFailed"), err)
	}
}

// applySystemProxy sets the system's proxy as the settings say, and
// guards it.
func (a *App) applySystemProxy(st config.Settings) error {
	sp := st.SystemProxy
	if !sp.Enabled {
		a.guard.Stop()
		a.pac.Close()
		if !a.snapshot().SystemProxy {
			return nil
		}
		err := sysproxy.Set(sysproxy.Proxy{Enabled: false})
		a.updateState(func(s *AppState) {
			s.SystemProxy = false
			s.SystemProxyError = errString(err)
		})
		return err
	}
	if a.core.Client() == nil {
		// The proxy goes on once the core runs: until then it would break
		// every connection.
		return nil
	}
	want := sysproxy.Proxy{Enabled: true, Host: sp.Host, Port: st.Clash.MixedPort}
	if sp.UseDefaultBypass {
		want.Bypass = append(want.Bypass, sysproxy.DefaultBypass...)
	}
	for _, b := range sysproxy.ParseBypass(sp.Bypass) {
		if !slices.Contains(want.Bypass, b) {
			want.Bypass = append(want.Bypass, b)
		}
	}
	if st.Tailscale.Mode == "system" && st.Tailscale.Coexist {
		want.Bypass = append(want.Bypass, "*.ts.net", "100.64.0.0/10")
	}
	if sp.PAC {
		url, err := a.pac.Serve(sysproxy.RenderPAC(sp.PACScript, sp.Host, st.Clash.MixedPort))
		if err != nil {
			return err
		}
		want.PAC = url
	} else {
		a.pac.Close()
	}
	err := sysproxy.Set(want)
	a.updateState(func(s *AppState) {
		s.SystemProxy = err == nil
		s.SystemProxyError = errString(err)
	})
	if err != nil {
		return err
	}
	if sp.Guard {
		a.guard.OnFix = func() { log.Printf("the system proxy was changed; set it again") }
		a.guard.OnFail = func(err error) {
			a.notify(Notice{Level: "warning", Message: tr(a, "guardStopped"), Detail: err.Error(), Page: "settings/network"})
		}
		a.guard.Watch(want, time.Duration(sp.GuardInterval)*time.Second)
	} else {
		a.guard.Stop()
	}
	return nil
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func (a *App) applyTheme(st config.Settings) {
	switch st.Theme {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// registerHotkeys registers the global shortcuts of the settings.
func (a *App) registerHotkeys(st config.Settings) {
	a.mu.Lock()
	old := a.hotkeys
	a.hotkeys = nil
	a.mu.Unlock()
	for _, acc := range old {
		mygo.GlobalShortcut.Unregister(acc)
	}
	if !st.Hotkeys.Enabled {
		return
	}
	actions := map[string]func(){
		config.HotkeyToggleWindow:      a.toggleMain,
		config.HotkeyQuickPanel:        func() { a.panel.toggle() },
		config.HotkeyToggleSystemProxy: func() { go a.toggleSystemProxy() },
		config.HotkeyToggleTun:         func() { go a.toggleTun() },
		config.HotkeyModeRule:          func() { go a.setMode("rule") },
		config.HotkeyModeGlobal:        func() { go a.setMode("global") },
		config.HotkeyModeDirect:        func() { go a.setMode("direct") },
		config.HotkeyLightweight:       a.enterLightweight,
		config.HotkeyReactivate: func() {
			go func() {
				if err := a.applyConfig(context.Background()); err != nil {
					a.notifyErr("profiles", tr(a, "applyFailed"), err)
				}
			}()
		},
	}
	var registered []string
	for action, acc := range st.Hotkeys.Bindings {
		fn, ok := actions[action]
		if !ok || acc == "" {
			continue
		}
		if err := mygo.GlobalShortcut.Register(acc, fn); err != nil {
			a.notify(Notice{Level: "warning", Message: tr(a, "hotkeyFailed") + " " + acc, Detail: err.Error(), Page: "settings/general"})
			continue
		}
		registered = append(registered, acc)
	}
	a.mu.Lock()
	a.hotkeys = registered
	a.mu.Unlock()
}

// syncable is the part of the settings that syncs between devices:
// everything but what belongs to one device.
func syncable(s config.Settings) map[string]any {
	data, _ := json.Marshal(s)
	var m map[string]any
	_ = json.Unmarshal(data, &m)
	for _, k := range deviceKeys {
		delete(m, k)
	}
	if t, ok := m["tailscale"].(map[string]any); ok {
		// The device's name and node belong to the device.
		delete(t, "hostname")
		delete(t, "mode")
	}
	if c, ok := m["clash"].(map[string]any); ok {
		delete(c, "interface")
	}
	if t, ok := m["tun"].(map[string]any); ok {
		delete(t, "enabled")
		delete(t, "device")
	}
	if sp, ok := m["systemProxy"].(map[string]any); ok {
		delete(sp, "enabled")
	}
	return m
}

// deviceKeys are settings of one device, which never sync.
var deviceKeys = []string{"sync", "coreMode", "silentStart", "hotkeys", "lightweight", "logs", "backup", "dns", "updates"}
