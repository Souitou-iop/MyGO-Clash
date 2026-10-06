package tailnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/mihomo/component/dialer"
	"github.com/metacubex/mihomo/component/proxydialer"
	"github.com/metacubex/mihomo/component/resolver"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"
	"github.com/metacubex/tailscale/client/local"
	"github.com/metacubex/tailscale/envknob"
	"github.com/metacubex/tailscale/ipn"
	"github.com/metacubex/tailscale/tsnet"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

func init() {
	// No logs leave the computer: Tailscale's log upload is off.
	envknob.SetNoLogsNoSupport()
}

// ErrOff is returned when traffic reaches the tailnet outbound while the
// node is off.
var ErrOff = errors.New("tailscale is off")

// Node is the embedded Tailscale node of the core. Its server lives across
// configuration changes; Configure restarts it only when the identity of
// the node changes (hostname, control server, ephemeral, control proxy).
//
// The mutex guards the fields below it and is never held across calls to
// Tailscale's local API, which may block on the network.
type Node struct {
	dir   string       // tsnet's working directory: logs, certificates
	via   atomic.Value // string: the proxy of the control connection, "" for direct
	store *Store

	mu      sync.Mutex
	cfg     *coreapi.TailscaleConfig
	srv     *tsnet.Server
	lc      *local.Client
	gen     uint64             // bumped on every (re)start, to retire old watchers
	cancel  context.CancelFunc // stops the watcher of srv
	status  coreapi.TailscaleStatus
	authURL string
	running chan struct{} // closed while the backend is Running
	share   net.Listener

	watchMu  sync.Mutex
	watchers map[chan coreapi.TailscaleEvent]struct{}
}

// NewNode returns a node that keeps its files in dir. It runs nothing
// until Configure enables it.
func NewNode(dir string) *Node {
	n := &Node{
		dir:      dir,
		running:  make(chan struct{}),
		watchers: map[chan coreapi.TailscaleEvent]struct{}{},
		status:   offStatus(),
	}
	n.via.Store("")
	n.store = newStore(func(version uint64) {
		n.broadcast(coreapi.TailscaleEvent{StateVersion: version})
	})
	return n
}

func offStatus() coreapi.TailscaleStatus {
	return coreapi.TailscaleStatus{Source: "embedded", BackendState: "NoState", Peers: []coreapi.TailscalePeer{}, UpdatedAt: time.Now()}
}

// Configure starts, restarts, reconfigures or stops the node.
func (n *Node) Configure(cfg *coreapi.TailscaleConfig) error {
	n.mu.Lock()
	if cfg == nil || !cfg.Enabled {
		n.stopLocked()
		n.cfg = nil
		n.setStatusLocked(offStatus())
		n.mu.Unlock()
		return nil
	}
	via := cfg.ControlVia
	if strings.EqualFold(via, "DIRECT") {
		via = ""
	}
	n.via.Store(via)
	if n.srv != nil && n.cfg.SameNode(cfg) {
		old, lc := n.cfg, n.lc
		n.cfg = cfg
		shareChanged := old.ShareProxyPort != cfg.ShareProxyPort || old.ShareProxyTarget != cfg.ShareProxyTarget
		if shareChanged {
			n.restartShareLocked()
		}
		n.mu.Unlock()
		return applyPrefs(lc, old, cfg)
	}
	n.stopLocked()
	n.cfg = cfg
	if len(cfg.State) > 0 {
		n.store.load(cfg.State)
	}
	err := n.startLocked("")
	n.mu.Unlock()
	return err
}

