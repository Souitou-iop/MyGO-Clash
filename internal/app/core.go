package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime"
	"slices"
	"sync"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/enhance"
	"github.com/mygo-clash/mygo-clash/internal/paths"
	"github.com/mygo-clash/mygo-clash/internal/profiles"
	"github.com/mygo-clash/mygo-clash/internal/service"
	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

func serviceSupported() bool {
	switch runtime.GOOS {
	case "darwin", "linux", "windows":
		return true
	}
	return false
}

// bootstrap is what a core of the mode starts with.
func (a *App) bootstrap(mode string) coreapi.Bootstrap {
	st := a.settings.Get()
	if mode == "service" {
		// The service picks the core's home and socket; the app talks to
		// the core through the service's socket.
		return coreapi.Bootstrap{Socket: service.Layout(a.slug).Socket, Mode: mode, LogLevel: st.Clash.LogLevel}
	}
	owner := service.CurrentOwner()
	sock, err := paths.CoreSocket(a.slug, owner)
	if err != nil {
		log.Printf("core socket: %v", err)
	}
	b := coreapi.Bootstrap{Home: a.dirs.Core, Socket: sock, Mode: mode, LogLevel: st.Clash.LogLevel}
	if runtime.GOOS == "windows" {
		b.PipeSDDL = fmt.Sprintf("D:PAI(A;OICI;GWGR;;;%s)(A;OICI;GWGR;;;SY)", owner)
	}
	return b
}

// checkService asks the service how it is and records it.
func (a *App) checkService(ctx context.Context) ServiceState {
	s := ServiceState{Supported: serviceSupported()}
	if s.Supported {
		st, err := service.Check(ctx, a.slug)
		switch {
		case err == nil:
			s.Installed, s.Version = true, st.Version
			s.Outdated = st.Version != Version
		case !errors.Is(err, service.ErrNotInstalled):
			s.Installed, s.Error = true, err.Error()
		}
	}
	a.updateState(func(st *AppState) { st.Service = s })
	return s
}

// launcher chooses how to run the core: through the service when it is
// installed and usable (or required), else as a child process. A service
// of another version, left by an update of the app, runs nothing until the
// user updates it: the core runs without privileges meanwhile, and the
// user is told once.
func (a *App) launcher(ctx context.Context) coremgr.Launcher {
	st := a.settings.Get()
	svc := a.checkService(ctx)
	if st.CoreMode != "sidecar" && svc.Usable() {
		return coremgr.ServiceLauncher{Client: service.NewClient(a.slug)}
	}
	if st.CoreMode != "sidecar" && svc.Installed && svc.Outdated && !a.inFront() && a.outdatedTold.CompareAndSwap(false, true) {
		// The window offers to update it when it opens; away, a
		// notification says so.
		a.notify(Notice{Level: "warning", Message: tr(a, "serviceOutdated"), Action: "repair-service", Page: "settings/network"})
	}
	return coremgr.Sidecar{Log: a.coreLog}
}

// startCore starts (or restarts) the core.
func (a *App) startCore(ctx context.Context) {
	l := a.launcher(ctx)
	err := a.core.Start(ctx, l)
	if err != nil && l.Mode() == "service" {
		a.notify(Notice{Level: "warning", Message: tr(a, "serviceFallback"), Detail: err.Error(), Action: "repair-service", Page: "settings/network"})
		err = a.core.Start(ctx, coremgr.Sidecar{Log: a.coreLog})
	}
	if err != nil {
		a.notifyErr("", tr(a, "coreStartFailed"), err)
	}
}

func (a *App) onCoreState(s coremgr.State) {
	a.updateState(func(st *AppState) {
		st.Core = s
		st.TunAvailable = s.Privileged || st.Service.Usable()
		if s.Status != coremgr.StatusRunning {
			st.Tun = false
		}
	})
	if s.Status == coremgr.StatusError && s.Restarts == 1 {
		a.notify(Notice{Level: "error", Message: tr(a, "coreCrashed"), Detail: s.Error})
	}
}

