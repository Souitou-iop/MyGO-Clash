package enhance

import (
	"regexp"
	"strings"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// ProxyChain fronts the proxies whose name matches with another node or
// group: their traffic goes out through it first.
type ProxyChain struct {
	Match   string // the name of a proxy, or a regular expression of names
	Through string // the name of the node or group that fronts them
	Enabled bool
}

// ApplyChains sets the dialer-proxy of the matching proxies. A pattern that
// does not compile matches by being contained in the name, and a front
// nothing points at, or a chain that would close a loop, is left out: the
// front has to stay in front.
func ApplyChains(cfg *yamlx.Map, chains []ProxyChain) {
	for _, c := range chains {
		if !c.Enabled || strings.TrimSpace(c.Match) == "" || strings.TrimSpace(c.Through) == "" || c.Match == c.Through {
			continue
		}
		re, reErr := regexp.Compile(c.Match)
		through := false
		for _, p := range cfg.Slice("proxies") {
			if pm, ok := p.(*yamlx.Map); ok && pm.String("name") == c.Through {
				through = true
			}
		}
		for _, g := range cfg.Slice("proxy-groups") {
			if gm, ok := g.(*yamlx.Map); ok && gm.String("name") == c.Through {
				through = true
			}
		}
		if !through {
			continue // nothing to front with: the core would refuse the set
		}
		for _, p := range cfg.Slice("proxies") {
			pm, ok := p.(*yamlx.Map)
			if !ok {
				continue
			}
			name := pm.String("name")
			if name == "" || name == c.Through {
				continue
			}
			matched := false
			if reErr == nil {
				matched = re.MatchString(name)
			} else {
				matched = strings.Contains(name, c.Match)
			}
			if matched {
				pm.Set("dialer-proxy", c.Through)
			}
		}
	}
	undoLoops(cfg)
}

// undoLoops takes the dialer-proxy back off the proxies whose chain of
// fronts returns to itself: the core refuses such a configuration.
func undoLoops(cfg *yamlx.Map) {
	fronts := map[string]string{}
	for _, p := range cfg.Slice("proxies") {
		if pm, ok := p.(*yamlx.Map); ok {
			if dp := strings.TrimSpace(pm.String("dialer-proxy")); dp != "" {
				fronts[pm.String("name")] = dp
			}
		}
	}
looped:
	for name := range fronts {
		seen := map[string]bool{name: true}
		cur := name
		for i := 0; i < len(fronts)+1; i++ {
			next, ok := fronts[cur]
			if !ok {
				continue looped
			}
			if seen[next] {
				for _, p := range cfg.Slice("proxies") {
					if pm, ok := p.(*yamlx.Map); ok && pm.String("name") == name {
						pm.Delete("dialer-proxy")
					}
				}
				continue looped
			}
			seen[next] = true
			cur = next
		}
	}
}
