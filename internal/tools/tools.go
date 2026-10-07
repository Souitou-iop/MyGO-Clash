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
	"regexp"
	"strings"
	"sync"
	"time"
)

// Client makes requests through a proxy.
func Client(proxy string, timeout time.Duration, follow bool) *http.Client {
	tr := &http.Transport{
		TLSClientConfig:       &tls.Config{MinVersion: tls.VersionTLS12},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		DisableKeepAlives:     true,
	}
	if proxy != "" {
		if u, err := url.Parse(proxy); err == nil {
			tr.Proxy = http.ProxyURL(u)
		}
	}
	c := &http.Client{Transport: tr, Timeout: timeout}
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

// ConnectivitySites are the sites of the connectivity page: sites abroad,
// then sites in mainland China, each tested at a small file, as in MyIP.
var ConnectivitySites = []Site{
	{"google", "Google", "https://www.google.com/generate_204", "global"},
	{"youtube", "YouTube", "https://www.youtube.com/favicon.ico", "global"},
	{"github", "GitHub", "https://github.com/favicon.ico", "global"},
	{"cloudflare", "Cloudflare", "https://www.cloudflare.com/cdn-cgi/trace", "global"},
	{"chatgpt", "ChatGPT", "https://chatgpt.com/favicon.ico", "global"},
	{"claude", "Claude", "https://claude.ai/favicon.ico", "global"},
	{"telegram", "Telegram", "https://telegram.org/favicon.ico", "global"},
	{"x", "X", "https://x.com/favicon.ico", "global"},
	{"baidu", "Baidu", "https://www.baidu.com/favicon.ico", "domestic"},
	{"bilibili", "Bilibili", "https://www.bilibili.com/favicon.ico", "domestic"},
	{"wechat", "WeChat", "https://res.wx.qq.com/a/wx_fed/assets/res/NTI4MWU5.ico", "domestic"},
	{"taobao", "Taobao", "https://www.taobao.com/favicon.ico", "domestic"},
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

// Unlock is the availability of a service.
type Unlock struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // yes, no, partial, failed, pending
	Region string `json:"region,omitempty"`
	Detail string `json:"detail,omitempty"`
	At     string `json:"at,omitempty"`
}

type checker struct {
	id, name string
	check    func(ctx context.Context, c, follow *http.Client) Unlock
}

var traceLoc = regexp.MustCompile(`(?m)^loc=([A-Z]{2})$`)

// trace reads the country Cloudflare sees at a site's /cdn-cgi/trace.
func trace(ctx context.Context, c *http.Client, host string) string {
	_, body, err := get(ctx, c, "https://"+host+"/cdn-cgi/trace", nil)
	if err != nil {
		return ""
	}
	if m := traceLoc.FindStringSubmatch(body); m != nil {
		return m[1]
	}
	return ""
}

var checkers = []checker{
	{"netflix", "Netflix", func(ctx context.Context, c, f *http.Client) Unlock {
		// A Netflix original and a licensed title.
		r1, _, err1 := get(ctx, c, "https://www.netflix.com/title/81280792", nil)
		r2, _, err2 := get(ctx, c, "https://www.netflix.com/title/70143836", nil)
		if err1 != nil || err2 != nil {
			return Unlock{Status: "failed", Detail: "network error"}
		}
		region := ""
		if loc := r1.Header.Get("X-Originating-Url"); loc != "" {
			region = regionFromPath(loc)
		}
		if l, err := r1.Location(); err == nil {
			region = regionFromPath(l.Path)
		}
		switch {
		case r1.StatusCode == 403 || r2.StatusCode == 403:
			return Unlock{Status: "no"}
		case (r1.StatusCode < 400) && (r2.StatusCode < 400):
			return Unlock{Status: "yes", Region: orUS(region)}
		case r1.StatusCode < 400:
			return Unlock{Status: "partial", Region: orUS(region), Detail: "originals only"}
		}
		return Unlock{Status: "no"}
	}},
	{"disney", "Disney+", func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, f, "https://www.disneyplus.com/", nil)
		if err != nil {
			return Unlock{Status: "failed", Detail: "network error"}
		}
		if strings.Contains(resp.Request.URL.Path, "unavailable") || resp.StatusCode == 403 {
			return Unlock{Status: "no"}
		}
		return Unlock{Status: "yes", Region: regionFromPath(resp.Request.URL.Path)}
	}},
	{"youtube", "YouTube Premium", func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://www.youtube.com/premium", map[string]string{"Cookie": "CONSENT=YES+cb"})
		if err != nil {
			return Unlock{Status: "failed", Detail: "network error"}
		}
		region := ""
		if m := regexp.MustCompile(`"INNERTUBE_CONTEXT_GL":"([A-Z]{2})"`).FindStringSubmatch(body); m != nil {
			region = m[1]
		}
		if strings.Contains(body, "www.google.cn") || region == "CN" {
			return Unlock{Status: "no", Region: "CN"}
		}
		if strings.Contains(body, "Premium is not available in your country") {
			return Unlock{Status: "no", Region: region}
		}
		if strings.Contains(body, "ad-free") || strings.Contains(body, "Premium") {
			return Unlock{Status: "yes", Region: region}
		}
		return Unlock{Status: "failed", Detail: "unrecognized page"}
	}},
	{"chatgpt", "ChatGPT", func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://api.openai.com/compliance/cookie_requirements", map[string]string{"Authorization": "Bearer null"})
		resp2, body2, err2 := get(ctx, f, "https://ios.chat.openai.com/", nil)
		if err != nil && err2 != nil {
			return Unlock{Status: "failed", Detail: "network error"}
		}
		region := trace(ctx, f, "chatgpt.com")
		blockedWeb := strings.Contains(body, "unsupported_country")
		blockedApp := err2 == nil && (resp2.StatusCode == 403 || strings.Contains(body2, "VPN") || strings.Contains(body2, "unsupported_country"))
		switch {
		case !blockedWeb && !blockedApp:
			return Unlock{Status: "yes", Region: region}
		case blockedWeb && blockedApp:
			return Unlock{Status: "no", Region: region}
		case blockedApp:
			return Unlock{Status: "partial", Region: region, Detail: "web only"}
		}
		return Unlock{Status: "partial", Region: region, Detail: "app only"}
	}},
	{"claude", "Claude", func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, f, "https://claude.ai/", nil)
		if err != nil {
			return Unlock{Status: "failed", Detail: "network error"}
		}
		region := trace(ctx, f, "claude.ai")
		if strings.Contains(resp.Request.URL.Path, "unavailable") {
			return Unlock{Status: "no", Region: region}
		}
		return Unlock{Status: "yes", Region: region}
	}},
	{"gemini", "Gemini", func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://gemini.google.com/", nil)
		if err != nil {
			return Unlock{Status: "failed", Detail: "network error"}
		}
		region := ""
		if m := regexp.MustCompile(`,2,1,200,"([A-Z]{3})"`).FindStringSubmatch(body); m != nil {
			region = m[1]
		}
		if strings.Contains(body, "45631641,null,true") {
			return Unlock{Status: "yes", Region: region}
		}
		return Unlock{Status: "no", Region: region}
	}},
	{"spotify", "Spotify", func(ctx context.Context, c, f *http.Client) Unlock {
		resp, body, err := get(ctx, f, "https://spclient.wg.spotify.com/signup/public/v1/account?validate=1&key=4c7a36d5260abca4af282779720cf631", nil)
		if err != nil {
			return Unlock{Status: "failed", Detail: "network error"}
		}
		var m struct {
			Status   int    `json:"status"`
			Country  string `json:"country"`
			Launched bool   `json:"is_country_launched"`
		}
		_ = json.Unmarshal([]byte(body), &m)
		if resp.StatusCode == 200 && m.Status == 311 && m.Launched {
			return Unlock{Status: "yes", Region: m.Country}
		}
		return Unlock{Status: "no", Region: m.Country}
	}},
	{"tiktok", "TikTok", func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://www.tiktok.com/", nil)
		if err != nil {
			return Unlock{Status: "failed", Detail: "network error"}
		}
		if m := regexp.MustCompile(`"region":"([A-Z]{2})"`).FindStringSubmatch(body); m != nil {
			return Unlock{Status: "yes", Region: m[1]}
		}
		return Unlock{Status: "no"}
	}},
	{"steam", "Steam currency", func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://store.steampowered.com/app/761830", nil)
		if err != nil {
			return Unlock{Status: "failed", Detail: "network error"}
		}
		if m := regexp.MustCompile(`"priceCurrency" content="([A-Z]{3})"`).FindStringSubmatch(body); m != nil {
			return Unlock{Status: "yes", Region: m[1]}
		}
		return Unlock{Status: "failed", Detail: "unrecognized page"}
	}},
}

