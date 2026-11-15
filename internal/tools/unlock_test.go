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
