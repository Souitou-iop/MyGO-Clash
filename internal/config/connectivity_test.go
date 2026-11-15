package config

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/tools"
)

// A whole list from an older version becomes what differs from the
// presets: the user's own sites, and the presets removed from it, hidden.
func TestConnectivitySitesMigrate(t *testing.T) {
	mine := tools.Site{ID: "example.com", Name: "Mine", URL: "https://example.com", Group: "global"}
	// Since the defaults were every preset: the missing ones were removed.
	s := Defaults()
	s.ConnectivityModel = 0
	for _, st := range tools.ConnectivitySites {
		if st.ID != "netflix" && st.ID != "google" {
			s.ConnectivitySites = append(s.ConnectivitySites, st)
		}
	}
	s.ConnectivitySites = append(s.ConnectivitySites, mine, tools.Site{ID: "claude-2", Name: "Claude", URL: "https://claude.com", Group: "global"})
	s.ConnectivitySitesAll = true
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	slices.Sort(s.ConnectivityHidden)
	if !slices.Equal(s.ConnectivityHidden, []string{"google", "netflix"}) {
		t.Errorf("hidden = %v", s.ConnectivityHidden)
	}
	if len(s.ConnectivityCustom) != 1 || s.ConnectivityCustom[0].ID != "example.com" {
		t.Errorf("custom = %v", s.ConnectivityCustom)
	}
	if s.ConnectivitySites != nil || s.ConnectivityModel != connectivityModel {
		t.Errorf("old list kept, model %d", s.ConnectivityModel)
	}
	// A hidden site is no favorite.
	if slices.Contains(s.ConnectivityFavorites, "google") {
		t.Errorf("favorites keep hidden google: %v", s.ConnectivityFavorites)
	}
	// From before: the few presets imported say nothing of the others.
	s = Defaults()
	s.ConnectivityModel = 0
	s.ConnectivitySites = []tools.Site{{ID: "claude", Name: "Claude", URL: "https://claude.ai", Group: "global"}, mine}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	if len(s.ConnectivityHidden) != 0 || len(s.ConnectivityCustom) != 1 {
		t.Errorf("hidden %v, custom %v", s.ConnectivityHidden, s.ConnectivityCustom)
	}
	// Brought up to date once: a later normalization keeps it.
	s.ConnectivityHidden = []string{"netflix", "netflix", "not-a-site"}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.ConnectivityHidden, []string{"netflix"}) {
		t.Errorf("hidden = %v", s.ConnectivityHidden)
	}
}

// Unlock settings keep only the checks there are.
func TestUnlockSettingsNormalize(t *testing.T) {
	s := Defaults()
	s.UnlockHidden = []string{"netflix", "gone", "netflix"}
	s.UnlockEnabled = []string{"abema", ""}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.UnlockHidden, []string{"netflix"}) || !slices.Equal(s.UnlockEnabled, []string{"abema"}) {
		t.Errorf("hidden %v, enabled %v", s.UnlockHidden, s.UnlockEnabled)
	}
}

func TestConnectivityFavoritesDefaultsAndEmptyChoice(t *testing.T) {
	s := Defaults()
	if !slices.Equal(s.ConnectivityFavorites, tools.DefaultFavoriteSiteIDs) {
		t.Fatalf("default favorites = %v, want %v", s.ConnectivityFavorites, tools.DefaultFavoriteSiteIDs)
	}
	s.ConnectivityFavorites = nil
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.ConnectivityFavorites, tools.DefaultFavoriteSiteIDs) {
		t.Fatalf("missing favorites = %v, want %v", s.ConnectivityFavorites, tools.DefaultFavoriteSiteIDs)
	}
	s.ConnectivityFavorites = []string{}
	if err := s.Normalize(); err != nil {
		t.Fatal(err)
	}
	if len(s.ConnectivityFavorites) != 0 {
		t.Fatalf("explicit empty favorites = %v", s.ConnectivityFavorites)
	}
}

// An emptied 常用 stays empty across a save and a load.
func TestConnectivityFavoritesEmptyRoundTrip(t *testing.T) {
	s := Defaults()
	s.ConnectivityFavorites = []string{}
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var back Settings
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if err := back.Normalize(); err != nil {
		t.Fatal(err)
	}
	if len(back.ConnectivityFavorites) != 0 {
		t.Errorf("favorites back as %v", back.ConnectivityFavorites)
	}
}