func (a *App) onCoreReady(ctx context.Context, c *coreapi.Client) {
	a.updateState(func(st *AppState) { st.Ready = true })
	if err := a.applyConfig(ctx); err != nil {
		a.notifyErr("profiles", tr(a, "applyFailed"), err)
	}
	a.restoreSelections(ctx)
	a.applySystemProxy(a.settings.Get())
	a.applyWebRTC(a.settings.Get())
	a.ts.coreReady()
}

// inputs gathers what the runtime configuration depends on.
func (a *App) inputs(st config.Settings) profiles.Inputs {
	c := st.Clash
	port := func(p int, on bool) int {
		if on {
			return p
		}
		return 0
	}
	privileged := a.core.State().Privileged
	base := enhance.Base{
		Mode: c.Mode, MixedPort: c.MixedPort,
		SocksPort: port(c.SocksPort, c.SocksEnabled), HTTPPort: port(c.HTTPPort, c.HTTPEnabled),
		RedirPort:  port(c.RedirPort, c.RedirEnabled && runtime.GOOS != "windows"),
		TProxyPort: port(c.TProxyPort, c.TProxyEnabled && runtime.GOOS == "linux"),
		AllowLAN:   c.AllowLAN, IPv6: c.IPv6, LogLevel: c.LogLevel, UnifiedDelay: c.UnifiedDelay,
		TCPConcurrent: c.TCPConcurrent, FindProcessMode: c.FindProcessMode, Interface: c.Interface,
		Tun: enhance.Tun{
			Enable: st.Tun.Enabled && privileged, Stack: st.Tun.Stack, Device: st.Tun.Device,
			AutoRoute: st.Tun.AutoRoute, StrictRoute: st.Tun.StrictRoute, AutoDetectInterface: st.Tun.AutoDetectInterface,
			AutoRedirect: st.Tun.AutoRedirect && runtime.GOOS == "linux", DNSHijack: st.Tun.DNSHijack, MTU: st.Tun.MTU,
			RouteExcludeAddress: st.Tun.RouteExcludeAddress,
		},
	}
	if c.Controller.Enabled {
		base.ExternalController = c.Controller.Address
		base.Secret = c.Controller.Secret
		base.CORSOrigins = c.Controller.AllowOrigins
		base.CORSPrivateNetwork = c.Controller.AllowPrivateNetwork
	}
	in := profiles.Inputs{Base: base, Builtin: st.BuiltinEnhanced, Tailnet: a.ts.tailnet(st)}
	cur, _ := a.profiles.Current()
	if a.dnsOverride(st, cur.UID) {
		if m, err := yamlx.Parse([]byte(st.DNS.Config)); err == nil {
			in.DNS = m
		}
	}
	return in
}

// dnsOverride reports whether the DNS override applies to a profile.
func (a *App) dnsOverride(st config.Settings, uid string) bool {
	if on, ok := st.DNS.Profiles[uid]; ok {
		return on
	}
	return st.DNS.DefaultOverride
}

