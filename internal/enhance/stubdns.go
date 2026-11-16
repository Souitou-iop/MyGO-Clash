package enhance

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"runtime"
	"slices"
	"strings"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// Under TUN on Linux, a name server on this machine is a loop: the "system"
// one is what /etc/resolv.conf names, which on systemd-resolved's
// distributions (Fedora, Ubuntu…) is its stub at 127.0.0.53. The core asks
// the stub, the stub asks its upstream, TUN hijacks that query back to the
// core, and the core answers a fake IP: it then dials its own fake IP and
// every DIRECT connection to a name times out. So the core is given the
// stub's upstreams instead.

// The keys of the dns section that list name servers.
var nameServerKeys = []string{"default-nameserver", "nameserver", "fallback", "proxy-server-nameserver", "direct-nameserver"}

// What a list emptied of local servers falls back to.
var stubFallback = map[string][]string{
	"default-nameserver": {"223.5.5.5", "119.29.29.29"},
	"nameserver":         {"https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"},
}

// AvoidStubDNS replaces the name servers on this machine in cfg's dns
// section with upstreams, the servers the machine's resolver itself asks,
// and reports what it changed. Lists that only had local servers and have
// no upstream to take fall back to public ones, or go when optional.
func AvoidStubDNS(cfg *yamlx.Map, upstreams []string) []LogEntry {
	dns := cfg.Map("dns")
	if dns == nil {
		return nil
	}
	var logs []LogEntry
	fix := func(where string, list []any) ([]any, bool) {
		var out []any
		changed := false
		for _, v := range list {
			s, ok := v.(string)
			if !ok || !isLocalNameServer(s) {
				out = append(out, v)
				continue
			}
			changed = true
			for _, u := range upstreams {
				if !slices.Contains(out, any(u)) {
					out = append(out, u)
				}
			}
		}
		if changed {
			logs = append(logs, LogEntry{"info", fmt.Sprintf("dns.%s: the resolver on this machine would loop through TUN; asking %s instead", where, describeUpstreams(upstreams))})
		}
		return out, changed
	}
	for _, key := range nameServerKeys {
		list := dns.Slice(key)
		out, changed := fix(key, list)
		if !changed {
			continue
		}
		if len(out) == 0 {
			if fb := stubFallback[key]; fb != nil {
				out = strAny(fb)
			} else {
				dns.Delete(key)
				continue
			}
		}
		dns.Set(key, out)
	}
	if policy := dns.Map("nameserver-policy"); policy != nil {
		policy.Range(func(k string, v any) bool {
			var list []any
			switch x := v.(type) {
			case string:
				list = []any{x}
			case []any:
				list = x
			default:
				return true
			}
			out, changed := fix("nameserver-policy."+k, list)
			if changed {
				if len(out) == 0 {
					out = strAny(stubFallback["nameserver"])
				}
				policy.Set(k, out)
			}
			return true
		})
	}
	return logs
}

func describeUpstreams(u []string) string {
	if len(u) == 0 {
		return "public servers"
	}
	return strings.Join(u, ", ")
}

func strAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// isLocalNameServer reports whether s, a name server as mihomo writes
// one, is the system's resolver or a server on this machine.
func isLocalNameServer(s string) bool {
	s = strings.TrimSpace(s)
	if s == "system" || s == "dhcp://system" || strings.HasPrefix(s, "system://") {
		return true
	}
	host := s
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/#"); i >= 0 {
		host = host[:i]
	}
	if ap, err := netip.ParseAddrPort(host); err == nil {
		return ap.Addr().IsLoopback()
	}
	if a, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return a.IsLoopback()
	}
	return host == "localhost" || strings.HasPrefix(host, "localhost:")
}

// StubUpstreams returns, on Linux when the system resolver is a stub on
// this machine, the servers the stub asks (systemd-resolved lists them in
// /run/systemd/resolve/resolv.conf), and whether the resolver is such a
// stub: there is nothing to avoid otherwise.
func StubUpstreams() (upstreams []string, stub bool) {
	if runtime.GOOS != "linux" {
		return nil, false
	}
	return stubUpstreams("/etc/resolv.conf", "/run/systemd/resolve/resolv.conf")
}

func stubUpstreams(resolvConf, resolvedConf string) ([]string, bool) {
	servers := readNameServers(resolvConf)
	if len(servers) == 0 {
		return nil, false
	}
	for _, s := range servers {
		if !s.IsLoopback() {
			return nil, false // the core can ask these itself
		}
	}
	var up []string
	for _, s := range readNameServers(resolvedConf) {
		if !s.IsLoopback() && !slices.Contains(up, s.String()) {
			up = append(up, s.String())
		}
	}
	return up, true
}

// readNameServers reads the nameserver lines of a resolv.conf.
func readNameServers(path string) []netip.Addr {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []netip.Addr
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 && fields[0] == "nameserver" {
			if a, err := netip.ParseAddr(fields[1]); err == nil {
				out = append(out, a.WithZone(""))
			}
		}
	}
	return out
}
