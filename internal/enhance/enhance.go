// Package enhance transforms Clash configurations: the merge, script and
// sequence (prepend/append/delete) extensions users attach to profiles, and
// the fields the app owns, such as ports, TUN and DNS.
//
// Every function works on an order-preserving *yamlx.Map and is pure: given
// the same inputs it produces the same configuration, which makes the
// pipeline testable without a core.
package enhance

import (
	"fmt"
	"slices"
	"strings"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// LogEntry is a line a script printed, or a note about an extension.
type LogEntry struct {
	Level   string `json:"level"` // log, info, warn, error, debug, table, exception
	Message string `json:"message"`
}

// SeqPatch is a rules, proxies or groups extension.
type SeqPatch struct {
	Prepend []any    `json:"prepend"`
	Append  []any    `json:"append"`
	Delete  []string `json:"delete"`
}

// Empty reports whether the patch changes nothing.
func (p SeqPatch) Empty() bool {
	return len(p.Prepend) == 0 && len(p.Append) == 0 && len(p.Delete) == 0
}

// ParseSeq reads a sequence extension: a YAML mapping with prepend, append
// and delete.
func ParseSeq(data []byte) (SeqPatch, error) {
	m, err := yamlx.Parse(data)
	if err != nil {
		return SeqPatch{}, err
	}
	var p SeqPatch
	p.Prepend = m.Slice("prepend")
	p.Append = m.Slice("append")
	for _, d := range m.Slice("delete") {
		if s, ok := d.(string); ok {
			p.Delete = append(p.Delete, s)
		} else if dm, ok := d.(*yamlx.Map); ok && dm.String("name") != "" {
			p.Delete = append(p.Delete, dm.String("name"))
		}
	}
	return p, nil
}

// itemName returns the name of a proxy or group entry, or the text of a rule.
func itemName(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case *yamlx.Map:
		return v.String("name")
	}
	return ""
}

// ApplySeq applies a sequence extension to the list under field ("rules",
// "proxies" or "proxy-groups"). Proxies it adds also join the first
// selector group, so that users can pick them, and deleted proxies leave
// every group.
func ApplySeq(cfg *yamlx.Map, field string, p SeqPatch) {
	if p.Empty() {
		return
	}
	deleted := map[string]bool{}
	for _, d := range p.Delete {
		deleted[d] = true
	}
	out := make([]any, 0, len(p.Prepend)+len(cfg.Slice(field))+len(p.Append))
	out = append(out, yamlx.CloneValue(p.Prepend).([]any)...)
	for _, item := range cfg.Slice(field) {
		if name := itemName(item); name != "" && deleted[name] {
			continue
		}
		out = append(out, item)
	}
	out = append(out, yamlx.CloneValue(p.Append).([]any)...)
	cfg.Set(field, out)

	if field != "proxies" {
		return
	}
	var added []string
	seen := map[string]bool{}
	for _, item := range append(slices.Clone(p.Prepend), p.Append...) {
		if name := itemName(item); name != "" && !seen[name] {
			seen[name] = true
			added = append(added, name)
		}
	}
	joined := false
	for _, g := range cfg.Slice("proxy-groups") {
		gm, ok := g.(*yamlx.Map)
		if !ok {
			continue
		}
		members, hasMembers := gm.Get("proxies")
		list, isList := members.([]any)
		if hasMembers && !isList {
			continue
		}
		kept := make([]any, 0, len(list))
		for _, m := range list {
			if s, ok := m.(string); ok && deleted[s] {
				continue
			}
			kept = append(kept, m)
		}
		if !joined && len(added) > 0 && isSelector(gm) {
			merged := make([]any, 0, len(added)+len(kept))
			have := map[string]bool{}
			for _, n := range added {
				have[n] = true
				merged = append(merged, n)
			}
			for _, m := range kept {
				if s, ok := m.(string); ok && have[s] {
					continue
				}
				merged = append(merged, m)
			}
			kept = merged
			joined = true
		}
		if hasMembers || len(kept) > 0 {
			gm.Set("proxies", kept)
		}
	}
}

