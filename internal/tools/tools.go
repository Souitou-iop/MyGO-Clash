// Package tools tests the network through the proxy: the addresses that
// sites at home and abroad see, the reachability of popular sites, and
// which streaming and AI services are available.
package tools

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Client makes requests through a proxy.
func Client(proxy string, timeout time.Duration, follow bool) *http.Client {
	return clientOf(transport(proxy, timeout, false), nil, timeout, follow)
}

// transport goes through a proxy; keep holds its connections open once
// done with, for the next request or for the core to see.
func transport(proxy string, timeout time.Duration, keep bool) *http.Transport {
	tr := &http.Transport{
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		DisableKeepAlives:     !keep,
	}
	if proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	return tr
}

// clientOf makes requests over tr, following redirects if follow, and
// noting the hosts in log if given.
func clientOf(tr *http.Transport, log *hostLog, timeout time.Duration, follow bool) *http.Client {
	var rt http.RoundTripper = tr
	if log != nil {
		rt = logged{tr, log}
	}
	c := &http.Client{Transport: rt, Timeout: timeout}
	if !follow {
		c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return c
}

const browserUA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"

func get(ctx context.Context, c *http.Client, rawURL string, hdr map[string]string) (*http.Response, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	return send(c, req, hdr)
}

// send sends req as a browser would, with hdr, and reads the answer.
func send(c *http.Client, req *http.Request, hdr map[string]string) (*http.Response, string, error) {
	req.Header.Set("User-Agent", browserUA)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return resp, string(body), nil
}

// IPInfo is what the world sees of the connection.
type IPInfo struct {
	IP          string  `json:"ip"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Region      string  `json:"region"`
	City        string  `json:"city"`
	ISP         string  `json:"isp"`
	ASN         string  `json:"asn"`
	Timezone    string  `json:"timezone"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Colo        string  `json:"colo,omitempty"` // the Cloudflare data center
	Source      string  `json:"source"`
}

// LookupIP asks public services for the address the proxy exits from.
func LookupIP(ctx context.Context, proxy string) (IPInfo, error) {
	return lookup(ctx, Client(proxy, 8*time.Second, true), "")
}

// lookup describes ip, or the address the client is seen from when ip is
// empty, from the first public service that answers.
func lookup(ctx context.Context, c *http.Client, ip string) (IPInfo, error) {
	type source struct {
		name, url string
		parse     func(map[string]any) IPInfo
	}
	str := func(m map[string]any, k string) string {
		switch v := m[k].(type) {
		case string:
			return v
		case float64:
			return fmt.Sprint(int64(v))
		}
		return ""
	}
	num := func(m map[string]any, k string) float64 { f, _ := m[k].(float64); return f }
	at := func(prefix, suffix string) string {
		if ip == "" {
			return prefix + suffix
		}
		return prefix + "/" + url.PathEscape(ip) + suffix
	}
	sources := []source{
		{"ip.sb", at("https://api.ip.sb/geoip", ""), func(m map[string]any) IPInfo {
			return IPInfo{IP: str(m, "ip"), Country: str(m, "country"), CountryCode: str(m, "country_code"), Region: str(m, "region"), City: str(m, "city"),
				ISP: str(m, "isp"), ASN: str(m, "asn"), Timezone: str(m, "timezone"), Latitude: num(m, "latitude"), Longitude: num(m, "longitude")}
		}},
		{"ipapi.co", at("https://ipapi.co", "/json/"), func(m map[string]any) IPInfo {
			return IPInfo{IP: str(m, "ip"), Country: str(m, "country_name"), CountryCode: str(m, "country_code"), Region: str(m, "region"), City: str(m, "city"),
				ISP: str(m, "org"), ASN: strings.TrimPrefix(str(m, "asn"), "AS"), Timezone: str(m, "timezone"), Latitude: num(m, "latitude"), Longitude: num(m, "longitude")}
		}},
		{"ipinfo.io", at("https://ipinfo.io", "/json"), func(m map[string]any) IPInfo {
			info := IPInfo{IP: str(m, "ip"), CountryCode: str(m, "country"), Country: str(m, "country"), Region: str(m, "region"), City: str(m, "city"),
				ISP: str(m, "org"), Timezone: str(m, "timezone")}
			if a, o, ok := strings.Cut(info.ISP, " "); ok && strings.HasPrefix(a, "AS") {
				info.ASN, info.ISP = strings.TrimPrefix(a, "AS"), o
			}
			return info
		}},
	}
	var errs []error
	for _, s := range sources {
		m, err := getJSON(ctx, c, s.url)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", s.name, err))
			continue
		}
		info := s.parse(m)
		if info.IP == "" {
			continue
		}
		info.Source = s.name
		return info, nil
	}
	return IPInfo{}, errors.Join(errs...)
}

