package tools

import (
	"slices"
	"testing"
)

func TestSiteCategory(t *testing.T) {
	for _, c := range []struct {
		site         Site
		want, region string
	}{
		{Site{ID: "google"}, "web", ""},
		{Site{ID: "apple"}, "web", ""},
		{Site{ID: "claude"}, "ai", ""},
		{Site{ID: "netflix"}, "media", ""},
		{Site{ID: "spotify"}, "media", ""},
		{Site{ID: "telegram"}, "social", ""},
		{Site{ID: "github"}, "dev", ""},
		{Site{ID: "cloudflare"}, "cloud", ""},
		{Site{ID: "wikipedia"}, "news", ""},
		{Site{ID: "binance"}, "money", ""},
		{Site{ID: "baidu", Group: "domestic"}, CategoryRegion, "china"},
		{Site{ID: "digikala"}, CategoryRegion, "iran"},
		{Site{ID: "my-own"}, CategoryOther, ""},
	} {
		if got := SiteCategory(c.site); got != c.want {
			t.Errorf("SiteCategory(%s) = %s, want %s", c.site.ID, got, c.want)
		}
		if got := SiteRegion(c.site); got != c.region {
			t.Errorf("SiteRegion(%s) = %q, want %q", c.site.ID, got, c.region)
		}
	}
}

func TestSiteCategoriesOrder(t *testing.T) {
	got := SiteCategories([]Site{{ID: "my-own"}, {ID: "baidu", Group: "domestic"}, {ID: "digikala"}, {ID: "github"}, {ID: "netflix"}, {ID: "claude"}, {ID: "google"}})
	want := []string{"web", "ai", "media", "dev", CategoryRegion, CategoryOther}
	if !slices.Equal(got, want) {
		t.Errorf("SiteCategories = %v, want %v", got, want)
	}
}

// The defaults are every site of the preset groups, once each, each in a
// category of its own kind, within the cap.
func TestDefaultSitesAll(t *testing.T) {
	ids := map[string]bool{}
	for _, s := range ConnectivitySites {
		if ids[s.ID] {
			t.Errorf("default site %s twice", s.ID)
		}
		ids[s.ID] = true
		if SiteCategory(s) == CategoryOther {
			t.Errorf("default site %s has no category", s.ID)
		}
	}
	for _, p := range SitePresets {
		for _, s := range p.Sites {
			if !ids[s.ID] {
				t.Errorf("preset site %s (%s) is not a default", s.ID, p.ID)
			}
		}
	}
	if n := len(NormalizeSites(ConnectivitySites)); n != len(ConnectivitySites) || n > MaxConnectivitySites {
		t.Errorf("defaults: %d kept of %d (cap %d)", n, len(ConnectivitySites), MaxConnectivitySites)
	}
	t.Logf("%d default sites in %d categories, %d regions", len(ConnectivitySites), len(SiteCategories(ConnectivitySites)), len(SiteRegions(ConnectivitySites)))
}

// A site two preset groups test at different addresses is kept once, and a
// list saved with a renamed copy loses it.
func TestNormalizeSitesKnownOnce(t *testing.T) {
	got := NormalizeSites([]Site{
		{"claude", "Claude", "https://claude.ai/favicon.ico", "global"},
		{"claude", "Claude", "https://claude.com/favicon.ico", "global"},
		{"cloudflare-2", "Cloudflare", "https://speed.cloudflare.com/favicon.ico", "global"},
		{"cloudflare", "Cloudflare", "https://www.cloudflare.com/cdn-cgi/trace", "global"},
		{"example.com", "Mine", "https://example.com", "global"},
		{"example.com", "Mine too", "https://www.example.com", "global"},
	})
	var ids []string
	for _, s := range got {
		ids = append(ids, s.ID)
	}
	if want := []string{"claude", "cloudflare", "example.com", "example.com-2"}; !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

// The favorites are never a site's category.
func TestNoSiteIsGeneral(t *testing.T) {
	for _, s := range ConnectivitySites {
		if SiteCategory(s) == CategoryGeneral {
			t.Errorf("%s has the favorites as its category", s.ID)
		}
	}
	for _, id := range DefaultFavoriteSiteIDs {
		if !IsKnownSite(id) {
			t.Errorf("default favorite %s is no site", id)
		}
	}
}

// The page's sites are the presets but those hidden, then the user's own;
// a site of the user's own that is a preset after all is not had twice.
func TestEffectiveSites(t *testing.T) {
	mine := Site{"example.com", "Mine", "https://example.com", "global"}
	got := EffectiveSites([]string{"google", "netflix"}, []Site{mine, {"claude", "Claude", "https://claude.ai", "global"}})
	ids := map[string]int{}
	for _, s := range got {
		ids[s.ID]++
	}
	if ids["google"] > 0 || ids["netflix"] > 0 {
		t.Errorf("hidden sites kept")
	}
	if ids["example.com"] != 1 || ids["claude"] != 1 || ids["apple"] != 1 {
		t.Errorf("ids = %v", ids)
	}
	if want := len(ConnectivitySites) - 1; len(got) != want {
		t.Errorf("%d sites, want %d", len(got), want)
	}
}
