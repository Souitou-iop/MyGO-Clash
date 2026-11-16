package enhance

import (
	"slices"
	"testing"
)

func TestAvoidSystemDNS(t *testing.T) {
	cfg := parse(t, `dns:
  respect-rules: false
  default-nameserver: [system, 223.6.6.6]
  nameserver: [system]
  fallback: [dhcp://system]
  direct-nameserver: [system]
  nameserver-policy:
    geosite:cn: [system, https://dns.alidns.com/dns-query]
    +.lan: system
    +.ts.net: 100.100.100.100
`)
	if logs := AvoidSystemDNS(cfg); len(logs) != 1 || logs[0].Level != "info" {
		t.Fatalf("logs = %#v", logs)
	}
	dns := cfg.Map("dns")
	if on, _ := dns.Bool("respect-rules"); on {
		t.Fatal("respect-rules changed")
	}
	if got := toStrings(dns.Slice("default-nameserver")); !slices.Equal(got, []string{"223.6.6.6"}) {
		t.Fatalf("default-nameserver = %q", got)
	}
	if got := toStrings(dns.Slice("nameserver")); !slices.Equal(got, systemFallback["nameserver"]) {
		t.Fatalf("nameserver = %q", got)
	}
	for _, key := range []string{"fallback", "direct-nameserver", "proxy-server-nameserver"} {
		if dns.Has(key) {
			t.Fatalf("%s remained", key)
		}
	}
	policy := dns.Map("nameserver-policy")
	if got := toStrings(policy.Slice("geosite:cn")); !slices.Equal(got, []string{"https://dns.alidns.com/dns-query"}) {
		t.Fatalf("policy geosite:cn = %q", got)
	}
	if policy.Has("+.lan") {
		t.Fatal("a policy that only asked the system remained")
	}
	if got := policy.String("+.ts.net"); got != "100.100.100.100" {
		t.Fatalf("policy +.ts.net = %q", got)
	}
}

func TestAvoidSystemDNSLeavesOthers(t *testing.T) {
	cfg := parse(t, "dns:\n  nameserver: [https://223.5.5.5/dns-query]\n  direct-nameserver: [https://doh.pub/dns-query]\n")
	if logs := AvoidSystemDNS(cfg); logs != nil {
		t.Fatalf("logs = %#v, want none", logs)
	}
	if got := toStrings(cfg.Map("dns").Slice("direct-nameserver")); !slices.Equal(got, []string{"https://doh.pub/dns-query"}) {
		t.Fatalf("direct-nameserver = %q", got)
	}
}

func toStrings(list []any) []string {
	out := make([]string, len(list))
	for i, v := range list {
		out[i], _ = v.(string)
	}
	return out
}
