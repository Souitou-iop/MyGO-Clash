package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOnboardingSeenOnLoad(t *testing.T) {
	dir := t.TempDir()
	load := func(name, body string) bool {
		t.Helper()
		path := filepath.Join(dir, name)
		if body != "" {
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		s, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		return s.Get().UI.OnboardingSeen
	}
	if load("new.json", "") {
		t.Error("a new install has seen the guide")
	}
	if !load("old.json", `{"theme":"dark","ui":{"collapseNav":true}}`) {
		t.Error("settings from before the guide must read as seen")
	}
	if load("fresh.json", `{"ui":{"onboardingSeen":false}}`) {
		t.Error("a saved false must stay false")
	}
}