// applyConfig generates the runtime configuration and applies it.
func (a *App) applyConfig(ctx context.Context) error {
	a.applyMu.Lock()
	defer a.applyMu.Unlock()
	st := a.settings.Get()
	rt, err := a.profiles.Generate(ctx, a.inputs(st))
	if err != nil {
		a.updateState(func(s *AppState) { s.ConfigError = err.Error() })
		return err
	}
	data, err := yamlx.Marshal(rt.Core)
	if err != nil {
		return err
	}
	req := coreapi.ApplyRequest{Config: string(data), Tailscale: a.ts.coreConfig(st)}
	res, err := a.core.Apply(ctx, req)
	if err != nil {
		if errors.Is(err, coremgr.ErrNotRunning) {
			return nil // it applies when the core starts
		}
		a.updateState(func(s *AppState) { s.ConfigError = err.Error() })
		return err
	}
	a.mu.Lock()
	a.runtime = rt
	a.mu.Unlock()
	tunOn := false
	if tun := rt.Core.Map("tun"); tun != nil {
		tunOn, _ = tun.Bool("enable")
	}
	now := time.Now()
	a.updateState(func(s *AppState) {
		s.ConfigError, s.AppliedAt = "", now
		s.ProfileUID, s.ProfileName = rt.ProfileUID, rt.ProfileName
		s.Mode, s.MixedPort, s.Tun = st.Clash.Mode, st.Clash.MixedPort, tunOn
	})
	notes := map[string]int{}
	for uid, l := range rt.Logs {
		notes[uid] = len(l)
	}
	_ = RuntimeEvent.Broadcast(RuntimeInfo{ProfileUID: rt.ProfileUID, AppliedAt: now, Notes: notes})
	for _, w := range res.Warnings {
		a.notify(Notice{Level: "warning", Message: w, Page: "profiles"})
	}
	if a.tray != nil {
		go a.tray.rebuild()
	}
	return nil
}

var reapply struct {
	sync.Mutex
	timer *time.Timer
}

// scheduleApply applies the configuration soon, coalescing bursts of
// changes into one.
func (a *App) scheduleApply() {
	reapply.Lock()
	defer reapply.Unlock()
	if reapply.timer != nil {
		reapply.timer.Stop()
	}
	reapply.timer = time.AfterFunc(300*time.Millisecond, func() {
		ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
		defer cancel()
		if err := a.applyConfig(ctx); err != nil {
			a.notifyErr("profiles", tr(a, "applyFailed"), err)
		}
	})
}

// restoreSelections selects again what the user selected in the current
// profile's groups.
func (a *App) restoreSelections(ctx context.Context) {
	c := a.core.Client()
	cur, ok := a.profiles.Current()
	if c == nil || !ok || len(cur.Selected) == 0 {
		return
	}
	proxies, err := c.Proxies(ctx)
	if err != nil {
		return
	}
	for _, s := range cur.Selected {
		g, ok := proxies[s.Name]
		if !ok || g.Type != "Selector" || g.Now == s.Now || !slices.Contains(g.All, s.Now) {
			continue
		}
		_ = c.SelectProxy(ctx, s.Name, s.Now)
	}
}

// runtimeConfig returns the configuration last applied.
func (a *App) runtimeConfig() *profiles.Runtime {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.runtime
}

// pollConnections streams the connections every second, with their speeds.
func (a *App) pollConnections(ctx context.Context, emit func(coreapi.Connections)) error {
	c, err := a.core.Must()
	if err != nil {
		return err
	}
	prev := map[string][2]int64{}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		snap, err := c.Connections(ctx)
		if err != nil {
			return err
		}
		next := make(map[string][2]int64, len(snap.Connections))
		for i := range snap.Connections {
			cn := &snap.Connections[i]
			if p, ok := prev[cn.ID]; ok {
				cn.UploadSpeed, cn.DownloadSpeed = max(0, cn.Upload-p[0]), max(0, cn.Download-p[1])
			}
			next[cn.ID] = [2]int64{cn.Upload, cn.Download}
		}
		prev = next
		emit(snap)
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
		}
	}
}

// collectUsage counts the traffic of the connections for the statistics,
// from the same snapshots the connections page gets. Being a subscriber
// keeps them coming while the page is not open.
func (a *App) collectUsage() {
	ch, stop := a.conns.Subscribe()
	defer stop()
	for {
		select {
		case <-a.ctx.Done():
			return
		case snap, ok := <-ch:
			if !ok {
				return
			}
			a.usage.Observe(snap.Connections)
		}
	}
}

// ---- Logs ----

// logRing keeps the core's recent logs, and streams new ones.
type logRing struct {
	mu   sync.Mutex
	buf  []coreapi.LogEvent
	max  int
	hub  *hub[coreapi.LogEvent]
	stop func()
}

func newLogRing(n int) *logRing { return &logRing{max: n} }

