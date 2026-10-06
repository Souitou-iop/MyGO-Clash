// Package tailnet connects the app to Tailscale networks in two ways:
//
//   - Node, in the core, an embedded Tailscale node (tsnet, userspace) that
//     mihomo reaches through an outbound and a DNS client, and that lives
//     across configuration changes;
//   - System, in the app, a client of the Tailscale app installed on the
//     computer, through its local API, to show and drive it and to keep the
//     core's TUN from fighting with it.
package tailnet

import (
	"net/netip"
	"slices"
	"strings"
	"time"

	"github.com/metacubex/tailscale/ipn"
	"github.com/metacubex/tailscale/ipn/ipnstate"
	"github.com/metacubex/tailscale/tailcfg"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

// FromStatus converts Tailscale's status and preferences into the app's.
func FromStatus(st *ipnstate.Status, prefs *ipn.Prefs, source string) coreapi.TailscaleStatus {
	out := coreapi.TailscaleStatus{Source: source, Available: st != nil, UpdatedAt: time.Now(), Peers: []coreapi.TailscalePeer{}}
	if st == nil {
		return out
	}
	out.BackendState = st.BackendState
	out.AuthURL = st.AuthURL
	out.Version = st.Version
	out.Health = slices.Clone(st.Health)
	out.MagicDNSSuffix = strings.TrimSuffix(st.MagicDNSSuffix, ".")
	if t := st.CurrentTailnet; t != nil {
		out.TailnetName = t.Name
		out.MagicDNS = t.MagicDNSEnabled
		if t.MagicDNSSuffix != "" {
			out.MagicDNSSuffix = strings.TrimSuffix(t.MagicDNSSuffix, ".")
		}
	}
	userName := func(id tailcfg.UserID) string {
		if u, ok := st.User[id]; ok {
			return u.LoginName
		}
		return ""
	}
	if st.Self != nil {
		self := peer(st.Self, userName)
		out.Self = &self
		if u, ok := st.User[st.Self.UserID]; ok {
			out.User = &coreapi.TailscaleUser{LoginName: u.LoginName, DisplayName: u.DisplayName, ProfilePicURL: u.ProfilePicURL}
		}
	}
	routes := map[string]bool{}
	for _, ps := range st.Peer {
		p := peer(ps, userName)
		if p.ExitNode {
			out.ExitNodeID = p.ID
		}
		for _, r := range p.PrimaryRoutes {
			routes[r] = true
		}
		out.Peers = append(out.Peers, p)
	}
	slices.SortFunc(out.Peers, func(a, b coreapi.TailscalePeer) int {
		if a.Online != b.Online {
			if a.Online {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.HostName), strings.ToLower(b.HostName))
	})
	for r := range routes {
		out.Routes = append(out.Routes, r)
	}
	slices.Sort(out.Routes)
	if prefs != nil {
		out.AcceptRoutes = prefs.RouteAll
		out.ShieldsUp = prefs.ShieldsUp
		if out.ExitNodeID == "" && prefs.ExitNodeID != "" {
			out.ExitNodeID = string(prefs.ExitNodeID)
		}
	}
	return out
}

func peer(ps *ipnstate.PeerStatus, userName func(tailcfg.UserID) string) coreapi.TailscalePeer {
	p := coreapi.TailscalePeer{
		ID:             string(ps.ID),
		HostName:       ps.HostName,
		DNSName:        strings.TrimSuffix(ps.DNSName, "."),
		OS:             ps.OS,
		User:           userName(ps.UserID),
		Online:         ps.Online,
		Active:         ps.Active,
		ExitNode:       ps.ExitNode,
		ExitNodeOption: ps.ExitNodeOption,
		Relay:          ps.Relay,
		CurAddr:        ps.CurAddr,
		RxBytes:        ps.RxBytes,
		TxBytes:        ps.TxBytes,
		Expired:        ps.Expired,
		TailscaleIPs:   []string{},
	}
	for _, ip := range ps.TailscaleIPs {
		p.TailscaleIPs = append(p.TailscaleIPs, ip.String())
	}
	if !ps.LastSeen.IsZero() {
		t := ps.LastSeen
		p.LastSeen = &t
	}
	if ps.KeyExpiry != nil {
		t := *ps.KeyExpiry
		p.KeyExpiry = &t
	}
	if ps.PrimaryRoutes != nil {
		for _, r := range ps.PrimaryRoutes.All() {
			p.PrimaryRoutes = append(p.PrimaryRoutes, r.String())
		}
	}
	if ps.Tags != nil {
		for _, t := range ps.Tags.All() {
			p.Tags = append(p.Tags, t)
		}
	}
	return p
}

// exitNodePrefs builds the preferences that select an exit node: a peer's
// IP, stable ID or DNS name, an "auto:" expression, or "" for none.
func exitNodePrefs(target string, st *ipnstate.Status) (*ipn.MaskedPrefs, error) {
	mp := &ipn.MaskedPrefs{ExitNodeIDSet: true, ExitNodeIPSet: true, AutoExitNodeSet: true}
	target = strings.TrimSpace(target)
	if target == "" {
		return mp, nil
	}
	if expr, ok := ipn.ParseAutoExitNodeString(target); ok {
		mp.AutoExitNode = expr
		mp.ExitNodeIDSet, mp.ExitNodeIPSet = false, false
		return mp, nil
	}
	if st != nil {
		for _, ps := range st.Peer {
			if string(ps.ID) == target {
				mp.ExitNodeID = ps.ID
				return mp, nil
			}
		}
	}
	if err := mp.SetExitNodeIP(target, st); err != nil {
		return nil, err
	}
	return mp, nil
}

// pingType parses the type of a ping.
func pingType(s string) tailcfg.PingType {
	switch strings.ToUpper(s) {
	case "TSMP":
		return tailcfg.PingTSMP
	case "ICMP":
		return tailcfg.PingICMP
	default:
		return tailcfg.PingDisco
	}
}

func pingResult(r *ipnstate.PingResult) coreapi.TailscalePingResult {
	out := coreapi.TailscalePingResult{
		LatencyMs: r.LatencySeconds * 1000,
		Endpoint:  r.Endpoint,
		Err:       r.Err,
	}
	if r.Endpoint == "" && r.DERPRegionCode != "" {
		out.DERP = r.DERPRegionCode
	}
	return out
}

func parseIP(s string) (netip.Addr, error) {
	return netip.ParseAddr(strings.TrimSpace(s))
}
