package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Service unlock: whether a service takes the connection as it leaves,
// rather than whether its site opens, which the site list tests. A site
// may open and still turn the visitor away: Netflix with its own shows
// only, ChatGPT not in the visitor's country, Google behind a captcha.
// Each check reads what the service itself says, and a page it does not
// recognize is "failed", never a guess. The verdicts are functions of the
// answers alone (judge*), tested against their markers.

// Unlock is the availability of a service.
type Unlock struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"` // yes, no, partial, failed, pending
	Region string `json:"region,omitempty"`
	Detail string `json:"detail,omitempty"`
	At     string `json:"at,omitempty"`
	// Via is the proxy the check went out through, as the core's rules
	// chose it for the service's own addresses.
	Via string `json:"via,omitempty"`
	// Group is the kind of service: media, social, ai, platform, region.
	Group string `json:"group,omitempty"`
	// Site is the connectivity page's site of the same service, whose
	// card shows the result too; "" for checks with no site of their own.
	Site string `json:"site,omitempty"`
	// Optional checks run only once turned on.
	Optional bool `json:"optional,omitempty"`
}

// Unlock groups, in the order the page shows them.
var UnlockGroups = []string{"media", "social", "ai", "platform", "region"}

type checker struct {
	id, name, group, site string
	optional              bool
	check                 func(ctx context.Context, c, follow *http.Client) Unlock
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

func failed(detail string) Unlock { return Unlock{Status: "failed", Detail: detail} }

var netErr = failed("network error")

func unrecognized() Unlock { return failed("unrecognized page") }

var (
	reGL        = regexp.MustCompile(`"INNERTUBE_CONTEXT_GL":"([A-Z]{2})"`)
	reGemini    = regexp.MustCompile(`,2,1,200,"([A-Z]{3})"`)
	reTikTok    = regexp.MustCompile(`"region":"([A-Z]{2})"`)
	reSteam     = regexp.MustCompile(`"priceCurrency" content="([A-Z]{3})"`)
	rePrime     = regexp.MustCompile(`"currentTerritory":"([A-Z]{2})"`)
	reTwoLetter = regexp.MustCompile(`^[A-Z]{2}$`)
	reBing      = regexp.MustCompile(`Region:"([A-Z]{2})"`)
)

var checkers = []checker{
	// ---- Video and music ----
	{"netflix", "Netflix", "media", "netflix", false, func(ctx context.Context, c, f *http.Client) Unlock {
		// A Netflix original and a licensed title; the country Netflix takes
		// the visitor for is the one its speed test names.
		r1, _, err1 := get(ctx, c, "https://www.netflix.com/title/81280792", nil)
		r2, _, err2 := get(ctx, c, "https://www.netflix.com/title/70143836", nil)
		if err1 != nil || err2 != nil {
			return netErr
		}
		region := ""
		if _, body, err := get(ctx, f, netflixSpeedtest, nil); err == nil {
			region = judgeNetflixRegion(body)
		}
		if region == "" {
			if loc := r1.Header.Get("X-Originating-Url"); loc != "" {
				region = regionFromPath(loc)
			}
			if l, err := r1.Location(); err == nil {
				region = regionFromPath(l.Path)
			}
		}
		return judgeNetflix(r1.StatusCode, r2.StatusCode, region)
	}},
	{"disneyplus", "Disney+", "media", "disneyplus", false, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, f, "https://www.disneyplus.com/", nil)
		if err != nil {
			return netErr
		}
		return judgeDisney(resp.StatusCode, resp.Request.URL.Path)
	}},
	{"primevideo", "Prime Video", "media", "primevideo", false, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://www.primevideo.com", nil)
		if err != nil {
			return netErr
		}
		return judgePrime(body)
	}},
	{"youtube", "YouTube Premium", "media", "youtube", false, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://www.youtube.com/premium", map[string]string{"Cookie": "CONSENT=YES+cb"})
		if err != nil {
			return netErr
		}
		return judgeYouTube(body)
	}},
	{"dazn", "DAZN", "media", "dazn", false, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := post(ctx, f, "https://startup.core.indazn.com/misl/v5/Startup",
			`{"LandingPageKey":"generic","languages":"en-US,en","Platform":"web","PlatformAttributes":{},"Manufacturer":"","PromoCode":"","Version":"2"}`)
		if err != nil {
			return netErr
		}
		return judgeDAZN(body)
	}},
	{"spotify", "Spotify", "media", "spotify", false, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, body, err := get(ctx, f, "https://spclient.wg.spotify.com/signup/public/v1/account?validate=1&key=4c7a36d5260abca4af282779720cf631", nil)
		if err != nil {
			return netErr
		}
		return judgeSpotify(resp.StatusCode, body)
	}},
	{"peacock", "Peacock", "media", "peacock", true, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, f, "https://www.peacocktv.com/", nil)
		if err != nil {
			return netErr
		}
		return judgePeacock(resp.StatusCode, resp.Request.URL.Path)
	}},
	{"paramount", "Paramount+", "media", "paramount", true, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, f, "https://www.paramountplus.com/", nil)
		if err != nil {
			return netErr
		}
		return judgeParamount(resp.StatusCode, resp.Request.URL.Path)
	}},
	{"tubi", "Tubi", "media", "tubi", true, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, body, err := get(ctx, f, "https://tubitv.com/home", nil)
		if err != nil {
			return netErr
		}
		return judgeTubi(resp.Request.URL.Host, body)
	}},
	{"youtube-cdn", "YouTube CDN", "media", "youtube", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://redirector.googlevideo.com/report_mapping", nil)
		if err != nil {
			return netErr
		}
		return judgeYouTubeCDN(body)
	}},

	// ---- Social ----
	{"tiktok", "TikTok", "social", "tiktok", false, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://www.tiktok.com/", nil)
		if err != nil {
			return netErr
		}
		return judgeTikTok(body)
	}},
	{"reddit", "Reddit", "social", "reddit", false, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, c, "https://www.reddit.com/", nil)
		if err != nil {
			return netErr
		}
		return judgeReddit(resp.StatusCode)
	}},

	// ---- AI ----
	{"chatgpt", "ChatGPT", "ai", "chatgpt", false, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://api.openai.com/compliance/cookie_requirements", map[string]string{"Authorization": "Bearer null"})
		resp2, body2, err2 := get(ctx, f, "https://ios.chat.openai.com/", nil)
		if err != nil && err2 != nil {
			return netErr
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
	{"claude", "Claude", "ai", "claude", false, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, f, "https://claude.ai/", nil)
		if err != nil {
			return netErr
		}
		res := judgeClaude(resp.StatusCode, resp.Request.URL.Path)
		if res.Status != "failed" {
			res.Region = trace(ctx, f, "claude.ai")
		}
		return res
	}},
	{"claude-api", "Claude API", "ai", "anthropic-api", false, func(ctx context.Context, c, f *http.Client) Unlock {
		// A made-up key: the API answers it by country before it looks at it.
		resp, body, err := get(ctx, f, "https://api.anthropic.com/v1/models", map[string]string{"x-api-key": "x", "anthropic-version": "2023-06-01"})
		if err != nil {
			return netErr
		}
		res := judgeClaudeAPI(resp.StatusCode, body)
		if res.Status != "failed" {
			res.Region = trace(ctx, f, "api.anthropic.com")
		}
		return res
	}},
	{"gemini", "Gemini", "ai", "gemini", false, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, body, err := get(ctx, f, "https://gemini.google.com/", nil)
		if err != nil {
			return netErr
		}
		return judgeGemini(resp.StatusCode, resp.Request.URL.Path, body)
	}},
	// ---- Platforms ----
	{"google-search", "Google", "platform", "google", false, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, body, err := get(ctx, f, "https://www.google.com/search?q=mygo&hl=en", nil)
		if err != nil {
			return netErr
		}
		return judgeGoogleSearch(resp.StatusCode, resp.Request.URL.Path, body)
	}},
	{"apple-region", "Apple", "platform", "apple", false, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, body, err := get(ctx, f, "https://gspe1-ssl.ls.apple.com/pep/gcc", nil)
		if err != nil {
			return netErr
		}
		return judgeAppleRegion(resp.StatusCode, body)
	}},
	{"bing-region", "Bing", "platform", "microsoft", false, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, body, err := get(ctx, f, "https://www.bing.com/search?q=mygo", nil)
		if err != nil {
			return netErr
		}
		return judgeBingRegion(resp.StatusCode, body)
	}},
	{"steam", "Steam", "platform", "steam", false, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://store.steampowered.com/app/761830", nil)
		if err != nil {
			return netErr
		}
		if m := reSteam.FindStringSubmatch(body); m != nil {
			return Unlock{Status: "yes", Region: m[1]}
		}
		return unrecognized()
	}},
	{"wikipedia-edit", "Wikipedia", "platform", "wikipedia", true, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, body, err := get(ctx, f, "https://en.wikipedia.org/w/index.php?title=Wikipedia:Sandbox&action=edit", nil)
		if err != nil {
			return netErr
		}
		return judgeWikipediaEdit(resp.StatusCode, body)
	}},
	{"google-play", "Google Play", "platform", "", true, func(ctx context.Context, c, f *http.Client) Unlock {
		// The page is large; the country is in the first part of it.
		resp, body, err := get(ctx, f, "https://play.google.com/", map[string]string{"Accept-Language": "en-US;q=0.9"})
		if err != nil {
			return netErr
		}
		return judgeGooglePlay(resp.StatusCode, body)
	}},

	// ---- Services of one region ----
	{"bilibili-hkmotw", "Bilibili", "region", "bilibili", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://api.bilibili.com/pgc/player/web/playurl?avid=18281381&cid=29892777&qn=0&type=&otype=json&ep_id=183799&fourk=1&fnver=0&fnval=16&module=bangumi", nil)
		if err != nil {
			return netErr
		}
		return judgeBilibili(body)
	}},
	{"bilibili-tw", "Bilibili", "region", "bilibili", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://api.bilibili.com/pgc/player/web/playurl?avid=50762638&cid=100279344&qn=0&type=&otype=json&ep_id=268176&fourk=1&fnver=0&fnval=16&module=bangumi", nil)
		if err != nil {
			return netErr
		}
		return judgeBilibili(body)
	}},
	{"abema", "ABEMA", "region", "abema", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://api.abema.io/v1/ip/check?device=android", nil)
		if err != nil {
			return netErr
		}
		return judgeAbema(body)
	}},
	{"bbc-iplayer", "BBC iPlayer", "region", "bbc", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://open.live.bbc.co.uk/mediaselector/6/select/version/2.0/mediaset/pc/vpid/bbc_one_london/format/json/jsfunc/JS_callbacks0", nil)
		if err != nil {
			return netErr
		}
		return judgeBBC(body)
	}},
	{"tvb-anywhere", "TVBAnywhere+", "region", "", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://uapisfm.tvbanywhere.com.sg/geoip/check/platform/android", nil)
		if err != nil {
			return netErr
		}
		return judgeTVB(body)
	}},
	{"kktv", "KKTV", "region", "kktv", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://api.kktv.me/v3/ipcheck", nil)
		if err != nil {
			return netErr
		}
		return judgeKKTV(body)
	}},
	{"hami", "Hami Video", "region", "hami", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://hamivideo.hinet.net/api/play.do?id=OTT_VOD_0000249064&freeProduct=1", nil)
		if err != nil {
			return netErr
		}
		return judgeHami(body)
	}},
	{"mytvsuper", "myTV SUPER", "region", "mytvsuper", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://www.mytvsuper.com/api/auth/getSession/self/", nil)
		if err != nil {
			return netErr
		}
		return judgeMyTVSuper(body)
	}},
	{"viu", "Viu", "region", "viu", true, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, c, "https://www.viu.com/", nil)
		if err != nil {
			return netErr
		}
		return judgeViu(resp.Header.Get("Location"))
	}},
	{"tving", "TVING", "region", "tving", true, func(ctx context.Context, c, f *http.Client) Unlock {
		// The web player's public key, as the site's own pages send it.
		_, body, err := get(ctx, f, "https://api.tving.com/v2/media/stream/info?apiKey=1e7952d0917d6aab1f0293a063697610&mediaCode=RV60891248", nil)
		if err != nil {
			return netErr
		}
		return judgeTving(body)
	}},
	{"coupang-play", "Coupang Play", "region", "coupangplay", true, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, c, "https://www.coupangplay.com/", nil)
		if err != nil {
			return netErr
		}
		return judgeCoupangPlay(resp.StatusCode, resp.Header.Get("Location"))
	}},
	{"radiko", "radiko", "region", "radiko", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := get(ctx, f, "https://radiko.jp/area", nil)
		if err != nil {
			return netErr
		}
		return judgeRadiko(body)
	}},
	{"dmm-tv", "DMM TV", "region", "dmm-tv", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := post(ctx, f, "https://api.beacon.dmm.com/v1/streaming/start",
			`{"player_name":"dmmtv_browser","player_version":"0.0.0","content_type_detail":"VOD_SVOD","content_id":"11uvjcm4fw2wdu7drtd1epnvz","purchase_product_id":null}`)
		if err != nil {
			return netErr
		}
		return judgeDMMTV(body)
	}},
	{"channel4", "Channel 4", "region", "channel4", true, func(ctx context.Context, c, f *http.Client) Unlock {
		resp, _, err := get(ctx, c, "https://www.channel4.com/simulcast/channels/C4", nil)
		if err != nil {
			return netErr
		}
		return judgeChannel4(resp.StatusCode)
	}},
	{"itvx", "ITVX", "region", "", true, func(ctx context.Context, c, f *http.Client) Unlock {
		_, body, err := postWith(ctx, f, "https://magni.itv.com/playlist/itvonline/ITV/10_4782_0001.001",
			`{"client":{"id":"browser"},"device":{"deviceGroup":"dotcom"},"variantAvailability":{"player":"hls","featureset":["mpeg-dash","hls"],"platformTag":"dotcom","drm":{"system":"widevine","maxSupported":"L3"}},"user":{}}`,
			map[string]string{"Accept": "application/vnd.itv.vod.playlist.v2+json", "Origin": "https://www.itv.com", "Referer": "https://www.itv.com/"})
		if err != nil {
			return netErr
		}
		return judgeITVX(body)
	}},
}

