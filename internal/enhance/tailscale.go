package enhance

import (
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// Tailnet address ranges: the CGNAT range Tailscale assigns IPv4
// addresses from, and its IPv6 ULA prefix.
const (
	TailnetIPv4 = "100.64.0.0/10"
	TailnetIPv6 = "fd7a:115c:a1e0::/48"
	MagicDNSIP  = "100.100.100.100"
)

// Tailnet describes how the configuration reaches a tailnet.
type Tailnet struct {
	// Mode is "embedded", for the node the core runs, reached through the
	// outbound named ProxyName, or "system", for the Tailscale client
	// installed on the computer, which traffic reaches directly.
	Mode string
	// ProxyName names the outbound of the embedded node.
	ProxyName string
	// Proxy is the outbound's entry in proxies (embedded mode).
	Proxy *yamlx.Map
	// JoinSelector adds the outbound to the first selector group, so that
	// all traffic can go out through an exit node.
	JoinSelector bool
	// Suffixes are the tailnet's MagicDNS domains, e.g. tail1234.ts.net.
	Suffixes []string
	// Routes are subnets peers advertise, which go through the tailnet
	// too (embedded mode, with accepted routes).
	Routes []string
	// Interface is the network interface of the installed client, which
	// the DNS server binds to while TUN binds everything else to the
	// physical interface (system mode).
	Interface string
	// MagicDNS resolves tailnet names through Tailscale's DNS.
	MagicDNS bool
}

// ApplyTailscale routes tailnet traffic and names. It returns notes for
// the log, such as a proxy name the profile already used.
func ApplyTailscale(cfg *yamlx.Map, t Tailnet) []LogEntry {
	var logs []LogEntry
	target := "DIRECT"
	if t.Mode == "embedded" {
		if t.ProxyName == "" || t.Proxy == nil {
			return nil
		}
		proxies, groups := ProxyNames(cfg)
		if slices.Contains(proxies, t.ProxyName) || slices.Contains(groups, t.ProxyName) {
			logs = append(logs, LogEntry{"warn", fmt.Sprintf("the profile already has a proxy named %q; the tailnet is not routed", t.ProxyName)})
			return logs
		}
		entry := t.Proxy.Clone()
		entry.SetFirst("name", t.ProxyName)
		cfg.Set("proxies", append(cfg.Slice("proxies"), entry))
		if t.JoinSelector {
			joinFirstSelector(cfg, t.ProxyName)
		}
		target = t.ProxyName
	}

	var rules []any
	for _, s := range suffixes(t) {
		rules = append(rules, "DOMAIN-SUFFIX,"+s+","+target)
	}
	rules = append(rules,
		"IP-CIDR,"+TailnetIPv4+","+target+",no-resolve",
		"IP-CIDR6,"+TailnetIPv6+","+target+",no-resolve",
	)
	if t.Mode == "embedded" {
		for _, r := range t.Routes {
			p, err := netip.ParsePrefix(r)
			if err != nil || p.Bits() == 0 { // exit node routes are not subnets
				continue
			}
			kind := "IP-CIDR"
			if p.Addr().Is6() {
				kind = "IP-CIDR6"
			}
			rules = append(rules, kind+","+p.String()+","+target+",no-resolve")
		}
	}
	ApplySeq(cfg, "rules", SeqPatch{Prepend: rules})

	if t.MagicDNS {
		applyTailnetDNS(cfg, t)
	}
	if t.Mode == "system" {
		if tun := cfg.Map("tun"); tun != nil {
			if on, _ := tun.Bool("enable"); on {
				addUnique(tun, "route-exclude-address", TailnetIPv4, TailnetIPv6)
			}
		}
	}
	return logs
}

func suffixes(t Tailnet) []string {
	out := []string{"ts.net"}
	for _, s := range t.Suffixes {
		s = strings.Trim(strings.ToLower(s), ".")
		if s != "" && !slices.Contains(out, s) && !strings.HasSuffix(s, ".ts.net") {
			out = append(out, s)
		}
	}
	return out
}

func applyTailnetDNS(cfg *yamlx.Map, t Tailnet) {
	dns := cfg.Map("dns")
	if dns == nil {
		return // the core resolves names with the system; Tailscale's own DNS applies
	}
	server := MagicDNSIP
	switch {
	case t.Mode == "embedded":
		server = "ts://" + t.ProxyName
	case t.Interface != "":
		server = MagicDNSIP + "#" + t.Interface
	}
	policy := dns.EnsureMap("nameserver-policy")
	for _, s := range suffixes(t) {
		key := "+." + s
		if !policy.Has(key) {
			policy.Set(key, server)
		}
	}
	// Tailnet names resolve to their real addresses, which the IP rules
	// route, rather than to fake ones.
	if mode := dns.String("enhanced-mode"); mode == "" || mode == "fake-ip" {
		var filters []any
		for _, s := range suffixes(t) {
			filters = append(filters, "+."+s)
		}
		addUnique(dns, "fake-ip-filter", filters...)
	}
}

func joinFirstSelector(cfg *yamlx.Map, name string) {
	for _, g := range cfg.Slice("proxy-groups") {
		gm, ok := g.(*yamlx.Map)
		if !ok || !isSelector(gm) {
			continue
		}
		members := gm.Slice("proxies")
		for _, m := range members {
			if m == name {
				return
			}
		}
		gm.Set("proxies", append(members, name))
		return
	}
}

func addUnique(m *yamlx.Map, key string, values ...any) {
	list := m.Slice(key)
	for _, v := range values {
		if !slices.ContainsFunc(list, func(e any) bool { return yamlx.Equal(e, v) }) {
			list = append(list, v)
		}
	}
	m.Set(key, list)
}
