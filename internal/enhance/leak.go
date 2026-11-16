package enhance

import (
	"fmt"
	"strings"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// systemFallback is what a list of name servers that only asked the
// system asks instead, by key, when the list cannot be left empty: the
// domestic servers of DefaultDNSConfig, named by address so that nothing
// has to be resolved first. Lists left out go when they empty.
var systemFallback = map[string][]string{
	"default-nameserver":      {"223.5.5.5", "119.29.29.29"},
	"nameserver":              {"https://223.5.5.5/dns-query", "https://120.53.53.53/dns-query"},
	"proxy-server-nameserver": {"https://223.5.5.5/dns-query", "https://120.53.53.53/dns-query"},
}

// AvoidSystemDNS takes the system's resolver out of cfg's dns section,
// for TUN with dns-mode native: the system's name server is then the
// core itself, behind TUN, so a query the core sends "the system" would
// only come back to it. Nothing else of the profile's DNS changes.
func AvoidSystemDNS(cfg *yamlx.Map) []LogEntry {
	dns := cfg.Map("dns")
	if dns == nil {
		return nil
	}
	var changed []string
	for _, key := range []string{"default-nameserver", "nameserver", "fallback", "proxy-server-nameserver", "direct-nameserver"} {
		list := dns.Slice(key)
		out := withoutSystem(list)
		if len(out) == len(list) {
			continue
		}
		changed = append(changed, key)
		switch {
		case len(out) > 0:
			dns.Set(key, out)
		case systemFallback[key] != nil:
			dns.Set(key, strAny(systemFallback[key]))
		default:
			dns.Delete(key)
		}
	}
	if policy := dns.Map("nameserver-policy"); policy != nil {
		var empty []string
		policy.Range(func(k string, v any) bool {
			var list []any
			switch v := v.(type) {
			case []any:
				list = v
			case string:
				list = []any{v}
			default:
				return true
			}
			out := withoutSystem(list)
			if len(out) == len(list) {
				return true
			}
			if len(out) == 0 {
				empty = append(empty, k)
			} else {
				policy.Set(k, out)
			}
			if len(changed) == 0 || changed[len(changed)-1] != "nameserver-policy" {
				changed = append(changed, "nameserver-policy")
			}
			return true
		})
		for _, k := range empty {
			policy.Delete(k)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return []LogEntry{{"info", fmt.Sprintf("dns: %s no longer ask the system, whose name server is the core itself under TUN", strings.Join(changed, ", "))}}
}

// withoutSystem returns list without the entries that ask the system's
// resolver.
func withoutSystem(list []any) []any {
	out := make([]any, 0, len(list))
	for _, v := range list {
		if s, ok := v.(string); ok && isSystemNameServer(s) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func isSystemNameServer(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "system" || s == "dhcp://system" || strings.HasPrefix(s, "system://")
}
