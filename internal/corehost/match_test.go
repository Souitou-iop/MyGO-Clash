package corehost

import (
	"context"
	"testing"

	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	R "github.com/metacubex/mihomo/rules"
	"github.com/metacubex/mihomo/rules/wrapper"
	"github.com/metacubex/mihomo/tunnel"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

func TestMatchRule(t *testing.T) {
	direct := adapter.NewProxy(outbound.NewDirect())
	reject := adapter.NewProxy(outbound.NewReject())
	proxies := map[string]C.Proxy{"DIRECT": direct, "REJECT": reject}
	var rules []C.Rule
	for _, r := range [][]string{
		{"DOMAIN-SUFFIX", "ads.example.com", "REJECT"},
		{"DOMAIN-KEYWORD", "intranet", "DIRECT"},
		{"MATCH", "", "DIRECT"},
	} {
		parsed, err := R.ParseRule(r[0], r[1], r[2], nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		rules = append(rules, wrapper.NewRuleWrapper(parsed))
	}
	tunnel.UpdateProxies(proxies, nil)
	tunnel.UpdateRules(rules, nil, nil)
	tunnel.SetMode(tunnel.Rule)

	ctx := context.Background()
	res, err := matchRule(ctx, coreapi.MatchRequest{Target: "https://x.ads.example.com/a"})
	if err != nil || res.Source != "rule" || res.Index != 0 || res.Policy != "REJECT" || res.Port != 443 || res.RuleType != "DomainSuffix" {
		t.Fatalf("got %+v, %v", res, err)
	}
	res, _ = matchRule(ctx, coreapi.MatchRequest{Target: "my.intranet.local", Port: 8080})
	if res.Index != 1 || res.Port != 8080 || len(res.Chain) != 1 || res.Chain[0].Name != "DIRECT" {
		t.Fatalf("got %+v", res)
	}
	res, _ = matchRule(ctx, coreapi.MatchRequest{Target: "1.2.3.4"})
	if res.Index != 2 || res.RuleType != "Match" || len(res.IPs) != 1 {
		t.Fatalf("got %+v", res)
	}

	// A rule switched off by hand is skipped, and counted.
	rules[0].(C.RuleWrapper).SetDisabled(true)
	res, _ = matchRule(ctx, coreapi.MatchRequest{Target: "x.ads.example.com"})
	if res.Index != 2 || res.Disabled != 1 {
		t.Fatalf("got %+v", res)
	}
	if h := rules[2].(C.RuleWrapper).HitCount(); h != 0 {
		t.Errorf("testing counted %d hits", h)
	}

	tunnel.SetMode(tunnel.Direct)
	res, _ = matchRule(ctx, coreapi.MatchRequest{Target: "x.ads.example.com"})
	if res.Source != "mode" || res.Index != -1 || res.Policy != "DIRECT" {
		t.Fatalf("direct mode: %+v", res)
	}
	tunnel.SetMode(tunnel.Rule)
}

func TestNormalizeTarget(t *testing.T) {
	cases := []struct {
		in, host string
		port     int
	}{
		{"google.com", "google.com", 0},
		{"  Www.Google.COM. ", "www.google.com", 0},
		{"https://www.google.com/search?q=a#x", "www.google.com", 443},
		{"http://example.org", "example.org", 80},
		{"https://user:pw@example.org:8443/p", "example.org", 8443},
		{"example.org:8080/path", "example.org", 8080},
		{"1.2.3.4", "1.2.3.4", 0},
		{"1.2.3.4:53", "1.2.3.4", 53},
		{"http://[2001:db8::1]:8080/x", "2001:db8::1", 8080},
		{"[2001:db8::1]", "2001:db8::1", 0},
		{"2001:db8::1", "2001:db8::1", 0},
		{"::ffff:1.2.3.4", "::ffff:1.2.3.4", 0},
	}
	for _, c := range cases {
		host, port, err := normalizeTarget(c.in)
		if err != nil || host != c.host || port != c.port {
			t.Errorf("normalizeTarget(%q) = %q, %d, %v; want %q, %d", c.in, host, port, err, c.host, c.port)
		}
	}
	for _, in := range []string{"", "   ", "https://", "a b"} {
		if _, _, err := normalizeTarget(in); err == nil {
			t.Errorf("normalizeTarget(%q) did not fail", in)
		}
	}
}

// fake is a proxy that is a node, or a group selecting next.
type fake struct {
	C.Proxy
	name string
	typ  C.AdapterType
	next C.Proxy
}

func (f *fake) Name() string                     { return f.name }
func (f *fake) Type() C.AdapterType              { return f.typ }
func (f *fake) Unwrap(*C.Metadata, bool) C.Proxy { return f.next }

func TestWalkChain(t *testing.T) {
	node := &fake{name: "HK 01", typ: C.Trojan}
	auto := &fake{name: "Auto", typ: C.URLTest, next: node}
	main := &fake{name: "Proxy", typ: C.Selector, next: auto}
	chain := walkChain(main, &C.Metadata{})
	want := []string{"Proxy", "Auto", "HK 01"}
	if len(chain) != len(want) {
		t.Fatalf("got %+v", chain)
	}
	for i, hop := range chain {
		if hop.Name != want[i] {
			t.Errorf("hop %d is %q, want %q", i, hop.Name, want[i])
		}
	}
	if chain[0].Type != "Selector" || chain[2].Type != "Trojan" {
		t.Errorf("types: %+v", chain)
	}

	// A loop of groups ends.
	a := &fake{name: "A", typ: C.Selector}
	b := &fake{name: "B", typ: C.Selector, next: a}
	a.next = b
	if got := walkChain(a, &C.Metadata{}); len(got) != 32 {
		t.Errorf("a loop gave %d hops", len(got))
	}
}
