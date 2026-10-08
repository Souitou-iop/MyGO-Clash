package tools

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// The kinds of address an IPQuality tells apart.
const (
	KindResidential = "residential" // a home connection
	KindDatacenter  = "datacenter"  // hosting, a data center
	KindMobile      = "mobile"      // a mobile carrier
	KindBusiness    = "business"    // a company's or an institution's line
	KindUnknown     = "unknown"
)

// IPQuality is what public reputation services say about an address:
// whether it belongs to a home, a data center or a carrier, whether it is
// a known proxy, and how risky it looks.
type IPQuality struct {
	IP     string `json:"ip"`
	Kind   string `json:"kind"`
	Proxy  bool   `json:"proxy"` // a known proxy of any kind
	VPN    bool   `json:"vpn"`
	Tor    bool   `json:"tor"`
	Risk   int    `json:"risk"` // 0 to 100, or -1 when no service scored it
	Org    string `json:"org,omitempty"`
	Source string `json:"source"`
}

// IPQualityOf asks ipapi.is and proxycheck.io about ip at once and merges
// what they say. It fails only when neither answers.
func IPQualityOf(ctx context.Context, proxy, ip string) (IPQuality, error) {
	if net.ParseIP(ip) == nil {
		return IPQuality{}, errors.New("not an address")
	}
	c := Client(proxy, 6*time.Second, true)
	var (
		wg       sync.WaitGroup
		a, b     IPQuality
		okA, okB bool
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, body, err := get(ctx, c, "https://api.ipapi.is/?q="+url.QueryEscape(ip), nil); err == nil {
			a, okA = parseIPAPIIs(body, ip)
		}
	}()
	go func() {
		defer wg.Done()
		if _, body, err := get(ctx, c, "https://proxycheck.io/v2/"+url.PathEscape(ip)+"?vpn=1&asn=1&risk=1", nil); err == nil {
			b, okB = parseProxyCheck(body, ip)
		}
	}()
	wg.Wait()
	switch {
	case okA && okB:
		return mergeQuality(a, b), nil
	case okA:
		return a, nil
	case okB:
		return b, nil
	}
	return IPQuality{}, errors.New("no reputation service answered")
}

// mergeQuality combines two answers about one address. Where they
// disagree on the kind, the more suspicious one wins.
func mergeQuality(a, b IPQuality) IPQuality {
	out := a
	out.Source = a.Source + " + " + b.Source
	out.Proxy = a.Proxy || b.Proxy
	out.VPN = a.VPN || b.VPN
	out.Tor = a.Tor || b.Tor
	out.Risk = max(a.Risk, b.Risk)
	if out.Org == "" {
		out.Org = b.Org
	}
	rank := map[string]int{KindDatacenter: 4, KindMobile: 3, KindResidential: 2, KindBusiness: 1}
	if rank[b.Kind] > rank[a.Kind] {
		out.Kind = b.Kind
	}
	return out
}

// parseIPAPIIs reads an answer of api.ipapi.is. Its free tier may leave
// the flags out, and the company and ASN come as text or as objects, so
// every field is optional.
func parseIPAPIIs(body, ip string) (IPQuality, bool) {
	var m map[string]any
	if json.Unmarshal([]byte(body), &m) != nil || m["ip"] == nil {
		return IPQuality{}, false
	}
	q := IPQuality{IP: ip, Kind: KindUnknown, Risk: -1, Source: "ipapi.is"}
	datacenter, hasDC := m["is_datacenter"].(bool)
	mobile, _ := m["is_mobile"].(bool)
	q.Tor, _ = m["is_tor"].(bool)
	q.VPN, _ = m["is_vpn"].(bool)
	q.Proxy, _ = m["is_proxy"].(bool)
	q.Proxy = q.Proxy || q.VPN || q.Tor

	company, _ := m["company"].(map[string]any)
	asn, _ := m["asn"].(map[string]any)
	switch v := m["company"].(type) {
	case string:
		q.Org = v
	case map[string]any:
		q.Org = strOf(v["name"])
	}
	if q.Org == "" {
		if org := strOf(asn["org"]); org != "" {
			q.Org = org
		} else if s, ok := m["asn"].(string); ok {
			_, q.Org, _ = strings.Cut(s, " ")
		}
	}
	types := []string{strOf(company["type"]), strOf(asn["type"])}
	hosting := datacenter || types[0] == "hosting" || types[1] == "hosting"
	switch {
	case hosting:
		q.Kind = KindDatacenter
	case mobile:
		q.Kind = KindMobile
	case types[0] == "isp" || types[1] == "isp":
		q.Kind = KindResidential
	case types[0] != "" || types[1] != "" || hasDC:
		q.Kind = KindBusiness
	}
	for _, s := range []string{strOf(company["abuser_score"]), strOf(asn["abuser_score"])} {
		q.Risk = max(q.Risk, abuserRisk(s))
	}
	if q.Kind == KindUnknown && q.Risk < 0 && !q.Proxy {
		return IPQuality{}, false // nothing but the location and the owner
	}
	return q, true
}

func strOf(v any) string { s, _ := v.(string); return s }

// abuserRisk turns ipapi.is's abuser score, a number from 0 to 1 with a
// rating after it ("0.0039 (Low)"), to 0 to 100 on a log scale: 0.0001
// and below is 0, 1 is 100. It returns -1 when there is no score.
func abuserRisk(s string) int {
	num, _, _ := strings.Cut(strings.TrimSpace(s), " ")
	f, err := strconv.ParseFloat(num, 64)
	if err != nil || f < 0 {
		return -1
	}
	if f <= 0.0001 {
		return 0
	}
	return min(100, int(math.Round((math.Log10(f)+4)/4*100)))
}

// parseProxyCheck reads an answer of proxycheck.io.
func parseProxyCheck(body, ip string) (IPQuality, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(body), &m) != nil {
		return IPQuality{}, false
	}
	var status string
	_ = json.Unmarshal(m["status"], &status)
	raw, ok := m[ip]
	if !ok {
		// An IPv6 address may come back written another way.
		for k, v := range m {
			if p := net.ParseIP(k); p != nil && p.Equal(net.ParseIP(ip)) {
				raw, ok = v, true
			}
		}
	}
	if !ok || (status != "ok" && status != "warning") {
		return IPQuality{}, false
	}
	var r struct {
		Provider     string `json:"provider"`
		Organisation string `json:"organisation"`
		Proxy        string `json:"proxy"`
		Type         string `json:"type"`
		Risk         *int   `json:"risk"`
	}
	if json.Unmarshal(raw, &r) != nil {
		return IPQuality{}, false
	}
	q := IPQuality{IP: ip, Kind: KindUnknown, Risk: -1, Org: r.Provider, Source: "proxycheck.io"}
	if q.Org == "" {
		q.Org = r.Organisation
	}
	if r.Risk != nil {
		q.Risk = min(100, max(0, *r.Risk))
	}
	q.Proxy = strings.EqualFold(r.Proxy, "yes")
	switch t := strings.ToLower(r.Type); {
	case t == "residential":
		q.Kind = KindResidential
	case t == "wireless":
		q.Kind = KindMobile
	case t == "business":
		q.Kind = KindBusiness
	case t == "hosting":
		q.Kind = KindDatacenter
	case t == "tor":
		q.Tor = true
	case strings.Contains(t, "vpn"):
		q.VPN = true
	}
	q.Proxy = q.Proxy || q.Tor || q.VPN
	return q, true
}