func isSelector(g *yamlx.Map) bool {
	t := strings.ToLower(g.String("type"))
	return t == "select" || t == "selector"
}

// mergeSeqKeys are the keys of merge extensions that prepend or append to a
// list, as Clash for Windows' mixins did, instead of replacing it.
var mergeSeqKeys = map[string]struct {
	field   string
	prepend bool
}{
	"prepend-rules":        {"rules", true},
	"append-rules":         {"rules", false},
	"prepend-proxies":      {"proxies", true},
	"append-proxies":       {"proxies", false},
	"prepend-proxy-groups": {"proxy-groups", true},
	"append-proxy-groups":  {"proxy-groups", false},
}

// Merge applies a merge extension: its keys (lowercased) replace or deeply
// merge into the configuration. "dns" extends the existing mapping one level
// deep, "hosts" replaces it, and the prepend-*/append-* keys add to lists.
func Merge(cfg, patch *yamlx.Map) {
	patch.Range(func(rawKey string, v any) bool {
		key := strings.ToLower(rawKey)
		if sk, ok := mergeSeqKeys[key]; ok {
			items, _ := v.([]any)
			p := SeqPatch{}
			if sk.prepend {
				p.Prepend = items
			} else {
				p.Append = items
			}
			ApplySeq(cfg, sk.field, p)
			return true
		}
		v = yamlx.CloneValue(v)
		existing, has := cfg.Get(key)
		switch {
		case key == "hosts" || !has:
			cfg.Set(key, v)
		case key == "dns":
			em, eok := existing.(*yamlx.Map)
			vm, vok := v.(*yamlx.Map)
			if eok && vok {
				vm.Range(func(k string, vv any) bool {
					em.Set(k, vv)
					return true
				})
			} else {
				cfg.Set(key, v)
			}
		default:
			cfg.Set(key, deepMerge(existing, v))
		}
		return true
	})
}

func deepMerge(a, b any) any {
	am, aok := a.(*yamlx.Map)
	bm, bok := b.(*yamlx.Map)
	if !aok || !bok {
		return b
	}
	bm.Range(func(k string, v any) bool {
		if ev, ok := am.Get(k); ok {
			am.Set(k, deepMerge(ev, v))
		} else {
			am.Set(k, v)
		}
		return true
	})
	return am
}

// LowercaseKeys lowercases the top-level keys, as Clash reads them.
func LowercaseKeys(cfg *yamlx.Map) *yamlx.Map {
	out := yamlx.NewMap()
	cfg.Range(func(k string, v any) bool {
		out.Set(strings.ToLower(k), v)
		return true
	})
	return out
}

// TopKeys returns the lowercased top-level keys of a map.
func TopKeys(m *yamlx.Map) []string {
	keys := make([]string, 0, m.Len())
	for _, k := range m.Keys() {
		keys = append(keys, strings.ToLower(k))
	}
	return keys
}

// handleFields are the keys the app owns, first in the runtime
// configuration; defaultFields come last, in this order.
var (
	handleFields = []string{
		"mode", "redir-port", "tproxy-port", "mixed-port", "socks-port", "port",
		"allow-lan", "bind-address", "log-level", "ipv6", "external-controller",
		"external-controller-unix", "external-controller-pipe", "secret", "unified-delay",
	}
	defaultFields = []string{"proxies", "proxy-providers", "proxy-groups", "rule-providers", "rules"}
)

// Sort orders the top-level keys for reading: the fields the app owns
// first, then the rest as they were, then proxies, groups, providers and
// rules.
func Sort(cfg *yamlx.Map) *yamlx.Map {
	out := yamlx.NewMap()
	for _, k := range handleFields {
		if v, ok := cfg.Get(k); ok {
			out.Set(k, v)
		}
	}
	cfg.Range(func(k string, v any) bool {
		if !slices.Contains(handleFields, k) && !slices.Contains(defaultFields, k) {
			out.Set(k, v)
		}
		return true
	})
	for _, k := range defaultFields {
		if v, ok := cfg.Get(k); ok {
			out.Set(k, v)
		}
	}
	return out
}

