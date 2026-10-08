package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestPatchMergesAndValidates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	old, cur, err := s.Patch([]byte(`{"clash":{"mixedPort":7890,"mode":"global"},"tun":{"enabled":true},"hotkeys":{"bindings":{"toggle-tun":"CmdOrCtrl+Shift+T","bogus":"X"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if old.Clash.MixedPort != 7897 || cur.Clash.MixedPort != 7890 || cur.Clash.Mode != "global" || !cur.Tun.Enabled {
		t.Fatalf("patch: %+v", cur.Clash)
	}
	if cur.Clash.SocksPort != 7898 || cur.Tun.Stack != "mixed" {
		t.Fatal("a patch lost fields it did not name")
	}
	if _, ok := cur.Hotkeys.Bindings["bogus"]; ok || cur.Hotkeys.Bindings["toggle-tun"] == "" {
		t.Fatalf("bindings %v", cur.Hotkeys.Bindings)
	}

	// null removes a key: the field returns to its default.
	_, cur, err = s.Patch([]byte(`{"clash":{"mode":null}}`))
	if err != nil || cur.Clash.Mode != "rule" {
		t.Fatalf("null: %v %v", cur.Clash.Mode, err)
	}

	// Invalid settings are refused and nothing changes.
	for _, bad := range []string{
		`{"clash":{"socksEnabled":true,"socksPort":7890}}`,
		`{"accent":"blue"}`,
		`{"sync":{"enabled":true,"url":"http://dav.example.com"}}`,
		`{"tun":{"routeExcludeAddress":["not-a-cidr"]}}`,
		`{"clash":{"controller":{"enabled":true,"address":"0.0.0.0:9090","secret":""}}}`,
		`{"clash":{"lanAuth":{"enabled":true,"username":"","password":"x"}}}`,
		`{"clash":{"lanAuth":{"enabled":true,"username":"a:b","password":"x"}}}`,
	} {
		before := s.Get()
		if _, _, err := s.Patch([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
		if after := s.Get(); after.Clash.MixedPort != before.Clash.MixedPort || after.Accent != before.Accent {
			t.Errorf("%s changed the settings", bad)
		}
	}

	_, cur, err = s.Patch([]byte(`{"clash":{"lanAuth":{"enabled":true,"username":" me ","password":"p:w"}}}`))
	if err != nil || cur.Clash.LANAuth.Username != "me" || cur.Clash.LANAuth.Password != "p:w" {
		t.Fatalf("lanAuth: %+v %v", cur.Clash.LANAuth, err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Get().Clash.MixedPort != 7890 {
		t.Fatal("not saved")
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", fi.Mode().Perm())
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"mixedPort": 7890`) {
		t.Fatalf("file:\n%s", data)
	}
}

func TestHomeCardsMigrate(t *testing.T) {
	d := Defaults().UI.HomeCards
	old := []HomeCard{
		{"profile", true}, {"proxy", true}, {"network", false}, {"mode", true},
		{"traffic", true}, {"tailscale", false}, {"bogus", true}, {"system", true},
	}
	got := normalizeHomeCards(old, d)
	if len(got) != len(d) {
		t.Fatalf("got %d cards, want %d", len(got), len(d))
	}
	want := map[string]bool{"control": true, "tailscale": false, "system": true, "test": true}
	for i, c := range got {
		if c.ID != d[i].ID {
			t.Fatalf("card %d is %q, want the default order (%q)", i, c.ID, d[i].ID)
		}
		if v, ok := want[c.ID]; ok && c.Visible != v {
			t.Errorf("%s visible = %v, want %v", c.ID, c.Visible, v)
		}
	}

	// A current layout keeps its order; unknown and repeated cards go,
	// missing ones come back hidden.
	cur := []HomeCard{{"traffic", true}, {"control", false}, {"traffic", false}, {"gone", true}}
	got = normalizeHomeCards(cur, d)
	if got[0] != (HomeCard{"traffic", true}) || got[1] != (HomeCard{"control", false}) || len(got) != len(d) {
		t.Fatalf("got %v", got)
	}
	for _, c := range got[2:] {
		if c.Visible {
			t.Errorf("added card %s is visible", c.ID)
		}
	}
}

func TestNavUnlockBecomesConnectivity(t *testing.T) {
	s := Defaults()
	s.UI.Nav = []string{"home", "unlock", "proxies"}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	want := []string{"home", "connectivity", "proxies", "settings"}
	if !slices.Equal(s.UI.Nav, want) {
		t.Fatalf("nav = %v, want %v", s.UI.Nav, want)
	}
}
