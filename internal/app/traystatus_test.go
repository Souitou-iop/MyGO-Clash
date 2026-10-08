package app

import (
	"strings"
	"testing"
	"unicode/utf16"
)

func TestNodePath(t *testing.T) {
	v := &ProxiesView{
		Groups: []ProxyGroup{
			{Name: "Auto", Type: "URLTest", Now: "HK 01"},
			{Name: "Hidden", Type: "Selector", Hidden: true, Now: "DIRECT"},
			{Name: "Proxy", Type: "Selector", Now: "Auto"},
			{Name: "Media", Type: "Selector", Now: "Proxy"},
		},
		Global: &ProxyGroup{Name: "GLOBAL", Type: "Selector", Now: "Media"},
	}
	for mode, want := range map[string]string{
		"rule":   "Proxy › Auto › HK 01",
		"global": "GLOBAL › … › HK 01",
	} {
		if got := nodePath(v, mode); got != want {
			t.Errorf("%s: %q, want %q", mode, got, want)
		}
	}
	if got := nodePath(nil, "rule"); got != "" {
		t.Errorf("nil view: %q", got)
	}
}

func TestClipUTF16(t *testing.T) {
	s := strings.Repeat("🇭🇰香港", 30)
	got := clipUTF16(s, 127)
	if n := len(utf16.Encode([]rune(got))); n > 127 {
		t.Fatalf("%d units", n)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("no ellipsis: %q", got)
	}
	if clipUTF16("short", 127) != "short" {
		t.Fatal("clipped a short tip")
	}
}