func (n *Node) startLocked(authKey string) error {
	cfg := n.cfg
	dir := filepath.Join(n.dir, "tailscale")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	srv := &tsnet.Server{
		Dir:        dir,
		Store:      n.store,
		Hostname:   cfg.Hostname,
		AuthKey:    authKey,
		ControlURL: cfg.ControlURL,
		Ephemeral:  cfg.Ephemeral,
		// The node's own connections, to the coordination server, relays
		// and peers, leave through the physical interface (or a proxy), not
		// back into the core's TUN.
		SystemDialer: func(ctx context.Context, network, address string) (net.Conn, error) {
			if via := n.via.Load().(string); via != "" {
				return proxydialer.NewByName(via, tunnel.Tunnel).DialContext(ctx, network, address)
			}
			return dialer.DialContext(ctx, network, address)
		},
		SystemPacketListener: func(ctx context.Context, network, address string) (net.PacketConn, error) {
			if via := n.via.Load().(string); via != "" {
				return proxydialer.NewByName(via, tunnel.Tunnel).ListenPacket(ctx, network, address, netip.AddrPort{})
			}
			return dialer.ListenPacket(ctx, network, address, netip.AddrPort{})
		},
		LookupHook: func(ctx context.Context, host string) ([]netip.Addr, error) {
			return resolver.LookupIPWithResolver(ctx, host, resolver.ProxyServerHostResolver)
		},
		UserLogf: func(format string, args ...any) { log.Infoln("[Tailscale] %s", fmt.Sprintf(format, args...)) },
		Logf:     func(format string, args ...any) { log.Debugln("[Tailscale] %s", fmt.Sprintf(format, args...)) },
	}
	if err := srv.Start(); err != nil {
		return fmt.Errorf("start tailscale: %w", err)
	}
	lc, err := srv.LocalClient()
	if err != nil {
		go func() { _ = srv.Close() }()
		return err
	}
	n.srv, n.lc = srv, lc
	n.authURL = ""
	n.gen++
	gen := n.gen
	ctx, cancel := context.WithCancel(context.Background())
	n.cancel = cancel
	n.setStatusLocked(coreapi.TailscaleStatus{Source: "embedded", Available: true, BackendState: "Starting", Peers: []coreapi.TailscalePeer{}})
	go n.watch(ctx, gen, lc)
	go func() {
		// Preferences apply once the backend takes them.
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if err := n.waitBackend(pctx, gen); err != nil {
			return
		}
		n.mu.Lock()
		cfg := n.cfg
		if n.gen != gen || cfg == nil {
			n.mu.Unlock()
			return
		}
		n.restartShareLocked()
		n.mu.Unlock()
		if err := applyPrefs(lc, nil, cfg); err != nil {
			log.Warnln("[Tailscale] apply preferences: %v", err)
		}
	}()
	return nil
}

func (n *Node) stopLocked() {
	if n.share != nil {
		_ = n.share.Close()
		n.share = nil
	}
	if n.cancel != nil {
		n.cancel()
		n.cancel = nil
	}
	if n.srv != nil {
		srv := n.srv
		n.srv, n.lc = nil, nil
		go func() { _ = srv.Close() }() // closing waits on the network
	}
	n.gen++
	n.markRunningLocked(false)
}

// waitBackend waits until the backend of generation gen left NoState and
// Starting, which is when it takes preferences.
func (n *Node) waitBackend(ctx context.Context, gen uint64) error {
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		n.mu.Lock()
		state, cur := n.status.BackendState, n.gen
		n.mu.Unlock()
		if cur != gen {
			return context.Canceled
		}
		if state != "" && state != "NoState" && state != "Starting" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

// applyPrefs sends the preferences that changed between old and cfg (all
// of them when old is nil).
func applyPrefs(lc *local.Client, old, cfg *coreapi.TailscaleConfig) error {
	if lc == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	mp := &ipn.MaskedPrefs{}
	if old == nil || old.AcceptRoutes != cfg.AcceptRoutes {
		mp.RouteAll, mp.RouteAllSet = cfg.AcceptRoutes, true
	}
	if old == nil || old.ExitNodeAllowLAN != cfg.ExitNodeAllowLAN {
		mp.ExitNodeAllowLANAccess, mp.ExitNodeAllowLANAccessSet = cfg.ExitNodeAllowLAN, true
	}
	if mp.RouteAllSet || mp.ExitNodeAllowLANAccessSet {
		if _, err := lc.EditPrefs(ctx, mp); err != nil {
			return err
		}
	}
	if old == nil || old.ExitNode != cfg.ExitNode {
		st, _ := lc.Status(ctx)
		ep, err := exitNodePrefs(cfg.ExitNode, st)
		if err != nil {
			return fmt.Errorf("exit node %q: %w", cfg.ExitNode, err)
		}
		if _, err := lc.EditPrefs(ctx, ep); err != nil {
			return err
		}
	}
	return nil
}

// watch follows the backend's notifications and refreshes the status.
func (n *Node) watch(ctx context.Context, gen uint64, lc *local.Client) {
	refresh := make(chan struct{}, 1)
	poke := func() {
		select {
		case refresh <- struct{}{}:
		default:
		}
	}
	go func() {
		for ctx.Err() == nil {
			w, err := lc.WatchIPNBus(ctx, ipn.NotifyInitialState|ipn.NotifyNoPrivateKeys)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Second):
				}
				continue
			}
			for {
				nt, err := w.Next()
				if err != nil {
					break
				}
				if nt.BrowseToURL != nil {
					n.mu.Lock()
					if n.gen == gen {
						n.authURL = *nt.BrowseToURL
					}
					n.mu.Unlock()
				}
				poke()
			}
			_ = w.Close()
		}
	}()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return
		case <-refresh:
			// Coalesce bursts of notifications.
			if d := time.Since(last); d < 300*time.Millisecond {
				select {
				case <-ctx.Done():
					return
				case <-time.After(300*time.Millisecond - d):
				}
			}
		case <-ticker.C:
			if !n.hasWatchers() {
				continue
			}
		}
		last = time.Now()
		n.refresh(ctx, gen, lc)
	}
}

