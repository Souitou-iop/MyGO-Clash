package tools

import "testing"

func TestParseProxyCheck(t *testing.T) {
	cases := []struct {
		ip, body, kind string
		proxy, vpn     bool
		risk           int
	}{
		{"73.0.0.1", `{"status":"ok","73.0.0.1":{"provider":"Comcast","proxy":"no","type":"Residential","risk":0}}`, KindResidential, false, false, 0},
		{"46.4.0.1", `{"status":"ok","46.4.0.1":{"provider":"Hetzner","proxy":"yes","type":"VPN","risk":66}}`, KindUnknown, true, true, 66},
		{"172.58.0.1", `{"status":"ok","172.58.0.1":{"proxy":"no","type":"Wireless","risk":3}}`, KindMobile, false, false, 3},
		{"1.1.1.1", `{"status":"ok","1.1.1.1":{"proxy":"no","type":"Business","risk":0}}`, KindBusiness, false, false, 0},
	}
	for _, c := range cases {
		q, ok := parseProxyCheck(c.body, c.ip)
		if !ok || q.Kind != c.kind || q.Proxy != c.proxy || q.VPN != c.vpn || q.Risk != c.risk {
			t.Errorf("%s: got %+v ok=%v", c.ip, q, ok)
		}
	}
	if _, ok := parseProxyCheck(`{"status":"denied","message":"quota"}`, "1.1.1.1"); ok {
		t.Error("an error answer was accepted")
	}
}

func TestParseIPAPIIs(t *testing.T) {
	// The full answer: objects, flags and abuser scores.
	q, ok := parseIPAPIIs(`{"ip":"3.5.140.2","is_datacenter":true,"is_mobile":false,"is_proxy":false,"is_vpn":true,"is_tor":false,
		"company":{"name":"Amazon","abuser_score":"0.0039 (Low)","type":"hosting"},"asn":{"asn":16509,"org":"Amazon.com, Inc.","type":"hosting","abuser_score":"0.0001 (Very Low)"}}`, "3.5.140.2")
	if !ok || q.Kind != KindDatacenter || !q.VPN || !q.Proxy || q.Org != "Amazon" || q.Risk < 35 || q.Risk > 45 {
		t.Errorf("full answer: %+v ok=%v", q, ok)
	}
	q, ok = parseIPAPIIs(`{"ip":"73.0.0.1","is_datacenter":false,"is_mobile":false,"is_proxy":false,"is_vpn":false,"is_tor":false,"company":{"name":"Comcast","abuser_score":"0.0002 (Low)","type":"isp"},"asn":{"org":"Comcast","type":"isp"}}`, "73.0.0.1")
	if !ok || q.Kind != KindResidential || q.Proxy || q.Risk < 0 || q.Risk > 15 {
		t.Errorf("isp answer: %+v ok=%v", q, ok)
	}
	// The free tier: text only, nothing to judge by.
	if _, ok := parseIPAPIIs(`{"ip":"3.5.140.2","is_bogon":false,"company":"Amazon Data Services","asn":"AS16509 Amazon.com, Inc.","city":"Incheon"}`, "3.5.140.2"); ok {
		t.Error("an answer with no reputation was accepted")
	}
	if _, ok := parseIPAPIIs(`{"error":"quota"}`, "1.1.1.1"); ok {
		t.Error("an error answer was accepted")
	}
}

func TestMergeQuality(t *testing.T) {
	a := IPQuality{Kind: KindResidential, Risk: 5, Source: "a"}
	b := IPQuality{Kind: KindDatacenter, Proxy: true, Risk: 66, Org: "Hetzner", Source: "b"}
	m := mergeQuality(a, b)
	if m.Kind != KindDatacenter || !m.Proxy || m.Risk != 66 || m.Org != "Hetzner" {
		t.Errorf("got %+v", m)
	}
}

func TestAbuserRisk(t *testing.T) {
	for s, want := range map[string]int{"0.00001 (Very Low)": 0, "1 (Very High)": 100, "0.01 (High)": 50, "garbage": -1} {
		if got := abuserRisk(s); got != want {
			t.Errorf("abuserRisk(%q) = %d, want %d", s, got, want)
		}
	}
}
