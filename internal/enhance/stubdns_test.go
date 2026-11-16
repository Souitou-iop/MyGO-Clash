package enhance

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

func TestStubUpstreams(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	stub := write("stub.conf", "# systemd-resolved\nnameserver 127.0.0.53\noptions edns0 trust-ad\n")
	real := write("real.conf", "nameserver 192.168.31.1\nnameserver 192.168.31.1\nnameserver fe80::1%eth0\n")
	router := write("router.conf", "nameserver 10.0.0.1\n")

	if up, ok := stubUpstreams(stub, real); !ok || !reflect.DeepEqual(up, []string{"192.168.31.1", "fe80::1"}) {
		t.Errorf("behind systemd-resolved: %v %v", up, ok)
	}
	if up, ok := stubUpstreams(stub, filepath.Join(dir, "missing")); !ok || len(up) != 0 {
		t.Errorf("a stub without its upstreams: %v %v", up, ok)
	}
	if _, ok := stubUpstreams(router, real); ok {
		t.Error("a resolver the core can ask itself is no stub")
	}
	if _, ok := stubUpstreams(filepath.Join(dir, "missing"), real); ok {
		t.Error("no resolv.conf is no stub")
	}
}

func TestAvoidStubDNS(t *testing.T) {
	cfg, err := yamlx.Parse([]byte(`
dns:
  default-nameserver: [system, 223.5.5.5]
  nameserver: [system, "https://doh.pub/dns-query", "udp://127.0.0.53:53"]
  proxy-server-nameserver: [system]
  nameserver-policy:
    "+.lan": system
    "+.corp": ["10.1.1.1"]
`))
	if err != nil {
		t.Fatal(err)
	}
	logs := AvoidStubDNS(cfg, []string{"192.168.31.1"})
	dns := cfg.Map("dns")
	want := map[string][]any{
		"default-nameserver":      {"192.168.31.1", "223.5.5.5"},
		"nameserver":              {"192.168.31.1", "https://doh.pub/dns-query"},
		"proxy-server-nameserver": {"192.168.31.1"},
	}
	for k, w := range want {
		if got := dns.Slice(k); !reflect.DeepEqual(got, w) {
			t.Errorf("%s: %v, want %v", k, got, w)
		}
	}
	policy := dns.Map("nameserver-policy")
	if got := policy.Slice("+.lan"); !reflect.DeepEqual(got, []any{"192.168.31.1"}) {
		t.Errorf("policy +.lan: %v", got)
	}
	if got := policy.Slice("+.corp"); !reflect.DeepEqual(got, []any{"10.1.1.1"}) {
		t.Errorf("policy +.corp changed: %v", got)
	}
	if len(logs) != 4 {
		t.Errorf("%d notes, want one per list changed: %v", len(logs), logs)
	}

	// No upstream known: lists fall back to public servers, or go.
	cfg, _ = yamlx.Parse([]byte("dns:\n  default-nameserver: [system]\n  nameserver: [system]\n  fallback: [system]\n"))
	AvoidStubDNS(cfg, nil)
	dns = cfg.Map("dns")
	if got := dns.Slice("default-nameserver"); !reflect.DeepEqual(got, strAny(stubFallback["default-nameserver"])) {
		t.Errorf("default-nameserver: %v", got)
	}
	if got := dns.Slice("nameserver"); !reflect.DeepEqual(got, strAny(stubFallback["nameserver"])) {
		t.Errorf("nameserver: %v", got)
	}
	if dns.Has("fallback") {
		t.Errorf("fallback kept: %v", dns.Slice("fallback"))
	}
}

func TestIsLocalNameServer(t *testing.T) {
	for s, want := range map[string]bool{
		"system": true, "dhcp://system": true, "127.0.0.53": true, "udp://127.0.0.53:53": true,
		"tcp://[::1]:53": true, "localhost": true,
		"223.5.5.5": false, "https://doh.pub/dns-query": false, "dhcp://en0": false, "192.168.1.1:53": false,
	} {
		if got := isLocalNameServer(s); got != want {
			t.Errorf("%s: %v, want %v", s, got, want)
		}
	}
}