func (n *Node) refresh(ctx context.Context, gen uint64, lc *local.Client) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	st, err := lc.Status(ctx)
	if err != nil {
		return
	}
	prefs, _ := lc.GetPrefs(ctx)
	s := FromStatus(st, prefs, "embedded")
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.gen != gen {
		return
	}
	if s.AuthURL == "" && (s.BackendState == "NeedsLogin" || s.BackendState == "NeedsMachineAuth") {
		s.AuthURL = n.authURL
	}
	if n.share != nil {
		s.ShareProxy = n.share.Addr().String()
	}
	n.setStatusLocked(s)
}

func (n *Node) setStatusLocked(s coreapi.TailscaleStatus) {
	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = time.Now()
	}
	s.StateVersion = n.store.version()
	n.status = s
	n.markRunningLocked(s.BackendState == "Running")
	n.broadcast(coreapi.TailscaleEvent{Status: &s})
}

func (n *Node) markRunningLocked(on bool) {
	select {
	case <-n.running:
		if !on {
			n.running = make(chan struct{})
		}
	default:
		if on {
			close(n.running)
		}
	}
}

// Status returns the node's status.
func (n *Node) Status() coreapi.TailscaleStatus {
	n.mu.Lock()
	defer n.mu.Unlock()
	s := n.status
	s.StateVersion = n.store.version()
	return s
}

// ready waits until the node runs, for up to the context's deadline or 10
// seconds, and returns its server.
func (n *Node) ready(ctx context.Context) (*tsnet.Server, error) {
	n.mu.Lock()
	srv, running, state := n.srv, n.running, n.status.BackendState
	n.mu.Unlock()
	if srv == nil {
		return nil, ErrOff
	}
	switch state {
	case "NeedsLogin", "NeedsMachineAuth", "Stopped":
		return nil, fmt.Errorf("tailscale: %s", state)
	}
	select {
	case <-running:
		return srv, nil
	default:
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	select {
	case <-running:
		return srv, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("tailscale is not running yet (%s)", state)
	}
}

func (n *Node) client() (*local.Client, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.lc == nil {
		return nil, ErrOff
	}
	return n.lc, nil
}

// Login logs the node in, with an auth key, or interactively: the status
// then carries the URL to open.
func (n *Node) Login(ctx context.Context, authKey string) error {
	if authKey != "" {
		n.mu.Lock()
		defer n.mu.Unlock()
		if n.cfg == nil {
			return ErrOff
		}
		n.stopLocked()
		return n.startLocked(authKey)
	}
	n.mu.Lock()
	n.authURL = ""
	n.mu.Unlock()
	lc, err := n.client()
	if err != nil {
		return err
	}
	return lc.StartLoginInteractive(ctx)
}

// Logout logs the node out and forgets its keys.
func (n *Node) Logout(ctx context.Context) error {
	lc, err := n.client()
	if err != nil {
		return err
	}
	return lc.Logout(ctx)
}

// SetPrefs changes preferences of the running node, until the next
// Configure with other values.
func (n *Node) SetPrefs(ctx context.Context, p coreapi.TailscalePrefs) error {
	lc, err := n.client()
	if err != nil {
		return err
	}
	mp := &ipn.MaskedPrefs{}
	if p.AcceptRoutes != nil {
		mp.RouteAll, mp.RouteAllSet = *p.AcceptRoutes, true
	}
	if p.ExitNodeAllowLAN != nil {
		mp.ExitNodeAllowLANAccess, mp.ExitNodeAllowLANAccessSet = *p.ExitNodeAllowLAN, true
	}
	if p.ShieldsUp != nil {
		mp.ShieldsUp, mp.ShieldsUpSet = *p.ShieldsUp, true
	}
	if mp.RouteAllSet || mp.ExitNodeAllowLANAccessSet || mp.ShieldsUpSet {
		if _, err := lc.EditPrefs(ctx, mp); err != nil {
			return err
		}
	}
	if p.ExitNode != nil {
		st, _ := lc.Status(ctx)
		ep, err := exitNodePrefs(*p.ExitNode, st)
		if err != nil {
			return err
		}
		if _, err := lc.EditPrefs(ctx, ep); err != nil {
			return err
		}
	}
	return nil
}

// Ping pings a peer.
func (n *Node) Ping(ctx context.Context, p coreapi.TailscalePing) (coreapi.TailscalePingResult, error) {
	lc, err := n.client()
	if err != nil {
		return coreapi.TailscalePingResult{}, err
	}
	ip, err := parseIP(p.IP)
	if err != nil {
		return coreapi.TailscalePingResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	r, err := lc.Ping(ctx, ip, pingType(p.Type))
	if err != nil {
		return coreapi.TailscalePingResult{}, err
	}
	return pingResult(r), nil
}

// QueryDNS resolves a name with Tailscale's DNS (MagicDNS and the tailnet's
// resolvers), returning the packed DNS response.
func (n *Node) QueryDNS(ctx context.Context, name, qtype string) ([]byte, error) {
	if _, err := n.ready(ctx); err != nil {
		return nil, err
	}
	lc, err := n.client()
	if err != nil {
		return nil, err
	}
	resp, _, err := lc.QueryDNS(ctx, name, qtype)
	return resp, err
}

// State returns the node's saved state.
func (n *Node) State() coreapi.TailscaleState {
	v, s := n.store.snapshot()
	return coreapi.TailscaleState{Version: v, State: s}
}

// Watch returns a channel of the node's events, starting with its status,
// and a function that stops them.
func (n *Node) Watch() (<-chan coreapi.TailscaleEvent, func()) {
	ch := make(chan coreapi.TailscaleEvent, 64)
	st := n.Status()
	ch <- coreapi.TailscaleEvent{Status: &st}
	n.watchMu.Lock()
	n.watchers[ch] = struct{}{}
	n.watchMu.Unlock()
	return ch, func() {
		n.watchMu.Lock()
		delete(n.watchers, ch)
		n.watchMu.Unlock()
	}
}

func (n *Node) hasWatchers() bool {
	n.watchMu.Lock()
	defer n.watchMu.Unlock()
	return len(n.watchers) > 0
}

// broadcast never blocks: a slow watcher misses events, but every status
// carries the state version, so the app catches up on the next one.
func (n *Node) broadcast(e coreapi.TailscaleEvent) {
	n.watchMu.Lock()
	defer n.watchMu.Unlock()
	for ch := range n.watchers {
		select {
		case ch <- e:
		default:
		}
	}
}

// Close stops the node.
func (n *Node) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stopLocked()
}

// restartShareLocked serves the mixed proxy to the tailnet when asked.
func (n *Node) restartShareLocked() {
	if n.share != nil {
		_ = n.share.Close()
		n.share = nil
	}
	cfg := n.cfg
	if n.srv == nil || cfg == nil || cfg.ShareProxyPort == 0 || cfg.ShareProxyTarget == "" {
		return
	}
	ln, err := n.srv.Listen("tcp", fmt.Sprintf(":%d", cfg.ShareProxyPort))
	if err != nil {
		log.Warnln("[Tailscale] share proxy: %v", err)
		return
	}
	n.share = ln
	go n.serveShare(ln, n.lc, cfg.ShareProxyTarget)
}

// serveShare forwards connections from the tailnet to the mixed proxy.
// Only devices of the node's own user may use it.
func (n *Node) serveShare(ln net.Listener, lc *local.Client, target string) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			who, err := lc.WhoIs(ctx, c.RemoteAddr().String())
			cancel()
			st := n.Status()
			if err != nil || who.UserProfile == nil || st.User == nil || who.UserProfile.LoginName != st.User.LoginName {
				log.Warnln("[Tailscale] share proxy: refused %s, not a device of %s", c.RemoteAddr(), loginName(st))
				return
			}
			up, err := net.DialTimeout("tcp", target, 5*time.Second)
			if err != nil {
				return
			}
			defer up.Close()
			done := make(chan struct{}, 2)
			go func() { _, _ = io.Copy(up, c); done <- struct{}{} }()
			go func() { _, _ = io.Copy(c, up); done <- struct{}{} }()
			<-done
		}()
	}
}

