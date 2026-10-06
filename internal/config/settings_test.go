package config

import (
	"os"
	"path/filepath"
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
	} {
		before := s.Get()
		if _, _, err := s.Patch([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
		if after := s.Get(); after.Clash.MixedPort != before.Clash.MixedPort || after.Accent != before.Accent {
			t.Errorf("%s changed the settings", bad)
		}
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