// BuiltinPolicies are the targets every configuration has.
var BuiltinPolicies = []string{"DIRECT", "REJECT", "REJECT-DROP", "PASS", "PASS-RULE", "COMPATIBLE"}

// CleanupGroups removes members of proxy groups that name nothing: proxies,
// groups and providers that an extension deleted. A group that uses a
// provider keeps its members, which may come from it.
func CleanupGroups(cfg *yamlx.Map) []LogEntry {
	allowed := map[string]bool{}
	for _, p := range BuiltinPolicies {
		allowed[p] = true
	}
	for _, p := range cfg.Slice("proxies") {
		if n := itemName(p); n != "" {
			allowed[n] = true
		}
	}
	for _, g := range cfg.Slice("proxy-groups") {
		if n := itemName(g); n != "" {
			allowed[n] = true
		}
	}
	providers := map[string]bool{}
	for _, k := range cfg.Map("proxy-providers").Keys() {
		providers[k] = true
	}
	var logs []LogEntry
	for _, g := range cfg.Slice("proxy-groups") {
		gm, ok := g.(*yamlx.Map)
		if !ok {
			continue
		}
		hasProvider := false
		if uses := gm.Slice("use"); uses != nil {
			kept := uses[:0:0]
			for _, u := range uses {
				if s, ok := u.(string); ok && providers[s] {
					kept = append(kept, s)
					hasProvider = true
				} else {
					logs = append(logs, LogEntry{"warn", fmt.Sprintf("group %q uses the missing provider %v", gm.String("name"), u)})
				}
			}
			gm.Set("use", kept)
		}
		if members := gm.Slice("proxies"); members != nil && !hasProvider {
			kept := members[:0:0]
			for _, m := range members {
				if s, ok := m.(string); ok && !allowed[s] {
					logs = append(logs, LogEntry{"warn", fmt.Sprintf("group %q lists the missing proxy %q", gm.String("name"), s)})
					continue
				}
				kept = append(kept, m)
			}
			gm.Set("proxies", kept)
		}
	}
	return logs
}

// ApplyBuiltins makes old profiles work with mihomo: script mode becomes
// rule mode, and hysteria's string alpn becomes a list.
func ApplyBuiltins(cfg *yamlx.Map) {
	if strings.EqualFold(cfg.String("mode"), "script") {
		cfg.Set("mode", "rule")
	}
	for _, p := range cfg.Slice("proxies") {
		pm, ok := p.(*yamlx.Map)
		if !ok || pm.String("type") != "hysteria" {
			continue
		}
		if s, ok := pm.Value("alpn").(string); ok {
			pm.Set("alpn", []any{s})
		}
	}
}

// EnsureLANBind makes allow-lan listen on every interface when the profile
// pinned bind-address to the loopback.
func EnsureLANBind(cfg *yamlx.Map) {
	allow, _ := cfg.Bool("allow-lan")
	if !allow {
		return
	}
	if isLoopback(cfg.String("bind-address")) {
		cfg.Set("bind-address", "*")
	}
}

func isLoopback(addr string) bool {
	addr = strings.Trim(strings.TrimSpace(addr), "[]")
	return strings.EqualFold(addr, "localhost") || strings.HasPrefix(addr, "127.") || addr == "::1"
}

// ProxyNames returns the names of the proxies and groups of a configuration.
func ProxyNames(cfg *yamlx.Map) (proxies, groups []string) {
	for _, p := range cfg.Slice("proxies") {
		if n := itemName(p); n != "" {
			proxies = append(proxies, n)
		}
	}
	for _, g := range cfg.Slice("proxy-groups") {
		if n := itemName(g); n != "" {
			groups = append(groups, n)
		}
	}
	return proxies, groups
}
