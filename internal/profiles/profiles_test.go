package profiles

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/enhance"
	"github.com/mygo-clash/mygo-clash/internal/secure"
	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	s, err := secure.NewSealer(secure.RandomBytes(32), nil)
	if err != nil {
		t.Fatal(err)
	}
	m, err := Open(t.TempDir(), s)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

const sub = `proxies:
  - {name: HK, type: ss, server: hk.example, port: 443, cipher: aes-128-gcm, password: secret-password}
proxy-groups:
  - {name: Proxy, type: select, proxies: [HK, DIRECT]}
rules:
  - MATCH,Proxy
`

func TestFetchReadsHeadersAndConvertsShareLinks(t *testing.T) {
	links := "ss://YWVzLTEyOC1nY206cGFzcw@1.2.3.4:8388#Tokyo\ntrojan://pw@jp.example:443?sni=jp.example#Osaka\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.UserAgent(), "mihomo") {
			t.Errorf("user agent %q", r.UserAgent())
		}
		w.Header().Set("Subscription-Userinfo", "upload=1024; download=2048; total=1073741824; expire=1893456000")
		w.Header().Set("Profile-Update-Interval", "12")
		w.Header().Set("Content-Disposition", `attachment; filename*=UTF-8''%E6%9C%BA%E5%9C%BA.yaml`)
		w.Header().Set("Profile-Web-Page-Url", "https://provider.example")
		if r.URL.Path == "/links" {
			fmt.Fprint(w, links)
			return
		}
		fmt.Fprint(w, sub)
	}))
	defer srv.Close()

	f, err := Fetch(context.Background(), srv.URL+"/clash", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "机场" || f.UpdateInterval != 720 || f.Home != "https://provider.example" || f.Converted {
		t.Fatalf("fetched %+v", f)
	}
	if f.Usage == nil || f.Usage.Total != 1<<30 || f.Usage.Expire != 1893456000 {
		t.Fatalf("usage %+v", f.Usage)
	}
	f, err = Fetch(context.Background(), srv.URL+"/links", FetchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !f.Converted || f.Proxies != 2 || f.Groups != 2 {
		t.Fatalf("converted %+v\n%s", f, f.Content)
	}
}

func TestProfilesSealedAndGenerated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, sub) }))
	defer srv.Close()
	m := newManager(t)
	ctx := context.Background()
	p, err := m.Create(ctx, NewProfile{Type: TypeRemote, URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if cur, _ := m.Current(); cur.UID != p.UID {
		t.Fatal("the first profile is not current")
	}

	// Extensions: a rule prepended, a proxy added, a merge that tries to
	// change an owned key, and a script.
	set := func(kind, content string) string {
		uid, err := m.Extension(p.UID, kind)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.SetContent(uid, []byte(content)); err != nil {
			t.Fatal(err)
		}
		return uid
	}
	set(TypeRules, "prepend: ['DOMAIN-SUFFIX,lan,DIRECT']\n")
	set(TypeProxies, "append: [{name: JP, type: ss, server: jp.example, port: 443, cipher: aes-128-gcm, password: x}]\n")
	mergeUID := set(TypeMerge, "mixed-port: 1\nsniffer: {enable: true}\n")
	set(TypeScript, "function main(c, n) { c.rules.push('DOMAIN,from-script.example,DIRECT'); return c }")
	if err := m.SetContent(GlobalMerge, []byte("unified-delay: false\n")); err != nil {
		t.Fatal(err)
	}

	rt, err := m.Generate(ctx, Inputs{
		Base: enhance.Base{Mode: "rule", MixedPort: 7897, LogLevel: "info", UnifiedDelay: true, Tun: enhance.Tun{Stack: "mixed"}},
		Tailnet: &enhance.Tailnet{Mode: "embedded", ProxyName: "Tailscale",
			Proxy: yamlx.MapOf("type", "tailscale", "hostname", "laptop", "udp", true)},
	})
	if err != nil {
		t.Fatal(err)
	}
	d := rt.Display
	if n, _ := d.Int("mixed-port"); n != 7897 {
		t.Fatalf("mixed-port %v", d.Value("mixed-port"))
	}
	if on, _ := d.Bool("unified-delay"); !on {
		t.Fatal("a global merge changed an owned key")
	}
	if !d.Map("sniffer").Has("enable") {
		t.Fatal("the merge did not apply")
	}
	rules := d.Slice("rules")
	if !slices.Contains(rules, any("DOMAIN-SUFFIX,lan,DIRECT")) || rules[len(rules)-1] != "DOMAIN,from-script.example,DIRECT" {
		t.Fatalf("rules %v", rules)
	}
	sel := d.Slice("proxy-groups")[0].(*yamlx.Map).Slice("proxies")
	if sel[0] != "JP" {
		t.Fatalf("the added proxy did not join the selector: %v", sel)
	}
	if len(rt.Logs[mergeUID]) == 0 || !strings.Contains(rt.Logs[mergeUID][0].Message, "mixed-port") {
		t.Fatalf("no note for the discarded key: %v", rt.Logs)
	}
	// The display keeps the portable outbound; the core gets a placeholder.
	var shown, run string
	for _, x := range d.Slice("proxies") {
		if x.(*yamlx.Map).String("name") == "Tailscale" {
			shown = x.(*yamlx.Map).String("type")
		}
	}
	for _, x := range rt.Core.Slice("proxies") {
		if x.(*yamlx.Map).String("name") == "Tailscale" {
			run = x.(*yamlx.Map).String("type")
		}
	}
	if shown != "tailscale" || run != "reject" {
		t.Fatalf("display %q core %q", shown, run)
	}

	// Everything on disk is sealed: no password, no URL.
	b, err := m.Export()
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Contents) < 7 {
		t.Fatalf("exported %d items", len(b.Contents))
	}
	entries, _ := readAll(m.dir)
	for name, data := range entries {
		if strings.Contains(string(data), "secret-password") || strings.Contains(string(data), srv.URL) {
			t.Fatalf("%s is not sealed", name)
		}
	}

	// Deleting the profile deletes its extensions.
	if err := m.Delete(p.UID); err != nil {
		t.Fatal(err)
	}
	if left := m.List().Items; len(left) != 2 {
		t.Fatalf("left %v", left)
	}
}

func readAll(dir string) (map[string][]byte, error) {
	out := map[string][]byte{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		out[e.Name()] = data
	}
	return out, nil
}
