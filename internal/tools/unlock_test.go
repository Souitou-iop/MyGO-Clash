package tools

import (
	"slices"
	"testing"
)

func TestUnlockVerdicts(t *testing.T) {
	for _, c := range []struct {
		name         string
		got          Unlock
		status, area string
	}{
		{"prime ok", judgePrime(`..."currentTerritory":"JP",...`), "yes", "JP"},
		{"prime no", judgePrime(`{"isServiceRestricted":true}`), "no", ""},
		{"prime ?", judgePrime(`<html>`), "failed", ""},
		{"youtube cn", judgeYouTube(`https://www.google.cn/`), "no", "CN"},
		{"youtube ok", judgeYouTube(`"INNERTUBE_CONTEXT_GL":"US" ad-free`), "yes", "US"},
		{"dazn ok", judgeDAZN(`{"Region":{"isAllowed":true,"GeolocatedCountry":"de"}}`), "yes", "DE"},
		{"dazn no", judgeDAZN(`{"Region":{"isAllowed":false,"GeolocatedCountry":"cn"}}`), "no", "CN"},
		{"dazn ?", judgeDAZN(`{}`), "failed", ""},
		{"spotify ok", judgeSpotify(200, `{"status":1,"country":"JP","is_country_launched":true}`), "yes", "JP"},
		{"spotify old ok", judgeSpotify(200, `{"status":311,"country":"US","is_country_launched":true}`), "yes", "US"},
		{"spotify no", judgeSpotify(200, `{"status":320,"country":"CN","is_country_launched":false}`), "no", "CN"},
		{"bing", judgeBingRegion(200, `_G={Region:"JP",Lang:"ja-JP"}`), "yes", "JP"},
		{"gemini ok", judgeGemini(200, "/", `x,2,1,200,"JPN",y`), "yes", "JPN"},
		{"gemini no", judgeGemini(200, "/faq", ``), "no", ""},
		{"gemini ?", judgeGemini(200, "/", `<html>`), "failed", ""},
		{"tiktok", judgeTikTok(`{"region":"JP","language":"en"}`), "yes", "JP"},
		{"tiktok ?", judgeTikTok(`<html>challenge</html>`), "failed", ""},
		{"reddit ok", judgeReddit(200), "yes", ""},
		{"reddit no", judgeReddit(403), "no", ""},
		{"google captcha", judgeGoogleSearch(200, "/sorry/index", ""), "partial", ""},
		{"google ok", judgeGoogleSearch(200, "/search", "mygo - Google Search"), "yes", ""},
		{"apple", judgeAppleRegion(200, "SG\n"), "yes", "SG"},
		{"apple ?", judgeAppleRegion(200, "<html>"), "failed", ""},
		{"wiki ok", judgeWikipediaEdit(200, `<textarea id="wpTextbox1">`), "yes", ""},
		{"wiki no", judgeWikipediaEdit(200, `You are currently blocked from editing`), "no", ""},
		{"bili ok", judgeBilibili(`{"code":0,"result":{}}`), "yes", ""},
		{"bili no", judgeBilibili(`{"code":-10403,"message":"抱歉您所在地区不可观看！"}`), "no", ""},
		{"abema jp", judgeAbema(`{"isoCountryCode":"JP"}`), "yes", "JP"},
		{"abema abroad", judgeAbema(`{"isoCountryCode":"US"}`), "partial", "US"},
		{"abema vpn", judgeAbema(`{"message":"anonymous_ip"}`), "no", ""},
		{"bbc ok", judgeBBC(`JS_callbacks0({"media":[{"kind":"video"}]})`), "yes", "GB"},
		{"bbc no", judgeBBC(`JS_callbacks0({"result":"geolocation"})`), "no", ""},
		{"tvb ok", judgeTVB(`{"allow_in_this_country":true,"country":"HK"}`), "yes", "HK"},
	} {
		if c.got.Status != c.status || c.got.Region != c.area {
			t.Errorf("%s: %s %q, want %s %q", c.name, c.got.Status, c.got.Region, c.status, c.area)
		}
	}
}

