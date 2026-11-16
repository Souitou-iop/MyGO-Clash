package tools

import (
	"encoding/json"
	"regexp"
	"strings"
)

// The verdicts of the checks that read one service's own answer. Like the
// others they are functions of the answer alone, and a page that is none of
// the service's answers is "failed".

var (
	rePlayCountry   = regexp.MustCompile(`"zQmIje":"([A-Z]{2})"`)
	rePlayName      = regexp.MustCompile(`<div class="yVZQTb">([^<(]+)`)
	reYouTubeCDN    = regexp.MustCompile(`^\S+ => ([a-z0-9]+) \(`)
	reViuArea       = regexp.MustCompile(`/ott/([a-z]{2})(?:/|$)`)
	reRadikoArea    = regexp.MustCompile(`class="(JP\d+)"`)
	reParamountArea = regexp.MustCompile(`^/([a-z]{2})(?:/|$)`)
)

// netflixSpeedtest is the speed test Netflix's fast.com asks of its
// servers: it names the country Netflix takes the visitor for. The token is
// the placeholder fast.com's own examples carry.
const netflixSpeedtest = "https://api.fast.com/netflix/speedtest/v2?https=true&token=YXNkZmFzZGxmbnNkYWZoYXNkZmhrYWxm&urlCount=1"

// judgeNetflixRegion reads the country of the visitor from the speed test's
// answer, "" when it names none.
func judgeNetflixRegion(body string) string {
	var m struct {
		Client struct {
			Location struct {
				Country string `json:"country"`
			} `json:"location"`
		} `json:"client"`
	}
	if json.Unmarshal([]byte(body), &m) != nil {
		return ""
	}
	if c := strings.ToUpper(m.Client.Location.Country); reTwoLetter.MatchString(c) {
		return c
	}
	return ""
}

// judgeNetflix: a 403 on either title is Netflix's refusal; a Netflix
// original alone is the catalog of a region without the licensed titles.
// The region is what the speed test or the redirect named, never a guess.
func judgeNetflix(original, licensed int, region string) Unlock {
	switch {
	case original == 403 || licensed == 403:
		return Unlock{Status: "no"}
	case original < 400 && licensed < 400:
		return Unlock{Status: "yes", Region: region}
	case original < 400:
		return Unlock{Status: "partial", Region: region, Detail: "originals only"}
	}
	return Unlock{Status: "no"}
}

// judgeDisney: Disney+ sends the visitors it turns away to a path of
// "unavailable". Its edge also answers a 403 to the addresses it distrusts,
// which is a challenge, not an answer about the region.
func judgeDisney(status int, path string) Unlock {
	switch {
	case strings.Contains(path, "unavailable"):
		return Unlock{Status: "no"}
	case status == 403:
		return failed("challenge")
	case status < 400:
		return Unlock{Status: "yes", Region: regionFromPath(path)}
	}
	return unrecognized()
}

// judgeClaude: claude.ai sends the visitors it turns away to "unavailable";
// a 403 is the challenge page of its edge, which says nothing either way.
func judgeClaude(status int, path string) Unlock {
	switch {
	case strings.Contains(path, "unavailable"):
		return Unlock{Status: "no"}
	case status == 403:
		return failed("challenge")
	case status < 400:
		return Unlock{Status: "yes"}
	}
	return unrecognized()
}

// judgeClaudeAPI reads the API's answer to a made-up key: it is refused as
// a key (401) wherever the API takes the visitor, and "Request not allowed"
// (403) where it does not, or where it distrusts the address.
func judgeClaudeAPI(status int, body string) Unlock {
	switch {
	case status == 401 && strings.Contains(body, `"authentication_error"`):
		return Unlock{Status: "yes"}
	case status == 403 && strings.Contains(body, `"forbidden"`):
		return Unlock{Status: "no", Detail: "blocked"}
	}
	return unrecognized()
}

// judgePeacock: Peacock sends the visitors outside the US to a path of
// "unavailable".
func judgePeacock(status int, path string) Unlock {
	switch {
	case strings.Contains(path, "/unavailable"):
		return Unlock{Status: "no"}
	case status < 400:
		return Unlock{Status: "yes", Region: "US"}
	}
	return unrecognized()
}

// judgeParamount: outside its countries Paramount+ sends visitors to its
// international page under /intl/; elsewhere its page is under the country.
func judgeParamount(status int, path string) Unlock {
	switch {
	case strings.HasPrefix(path, "/intl"):
		return Unlock{Status: "no"}
	case status == 200:
		region := "US"
		if m := reParamountArea.FindStringSubmatch(path); m != nil {
			region = strings.ToUpper(m[1])
		}
		return Unlock{Status: "yes", Region: region}
	}
	return unrecognized()
}

// judgeTubi: Tubi sends the visitors it may not serve (the EU's) to a page
// of its own host, and says so in the page; its player's manifest is in the
// page it serves elsewhere.
func judgeTubi(host, body string) Unlock {
	switch {
	case host == "gdpr.tubi.tv" || strings.Contains(body, "not currently available in your area"):
		return Unlock{Status: "no"}
	case strings.Contains(body, "manifest"):
		return Unlock{Status: "yes"}
	}
	return unrecognized()
}

// judgeGooglePlay reads the country of the store the page is for: Google's
// own key for it, else the name of the country in the page's footer.
func judgeGooglePlay(status int, body string) Unlock {
	if status >= 400 {
		return unrecognized()
	}
	if m := rePlayCountry.FindStringSubmatch(body); m != nil {
		return Unlock{Status: "yes", Region: m[1]}
	}
	if m := rePlayName.FindStringSubmatch(body); m != nil {
		if name := strings.TrimSpace(m[1]); name != "" {
			return Unlock{Status: "yes", Detail: name}
		}
	}
	return unrecognized()
}