func getJSON(ctx context.Context, c *http.Client, rawURL string) (map[string]any, error) {
	resp, body, err := get(ctx, c, rawURL, map[string]string{"Accept": "application/json"})
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil, err
	}
	return m, nil
}

// The views of ExitIP.
const (
	ViewDomestic   = "domestic"   // what sites in mainland China see
	ViewGlobal     = "global"     // what sites abroad see
	ViewCloudflare = "cloudflare" // what Cloudflare sees
)

// ExitIP returns the address that a kind of site sees through the proxy.
// The proxy's rules route each kind its own way, so the addresses differ
// when the rules split the traffic.
func ExitIP(ctx context.Context, proxy, view string) (IPInfo, error) {
	c := Client(proxy, 8*time.Second, true)
	switch view {
	case ViewGlobal:
		return lookup(ctx, c, "")
	case ViewDomestic:
		return domesticIP(ctx, c)
	case ViewCloudflare:
		return cloudflareIP(ctx, c)
	}
	return IPInfo{}, fmt.Errorf("unknown view %q", view)
}

// domesticIP asks IPIP.net, then Upyun, both in mainland China.
func domesticIP(ctx context.Context, c *http.Client) (IPInfo, error) {
	var errs []error
	m, err := getJSON(ctx, c, "https://myip.ipip.net/json")
	if err == nil {
		data, _ := m["data"].(map[string]any)
		ip, _ := data["ip"].(string)
		if net.ParseIP(ip) != nil {
			// The location is [country, province, city, district, ISP].
			var loc []string
			list, _ := data["location"].([]any)
			for _, v := range list {
				s, _ := v.(string)
				loc = append(loc, s)
			}
			for len(loc) < 5 {
				loc = append(loc, "")
			}
			fallback := IPInfo{IP: ip, Country: loc[0], Region: loc[1], City: loc[2], ISP: loc[4]}
			if loc[0] == "中国" {
				fallback.CountryCode = "CN"
			}
			return describe(ctx, c, ip, "IPIP.net", fallback), nil
		}
		err = fmt.Errorf("no address in the answer")
	}
	errs = append(errs, fmt.Errorf("IPIP.net: %w", err))
	m, err = getJSON(ctx, c, fmt.Sprintf("https://pubstatic.b0.upaiyun.com/?_upnode&t=%d", time.Now().Unix()))
	if err == nil {
		if ip, _ := m["remote_addr"].(string); net.ParseIP(ip) != nil {
			return describe(ctx, c, ip, "Upyun", IPInfo{IP: ip}), nil
		}
		err = fmt.Errorf("no address in the answer")
	}
	errs = append(errs, fmt.Errorf("Upyun: %w", err))
	return IPInfo{}, errors.Join(errs...)
}

// cloudflareIP reads the address and data center from Cloudflare's trace.
func cloudflareIP(ctx context.Context, c *http.Client) (IPInfo, error) {
	_, body, err := get(ctx, c, "https://www.cloudflare.com/cdn-cgi/trace", nil)
	if err != nil {
		return IPInfo{}, fmt.Errorf("Cloudflare: %w", err)
	}
	kv := parseTrace(body)
	ip := kv["ip"]
	if net.ParseIP(ip) == nil {
		return IPInfo{}, errors.New("Cloudflare: no address in the trace")
	}
	info := describe(ctx, c, ip, "Cloudflare", IPInfo{IP: ip, CountryCode: kv["loc"], Country: kv["loc"]})
	info.Colo = kv["colo"]
	return info, nil
}

// parseTrace reads the key=value lines of a /cdn-cgi/trace.
func parseTrace(body string) map[string]string {
	kv := map[string]string{}
	for _, line := range strings.Split(body, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			kv[k] = strings.TrimSpace(v)
		}
	}
	return kv
}

// describe looks the details of ip up, keeping what the source said when
// the lookup fails. The source stays the service that saw the address.
func describe(ctx context.Context, c *http.Client, ip, source string, fallback IPInfo) IPInfo {
	info, err := lookup(ctx, c, ip)
	if err != nil || info.IP != ip {
		info = fallback
	}
	info.Source = source
	return info
}

// Site is a site whose delay is tested.
type Site struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	URL   string `json:"url"`
	Group string `json:"group,omitempty"` // global or domestic
}

// DefaultSites are tested unless the user chose others.
var DefaultSites = []Site{
	{"google", "Google", "https://www.google.com/generate_204", "global"},
	{"github", "GitHub", "https://github.com", "global"},
	{"youtube", "YouTube", "https://www.youtube.com", "global"},
	{"cloudflare", "Cloudflare", "https://www.cloudflare.com/cdn-cgi/trace", "global"},
	{"apple", "Apple", "https://www.apple.com/library/test/success.html", "global"},
	{"bilibili", "Bilibili", "https://www.bilibili.com", "domestic"},
}

