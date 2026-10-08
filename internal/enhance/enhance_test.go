package enhance

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

func parse(t *testing.T, s string) *yamlx.Map {
	t.Helper()
	m, err := yamlx.Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func names(list []any) []string {
	var out []string
	for _, v := range list {
		out = append(out, itemName(v))
	}
	return out
}

const profile = `
mixed-port: 7890
proxies:
  - &hk {name: HK, type: ss, server: hk.example, port: 443, cipher: aes-128-gcm, password: x}
  - <<: *hk
    name: JP
    server: jp.example
proxy-groups:
  - {name: Proxy, type: select, proxies: [HK, JP, DIRECT]}
  - {name: Auto, type: url-test, proxies: [HK, JP], url: "http://www.gstatic.com/generate_204"}
rules:
  - DOMAIN-SUFFIX,google.com,Proxy
  - MATCH,DIRECT
`

func TestParseKeepsOrderAndResolvesMerges(t *testing.T) {
	cfg := parse(t, profile)
	if got := cfg.Keys(); !slices.Equal(got, []string{"mixed-port", "proxies", "proxy-groups", "rules"}) {
		t.Fatalf("keys %v", got)
	}
	jp := cfg.Slice("proxies")[1].(*yamlx.Map)
	if jp.String("server") != "jp.example" || jp.String("cipher") != "aes-128-gcm" || jp.String("name") != "JP" {
		t.Fatalf("merge key not resolved: %v", jp.ToStd())
	}
	out, err := yamlx.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(out), "mixed-port: 7890\nproxies:") {
		t.Fatalf("marshal lost order:\n%s", out)
	}
	back, err := yamlx.Parse(out)
	if err != nil || !yamlx.Equal(back, cfg) {
		t.Fatalf("round trip differs: %v", err)
	}
}

func TestJSONRoundTripKeepsOrder(t *testing.T) {
	cfg := parse(t, "z: 1\na: {y: true, b: [1, 2.5, x]}\n")
	data, err := cfg.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"z":1,"a":{"y":true,"b":[1,2.5,"x"]}}` {
		t.Fatalf("json %s", data)
	}
	v, err := yamlx.DecodeJSON(data)
	if err != nil || !yamlx.Equal(v, cfg) {
		t.Fatalf("decode %v %v", v, err)
	}
}

func TestApplySeq(t *testing.T) {
	cfg := parse(t, profile)
	ApplySeq(cfg, "proxies", SeqPatch{
		Prepend: []any{yamlx.MapOf("name", "US", "type", "ss")},
		Delete:  []string{"HK"},
	})
	if got := names(cfg.Slice("proxies")); !slices.Equal(got, []string{"US", "JP"}) {
		t.Fatalf("proxies %v", got)
	}
	sel := cfg.Slice("proxy-groups")[0].(*yamlx.Map)
	if got := sel.Slice("proxies"); !slices.Equal(names(got), []string{"US", "JP", "DIRECT"}) {
		t.Fatalf("selector %v", got)
	}
	auto := cfg.Slice("proxy-groups")[1].(*yamlx.Map)
	if got := auto.Slice("proxies"); !slices.Equal(names(got), []string{"JP"}) {
		t.Fatalf("url-test %v", got)
	}

	ApplySeq(cfg, "rules", SeqPatch{Prepend: []any{"DOMAIN,a.com,DIRECT"}, Append: []any{"IP-CIDR,1.1.1.1/32,Proxy"}, Delete: []string{"MATCH,DIRECT"}})
	if got := names(cfg.Slice("rules")); !slices.Equal(got, []string{"DOMAIN,a.com,DIRECT", "DOMAIN-SUFFIX,google.com,Proxy", "IP-CIDR,1.1.1.1/32,Proxy"}) {
		t.Fatalf("rules %v", got)
	}
}

func TestMerge(t *testing.T) {
	cfg := parse(t, profile+"dns: {enable: true, nameserver: [1.1.1.1], fallback-filter: {geoip: true}}\nhosts: {a.example: 1.2.3.4}\n")
	Merge(cfg, parse(t, `
Mixed-Port: 7891
prepend-rules: ["DOMAIN,first.example,DIRECT"]
dns: {nameserver: [8.8.8.8], fallback-filter: {ipcidr: [240.0.0.0/4]}}
hosts: {b.example: 5.6.7.8}
tun: {stack: system}
`))
	if v, _ := cfg.Int("mixed-port"); v != 7891 {
		t.Fatalf("mixed-port %v", cfg.Value("mixed-port"))
	}
	if cfg.Slice("rules")[0] != "DOMAIN,first.example,DIRECT" {
		t.Fatalf("rules %v", cfg.Slice("rules"))
	}
	dns := cfg.Map("dns")
	if on, _ := dns.Bool("enable"); !on || dns.Slice("nameserver")[0] != "8.8.8.8" {
		t.Fatalf("dns %v", dns.ToStd())
	}
	// dns extends one level deep: fallback-filter is replaced, not merged.
	if dns.Map("fallback-filter").Has("geoip") {
		t.Fatalf("fallback-filter merged: %v", dns.ToStd())
	}
	if cfg.Map("hosts").Has("a.example") || !cfg.Map("hosts").Has("b.example") {
		t.Fatalf("hosts not replaced: %v", cfg.Map("hosts").ToStd())
	}
	if cfg.Map("tun").String("stack") != "system" {
		t.Fatal("tun not merged")
	}
}

func TestScript(t *testing.T) {
	cfg := parse(t, profile)
	out, logs, err := RunScript(context.Background(), `
