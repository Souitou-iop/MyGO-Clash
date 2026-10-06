package tailnet

import (
	"context"
	"net"
	"net/netip"
	"time"

	"github.com/metacubex/tailscale/client/local"
	"github.com/metacubex/tailscale/ipn"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

// System is a client of the Tailscale app installed on the computer,
// through its local API: tailscaled's socket, or the port and token of the
// macOS app.
type System struct {
	lc local.Client
}

// NewSystem returns a client of the installed Tailscale.
func NewSystem() *System { return &System{} }

// Status returns the installed client's status. Available is false when
// it is not installed or not running.
func (s *System) Status(ctx context.Context) coreapi.TailscaleStatus {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	st, err := s.lc.Status(ctx)
	if err != nil {
		out := coreapi.TailscaleStatus{Source: "system", BackendState: "NoState", Peers: []coreapi.TailscalePeer{}, UpdatedAt: time.Now()}
		out.Error = err.Error()
		return out
	}
	prefs, _ := s.lc.GetPrefs(ctx)
	out := FromStatus(st, prefs, "system")
	if out.Self != nil {
		out.Interface = interfaceOf(out.Self.TailscaleIPs)
	}
	return out
}

// interfaceOf finds the network interface that has one of the addresses.
func interfaceOf(ips []string) string {
	want := map[netip.Addr]bool{}
	for _, s := range ips {
		if ip, err := netip.ParseAddr(s); err == nil {
			want[ip] = true
		}
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if p, err := netip.ParsePrefix(a.String()); err == nil && want[p.Addr()] {
				return iface.Name
			}
		}
	}
	return ""
}

// SetRunning connects or disconnects the installed client, as its Connect
// and Disconnect menu items do.
func (s *System) SetRunning(ctx context.Context, on bool) error {
	_, err := s.lc.EditPrefs(ctx, &ipn.MaskedPrefs{Prefs: ipn.Prefs{WantRunning: on}, WantRunningSet: true})
	return err
}

// SetPrefs changes preferences of the installed client.
func (s *System) SetPrefs(ctx context.Context, p coreapi.TailscalePrefs) error {
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
		if _, err := s.lc.EditPrefs(ctx, mp); err != nil {
			return err
		}
	}
	if p.ExitNode != nil {
		st, _ := s.lc.Status(ctx)
		ep, err := exitNodePrefs(*p.ExitNode, st)
		if err != nil {
			return err
		}
		if _, err := s.lc.EditPrefs(ctx, ep); err != nil {
			return err
		}
	}
	return nil
}

// Ping pings a peer through the installed client.
func (s *System) Ping(ctx context.Context, p coreapi.TailscalePing) (coreapi.TailscalePingResult, error) {
	ip, err := parseIP(p.IP)
	if err != nil {
		return coreapi.TailscalePingResult{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	r, err := s.lc.Ping(ctx, ip, pingType(p.Type))
	if err != nil {
		return coreapi.TailscalePingResult{}, err
	}
	return pingResult(r), nil
}

// Watch calls fn with the installed client's status whenever it changes,
// and every interval while nothing does, until ctx ends. It keeps trying
// while the client is not running.
func (s *System) Watch(ctx context.Context, interval time.Duration, fn func(coreapi.TailscaleStatus)) {
	for ctx.Err() == nil {
		fn(s.Status(ctx))
		w, err := s.lc.WatchIPNBus(ctx, ipn.NotifyNoPrivateKeys)
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(interval):
			}
			continue
		}
		events := make(chan struct{}, 1)
		go func() {
			defer close(events)
			for {
				if _, err := w.Next(); err != nil {
					return
				}
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}()
		t := time.NewTicker(interval)
	loop:
		for {
			select {
			case <-ctx.Done():
				break loop
			case _, ok := <-events:
				if !ok {
					break loop
				}
				time.Sleep(300 * time.Millisecond) // coalesce bursts
				fn(s.Status(ctx))
			case <-t.C:
				fn(s.Status(ctx))
			}
		}
		t.Stop()
		_ = w.Close()
	}
}
