package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/enhance"
	"github.com/mygo-clash/mygo-clash/internal/tailnet"
	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

const tsStateLabel = "tailscale/state"

// tailscaleManager connects the app to a tailnet: through the core's
// embedded node, or the installed Tailscale app.
type tailscaleManager struct {
	a   *App
	sys *tailnet.System

	mu         sync.Mutex
	status     coreapi.TailscaleStatus
	state      coreapi.TailscaleState // the embedded node's, persisted sealed
	cancel     context.CancelFunc
	routingKey string // what the configuration depends on
	trayKey    string // what the tray menu shows
}

func newTailscaleManager(a *App) *tailscaleManager {
	m := &tailscaleManager{a: a, sys: tailnet.NewSystem(), status: coreapi.TailscaleStatus{Source: "off", Peers: []coreapi.TailscalePeer{}}}
	data, err := a.sealer.ReadFile(m.statePath(), tsStateLabel)
	if err == nil {
		_ = json.Unmarshal(data, &m.state)
	} else if !errors.Is(err, os.ErrNotExist) {
		log.Printf("tailscale state: %v", err)
	}
	return m
}

func (m *tailscaleManager) statePath() string { return filepath.Join(m.a.dirs.Data, "tailscale.bin") }

var hostnameClean = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

func defaultHostname() string {
	h, _ := os.Hostname()
	h = strings.TrimSuffix(strings.TrimSuffix(h, ".local"), ".lan")
	h = strings.Trim(hostnameClean.ReplaceAllString(h, "-"), "-")
	if h == "" {
		h = "mygo-clash"
	}
	return strings.ToLower(h)
}

// coreConfig is the embedded node's configuration for the core, or nil.
func (m *tailscaleManager) coreConfig(st config.Settings) *coreapi.TailscaleConfig {
	t := st.Tailscale
	if t.Mode != "embedded" {
		return &coreapi.TailscaleConfig{Enabled: false}
	}
	host := t.Hostname
	if host == "" {
		host = defaultHostname()
	}
	cfg := &coreapi.TailscaleConfig{
		Enabled: true, ProxyName: t.ProxyName, Hostname: host, ControlURL: t.ControlURL, Ephemeral: t.Ephemeral,
		AcceptRoutes: t.AcceptRoutes, ExitNode: t.ExitNode, ExitNodeAllowLAN: t.ExitNodeAllowLAN, ControlVia: t.ControlVia,
	}
	if t.ShareProxy {
		cfg.ShareProxyPort = t.ShareProxyPort
		cfg.ShareProxyTarget = "127.0.0.1:" + itoa(st.Clash.MixedPort)
	}
	m.mu.Lock()
	cfg.State = m.state.State
	m.mu.Unlock()
	return cfg
}

// tailnet is how the configuration routes the tailnet, or nil.
func (m *tailscaleManager) tailnet(st config.Settings) *enhance.Tailnet {
	t := st.Tailscale
	m.mu.Lock()
	s := m.status
	m.mu.Unlock()
	switch t.Mode {
	case "embedded":
		host := t.Hostname
		if host == "" {
			host = defaultHostname()
		}
		proxy := yamlx.MapOf("type", "tailscale", "hostname", host, "udp", true, "accept-routes", t.AcceptRoutes)
		if t.ControlURL != "" {
			proxy.Set("control-url", t.ControlURL)
		}
		if t.ExitNode != "" {
			proxy.Set("exit-node", t.ExitNode)
			proxy.Set("exit-node-allow-lan-access", t.ExitNodeAllowLAN)
		}
		if t.Ephemeral {
			proxy.Set("ephemeral", true)
		}
		if t.ControlVia != "" {
			proxy.Set("dialer-proxy", t.ControlVia)
		}
		tn := &enhance.Tailnet{Mode: "embedded", ProxyName: t.ProxyName, Proxy: proxy, JoinSelector: t.JoinSelector, MagicDNS: t.MagicDNS}
		if s.Source == "embedded" {
			tn.Suffixes = suffixesOf(s)
			if t.RouteSubnets && t.AcceptRoutes {
				tn.Routes = s.Routes
			}
		}
		return tn
	case "system":
		if !t.Coexist || s.Source != "system" || s.BackendState != "Running" {
			return nil
		}
		return &enhance.Tailnet{Mode: "system", Suffixes: suffixesOf(s), Interface: s.Interface, MagicDNS: t.MagicDNS}
	}
	return nil
}