// ConnectivitySites are the sites of the connectivity page unless the user
// changed the list: every site of the preset groups, each tested at a small
// file, as in MyIP. The page shows them by category.
var ConnectivitySites = defaultSites()

// MaxConnectivitySites caps the site list the page keeps: the defaults
// and room for the user's own.
const MaxConnectivitySites = 600

// NormalizeSites prepares a site list to keep: valid http(s) addresses
// only, one per host, unique ids, at most MaxConnectivitySites.
func NormalizeSites(sites []Site) []Site {
	keep := make([]Site, 0, min(len(sites), MaxConnectivitySites))
	hosts, ids := map[string]bool{}, map[string]bool{}
	for _, s := range sites {
		u, err := url.Parse(strings.TrimSpace(s.URL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			continue
		}
		host := strings.ToLower(u.Host)
		if hosts[host] {
			continue
		}
		hosts[host] = true
		group := s.Group
		if group != "domestic" {
			group = "global"
		}
		id := strings.TrimSpace(s.ID)
		if id == "" {
			id = host
		}
		// A known site is kept once, whatever address it came in at: two
		// preset groups may test it at different ones. Lists saved before
		// held such a second copy as "id-2".
		if known := knownSiteID(id); known != "" {
			if ids[known] {
				delete(hosts, host)
				continue
			}
			id = known
		}
		for base, n := id, 2; ids[id]; n++ {
			id = fmt.Sprintf("%s-%d", base, n)
		}
		ids[id] = true
		name := strings.TrimSpace(s.Name)
		if name == "" {
			name = host
		}
		if len(keep) == MaxConnectivitySites {
			break
		}
		keep = append(keep, Site{ID: id, Name: name, URL: u.String(), Group: group})
	}
	return keep
}

// SiteResult is the delay of a site, 0 when it failed.
type SiteResult struct {
	ID      string `json:"id"`
	DelayMs int    `json:"delayMs"`
	Status  int    `json:"status"`
	Error   string `json:"error,omitempty"`
}

// TestSite measures the time to the first byte of a site through the proxy.
func TestSite(ctx context.Context, proxy string, s Site) SiteResult {
	c := Client(proxy, 10*time.Second, false)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return SiteResult{ID: s.ID, Error: err.Error()}
	}
	req.Header.Set("User-Agent", browserUA)
	start := time.Now()
	resp, err := c.Do(req)
	if err != nil {
		return SiteResult{ID: s.ID, Error: shortError(err)}
	}
	resp.Body.Close()
	return SiteResult{ID: s.ID, DelayMs: max(1, int(time.Since(start)/time.Millisecond)), Status: resp.StatusCode}
}

func shortError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "Timeout") {
		return "timeout"
	}
	return err.Error()
}

// knownSiteID is the id of a site the app knows, from presets or the
// defaults, of id or of the "id-2" a duplicate of it was once renamed to;
// "" for sites of the user's own.
func knownSiteID(id string) string {
	if isKnownSite(id) {
		return id
	}
	if i := strings.LastIndexByte(id, '-'); i > 0 {
		if _, err := strconv.Atoi(id[i+1:]); err == nil && isKnownSite(id[:i]) {
			return id[:i]
		}
	}
	return ""
}

// IsKnownSite reports whether a site is one the app knows, from a preset
// group or the defaults, rather than one of the user's own.
func IsKnownSite(id string) bool { return knownSiteID(id) != "" }

// KnownSiteID is the id of the preset site id names, as a renamed
// duplicate ("id-2") of it once did; "" for sites of the user's own.
func KnownSiteID(id string) string { return knownSiteID(id) }

// EffectiveSites are the sites the connectivity page tests: every preset
// site but those hidden, then the user's own. Presets a later version adds
// show up by themselves, since only what is hidden is kept.
func EffectiveSites(hidden []string, custom []Site) []Site {
	out := make([]Site, 0, len(ConnectivitySites)+len(custom))
	for _, s := range ConnectivitySites {
		if !slices.Contains(hidden, s.ID) {
			out = append(out, s)
		}
	}
	for _, s := range custom {
		if !IsKnownSite(s.ID) {
			out = append(out, s)
		}
	}
	return NormalizeSites(out)
}

func isKnownSite(id string) bool {
	if _, ok := categoryOf[id]; ok {
		return true
	}
	return slices.ContainsFunc(ConnectivitySites, func(s Site) bool { return s.ID == id })
}