function main(config, name) {
  console.log("profile", name);
  config.proxies = config.proxies.filter(p => p.name !== "HK");
  config["proxy-groups"][0].proxies = ["JP"];
  return config;
}`, cfg, "Work")
	if err != nil {
		t.Fatal(err)
	}
	if got := names(out.Slice("proxies")); !slices.Equal(got, []string{"JP"}) {
		t.Fatalf("proxies %v", got)
	}
	if !slices.Equal(out.Keys(), cfg.Keys()) {
		t.Fatalf("script lost key order: %v", out.Keys())
	}
	if len(logs) != 1 || logs[0].Message != "profile Work" {
		t.Fatalf("logs %v", logs)
	}
	if n, _ := out.Int("mixed-port"); n != 7890 {
		t.Fatalf("number changed type: %#v", out.Value("mixed-port"))
	}
}

func TestScriptFailures(t *testing.T) {
	cfg := parse(t, profile)
	for name, script := range map[string]string{
		"throws":   `function main(c) { throw new Error("nope") }`,
		"no main":  `var x = 1`,
		"returns":  `function main(c) { return 42 }`,
		"loops":    `function main(c) { while (true) {} }`,
		"no I/O":   `function main(c) { require("fs"); return c }`,
		"too loud": `function main(c) { for (;;) console.log("x".repeat(10000)) }`,
	} {
		t.Run(name, func(t *testing.T) {
			out, logs, err := RunScript(context.Background(), script, cfg, "p")
			if !errors.Is(err, ErrScript) {
				t.Fatalf("err %v", err)
			}
			if out != cfg {
				t.Fatal("a failed script changed the configuration")
			}
			if len(logs) == 0 || logs[len(logs)-1].Level != "exception" {
				t.Fatalf("logs %v", logs)
			}
		})
	}
}

func TestOwnedFieldsWinOverExtensions(t *testing.T) {
	cfg := parse(t, profile)
	ApplyBase(cfg, Base{Mode: "rule", MixedPort: 7897, LogLevel: "info", Tun: Tun{Enable: true, Stack: "gvisor", AutoRoute: true, DNSHijack: []string{"any:53"}}})
	applied := ApplyDNSOverride(cfg, parse(t, "dns: {enhanced-mode: fake-ip, nameserver: [1.1.1.1], fallback: []}\nhosts: {}"))
	owned := Capture(cfg, applied)

	Merge(cfg, parse(t, "mixed-port: 1234\nmode: global\ntun: {stack: system}\ndns: {nameserver: [9.9.9.9], listen: ':53'}\n"))
	changed := owned.Changed(cfg)
	for _, k := range []string{"mixed-port", "mode", "tun.stack", "dns.nameserver"} {
		if !slices.Contains(changed, k) {
			t.Errorf("Changed missed %s: %v", k, changed)
		}
	}
	owned.Enforce(cfg)
	if n, _ := cfg.Int("mixed-port"); n != 7897 || cfg.String("mode") != "rule" || cfg.Map("tun").String("stack") != "gvisor" {
		t.Fatalf("not enforced: %v", cfg.ToStd())
	}
	dns := cfg.Map("dns")
	if dns.Slice("nameserver")[0] != "1.1.1.1" || dns.String("listen") != ":53" {
		t.Fatalf("dns %v", dns.ToStd())
	}
	if dns.Has("fallback") {
		t.Fatal("an empty override value was applied")
	}
	if len(owned.Changed(cfg)) != 0 {
		t.Fatalf("still changed after Enforce: %v", owned.Changed(cfg))
	}
}

func TestCleanupGroups(t *testing.T) {
	cfg := parse(t, profile+`proxy-providers: {sub: {type: http, url: "https://x"}}