func regionFromPath(p string) string {
	parts := strings.Split(strings.Trim(p, "/"), "/")
	if len(parts) > 0 && len(parts[0]) >= 2 {
		code := strings.ToUpper(parts[0][:2])
		if len(parts[0]) == 2 || parts[0][2] == '-' {
			return code
		}
	}
	return ""
}

func orUS(r string) string {
	if r == "" {
		return "US"
	}
	return r
}

// UnlockList returns the services that can be checked, all pending.
func UnlockList() []Unlock {
	out := make([]Unlock, len(checkers))
	for i, c := range checkers {
		out[i] = Unlock{ID: c.id, Name: c.name, Status: "pending"}
	}
	return out
}

// CheckUnlock checks the services named by ids (all when empty) through
// the proxy, calling report as each result arrives.
func CheckUnlock(ctx context.Context, proxy string, ids []string, report func(Unlock)) {
	plain := Client(proxy, 12*time.Second, false)
	follow := Client(proxy, 12*time.Second, true)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for _, ch := range checkers {
		if len(ids) > 0 && !contains(ids, ch.id) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			res := ch.check(ctx, plain, follow)
			res.ID, res.Name, res.At = ch.id, ch.name, time.Now().Format(time.RFC3339)
			report(res)
		}()
	}
	wg.Wait()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