// ---- Verdicts ----

func judgePrime(body string) Unlock {
	if strings.Contains(body, "isServiceRestricted") {
		return Unlock{Status: "no"}
	}
	if m := rePrime.FindStringSubmatch(body); m != nil {
		return Unlock{Status: "yes", Region: m[1]}
	}
	return unrecognized()
}

func judgeYouTube(body string) Unlock {
	region := ""
	if m := reGL.FindStringSubmatch(body); m != nil {
		region = m[1]
	}
	switch {
	case strings.Contains(body, "www.google.cn") || region == "CN":
		return Unlock{Status: "no", Region: "CN"}
	case strings.Contains(body, "Premium is not available in your country"):
		return Unlock{Status: "no", Region: region}
	case strings.Contains(body, "ad-free") || strings.Contains(body, "Premium"):
		return Unlock{Status: "yes", Region: region}
	}
	return unrecognized()
}

// judgeSpotify reads Spotify's sign-up check: whether the country it
// takes the visitor for is one it has launched in, and takes sign-ups
// from (120 and 320 are its refusals).
func judgeSpotify(status int, body string) Unlock {
	var m struct {
		Status   *int   `json:"status"`
		Country  string `json:"country"`
		Launched bool   `json:"is_country_launched"`
	}
	if status >= 400 || json.Unmarshal([]byte(body), &m) != nil || m.Status == nil {
		return unrecognized()
	}
	if !m.Launched || *m.Status == 120 || *m.Status == 320 {
		return Unlock{Status: "no", Region: m.Country}
	}
	return Unlock{Status: "yes", Region: m.Country}
}

