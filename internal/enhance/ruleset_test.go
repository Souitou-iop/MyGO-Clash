package enhance

import (
	"strings"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

func TestApplyRuleSets(t *testing.T) {
	cfg, err := yamlx.Parse([]byte(subscriptionRules))
	if err != nil {
		t.Fatal(err)
	}
	sets := []RuleSet{
		{ID: "ads", URL: "https://example.test/ads.mrs", Behavior: "domain", Format: "mrs", Policy: "REJECT", Enabled: true},
		{ID: "ai", URL: "https://example.test/ai.mrs", Behavior: "domain", Format: "mrs", Policy: "PROXY", Enabled: true},
		{ID: "off", URL: "https://example.test/off.yaml", Behavior: "domain", Policy: "DIRECT", Enabled: false},
		{ID: "bad id!", URL: "https://example.test/x.yaml", Behavior: "domain", Policy: "DIRECT", Enabled: true},
	}
	ApplyRuleSets(cfg, sets)

	p := cfg.Map("rule-providers")
	if p == nil || p.Map("user-ads") == nil || p.Map("user-ai") == nil {
		t.Fatalf("providers missing: %v", cfg.Value("rule-providers"))
	}
	if p.Map("user-off") != nil {
		t.Fatal("the disabled set became a provider")
	}
	ads := p.Map("user-ads")
	interval, _ := ads.Int("interval")
	if ads.String("format") != "mrs" || ads.String("behavior") != "domain" || interval != 86400 {
		t.Fatalf("the provider is wrong: %v", ads)
	}
	rules := cfg.Slice("rules")
	if len(rules) < 2 || !strings.Contains(toString(rules[0]), "user-ads,REJECT") {
		t.Fatalf("the ads rule does not lead: %v", rules[:min(3, len(rules))])
	}
	// PROXY resolves to the profile's first selector group.
	if !strings.Contains(toString(rules[1]), "user-ai,Proxy组") {
		t.Fatalf("PROXY did not resolve to the selector: %v", rules[1])
	}
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

const subscriptionRules = `
proxies: [{name: JP, type: ss, server: jp.example, port: 443, cipher: aes-128-gcm, password: x}]
proxy-groups:
  - {name: URLTest, type: url-test, proxies: [JP]}
  - {name: Proxy组, type: select, proxies: [JP]}
rules:
  - MATCH,DIRECT
`
