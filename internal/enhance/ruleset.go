package enhance

import (
	"strings"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// RuleSet is a rule set the user added from the rules page: a provider
// the core downloads and refreshes by itself, and a RULE-SET rule that
// sends what it matches to a policy. A policy of "PROXY" means the
// profile's main proxy group, resolved when the configuration is built.
type RuleSet struct {
	ID       string // names the provider and its file: [a-z0-9-]
	URL      string
	Behavior string // domain, ipcidr, classical
	Format   string // mrs, yaml, text; "" is yaml
	Policy   string // DIRECT, REJECT, PROXY
	Enabled  bool
}

// ApplyRuleSets adds the enabled rule sets: a provider each, and their
// RULE-SET rules above the subscription's own rules, in the order the
// sets were added. A set whose policy has nothing to point at, a broken
// id or address is skipped, not an error: the rest still apply.
func ApplyRuleSets(cfg *yamlx.Map, sets []RuleSet) {
	if len(sets) == 0 {
		return
	}
	providers := cfg.EnsureMap("rule-providers")
	var prepend []any
	for _, s := range sets {
		if !s.Enabled || !validID(s.ID) || !strings.HasPrefix(s.URL, "http") {
			continue
		}
		key := "user-" + s.ID
		pm := yamlx.MapOf(
			"type", "http",
			"behavior", s.Behavior,
			"url", s.URL,
			"path", "./rulesets/"+key+"."+ext(s.Format),
			"interval", int64(86400),
		)
		if s.Format != "" {
			pm.Set("format", s.Format)
		}
		providers.Set(key, pm)
		policy := s.Policy
		if policy == "PROXY" {
			policy = mainGroup(cfg)
		}
		if policy == "" {
			continue
		}
		prepend = append(prepend, "RULE-SET,"+key+","+policy)
	}
	if len(prepend) > 0 {
		ApplySeq(cfg, "rules", SeqPatch{Prepend: prepend})
	}
}

// mainGroup is the profile's main proxy group: its first selector, or its
// first group of any kind. "" when the profile has none. The type is
// "select" in a configuration, and "Selector" on the core's API.
func mainGroup(cfg *yamlx.Map) string {
	first := ""
	for _, g := range cfg.Slice("proxy-groups") {
		gm, ok := g.(*yamlx.Map)
		if !ok {
			continue
		}
		name := gm.String("name")
		if name == "" {
			continue
		}
		if first == "" {
			first = name
		}
		if strings.EqualFold(gm.String("type"), "select") {
			return name
		}
	}
	return first
}

// validID keeps the id to the shapes a file name and a provider key take.
func validID(id string) bool {
	if len(id) == 0 || len(id) > 40 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
}

// ext is the file name extension of a provider format.
func ext(format string) string {
	switch format {
	case "mrs":
		return "mrs"
	case "text":
		return "list"
	}
	return "yaml"
}