// judgeBingRegion: the market Bing serves the visitor, which Microsoft's
// services (Copilot among them) go by.
func judgeBingRegion(status int, body string) Unlock {
	if m := reBing.FindStringSubmatch(body); status < 400 && m != nil {
		return Unlock{Status: "yes", Region: m[1]}
	}
	return unrecognized()
}

// judgeGemini: Gemini sends visitors from where it is not offered to its
// FAQ, or says so on its page; elsewhere its page names their country.
func judgeGemini(status int, path, body string) Unlock {
	region := ""
	if m := reGemini.FindStringSubmatch(body); m != nil {
		region = m[1]
	}
	switch {
	case strings.HasPrefix(path, "/faq") || strings.Contains(body, "currently supported in your country") || strings.Contains(body, "not available in your country"):
		return Unlock{Status: "no", Region: region}
	case status < 400 && region != "":
		return Unlock{Status: "yes", Region: region}
	}
	return unrecognized()
}

// judgeTikTok reads the region TikTok's page is for; a page without one
// (a challenge, an error) says nothing either way.
func judgeTikTok(body string) Unlock {
	if m := reTikTok.FindStringSubmatch(body); m != nil {
		return Unlock{Status: "yes", Region: m[1]}
	}
	return unrecognized()
}

func judgeDAZN(body string) Unlock {
	var m struct {
		Region struct {
			Allowed *bool  `json:"isAllowed"`
			Country string `json:"GeolocatedCountry"`
		} `json:"Region"`
	}
	if json.Unmarshal([]byte(body), &m) != nil || m.Region.Allowed == nil {
		return unrecognized()
	}
	region := strings.ToUpper(m.Region.Country)
	if *m.Region.Allowed {
		return Unlock{Status: "yes", Region: region}
	}
	return Unlock{Status: "no", Region: region}
}

