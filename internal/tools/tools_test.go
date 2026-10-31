package tools

import "testing"

func TestParseTrace(t *testing.T) {
	kv := parseTrace("fl=12f\nh=www.cloudflare.com\nip=103.152.220.17\nts=1700000000.1\ncolo=SIN\nloc=SG\nwarp=off\n")
	if kv["ip"] != "103.152.220.17" || kv["colo"] != "SIN" || kv["loc"] != "SG" {
		t.Fatalf("got %v", kv)
	}
}

func TestConnectivitySites(t *testing.T) {
	ids := map[string]bool{}
	for _, s := range ConnectivitySites {
		if ids[s.ID] {
			t.Errorf("%s is listed twice", s.ID)
		}
		ids[s.ID] = true
		if s.Group != "global" && s.Group != "domestic" {
			t.Errorf("%s is in group %q", s.ID, s.Group)
		}
	}
}

func TestNormalizeSites(t *testing.T) {
	kept := NormalizeSites([]Site{
		{"a", "A", " https://a.example.com ", "domestic"},
		{"", "", "https://b.example.com", "weird"},
		{"dup", "Host of A", "https://a.example.com/favicon.ico", "global"},
		{"", "", "ftp://c.example.com", "global"},
		{"a", "Again A", "https://c.example.com", "global"},
	})
	want := []Site{
		{"a", "A", "https://a.example.com", "domestic"},
		{"b.example.com", "b.example.com", "https://b.example.com", "global"},
		{"a-2", "Again A", "https://c.example.com", "global"},
	}
	if len(kept) != len(want) {
		t.Fatalf("kept %v", kept)
	}
	for i, s := range want {
		if kept[i] != s {
			t.Errorf("site %d is %+v, want %+v", i, kept[i], s)
		}
	}
}