func suffixesOf(s coreapi.TailscaleStatus) []string {
	if s.MagicDNSSuffix == "" {
		return nil
	}
	return []string{s.MagicDNSSuffix}
}

// routing summarizes what of a status the configuration depends on.
func routing(st config.Settings, s coreapi.TailscaleStatus) string {
	running := s.BackendState == "Running"
	key := st.Tailscale.Mode + "|" + s.MagicDNSSuffix + "|" + strings.Join(s.Routes, ",") + "|" + s.Interface
	if running {
		key += "|running"
	}
	return key
}

// trayKey is what the tray's Tailscale menu shows of a status.
func trayKey(s coreapi.TailscaleStatus) string {
	var b strings.Builder
	b.WriteString(s.BackendState + "|" + s.ExitNodeID)
	if s.Self != nil {
		b.WriteString("|" + strings.Join(s.Self.TailscaleIPs, ","))
	}
	for _, p := range s.Peers {
		if p.ExitNodeOption {
			fmt.Fprintf(&b, "|%s %s %t", p.ID, p.HostName, p.Online)
		}
	}
	return b.String()
}

func (m *tailscaleManager) setStatus(s coreapi.TailscaleStatus) {
	st := m.a.settings.Get()
	m.mu.Lock()
	m.status = s
	key := routing(st, s)
	changed := key != m.routingKey
	m.routingKey = key
	saved := m.state.Version
	m.mu.Unlock()
	_ = TailscaleEvent.Broadcast(s)
	menu := trayKey(s)
	m.mu.Lock()
	menuChanged := menu != m.trayKey
	m.trayKey = menu
	m.mu.Unlock()
	if changed {
		m.a.scheduleApply()
	}
	if (changed || menuChanged) && m.a.tray != nil {
		go m.a.tray.rebuild()
	}
	if m.a.panel != nil {
		m.a.panel.invalidate()
	}
	if s.Source == "embedded" && s.StateVersion > saved {
		go m.persistState()
	}
}

// persistState saves the embedded node's state, sealed.
func (m *tailscaleManager) persistState() {
	c := m.a.core.Client()
	if c == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := c.TailscaleState(ctx)
	if err != nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if state.Version <= m.state.Version && len(m.state.State) > 0 {
		return
	}
	data, err := json.Marshal(state)
	if err != nil {
		return
	}
	if err := m.a.sealer.WriteFile(m.statePath(), data, tsStateLabel); err != nil {
		log.Printf("save the tailscale state: %v", err)
		return
	}
	m.state = state
}

// coreReady follows the embedded node of a new core.
func (m *tailscaleManager) coreReady() { m.restartWatch() }

func (m *tailscaleManager) settingsChanged(old, cur config.Tailscale) {
	if old.Mode != cur.Mode {
		m.restartWatch()
	}
}

// restartWatch follows the node of the mode of the settings.
func (m *tailscaleManager) restartWatch() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(m.a.ctx)
	m.cancel = cancel
	m.mu.Unlock()
	switch m.a.settings.Get().Tailscale.Mode {
	case "embedded":
		go m.watchEmbedded(ctx)
	case "system":
		go m.sys.Watch(ctx, 10*time.Second, m.setStatus)
	default:
		m.setStatus(coreapi.TailscaleStatus{Source: "off", Peers: []coreapi.TailscalePeer{}, UpdatedAt: time.Now()})
	}
}

func (m *tailscaleManager) watchEmbedded(ctx context.Context) {
	for ctx.Err() == nil {
		if c := m.a.core.Client(); c != nil {
			_ = c.WatchTailscale(ctx, func(e coreapi.TailscaleEvent) {
				if e.Status != nil {
					m.setStatus(*e.Status)
				}
				if e.StateVersion > 0 {
					m.mu.Lock()
					saved := m.state.Version
					m.mu.Unlock()
					if e.StateVersion > saved {
						go m.persistState()
					}
				}
			})
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *tailscaleManager) current() coreapi.TailscaleStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// forget deletes the embedded node's saved identity.
func (m *tailscaleManager) forget() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state = coreapi.TailscaleState{}
	_ = os.Remove(m.statePath())
}

// Tailscale is the tailnet page.
type Tailscale struct{ a *App }

// Status returns the tailnet's status: the embedded node's, or the
// installed client's.
func (s Tailscale) Status(ctx context.Context) (coreapi.TailscaleStatus, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return coreapi.TailscaleStatus{}, err
	}
	return s.a.ts.current(), nil
}