// follow collects the core's logs at level until the app quits.
func (r *logRing) follow(a *App) {
	r.hub = newHub(func(ctx context.Context, emit func(coreapi.LogEvent)) error {
		c, err := a.core.Must()
		if err != nil {
			return err
		}
		return c.StreamLogs(ctx, a.settings.Get().Clash.LogLevel, emit)
	})
	ch, stop := r.hub.Subscribe()
	r.stop = stop
	go func() {
		for e := range ch {
			r.mu.Lock()
			r.buf = append(r.buf, e)
			if len(r.buf) > r.max {
				r.buf = r.buf[len(r.buf)-r.max:]
			}
			r.mu.Unlock()
		}
	}()
}

func (r *logRing) recent() []coreapi.LogEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.buf)
}

func (r *logRing) clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = nil
}

// ---- Background ----

// background runs the periodic work: automatic delay tests and backups.
func (a *App) background() {
	a.logs.follow(a)
	go a.collectUsage()
	minute := time.NewTicker(time.Minute)
	defer minute.Stop()
	var lastDelay, lastBackup time.Time
	lastBackup = time.Now()
	for {
		select {
		case <-a.ctx.Done():
			return
		case <-minute.C:
		}
		if err := a.usage.Flush(); err != nil {
			log.Printf("save the traffic statistics: %v", err)
		}
		st := a.settings.Get()
		if st.Latency.AutoCheck && time.Since(lastDelay) >= time.Duration(st.Latency.AutoCheckMinutes)*time.Minute {
			lastDelay = time.Now()
			go a.checkCurrentDelays()
		}
		if h := st.Backup.AutoIntervalHours; h > 0 && time.Since(lastBackup) >= time.Duration(h)*time.Hour {
			lastBackup = time.Now()
			go func() {
				if _, err := a.createLocalBackup(); err != nil {
					log.Printf("automatic backup: %v", err)
				}
			}()
		}
	}
}

// checkCurrentDelays tests the delay of the proxies selected in groups.
func (a *App) checkCurrentDelays() {
	c := a.core.Client()
	if c == nil {
		return
	}
	st := a.settings.Get()
	ctx, cancel := context.WithTimeout(a.ctx, time.Minute)
	defer cancel()
	proxies, err := c.Proxies(ctx)
	if err != nil {
		return
	}
	seen := map[string]bool{}
	for _, p := range proxies {
		if p.Now == "" || seen[p.Now] {
			continue
		}
		seen[p.Now] = true
		_, _ = c.ProxyDelay(ctx, p.Now, st.Latency.URL, time.Duration(st.Latency.TimeoutMs)*time.Millisecond)
	}
}

// onProfileUpdated reports scheduled updates of profiles.
func (a *App) onProfileUpdated(uid string, err error) {
	p, _ := a.profiles.Get(uid)
	if err != nil {
		a.notify(Notice{Level: "warning", Message: fmt.Sprintf(tr(a, "updateFailed"), p.Name), Detail: err.Error(), Page: "profiles"})
	}
}

// onProfilesChanged follows changes of profiles: the page's list, the
// runtime configuration when the current profile changed, and sync.
func (a *App) onProfilesChanged(uid string, content bool) {
	_ = ProfilesEvent.Broadcast(a.profilesView())
	if content && a.affectsRuntime(uid) {
		a.scheduleApply()
	}
	if a.tray != nil {
		go a.tray.rebuild()
	}
	a.syncer.changed()
}

// affectsRuntime reports whether an item takes part in the runtime
// configuration: the current profile, its extensions, the global ones.
func (a *App) affectsRuntime(uid string) bool {
	if uid == "" || uid == profiles.GlobalMerge || uid == profiles.GlobalScript {
		return true
	}
	cur, ok := a.profiles.Current()
	if !ok {
		return true
	}
	if cur.UID == uid {
		return true
	}
	for _, k := range profiles.ExtensionKinds {
		if cur.Option.Extension(k) == uid {
			return true
		}
	}
	return false
}
