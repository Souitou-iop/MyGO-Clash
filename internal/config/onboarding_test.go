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

func TestInvitesToTour(t *testing.T) {
	for _, c := range []struct {
		name string
		ui   UI
		want bool
	}{
		{"a newcomer, whom the guide meets by itself", UI{}, false},
		{"one who knew the app before the guide", UI{OnboardingSeen: true}, true},
		{"on the last launch it invites on", UI{OnboardingSeen: true, TourInvites: TourInviteLaunches - 1}, true},
		{"after the launches it invites on, or the guide played", UI{OnboardingSeen: true, TourInvites: TourInviteLaunches}, false},
	} {
		if got := c.ui.InvitesToTour(); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
