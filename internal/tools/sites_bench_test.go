package tools

import "testing"

// The paths the connectivity page walks every frame, with the default
// sites, ~588 of them.

func BenchmarkSiteCategory(b *testing.B) {
	sites := ConnectivitySites
	b.ReportAllocs()
	for b.Loop() {
		for _, s := range sites {
			_ = SiteCategory(s)
		}
	}
}

func BenchmarkSiteRegionAll(b *testing.B) {
	sites := ConnectivitySites
	b.ReportAllocs()
	for b.Loop() {
		for _, s := range sites {
			_ = SiteRegion(s)
		}
	}
}

func BenchmarkSiteCategories(b *testing.B) {
	sites := ConnectivitySites
	b.ReportAllocs()
	for b.Loop() {
		_ = SiteCategories(sites)
	}
}

func BenchmarkSiteRegions(b *testing.B) {
	sites := ConnectivitySites
	b.ReportAllocs()
	for b.Loop() {
		_ = SiteRegions(sites)
	}
}

func BenchmarkEffectiveSites(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = EffectiveSites(nil, nil)
	}
}

func BenchmarkEffectiveSitesHidden(b *testing.B) {
	hidden := []string{"google", "apple", "github", "netflix", "claude", "kakao", "naver", "tving", "yelp", "imdb"}
	b.ReportAllocs()
	for b.Loop() {
		_ = EffectiveSites(hidden, nil)
	}
}

func BenchmarkNormalizeSites(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = NormalizeSites(ConnectivitySites)
	}
}
