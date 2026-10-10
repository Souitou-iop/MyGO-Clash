package tools

import "testing"

func TestDemoIPInfo(t *testing.T) {
	for _, view := range []string{ViewDomestic, ViewGlobal, ViewCloudflare} {
		info, err := DemoIPInfo(view)
		if err != nil {
			t.Errorf("%s: %v", view, err)
		}
		if !isDocumented(info.IP) {
			t.Errorf("%s: %q is not a documentation address", view, info.IP)
		}
		if info.Country == "" || info.ISP == "" || info.Source == "" {
			t.Errorf("%s: bare answer %+v", view, info)
		}
	}
	if _, err := DemoIPInfo("elsewhere"); err == nil {
		t.Error("an unknown view should fail")
	}
}

func TestDemoIPQuality(t *testing.T) {
	home := DemoIPQuality(demoDomesticIP)
	if home.Kind != KindResidential || home.Risk != 0 || home.Proxy || home.VPN {
		t.Errorf("the home line looks wrong: %+v", home)
	}
	datacenter := DemoIPQuality("198.51.100.21")
	if datacenter.Kind != KindDatacenter || !datacenter.VPN || datacenter.Risk < 0 {
		t.Errorf("the data center looks wrong: %+v", datacenter)
	}
}

// isDocumented says whether ip is from a range kept for documentation, the
// ones a demo may show.
func isDocumented(ip string) bool {
	for _, p := range []string{"192.0.2.", "198.51.100.", "203.0.113."} {
		if len(ip) > len(p) && ip[:len(p)] == p {
			return true
		}
	}
	return false
}
