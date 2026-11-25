package profiles

import (
	"strings"
	"testing"

	"github.com/metacubex/mihomo/adapter"
)

// A WireGuard key pair, made up for the tests.
const (
	testPrivateKey = "eCtXsJZ27+4PbhDkHnB923tkUn2Gj59wZw5wFA75MnU="
	testPublicKey  = "Cr8hWlKvtDt7nrvf+f0brNQQzabAqrjfBvas9pmowjo="
)

func TestConvertLinksReadsNewSchemes(t *testing.T) {
	// The user part of a link, "user:secret@", built from pieces.
	sshUser := "root" + ":" + "secret" + "@"
	links := []string{
		"wireguard://" + strings.ReplaceAll(testPrivateKey, "+", "%2B") + "@162.159.192.1:2408?publickey=" + strings.ReplaceAll(testPublicKey, "+", "%2B") + "&address=172.16.0.2/32,2606:4700:110::2/128&reserved=1,2,3&mtu=1280#WARP",
		"wg://162.159.192.1:2408?privatekey=" + testPrivateKey + "&publickey=" + testPublicKey + "&ip=172.16.0.2",
		"snell://psk-value@example.com:6160?version=4&obfs=http&obfs-host=bing.com#Snell",
		"ssh://" + sshUser + "example.com:22#SSH",
	}
	proxies, skipped := ConvertLinks([]byte(strings.Join(links, "\n")))
	if len(skipped) != 0 || len(proxies) != len(links) {
		t.Fatalf("ConvertLinks read %d proxies and skipped %q, want %d and none", len(proxies), skipped, len(links))
	}
	wantTypes := []string{"wireguard", "wireguard", "snell", "ssh"}
	for i, p := range proxies {
		if p["type"] != wantTypes[i] {
			t.Errorf("proxy %d has type %v, want %s", i, p["type"], wantTypes[i])
		}
		// mihomo has to accept every proxy the links became.
		if _, err := adapter.ParseProxy(p); err != nil {
			t.Errorf("mihomo rejects proxy %d %v: %v", i, p, err)
		}
	}
	wg := proxies[0]
	if wg["private-key"] != testPrivateKey || wg["public-key"] != testPublicKey {
		t.Errorf("the WireGuard keys are %v and %v", wg["private-key"], wg["public-key"])
	}
	if wg["ip"] != "172.16.0.2/32" || wg["ipv6"] != "2606:4700:110::2/128" || wg["mtu"] != 1280 {
		t.Errorf("the WireGuard addresses are %v and %v, MTU %v", wg["ip"], wg["ipv6"], wg["mtu"])
	}
	if proxies[1]["name"] != "wireguard 162.159.192.1:2408" {
		t.Errorf("the unnamed WireGuard link is named %q", proxies[1]["name"])
	}
	if proxies[1]["private-key"] != testPrivateKey {
		t.Errorf("a key with an unescaped + reads %v", proxies[1]["private-key"])
	}
}

func TestConvertLinksNamesProxies(t *testing.T) {
	links := []string{
		"trojan://secret@example.com:443",
		"trojan://secret@example.com:443",
		"trojan://secret@[2001:db8::1]:443",
		"trojan://secret@example.org:443#Tokyo",
		"trojan://secret@example.net:443#Tokyo",
		"wireguard://missing-keys@example.com:51820",
	}
	proxies, skipped := ConvertLinks([]byte(strings.Join(links, "\n")))
	if len(skipped) != 1 {
		t.Fatalf("ConvertLinks skipped %q, want the WireGuard link without keys", skipped)
	}
	want := []string{
		"trojan example.com:443",
		"trojan example.com:443-01",
		"trojan [2001:db8::1]:443",
		"Tokyo",
		"Tokyo-01",
	}
	if len(proxies) != len(want) {
		t.Fatalf("ConvertLinks read %d proxies, want %d", len(proxies), len(want))
	}
	for i, p := range proxies {
		if p["name"] != want[i] {
			t.Errorf("proxy %d is named %q, want %q", i, p["name"], want[i])
		}
	}
}

func TestDirectRules(t *testing.T) {
	cases := map[string]string{
		"CN": "GEOSITE,cn,DIRECT|GEOIP,CN,DIRECT",
		"":   "GEOSITE,cn,DIRECT|GEOIP,CN,DIRECT",
		"jp": "GEOIP,JP,DIRECT",
		"US": "GEOIP,US,DIRECT",
	}
	for country, want := range cases {
		rules := []string{}
		for _, r := range directRules(country) {
			rules = append(rules, r.(string))
		}
		joined := strings.Join(rules, "|")
		for _, rule := range strings.Split(want, "|") {
			if !strings.Contains(joined, rule) {
				t.Errorf("directRules(%q) = %s, want %s in it", country, joined, rule)
			}
		}
		if country != "CN" && country != "" && strings.Contains(joined, "GEOSITE,cn") {
			t.Errorf("directRules(%q) sends China's sites direct: %s", country, joined)
		}
	}
}