func loginName(st coreapi.TailscaleStatus) string {
	if st.User == nil {
		return "the node's user"
	}
	return st.User.LoginName
}

// Store keeps the node's state in memory and reports every write, so that
// the app persists it, encrypted, instead of tailscaled.state on disk.
type Store struct {
	mu      sync.Mutex
	state   map[string][]byte
	ver     uint64
	onWrite func(version uint64)
}

func newStore(onWrite func(uint64)) *Store {
	return &Store{state: map[string][]byte{}, onWrite: onWrite}
}

func (s *Store) load(state map[string][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = maps.Clone(state)
}

func (s *Store) version() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ver
}

func (s *Store) snapshot() (uint64, map[string][]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ver, maps.Clone(s.state)
}

// ReadState implements ipn.StateStore.
func (s *Store) ReadState(id ipn.StateKey) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.state[string(id)]
	if !ok {
		return nil, ipn.ErrStateNotExist
	}
	return append([]byte(nil), v...), nil
}

// WriteState implements ipn.StateStore.
func (s *Store) WriteState(id ipn.StateKey, bs []byte) error {
	s.mu.Lock()
	if bs == nil {
		delete(s.state, string(id))
	} else {
		s.state[string(id)] = append([]byte(nil), bs...)
	}
	s.ver++
	v := s.ver
	s.mu.Unlock()
	if s.onWrite != nil {
		s.onWrite(v)
	}
	return nil
}
