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
