package enhance

import (
	"strings"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

func TestApplyBaseGeo(t *testing.T) {
	cfg, err := yamlx.Parse([]byte("mode: rule\n"))
	if err != nil {
		t.Fatal(err)
	}
	ApplyBase(cfg, Base{Mode: "rule", GeoAutoUpdate: true, GeoUpdateInterval: 12, GeoMirror: true})
	DefaultGeoX(cfg, true)
	if on, _ := cfg.Bool("geo-auto-update"); !on {
		t.Fatal("geo-auto-update did not turn on")
	}
	if n, _ := cfg.Int("geo-update-interval"); n != 12 {
		t.Fatalf("interval %v", cfg.Value("geo-update-interval"))
	}
	urls := cfg.Map("geox-url")
	if urls == nil || !strings.Contains(urls.String("geosite"), "jsdelivr") {
		t.Fatalf("the mirror did not fill the empty addresses: %v", urls)
	}

	// Off: no interval is written, and the official source fills in.
	cfg, _ = yamlx.Parse([]byte("mode: rule\n"))
	ApplyBase(cfg, Base{Mode: "rule", GeoAutoUpdate: false, GeoMirror: false})
	DefaultGeoX(cfg, false)
	if on, _ := cfg.Bool("geo-auto-update"); on {
		t.Fatal("geo-auto-update should be off")
	}
	if cfg.Value("geo-update-interval") != nil {
		t.Fatal("an interval was written while off")
	}
	urls = cfg.Map("geox-url")
	if !strings.Contains(urls.String("geoip"), "github.com") {
		t.Fatalf("the official source did not fill in: %v", urls.String("geoip"))
	}
	// An address the profile brings stays untouched either way.
	cfg, _ = yamlx.Parse([]byte("geox-url: {geosite: https://example.test/geosite.dat}\n"))
	DefaultGeoX(cfg, true)
	if got := cfg.Map("geox-url").String("geosite"); got != "https://example.test/geosite.dat" {
		t.Fatalf("the profile's own address was replaced: %q", got)
	}
}
