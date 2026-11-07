package enhance

import (
	"strings"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

func TestApplyChains(t *testing.T) {
	cfg, err := yamlx.Parse([]byte(chainProxies))
	if err != nil {
		t.Fatal(err)
	}
	ApplyChains(cfg, []ProxyChain{
		{Match: "家宽", Through: "HK 01", Enabled: true},
		{Match: "US 01", Through: "不存在的节点", Enabled: true},
		{Match: "JP", Through: "HK 01", Enabled: false},
	})
	for _, p := range cfg.Slice("proxies") {
		pm := p.(*yamlx.Map)
		name, dp := pm.String("name"), pm.String("dialer-proxy")
		switch name {
		case "家宽 01":
			if dp != "HK 01" {
				t.Fatalf("%s: dialer-proxy %q", name, dp)
			}
		case "US 01":
			if dp != "" {
				t.Fatalf("%s: the front is not there, yet it chained: %q", name, dp)
			}
		case "JP 01":
			if dp != "" {
				t.Fatalf("%s: the disabled chain applied: %q", name, dp)
			}
		case "HK 01":
			if dp != "" {
				t.Fatalf("%s: the front chained to itself", name)
			}
		}
	}
}

func TestApplyChainsUndoesLoops(t *testing.T) {
	cfg, err := yamlx.Parse([]byte(chainProxies))
	if err != nil {
		t.Fatal(err)
	}
	ApplyChains(cfg, []ProxyChain{
		{Match: "HK 01", Through: "家宽 01", Enabled: true},
		{Match: "家宽", Through: "HK 01", Enabled: true},
	})
	loops := 0
	for _, p := range cfg.Slice("proxies") {
		if pm := p.(*yamlx.Map); strings.TrimSpace(pm.String("dialer-proxy")) != "" {
			loops++
		}
	}
	if loops != 0 {
		t.Fatalf("a loop survived: %d dialer-proxies left", loops)
	}
}

const chainProxies = `
proxies:
  - {name: HK 01, type: ss, server: hk.example, port: 443, cipher: aes-128-gcm, password: x}
  - {name: US 01, type: ss, server: us.example, port: 443, cipher: aes-128-gcm, password: x}
  - {name: JP 01, type: ss, server: jp.example, port: 443, cipher: aes-128-gcm, password: x}
  - {name: 家宽 01, type: ss, server: home.example, port: 443, cipher: aes-128-gcm, password: x}
proxy-groups:
  - {name: Proxy, type: select, proxies: [HK 01]}
rules:
  - MATCH,DIRECT
`