// judgeReddit: Reddit turns away the addresses it takes for data centers
// and VPNs with a 403 before its page.
func judgeReddit(status int) Unlock {
	switch {
	case status == 403 || status == 429:
		return Unlock{Status: "no", Detail: "blocked"}
	case status < 400:
		return Unlock{Status: "yes"}
	}
	return unrecognized()
}

// judgeGoogleSearch: Google answers addresses it distrusts with a captcha
// (/sorry/) rather than results.
func judgeGoogleSearch(status int, path, body string) Unlock {
	switch {
	case strings.HasPrefix(path, "/sorry") || strings.Contains(body, "unusual traffic") || status == 429:
		return Unlock{Status: "partial", Detail: "captcha"}
	case status < 400 && strings.Contains(body, "mygo"):
		return Unlock{Status: "yes"}
	}
	return unrecognized()
}

// judgeAppleRegion: the country Apple's services take the visitor for, as
// its location service answers it in two letters.
func judgeAppleRegion(status int, body string) Unlock {
	code := strings.ToUpper(strings.TrimSpace(body))
	if status < 400 && reTwoLetter.MatchString(code) {
		return Unlock{Status: "yes", Region: code}
	}
	return unrecognized()
}

// judgeWikipediaEdit: Wikipedia lets anyone edit, but not from addresses it
// has blocked, as many proxies' are.
func judgeWikipediaEdit(status int, body string) Unlock {
	switch {
	case strings.Contains(body, "blocked from editing") || strings.Contains(body, "Banned"):
		return Unlock{Status: "no", Detail: "blocked"}
	case status < 400 && strings.Contains(body, "wpTextbox1"):
		return Unlock{Status: "yes"}
	}
	return unrecognized()
}