`)
	ApplySeq(cfg, "proxies", SeqPatch{Delete: []string{"JP"}})
	cfg.Slice("proxy-groups")[1].(*yamlx.Map).Set("proxies", []any{"HK", "Gone"})
	cfg.Set("proxy-groups", append(cfg.Slice("proxy-groups"), yamlx.MapOf("name", "P", "type", "select", "use", []any{"sub", "nope"}, "proxies", []any{"from-provider"})))
	logs := CleanupGroups(cfg)
	groups := cfg.Slice("proxy-groups")
	if got := names(groups[1].(*yamlx.Map).Slice("proxies")); !slices.Equal(got, []string{"HK"}) {
		t.Fatalf("url-test %v", got)
	}
	p := groups[2].(*yamlx.Map)
	if !slices.Equal(names(p.Slice("use")), []string{"sub"}) || len(p.Slice("proxies")) != 1 {
		t.Fatalf("provider group %v", p.ToStd())
	}
	if len(logs) != 2 {
		t.Fatalf("logs %v", logs)
	}
}

func TestTailscaleEmbedded(t *testing.T) {
	cfg := parse(t, profile+"dns: {enable: true, enhanced-mode: fake-ip, nameserver: [1.1.1.1]}\n")
	logs := ApplyTailscale(cfg, Tailnet{
		Mode: "embedded", ProxyName: "Tailscale", Proxy: yamlx.MapOf("type", "tailscale", "udp", true),
		JoinSelector: true, Suffixes: []string{"tail1234.ts.net", "corp.example"}, Routes: []string{"192.168.10.0/24", "0.0.0.0/0"}, MagicDNS: true,
	})
	if len(logs) != 0 {
		t.Fatal(logs)
	}
	if got := names(cfg.Slice("proxies")); got[len(got)-1] != "Tailscale" {
		t.Fatalf("proxies %v", got)
	}
	rules := names(cfg.Slice("rules"))
	want := []string{
		"DOMAIN-SUFFIX,ts.net,Tailscale", "DOMAIN-SUFFIX,corp.example,Tailscale",
		"IP-CIDR,100.64.0.0/10,Tailscale,no-resolve", "IP-CIDR6,fd7a:115c:a1e0::/48,Tailscale,no-resolve",
		"IP-CIDR,192.168.10.0/24,Tailscale,no-resolve",
	}
	if !slices.Equal(rules[:len(want)], want) {
		t.Fatalf("rules %v", rules)
	}
	sel := cfg.Slice("proxy-groups")[0].(*yamlx.Map)
	if !slices.Contains(names(sel.Slice("proxies")), "Tailscale") {
		t.Fatal("not joined to the selector")
	}
	dns := cfg.Map("dns")
	if dns.Map("nameserver-policy").String("+.ts.net") != "ts://Tailscale" {
		t.Fatalf("dns %v", dns.ToStd())
	}
	if !slices.Contains(dns.Slice("fake-ip-filter"), any("+.corp.example")) {
		t.Fatalf("fake-ip-filter %v", dns.Slice("fake-ip-filter"))
	}
}

func TestTailscaleSystemWithTun(t *testing.T) {
	cfg := parse(t, profile)
	ApplyBase(cfg, Base{Mode: "rule", MixedPort: 7897, Tun: Tun{Enable: true, Stack: "mixed"}})
	ApplyTailscale(cfg, Tailnet{Mode: "system", Interface: "utun5", MagicDNS: true})
	if got := cfg.Map("tun").Slice("route-exclude-address"); len(got) != 2 {
		t.Fatalf("route-exclude-address %v", got)
	}
	if cfg.Slice("rules")[1] != "IP-CIDR,100.64.0.0/10,DIRECT,no-resolve" {
		t.Fatalf("rules %v", cfg.Slice("rules"))
	}
	if got := cfg.Map("dns").Map("nameserver-policy").String("+.ts.net"); got != "100.100.100.100#utun5" {
		t.Fatalf("policy %q", got)
	}
}

func TestSortAndBuiltins(t *testing.T) {
	cfg := parse(t, "rules: [MATCH,DIRECT]\nmode: Script\nproxies: [{name: h, type: hysteria, alpn: h3}]\nsniffer: {}\nmixed-port: 1\n")
	ApplyBuiltins(cfg)
	if cfg.String("mode") != "rule" {
		t.Fatal("script mode kept")
	}
	if _, ok := cfg.Slice("proxies")[0].(*yamlx.Map).Value("alpn").([]any); !ok {
		t.Fatal("alpn not a list")
	}
	if got := Sort(cfg).Keys(); !slices.Equal(got, []string{"mode", "mixed-port", "sniffer", "proxies", "rules"}) {
		t.Fatalf("sorted %v", got)
	}
}

func TestLANAuth(t *testing.T) {
	users := []string{"me:pa:ss"}
	// Off: the profile's own settings stay.
	cfg := parse(t, "authentication: ['a:b']\nskip-auth-prefixes: [10.0.0.0/8]")
	ApplyBase(cfg, Base{Mode: "rule", MixedPort: 7897, AllowLAN: true})
	ApplyLANAuth(cfg, nil)
	if got := cfg.Slice("authentication"); len(got) != 1 || got[0] != "a:b" {
		t.Errorf("no login asked: authentication = %v", got)
	}
	// On with allow-lan: our login, and only this device skips it.
	ApplyLANAuth(cfg, users)
	if got := cfg.Slice("authentication"); len(got) != 1 || got[0] != "me:pa:ss" {
		t.Errorf("authentication = %v", got)
	}
	if got := cfg.Slice("skip-auth-prefixes"); len(got) != 2 || got[0] != "127.0.0.1/8" || got[1] != "::1/128" {
		t.Errorf("skip-auth-prefixes = %v", got)
	}
	// Without allow-lan the ports are local anyway: nothing is written.
	cfg = parse(t, "mode: rule")
	ApplyBase(cfg, Base{Mode: "rule", MixedPort: 7897})
	ApplyLANAuth(cfg, users)
	if _, ok := cfg.Get("authentication"); ok {
		t.Error("authentication set without allow-lan")
	}
}
