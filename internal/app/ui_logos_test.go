package app

import (
	"cmp"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/tools"
)

// noLogo are the older sites whose own icons could not be had: they show
// their initial. No other site of the page's defaults may lack a logo.
var noLogo = map[string]bool{
	"magalu": true, "justdial": true, "redbus": true, "nairaland": true, "kallxo": true, "rtk": true,
	"procredit-ks": true, "gob-mx": true, "imss": true, "telcel": true, "totalplay": true,
}

// Every site of the connectivity page but those, and every check, has its
// logo bundled.
func TestLogosBundled(t *testing.T) {
	has := func(id string) bool {
		return svgImage("assets/logos/"+id+".svg") != nil || bitmapImage("assets/logos/"+id+".png") != nil
	}
	for _, s := range tools.ConnectivitySites {
		if !has(s.ID) && !noLogo[s.ID] {
			t.Errorf("site %s has no logo", s.ID)
		}
	}
	for _, u := range tools.UnlockCatalog() {
		if id := cmp.Or(u.Site, u.ID); !has(id) {
			t.Errorf("check %s has no logo (%s)", u.ID, id)
		}
	}
}