// judgeBilibili reads Bilibili's player for a title only some regions may
// play: code 0 plays it, -10403 is the region's refusal.
func judgeBilibili(body string) Unlock {
	var m struct {
		Code *int `json:"code"`
	}
	if json.Unmarshal([]byte(body), &m) != nil || m.Code == nil {
		return unrecognized()
	}
	switch *m.Code {
	case 0:
		return Unlock{Status: "yes"}
	case -10403, -404:
		return Unlock{Status: "no"}
	}
	return unrecognized()
}

// judgeAbema: ABEMA plays in Japan, and its overseas catalog elsewhere;
// it turns away the addresses it takes for VPNs.
func judgeAbema(body string) Unlock {
	var m struct {
		Country string `json:"isoCountryCode"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(body), &m) != nil {
		return unrecognized()
	}
	if m.Message == "anonymous_ip" { // a VPN's or data center's address
		return Unlock{Status: "no", Detail: "blocked"}
	}
	if m.Country == "" {
		return unrecognized()
	}
	if m.Country == "JP" {
		return Unlock{Status: "yes", Region: "JP"}
	}
	return Unlock{Status: "partial", Region: m.Country, Detail: "overseas only"}
}

func judgeBBC(body string) Unlock {
	switch {
	case strings.Contains(body, "geolocation"):
		return Unlock{Status: "no"}
	case strings.Contains(body, `"media":[`):
		return Unlock{Status: "yes", Region: "GB"}
	}
	return unrecognized()
}

func judgeTVB(body string) Unlock {
	var m struct {
		Allowed *bool  `json:"allow_in_this_country"`
		Country string `json:"country"`
	}
	if json.Unmarshal([]byte(body), &m) != nil || m.Allowed == nil {
		return unrecognized()
	}
	if *m.Allowed {
		return Unlock{Status: "yes", Region: m.Country}
	}
	return Unlock{Status: "no", Region: m.Country}
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

func post(ctx context.Context, c *http.Client, rawURL, body string) (*http.Response, string, error) {
	return postWith(ctx, c, rawURL, body, nil)
}

// postWith posts a JSON body with hdr besides the browser's own headers.
func postWith(ctx context.Context, c *http.Client, rawURL, body string, hdr map[string]string) (*http.Response, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(body))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	return send(c, req, hdr)
}

// ---- Running them ----

// UnlockCatalog is every check, pending, the optional ones too.
func UnlockCatalog() []Unlock {
	out := make([]Unlock, len(checkers))
	for i, c := range checkers {
		out[i] = Unlock{ID: c.id, Name: c.name, Status: "pending", Group: c.group, Site: c.site, Optional: c.optional}
	}
	return out
}

// UnlockList returns the checks that run: the default ones but those
// hidden, and the optional ones turned on.
func UnlockList(hidden, enabled []string) []Unlock {
	var out []Unlock
	for _, u := range UnlockCatalog() {
		if u.Optional && slices.Contains(enabled, u.ID) || !u.Optional && !slices.Contains(hidden, u.ID) {
			out = append(out, u)
		}
	}
	return out
}

// IsUnlockID reports whether id names a check.
func IsUnlockID(id string) bool {
	return slices.ContainsFunc(checkers, func(c checker) bool { return c.id == id })
}

// hostLog records the hosts a check's requests went to.
type hostLog struct {
	mu    sync.Mutex
	hosts []string
}

type logged struct {
	rt  http.RoundTripper
	log *hostLog
}

func (l logged) RoundTrip(req *http.Request) (*http.Response, error) {
	l.log.mu.Lock()
	if h := req.URL.Hostname(); !slices.Contains(l.log.hosts, h) {
		l.log.hosts = append(l.log.hosts, h)
	}
	l.log.mu.Unlock()
	return l.rt.RoundTrip(req)
}

// CheckUnlock checks the services named by ids (all the default ones when
// empty) through the proxy, calling report as each result arrives. With
// route, each check keeps its connections open until route has named the
// proxy they went through, from the hosts they went to.
func CheckUnlock(ctx context.Context, proxy string, ids []string, route func(hosts []string) string, report func(Unlock)) {
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for _, ch := range checkers {
		if len(ids) > 0 && !slices.Contains(ids, ch.id) || len(ids) == 0 && ch.optional {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			// A transport of its own, so that its connections stay open
			// for route to find, and are its alone.
			tr := transport(proxy, 12*time.Second, route != nil)
			log := &hostLog{}
			plain, follow := clientOf(tr, log, 12*time.Second, false), clientOf(tr, log, 12*time.Second, true)
			res := ch.check(ctx, plain, follow)
			if route != nil {
				log.mu.Lock()
				hosts := slices.Clone(log.hosts)
				log.mu.Unlock()
				res.Via = route(hosts)
			}
			tr.CloseIdleConnections()
			res.ID, res.Name, res.At = ch.id, ch.name, time.Now().Format(time.RFC3339)
			res.Group, res.Site, res.Optional = ch.group, ch.site, ch.optional
			report(res)
		}()
	}
	wg.Wait()
}