// judgeYouTubeCDN reads which of YouTube's servers takes the visitor: the
// first line of its mapping names the server, whose first three letters are
// the airport the server is by.
func judgeYouTubeCDN(body string) Unlock {
	line, _, _ := strings.Cut(body, "\n")
	if m := reYouTubeCDN.FindStringSubmatch(strings.TrimSpace(line)); m != nil && len(m[1]) >= 3 {
		return Unlock{Status: "yes", Region: strings.ToUpper(m[1][:3])}
	}
	return unrecognized()
}

// judgeKKTV reads KKTV's own check of the address.
func judgeKKTV(body string) Unlock {
	var m struct {
		Data struct {
			Allowed *bool  `json:"is_allowed"`
			Country string `json:"country"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(body), &m) != nil || m.Data.Allowed == nil {
		return unrecognized()
	}
	if *m.Data.Allowed {
		return Unlock{Status: "yes", Region: m.Data.Country}
	}
	return Unlock{Status: "no", Region: m.Data.Country}
}

// judgeHami reads the answer of Hami Video's player for a title: 06001-106
// is the refusal for the visitor's region, 06001-107 the title itself.
func judgeHami(body string) Unlock {
	var m struct {
		Code string `json:"code"`
	}
	if json.Unmarshal([]byte(body), &m) != nil {
		return unrecognized()
	}
	switch m.Code {
	case "06001-106":
		return Unlock{Status: "no"}
	case "06001-107":
		return Unlock{Status: "yes", Region: "TW"}
	}
	return unrecognized()
}

// judgeMyTVSuper reads myTV SUPER's session: whether it serves the country
// it takes the visitor for. A code that is no country ("ZP", which it gives
// the addresses it cannot place) is left out.
func judgeMyTVSuper(body string) Unlock {
	var m struct {
		Supported *bool  `json:"supported_country"`
		Country   string `json:"country_code"`
	}
	if json.Unmarshal([]byte(body), &m) != nil || m.Supported == nil {
		return unrecognized()
	}
	region := strings.ToUpper(m.Country)
	if region == "ZP" || !reTwoLetter.MatchString(region) {
		region = ""
	}
	if *m.Supported {
		return Unlock{Status: "yes", Region: region}
	}
	return Unlock{Status: "no", Region: region}
}

// judgeViu reads where Viu's home page sends the visitor: to a page of its
// own for the countries it is not in, else to the service of the country.
func judgeViu(location string) Unlock {
	switch {
	case strings.Contains(location, "/ott/no-service"):
		return Unlock{Status: "no"}
	}
	if m := reViuArea.FindStringSubmatch(location); m != nil {
		return Unlock{Status: "yes", Region: strings.ToUpper(m[1])}
	}
	return unrecognized()
}

// judgeTving reads the answer of TVING's player for a title: code 001 is its
// refusal of the visitors from outside South Korea.
func judgeTving(body string) Unlock {
	var m struct {
		Header struct {
			Status int `json:"status"`
		} `json:"header"`
		Body struct {
			Result struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"result"`
		} `json:"body"`
	}
	if json.Unmarshal([]byte(body), &m) != nil || m.Header.Status != 200 || m.Body.Result.Code == "" {
		return unrecognized()
	}
	if m.Body.Result.Code == "001" {
		if strings.Contains(m.Body.Result.Message, "only available in South Korea") {
			return Unlock{Status: "no", Region: ""}
		}
		return unrecognized()
	}
	return Unlock{Status: "yes", Region: "KR"}
}

// judgeCoupangPlay: Coupang Play sends the visitors from outside South
// Korea to a "not-available" page; its home page answers 200 inside.
func judgeCoupangPlay(status int, location string) Unlock {
	switch {
	case strings.Contains(location, "/not-available"):
		return Unlock{Status: "no"}
	case status == 200:
		return Unlock{Status: "yes", Region: "KR"}
	}
	return unrecognized()
}

// judgeRadiko reads the area radiko's page names: JP and a number inside
// Japan, OUT outside it.
func judgeRadiko(body string) Unlock {
	switch {
	case reRadikoArea.MatchString(body):
		return Unlock{Status: "yes", Region: "JP"}
	case strings.Contains(body, `class="OUT"`):
		return Unlock{Status: "no"}
	}
	return unrecognized()
}

// judgeDMMTV reads the answer of DMM TV's player to a request without a
// login: FOREIGN is its refusal of an address outside Japan (a data
// center's in Japan included); asking for the login is the answer inside.
func judgeDMMTV(body string) Unlock {
	var m struct {
		Blocked *bool  `json:"is_blocked"`
		Status  string `json:"block_status"`
	}
	if json.Unmarshal([]byte(body), &m) == nil && (m.Status == "FOREIGN" || m.Blocked != nil && *m.Blocked) {
		return Unlock{Status: "no"}
	}
	if strings.Contains(body, "UNAUTHORIZED") {
		return Unlock{Status: "yes", Region: "JP"}
	}
	return unrecognized()
}

// judgeChannel4: Channel 4's live stream answers 403 outside the UK.
func judgeChannel4(status int) Unlock {
	switch status {
	case 403:
		return Unlock{Status: "no"}
	case 200:
		return Unlock{Status: "yes", Region: "GB"}
	}
	return unrecognized()
}

// judgeITVX reads the answer of ITVX's player for a title: "geoblock" is
// its refusal outside the UK.
func judgeITVX(body string) Unlock {
	switch {
	case strings.Contains(body, `"geoblock"`) || strings.Contains(body, "Outside Of Allowed Geographic"):
		return Unlock{Status: "no"}
	case strings.Contains(body, "Playlist"):
		return Unlock{Status: "yes", Region: "GB"}
	}
	return unrecognized()
}
