package corehost

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	mhttp "github.com/metacubex/http"
	"github.com/metacubex/mihomo/component/resolver"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/tunnel"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

// normalizeTarget reads a domain, an IP address or a URL as typed, and
// returns the host to match, lower case without brackets or a trailing
// dot, and the port when the input names one (0 otherwise).
func normalizeTarget(in string) (host string, port int, err error) {
	s := strings.TrimSpace(in)
	scheme := ""
	if i := strings.Index(s, "://"); i >= 0 {
		scheme, s = strings.ToLower(s[:i]), s[i+3:]
	}
	if i := strings.IndexAny(s, "/?#"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if _, ok := parseAddr(s); !ok {
		if h, p, e := net.SplitHostPort(s); e == nil {
			s = h
			port, _ = strconv.Atoi(p)
		}
	}
	s = strings.Trim(s, "[]")
	if port == 0 {
		switch scheme {
		case "http", "ws":
			port = 80
		case "https", "wss":
			port = 443
		}
	}
	s = strings.ToLower(strings.TrimSuffix(s, "."))
	if s == "" || strings.ContainsAny(s, " \t") {
		return "", 0, errors.New("no host in " + strconv.Quote(in))
	}
	if port < 0 || port > 65535 {
		port = 0
	}
	return s, port, nil
}

func parseAddr(s string) (netip.Addr, bool) {
	a, err := netip.ParseAddr(strings.Trim(s, "[]"))
	return a.Unmap(), err == nil
}

// walkChain follows the selections from p, a proxy or a group, down to
// the proxy that dials.
func walkChain(p C.Proxy, md *C.Metadata) []coreapi.MatchHop {
	chain := []coreapi.MatchHop{}
	for i := 0; p != nil && i < 32; i++ {
		chain = append(chain, coreapi.MatchHop{Name: p.Name(), Type: p.Type().String()})
		p = p.Unwrap(md, false)
	}
	return chain
}

// matchRule decides what the core would do with a connection to the
// request's target, as the tunnel does when it dials: the mode first,
// then the rules in order, resolving the name when a rule needs the
// address. It changes no state, not even the hit counts of the rules.
func matchRule(ctx context.Context, req coreapi.MatchRequest) (coreapi.MatchResult, error) {
	host, port, err := normalizeTarget(req.Target)
	if err != nil {
		return coreapi.MatchResult{}, err
	}
	if req.Port > 0 && req.Port <= 65535 {
		port = req.Port
	}
	if port == 0 {
		port = 443
	}
	md := &C.Metadata{NetWork: C.TCP, Type: C.INNER, DstPort: uint16(port), Process: req.Process}
	network := "tcp"
	if strings.EqualFold(req.Network, "udp") {
		md.NetWork, network = C.UDP, "udp"
	}
	var ips []netip.Addr
	if ip, ok := parseAddr(host); ok {
		md.DstIP = ip
		ips = append(ips, ip)
		// An address the core handed out for a name matches as that name.
		if name, found := resolver.FindHostByIP(ip); found && !resolver.IsFakeIP(ip) {
			md.Host = name
		}
	} else {
		md.Host = host
	}

	mode := tunnel.Mode()
	res := coreapi.MatchResult{Host: host, Network: network, Port: port, Mode: mode.String(), Index: -1, Chain: []coreapi.MatchHop{}, IPs: []string{}}
	proxies := tunnel.Proxies()
	var proxy C.Proxy

	switch mode {
	case tunnel.Direct:
		res.Source, proxy = "mode", proxies["DIRECT"]
	case tunnel.Global:
		res.Source, proxy = "mode", proxies["GLOBAL"]
	default:
		resolved := false
		if node, ok := resolver.DefaultHosts.Search(md.Host, false); ok && md.Host != "" {
			if ip, err := node.RandIP(); err == nil {
				md.DstIP = ip
				ips = append(ips, ip)
			}
			resolved = true
		}
		helper := C.RuleMatchHelper{
			ResolveIP: func() {
				if resolved || md.Host == "" || md.Resolved() {
					return
				}
				resolved = true
				rctx, cancel := context.WithTimeout(ctx, resolver.DefaultDNSTimeout)
				defer cancel()
				if ip, err := resolver.ResolveIP(rctx, md.Host); err == nil {
					md.DstIP = ip
					ips = append(ips, ip)
				}
			},
			CheckPassRule: func(name string) bool {
				for a := proxies[name]; a != nil; a = a.Unwrap(md, false) {
					if a.Type() == C.PassRule {
						return true
					}
				}
				return false
			},
		}
		for _, rule := range tunnel.Rules() {
			if w, ok := rule.(C.RuleWrapper); ok && w.IsDisabled() {
				res.Disabled++
			}
		}
		res.Source = "none"
		res.Policy = "DIRECT"
		proxy = proxies["DIRECT"]
	Rules:
		for i, rule := range tunnel.Rules() {
			// The wrapper counts hits; ask the rule itself.
			if w, ok := rule.(C.RuleWrapper); ok {
				if w.IsDisabled() {
					continue
				}
				rule = w.Unwrap()
			}
			matched, name := rule.Match(md, helper)
			if !matched {
				continue
			}
			adapter, ok := proxies[name]
			if !ok {
				continue
			}
			for a := adapter; a != nil; a = a.Unwrap(md, false) {
				if a.Type() == C.Pass {
					continue Rules
				}
			}
			if md.NetWork == C.UDP && !adapter.SupportUDP() {
				continue
			}
			res.Source, res.Index, res.RuleType, res.Payload, proxy = "rule", i, rule.RuleType().String(), rule.Payload(), adapter
			break
		}
	}
	if proxy == nil {
		return res, errors.New("the core has no proxy for this decision")
	}
	res.Policy = proxy.Name()
	res.Chain = walkChain(proxy, md)
	for _, ip := range ips {
		res.IPs = append(res.IPs, ip.String())
	}
	return res, nil
}

func (h *host) rulesMatch(w mhttp.ResponseWriter, r *mhttp.Request) {
	var req coreapi.MatchRequest
	if !readJSON(w, r, &req) {
		return
	}
	res, err := matchRule(r.Context(), req)
	if err != nil {
		code := http.StatusInternalServerError
		if res.Host == "" {
			code = http.StatusBadRequest
		}
		writeError(w, code, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