// Every check has a group the page knows, a unique id, and a site of the
// page's when it names one.
func TestUnlockCatalog(t *testing.T) {
	ids := map[string]bool{}
	defaults := 0
	for _, u := range UnlockCatalog() {
		if ids[u.ID] {
			t.Errorf("check %s twice", u.ID)
		}
		ids[u.ID] = true
		if !slices.Contains(UnlockGroups, u.Group) {
			t.Errorf("check %s: group %q", u.ID, u.Group)
		}
		if u.Site != "" && !IsKnownSite(u.Site) {
			t.Errorf("check %s: site %s is no site", u.ID, u.Site)
		}
		if !u.Optional {
			defaults++
		}
	}
	if got := len(UnlockList(nil, nil)); got != defaults {
		t.Errorf("default list has %d, want %d", got, defaults)
	}
	got := UnlockList([]string{"netflix"}, []string{"abema"})
	if slices.ContainsFunc(got, func(u Unlock) bool { return u.ID == "netflix" }) || !slices.ContainsFunc(got, func(u Unlock) bool { return u.ID == "abema" }) {
		t.Errorf("hidden or enabled checks not heeded")
	}
	t.Logf("%d checks, %d by default", len(ids), defaults)
}

// The verdicts of the checks of one service, against what the services
// answered (addresses left out): a refusal that was caught, and the answer
// inside where the report of the check could not catch one. A page that is
// none of them is failed.
func TestUnlockVerdictsServices(t *testing.T) {
	const challenge = `<!DOCTYPE html><html><head><title>Just a moment...</title></head><body>Enable JavaScript and cookies to continue</body></html>`
	const tvingNo = `{"header":{"message":"OK","status":200},"body":{"server":{"time":"x","host":"api.tving.com","port":80,"context":""},"user":{"id":"GUEST","profile_no":"0","no":"0"},"result":{"code":"001","message":"Our service is only available in South Korea.\nTVING is making efforts to deliver our service to global audience soon.","sub_message":"","auth_type":null},"content":{}}}`
	const mapping = "x.x.x.x => nrt13s60 (52.69.128.0/17)\n\nDebug Info:\n  Client IP: x.x.x.x\n"
	for _, c := range []struct {
		name         string
		got          Unlock
		status, area string
		detail       string
	}{
		// Netflix
		{"netflix region", Unlock{Region: judgeNetflixRegion(`{"client":{"ip":"x.x.x.x","asn":"16509","location":{"city":"Tokyo","country":"JP"}},"targets":[{"name":"https://ipv4-c198-tyo001-ix.1.oca.nflxvideo.net/speedtest","location":{"city":"Tokyo","country":"JP"}}]}`), Status: "yes"}, "yes", "JP", ""},
		{"netflix region ?", Unlock{Region: judgeNetflixRegion(`<html>`), Status: "yes"}, "yes", "", ""},
		{"netflix region lower", Unlock{Region: judgeNetflixRegion(`{"client":{"location":{"country":"jp"}}}`), Status: "yes"}, "yes", "JP", ""},
		{"netflix region long", Unlock{Region: judgeNetflixRegion(`{"client":{"location":{"country":"Japan"}}}`), Status: "yes"}, "yes", "", ""},
		{"netflix all", judgeNetflix(200, 200, "JP"), "yes", "JP", ""},
		{"netflix no region is no US", judgeNetflix(200, 200, ""), "yes", "", ""},
		{"netflix originals", judgeNetflix(200, 404, "TW"), "partial", "TW", "originals only"},
		{"netflix 403", judgeNetflix(403, 200, "JP"), "no", "", ""},
		{"netflix none", judgeNetflix(404, 404, "JP"), "no", "", ""},
		// The three that misjudged a challenge.
		{"claude web ok", judgeClaude(200, "/login"), "yes", "", ""},
		{"claude web unavailable", judgeClaude(200, "/unavailable"), "no", "", ""},
		{"claude web challenge", judgeClaude(403, "/"), "failed", "", "challenge"},
		{"claude web 500", judgeClaude(500, "/"), "failed", "", "unrecognized page"},
		{"disney ok", judgeDisney(200, "/home"), "yes", "", ""},
		{"disney unavailable", judgeDisney(200, "/unavailable"), "no", "", ""},
		{"disney challenge", judgeDisney(403, "/"), "failed", "", "challenge"},
		{"disney 500", judgeDisney(500, "/"), "failed", "", "unrecognized page"},
		// Claude API: the 401 is caught, the 403 is the community's report.
		{"claude api ok", judgeClaudeAPI(401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"},"request_id":"req_011Ch5DXBX5maUL4qzM4wBDq"}`), "yes", "", ""},
		{"claude api no", judgeClaudeAPI(403, `{"error":{"type":"forbidden","message":"Request not allowed"}}`), "no", "", "blocked"},
		{"claude api challenge", judgeClaudeAPI(403, challenge), "failed", "", "unrecognized page"},
		{"claude api 500", judgeClaudeAPI(500, `{"error":{"type":"api_error"}}`), "failed", "", "unrecognized page"},
		{"claude api 401 other", judgeClaudeAPI(401, `<html>`), "failed", "", "unrecognized page"},
		// Media
		{"peacock no", judgePeacock(200, "/unavailable"), "no", "", ""},
		{"peacock ok", judgePeacock(200, "/watch/home"), "yes", "US", ""},
		{"peacock 403", judgePeacock(403, "/"), "failed", "", "unrecognized page"},
		{"paramount no", judgeParamount(200, "/intl/"), "no", "", ""},
		{"paramount ok", judgeParamount(200, "/"), "yes", "US", ""},
		{"paramount country", judgeParamount(200, "/ca/"), "yes", "CA", ""},
		{"paramount ?", judgeParamount(403, "/"), "failed", "", "unrecognized page"},
		{"tubi no host", judgeTubi("gdpr.tubi.tv", `<title>Tubi is the largest free movie and TV streaming service in the US. We are not available in Europe due to changes in EU laws.</title>`), "no", "", ""},
		{"tubi no text", judgeTubi("tubitv.com", `Tubi is not currently available in your area`), "no", "", ""},
		{"tubi ok", judgeTubi("tubitv.com", `{"manifest":"x"}`), "yes", "", ""},
		{"tubi ?", judgeTubi("tubitv.com", challenge), "failed", "", "unrecognized page"},
		{"youtube cdn", judgeYouTubeCDN(mapping), "yes", "NRT", ""},
		{"youtube cdn ?", judgeYouTubeCDN(`<html>nope</html>`), "failed", "", "unrecognized page"},
		{"youtube cdn empty", judgeYouTubeCDN(``), "failed", "", "unrecognized page"},
		// Platform
		{"play key", judgeGooglePlay(200, `...,"zQmIje":"JP",...`), "yes", "JP", ""},
		{"play footer", judgeGooglePlay(200, `<div class="yVZQTb">Japan (English)</div>`), "yes", "", "Japan"},
		{"play ?", judgeGooglePlay(200, challenge), "failed", "", "unrecognized page"},
		{"play 403", judgeGooglePlay(403, `"zQmIje":"JP"`), "failed", "", "unrecognized page"},
		// Region
		{"kktv no", judgeKKTV(`{"status":{"type":"OK","subtype":null,"message":null},"data":{"country":"JP","ip":"x.x.x.x","is_allowed":false}}`), "no", "JP", ""},
		{"kktv ok", judgeKKTV(`{"status":{"type":"OK"},"data":{"country":"TW","ip":"x.x.x.x","is_allowed":true}}`), "yes", "TW", ""},
		{"kktv ?", judgeKKTV(`{"status":{"type":"OK"},"data":{}}`), "failed", "", "unrecognized page"},
		{"hami no", judgeHami(`{"contentID":"OTT_VOD_0000249064","serverWaterWebUrl":"https://static-hamivideo.cdn.hinet.net/watermark.json","LogSN":"_","code":"06001-106","msg":"抱歉，本內容未授權於您目前所在地區、或國家收視，或請關閉VPN連線及 Apple裝置iCloud私密轉送。"}`), "no", "", ""},
		{"hami ok", judgeHami(`{"code":"06001-107"}`), "yes", "TW", ""},
		{"hami ?", judgeHami(`{"code":"00000-000"}`), "failed", "", "unrecognized page"},
		{"mytvsuper no", judgeMyTVSuper(`{"region":4,"country_code":"ZP","supported_country":false,"is_europe":false}`), "no", "", ""},
		{"mytvsuper ok", judgeMyTVSuper(`{"region":1,"country_code":"HK","supported_country":true}`), "yes", "HK", ""},
		{"mytvsuper ?", judgeMyTVSuper(`{"region":1}`), "failed", "", "unrecognized page"},
		{"viu no", judgeViu("https://www.viu.com/ott/no-service/"), "no", "", ""},
		{"viu ok", judgeViu("https://www.viu.com/ott/hk/index.php"), "yes", "HK", ""},
		{"viu ?", judgeViu(""), "failed", "", "unrecognized page"},
		{"tving no", judgeTving(tvingNo), "no", "", ""},
		{"tving ok", judgeTving(`{"header":{"message":"OK","status":200},"body":{"result":{"code":"000","message":""}}}`), "yes", "KR", ""},
		{"tving other 001", judgeTving(`{"header":{"status":200},"body":{"result":{"code":"001","message":"Sign in"}}}`), "failed", "", "unrecognized page"},
		{"tving ?", judgeTving(challenge), "failed", "", "unrecognized page"},
		{"tving error", judgeTving(`{"header":{"status":500},"body":{"result":{"code":"001","message":"only available in South Korea"}}}`), "failed", "", "unrecognized page"},
		{"coupang no", judgeCoupangPlay(302, "https://www.coupangplay.com/not-available"), "no", "", ""},
		{"coupang ok", judgeCoupangPlay(200, ""), "yes", "KR", ""},
		{"coupang ?", judgeCoupangPlay(503, ""), "failed", "", "unrecognized page"},
		{"radiko ok", judgeRadiko(`document.write('<span class="JP13">TOKYO JAPAN</span>');`), "yes", "JP", ""},
		{"radiko no", judgeRadiko(`document.write('<span class="OUT">OUT OF JAPAN</span>');`), "no", "", ""},
		{"radiko ?", judgeRadiko(challenge), "failed", "", "unrecognized page"},
		{"dmm no", judgeDMMTV(`{"is_blocked":true,"block_status":"FOREIGN"}`), "no", "", ""},
		{"dmm blocked", judgeDMMTV(`{"is_blocked":true}`), "no", "", ""},
		{"dmm ok", judgeDMMTV(`{"error":"UNAUTHORIZED"}`), "yes", "JP", ""},
		{"dmm ?", judgeDMMTV(challenge), "failed", "", "unrecognized page"},
		{"channel4 no", judgeChannel4(403), "no", "", ""},
		{"channel4 ok", judgeChannel4(200), "yes", "GB", ""},
		{"channel4 ?", judgeChannel4(503), "failed", "", "unrecognized page"},
		{"itvx no", judgeITVX(`{ "error":"geoblock",  "IP":"x.x.x.x"}`), "no", "", ""},
		{"itvx no old", judgeITVX(`Outside Of Allowed Geographic Region`), "no", "", ""},
		{"itvx ok", judgeITVX(`{"Playlist":{"Video":{}}}`), "yes", "GB", ""},
		{"itvx ?", judgeITVX(challenge), "failed", "", "unrecognized page"},
	} {
		if c.got.Status != c.status || c.got.Region != c.area || c.got.Detail != c.detail {
			t.Errorf("%s: %s %q %q, want %s %q %q", c.name, c.got.Status, c.got.Region, c.got.Detail, c.status, c.area, c.detail)
		}
	}
}

// The default checks are Claude's and the other services' it has always
// had; the ones of one region, or of a service only some countries have,
// run once turned on.
func TestUnlockDefaults(t *testing.T) {
	for _, id := range []string{"claude-api", "claude", "netflix"} {
		if !slices.ContainsFunc(UnlockList(nil, nil), func(u Unlock) bool { return u.ID == id }) {
			t.Errorf("%s is not a default check", id)
		}
	}
	for _, id := range []string{"peacock", "paramount", "tubi", "google-play", "youtube-cdn", "kktv", "hami", "mytvsuper", "viu", "tving", "coupang-play", "radiko", "dmm-tv", "channel4", "itvx"} {
		if slices.ContainsFunc(UnlockList(nil, nil), func(u Unlock) bool { return u.ID == id }) {
			t.Errorf("%s runs by default", id)
		}
		if !IsUnlockID(id) {
			t.Errorf("%s is no check", id)
		}
	}
}