// SystemStatus checks the installed Tailscale app, whatever the mode.
func (s Tailscale) SystemStatus(ctx context.Context) coreapi.TailscaleStatus {
	return s.a.ts.sys.Status(ctx)
}

// Login logs the embedded node in: with an auth key, or interactively, in
// which case it returns the URL to open.
func (s Tailscale) Login(ctx context.Context, authKey string) (string, error) {
	c, err := s.a.core.Must()
	if err != nil {
		return "", err
	}
	if err := c.TailscaleLogin(ctx, coreapi.TailscaleLogin{AuthKey: strings.TrimSpace(authKey)}); err != nil {
		return "", err
	}
	if authKey != "" {
		return "", nil
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		st, err := c.TailscaleStatus(ctx)
		if err == nil && st.AuthURL != "" {
			s.a.ts.setStatus(st)
			return st.AuthURL, nil
		}
		if err == nil && st.BackendState == "Running" {
			return "", nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return "", errors.New(tr(s.a, "tsNoLoginURL"))
}

// Logout logs the embedded node out; forget also deletes its identity, so
// that the next login makes a new device.
func (s Tailscale) Logout(ctx context.Context, forget bool) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	if err := c.TailscaleLogout(ctx); err != nil {
		return err
	}
	if forget {
		s.a.ts.forget()
	}
	return nil
}

// SetExitNode sends traffic to the internet through a peer ("" for none).
func (s Tailscale) SetExitNode(ctx context.Context, target string) error {
	st := s.a.settings.Get()
	switch st.Tailscale.Mode {
	case "embedded":
		_, err := s.a.updateSettings(ctx, func(cs *config.Settings) { cs.Tailscale.ExitNode = target })
		return err
	case "system":
		return s.a.ts.sys.SetPrefs(ctx, coreapi.TailscalePrefs{ExitNode: &target})
	}
	return errors.New(tr(s.a, "tsOff"))
}

// SetPrefs changes the node's preferences.
func (s Tailscale) SetPrefs(ctx context.Context, p coreapi.TailscalePrefs) error {
	st := s.a.settings.Get()
	switch st.Tailscale.Mode {
	case "embedded":
		_, err := s.a.updateSettings(ctx, func(cs *config.Settings) {
			if p.AcceptRoutes != nil {
				cs.Tailscale.AcceptRoutes = *p.AcceptRoutes
			}
			if p.ExitNodeAllowLAN != nil {
				cs.Tailscale.ExitNodeAllowLAN = *p.ExitNodeAllowLAN
			}
			if p.ExitNode != nil {
				cs.Tailscale.ExitNode = *p.ExitNode
			}
		})
		if err != nil {
			return err
		}
		if p.ShieldsUp != nil {
			if c := s.a.core.Client(); c != nil {
				return c.TailscalePrefs(ctx, coreapi.TailscalePrefs{ShieldsUp: p.ShieldsUp})
			}
		}
		return nil
	case "system":
		return s.a.ts.sys.SetPrefs(ctx, p)
	}
	return errors.New(tr(s.a, "tsOff"))
}

// Ping pings a peer.
func (s Tailscale) Ping(ctx context.Context, ip string) (coreapi.TailscalePingResult, error) {
	switch s.a.settings.Get().Tailscale.Mode {
	case "embedded":
		c, err := s.a.core.Must()
		if err != nil {
			return coreapi.TailscalePingResult{}, err
		}
		return c.TailscalePing(ctx, coreapi.TailscalePing{IP: ip})
	case "system":
		return s.a.ts.sys.Ping(ctx, coreapi.TailscalePing{IP: ip})
	}
	return coreapi.TailscalePingResult{}, errors.New(tr(s.a, "tsOff"))
}

// SetRunning connects or disconnects the installed Tailscale app.
func (s Tailscale) SetRunning(ctx context.Context, on bool) error {
	return s.a.ts.sys.SetRunning(ctx, on)
}

// OpenAdmin opens the tailnet's admin console.
func (s Tailscale) OpenAdmin() error {
	u := "https://login.tailscale.com/admin/machines"
	if cu := s.a.settings.Get().Tailscale.ControlURL; cu != "" && s.a.settings.Get().Tailscale.Mode == "embedded" {
		u = cu
	}
	return mygo.Shell.OpenExternal(u)
}

func itoa(n int) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
