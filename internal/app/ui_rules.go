package app

import (
	"cmp"
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
)

// rulesPage is what the rules page keeps between frames.
type rulesPage struct {
	view    RulesView
	groups  map[string]bool
	has     bool
	loading bool
	loaded  homeKey

	tab      int // rules, providers
	search   string
	typ      string
	status   int // all, hit, unhit, off
	byHits   bool
	desc     bool
	busy     map[string]bool
	restore  bool
	list     ui.ListState
	flash    int
	flashAt  time.Time
	scrollTo int

	testing  bool
	target   string
	net      int
	testBusy bool
	testErr  string
	result   *coreapi.MatchResult

	sets    bool
	custom  bool
	setName string
	setURL  string
	setBeh  string
	setFmt  string
	setPol  string
}

type rulePreset struct{ id, nameKey, name, url, behavior, format, policy string }

const (
	rulesDat = "https://fastly.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@meta/geo/geosite/"
	loyal    = "https://fastly.jsdelivr.net/gh/Loyalsoldier/clash-rules@release/"
	bm7      = "https://fastly.jsdelivr.net/gh/blackmatrix7/ios_rule_script@master/rule/Clash/"
)

// ruleCatalog is the rule sets the maintainers vouched for, their URLs
// checked to answer, with the policy most people want from each.
var ruleCatalog = []struct {
	key   string
	items []rulePreset
}{
	{"rulesets.catAds", []rulePreset{
		{"ads", "rulesets.pAds", "", rulesDat + "category-ads-all.mrs", "domain", "mrs", "REJECT"},
		{"ads-loyal", "rulesets.pAdsL", "", loyal + "reject.txt", "domain", "yaml", "REJECT"},
		{"ads-bm7", "rulesets.pAdsB", "", bm7 + "Advertising/Advertising.yaml", "classical", "yaml", "REJECT"},
	}},
	{"rulesets.catAI", []rulePreset{
		{"openai", "rulesets.pOpenai", "", rulesDat + "openai.mrs", "domain", "mrs", "PROXY"},
		{"anthropic", "", "Anthropic · Claude", rulesDat + "anthropic.mrs", "domain", "mrs", "PROXY"},
	}},
	{"rulesets.catStream", []rulePreset{
		{"netflix", "", "Netflix", rulesDat + "netflix.mrs", "domain", "mrs", "PROXY"},
		{"disney", "", "Disney", rulesDat + "disney.mrs", "domain", "mrs", "PROXY"},
		{"youtube", "", "YouTube", rulesDat + "youtube.mrs", "domain", "mrs", "PROXY"},
		{"spotify", "", "Spotify", rulesDat + "spotify.mrs", "domain", "mrs", "PROXY"},
	}},
	{"rulesets.catCN", []rulePreset{
		{"cn", "rulesets.pCn", "", rulesDat + "cn.mrs", "domain", "mrs", "DIRECT"},
		{"outbound", "rulesets.pOutbound", "", rulesDat + "geolocation-!cn.mrs", "domain", "mrs", "PROXY"},
		{"proxy-loyal", "rulesets.pProxyList", "", loyal + "proxy.txt", "domain", "yaml", "PROXY"},
		{"gfw", "rulesets.pGfw", "", loyal + "gfw.txt", "domain", "yaml", "PROXY"},
		{"direct-loyal", "rulesets.pDirectL", "", loyal + "direct.txt", "domain", "yaml", "DIRECT"},
	}},
	{"rulesets.catPlatform", []rulePreset{
		{"telegram", "rulesets.pTelegram", "", rulesDat + "telegram.mrs", "domain", "mrs", "PROXY"},
		{"telegram-ip", "rulesets.pTelegramIP", "", loyal + "telegramcidr.txt", "ipcidr", "yaml", "PROXY"},
		{"twitter", "", "Twitter / X", rulesDat + "twitter.mrs", "domain", "mrs", "PROXY"},
		{"facebook", "", "Facebook / Meta", rulesDat + "facebook.mrs", "domain", "mrs", "PROXY"},
		{"github", "", "GitHub", rulesDat + "github.mrs", "domain", "mrs", "PROXY"},
		{"google", "", "Google", rulesDat + "google.mrs", "domain", "mrs", "PROXY"},
		{"microsoft", "rulesets.pMicrosoft", "", rulesDat + "microsoft.mrs", "domain", "mrs", "DIRECT"},
		{"apple", "rulesets.pApple", "", rulesDat + "apple.mrs", "domain", "mrs", "DIRECT"},
	}},
	{"rulesets.catMisc", []rulePreset{
		{"steam", "", "Steam", rulesDat + "steam.mrs", "domain", "mrs", "PROXY"},
		{"bilibili", "", "Bilibili", rulesDat + "bilibili.mrs", "domain", "mrs", "DIRECT"},
		{"apps", "rulesets.pApps", "", loyal + "applications.txt", "classical", "yaml", "DIRECT"},
	}},
}

func ruleLine(r *coreapi.Rule) string {
	t := ruleTypeName(r.Type, r.Payload)
	if t == "MATCH" {
		return t + "," + r.Proxy
	}
	return t + "," + r.Payload + "," + r.Proxy
}

var (
	ipv4Re = regexp.MustCompile(`^(\d{1,3}\.){3}\d{1,3}$`)
	ipv6Re = regexp.MustCompile(`(?i)^[0-9a-f]*:[0-9a-f:]+$`)
	urlRe  = regexp.MustCompile(`(?i)^[a-z]+://`)
)

// draftOf turns a tested target into a rule to start from: a host becomes
// a domain suffix, an address a CIDR of one.
func draftOf(target string) [2]string {
	v := strings.TrimSpace(target)
	v = urlRe.ReplaceAllString(v, "")
	if i := strings.IndexAny(v, "/?#"); i >= 0 {
		v = v[:i]
	}
	if ipv6Re.MatchString(v) {
		return [2]string{"IP-CIDR6", v + "/128"}
	}
	if i := strings.LastIndex(v, ":"); i >= 0 {
		v = v[:i]
	}
	if ipv4Re.MatchString(v) {
		return [2]string{"IP-CIDR", v + "/32"}
	}
	return [2]string{"DOMAIN-SUFFIX", cmp.Or(v, strings.TrimSpace(target))}
}

func (m *mainUI) rulesView(c *ui.Context) {
	s, p, a := &m.rules, m.pal, m.a
	if s.busy == nil {
		s.busy, s.flash, s.scrollTo = map[string]bool{}, -1, -1
	}
	running := m.state.Core.Status == coremgr.StatusRunning
	reload := func() { s.loaded = homeKey{} }
	if key := (homeKey{m.runtime + 1, 0, running, ""}); key != s.loaded && !s.loading && running {
		s.loaded, s.loading = key, true
		win := m.window()
		go func() {
			ctx, cancel := context.WithTimeout(a.ctx, 8*time.Second)
			defer cancel()
			v, err := (Rules{a}).List(ctx)
			pv, _ := (Proxies{a}).View(ctx)
			win.Update(func() {
				s.loading = false
				if err != nil {
					m.notices = append(m.notices, Notice{Level: "error", Message: locales.CommonFailed(), Detail: err.Error()})
					return
				}
				s.view, s.has, s.groups = v, true, map[string]bool{}
				for _, g := range pv.Groups {
					s.groups[g.Name] = true
				}
			})
		}()
	}
	profile, profileName := m.state.ProfileUID, m.state.ProfileName
	all := s.view.Rules
	m.header(c, locales.NavRules(), func() {
		if s.has {
			ui.Text(c, locales.RulesCount(fmt.Sprint(len(all)))).FontSize(12).TextColor(p.TextMuted)
		}
		if running && m.button(c, "test", locales.RulesTest(), icCrosshair, "ghost").Tooltip(locales.RulesTestTip()).Clicked() {
			s.testing = true
		}
		if running && m.button(c, "quick", locales.RulesQuickRules(), icZap, "ghost").Tooltip(locales.RulesQuickTip()).Clicked() {
			m.openExt("Rules", profile, locales.RulesQuickRules(), "rules", nil)
		}
		if m.button(c, "sets", locales.RulesetsTitle(), icLayers2, "ghost").Tooltip(locales.RulesetsHint()).Clicked() {
			s.sets, s.custom = true, false
		}
		if running && profile != "" && m.button(c, "edit", locales.RulesEdit(), icPencil, "ghost").Tooltip(locales.RulesEditTip()).Clicked() {
			m.openExt(profile, profile, locales.ProfilesExtRules()+" · "+profileName, "rules", nil)
		}
		if m.spinButton(c, "reload", icRefreshCw, locales.CommonRefresh(), s.loading).Clicked() {
			reload()
		}
	})
	ui.Row(c).Padding(0, 22, 8).Shrink(0).Children(func() {
		m.tabs(c, "tabs", &s.tab, locales.RulesRules(), fmt.Sprintf("%s (%d)", locales.RulesProviders(), len(s.view.Providers)))
	})
	// Loading crossfades to the rules, and a tab slides in from its side.
	state := "ready"
	switch {
	case !running:
		state = "down"
	case !s.has:
		state = "loading"
	}
	m.swapFill(c, "rules-state", state, func() {
		switch state {
		case "down":
			m.coreDown(c)
		case "loading":
			ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Spinner(c) })
		default:
			m.switchFill(c, "rules-tab", s.tab, func() {
				switch {
				case s.tab == 1:
					m.ruleProviders(c, reload)
				case len(all) == 0:
					m.empty(c, locales.RulesEmpty(), func() {
						ui.Text(c, locales.RulesEmptyHint()).FontSize(12.5).TextColor(p.TextMuted)
						if m.button(c, "go", locales.RulesGoProfiles(), nil, "").Clicked() {
							m.goTo("profiles", "")
						}
					})
				default:
					ui.Column(c.Key("rule-tile")).Grow(1).MinHeight(0).Margin(0, 22, 22).PaddingY(10).Background(p.Card).Radius(tileRadius).Clip().Children(func() {
						m.ruleList(c, profile, reload)
					})
				}
			})
		}
	})
	m.ruleTestDialog(c, profile)
	m.ruleSetsDialog(c)
}

func (m *mainUI) ruleList(c *ui.Context, profile string, reload func()) {
	s, p, a := &m.rules, m.pal, m.a
	all := s.view.Rules
	off := 0
	types := map[string]bool{}
	for i := range all {
		types[ruleTypeName(all[i].Type, all[i].Payload)] = true
		if all[i].Extra != nil && all[i].Extra.Disabled {
			off++
		}
	}
	typeList := []string{locales.RulesAllTypes()}
	for t := range types {
		typeList = append(typeList, t)
	}
	slices.Sort(typeList[1:])
	q := strings.ToLower(strings.TrimSpace(s.search))
	hits := func(r *coreapi.Rule) uint64 {
		if r.Extra == nil {
			return 0
		}
		return r.Extra.HitCount
	}
	var rules []*coreapi.Rule
	for i := range all {
		r := &all[i]
		syn := ruleTypeName(r.Type, r.Payload)
		if s.typ != "" && syn != s.typ {
			continue
		}
		switch s.status {
		case 1:
			if hits(r) == 0 {
				continue
			}
		case 2:
			if hits(r) > 0 {
				continue
			}
		case 3:
			if r.Extra == nil || !r.Extra.Disabled {
				continue
			}
		}
		if q != "" && !strings.Contains(strings.ToLower(r.Payload), q) && !strings.Contains(strings.ToLower(r.Proxy), q) && !strings.Contains(strings.ToLower(syn), q) {
			continue
		}
		rules = append(rules, r)
	}
	if s.byHits {
		slices.SortStableFunc(rules, func(x, y *coreapi.Rule) int { return cmp.Or(cmp.Compare(hits(x), hits(y)), y.Index-x.Index) })
	}
	if s.desc {
		slices.Reverse(rules)
	}
	ui.Row(c).Padding(0, 22, 8).Gap(6).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Icon(c, icInfo).FontSize(13).TextColor(p.TextMuted)
		ui.Text(c, locales.RulesIntro()).FontSize(12).TextColor(p.TextMuted).Wrap().Grow(1).Basis(0)
	})
	if off > 0 {
		ui.Box(c).Padding(0, 22, 8).Shrink(0).Children(func() {
			m.banner(c, "info", locales.RulesOffCount(fmt.Sprint(off)), func() {
				if s.restore {
					ui.Spinner(c)
				} else if m.button(c, "enable-all", locales.RulesEnableAll(), nil, "").Clicked() {
					s.restore = true
					win := m.window()
					go func() {
						n, err := (Rules{a}).EnableAll(context.Background())
						win.Update(func() {
							s.restore = false
							if err != nil {
								m.notices = append(m.notices, Notice{Level: "error", Message: locales.CommonFailed(), Detail: err.Error()})
							} else {
								m.notices = append(m.notices, Notice{Level: "success", Message: locales.RulesEnabledAll(fmt.Sprint(n))})
							}
							reload()
						})
					}()
				}
			})
		})
	}
	ui.Row(c).Padding(0, 22, 10).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.SearchField(c.Key("search"), &s.search).Placeholder(locales.RulesSearch()).Width(260)
		typ := cmp.Or(s.typ, typeList[0])
		if ui.Select(c.Key("type"), &typ, typeList).Width(170).Changed() {
			s.typ = typ
			if typ == typeList[0] {
				s.typ = ""
			}
		}
		statuses := []string{locales.RulesStatusAll(), locales.RulesStatusHit(), locales.RulesStatusUnhit(), locales.RulesStatusOff()}
		st := statuses[s.status]
		if ui.Select(c.Key("status"), &st, statuses).Width(130).Changed() {
			s.status = slices.Index(statuses, st)
		}
		ui.Text(c, locales.RulesShown(fmt.Sprint(len(rules)))).FontSize(12).TextColor(p.TextMuted)
	})
	if len(rules) == 0 {
		m.empty(c, locales.RulesNoMatch(), func() {
			if m.button(c, "clear", locales.RulesClearFilters(), nil, "").Clicked() {
				s.search, s.typ, s.status = "", "", 0
			}
		})
		return
	}
	// A rule of the test, brought into view once the list is rebuilt.
	if s.scrollTo >= 0 {
		for i, r := range rules {
			if r.Index == s.scrollTo {
				s.list.ScrollTo(i, ui.Center)
				s.flash, s.flashAt = r.Index, c.Now()
			}
		}
		s.scrollTo = -1
	}
	if s.flash >= 0 {
		if c.Now().Sub(s.flashAt) > 2600*time.Millisecond {
			s.flash = -1
		} else {
			c.After(300 * time.Millisecond)
		}
	}
	type col struct {
		title string
		width float32
		sort  int // 0 none, 1 index, 2 hits
	}
	cols := []col{{"#", 52, 1}, {locales.RulesType(), 150, 0}, {locales.RulesPayload(), 0, 0}, {locales.RulesTarget(), 160, 0}, {locales.RulesHits(), 80, 2}, {"", 52, 0}, {"", 32, 0}}
	cell := func(cl col, fn func()) {
		e := ui.Row(c).Gap(4).AlignItems(ui.Center).MinWidth(0)
		if cl.width > 0 {
			e.Width(cl.width).Shrink(0)
		} else {
			e.Grow(1).Basis(0)
		}
		e.Children(fn)
	}
	ui.Row(c).Padding(0, 22).Height(32).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Border).Shrink(0).Children(func() {
		for i, cl := range cols {
			cell(cl, func() {
				if cl.sort == 0 {
					ui.Text(c, cl.title).FontSize(11.5).FontWeight(600).TextColor(p.TextMuted).SingleLine()
					return
				}
				b := ui.ButtonBase(c.Key(i)).Row().Gap(3).AlignItems(ui.Center)
				pressable(b)
				active := s.byHits == (cl.sort == 2)
				b.Children(func() {
					ui.Text(c, cl.title).FontSize(11.5).FontWeight(600).TextColor(p.TextMuted).SingleLine()
					icon, col := icChevronsUpDown, p.Faint
					if active {
						icon, col = icArrowUp, p.Text2
						if s.desc {
							icon = icArrowDown
						}
					}
					ui.Icon(c, icon).FontSize(11).TextColor(col)
				})
				if b.Clicked() {
					if active {
						s.desc = !s.desc
					} else {
						s.byHits, s.desc = cl.sort == 2, cl.sort == 2
					}
				}
			})
		}
	})
	s.list.Key = func(i int) any { return rules[i].Index }
	ui.List(c.Key("rules"), &s.list, len(rules), func(i int) {
		r := rules[i]
		disabled := r.Extra != nil && r.Extra.Disabled
		group := s.groups[r.Proxy]
		syn := ruleTypeName(r.Type, r.Payload)
		row := ui.Row(c.Key(r.Index)).Padding(0, 22).Height(36).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Border.Alpha(0.5))
		if s.flash == r.Index {
			row.Background(p.AccentSoft)
		}
		if disabled {
			row.Opacity(0.5)
		}
		row.Children(func() {
			cell(cols[0], func() { ui.Text(c, strconv.Itoa(r.Index+1)).FontSize(12).TextColor(p.Faint).FontFeatures("tnum") })
			cell(cols[1], func() { m.badge(c, syn, "") })
			cell(cols[2], func() {
				spans := []ui.Span{{Text: cmp.Or(r.Payload, "—")}}
				if r.Size > 0 {
					spans = append(spans, ui.Span{Text: fmt.Sprintf(" (%d)", r.Size), Color: p.Faint})
				}
				ui.RichText(c, spans...).FontSize(12).Font("ui-monospace").SingleLine().Selectable().Tooltip(r.Payload)
			})
			cell(cols[3], func() {
				if group {
					b := ui.ButtonBase(c.Key("go")).Tooltip(locales.RulesGoGroup())
					pressable(b)
					b.Children(func() {
						t := ui.Text(c, r.Proxy).FontSize(12.5).FontWeight(560).TextColor(p.AccentFg).SingleLine()
						if b.Hovered() {
							t.Underline()
						}
					})
					if b.Clicked() {
						m.proxies.focus = r.Proxy
						m.goTo("proxies", "")
					}
				} else {
					ui.Text(c, r.Proxy).FontSize(12.5).FontWeight(560).SingleLine()
				}
			})
			cell(cols[4], func() {
				t := ui.Text(c, strconv.FormatUint(hits(r), 10)).FontSize(12).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll)
				if r.Extra != nil && !r.Extra.HitAt.IsZero() {
					t.Tooltip(locales.RulesLastHit(m.relTime(r.Extra.HitAt)))
				}
			})
			cell(cols[5], func() {
				if r.Extra == nil {
					return
				}
				on := !disabled
				if ui.Switch(c.Key("on"), &on).Label(locales.RulesEnabled()).Tooltip(locales.RulesOffTip()).Changed() {
					r.Extra.Disabled = !on // at once; the core confirms
					idx := r.Index
					m.run(locales.CommonFailed(), func() error { return Rules{a}.SetDisabled(context.Background(), idx, !on) }, nil)
				}
			})
			cell(cols[6], func() {
				line := ruleLine(r)
				draftable := slices.Contains(ruleTypes, syn) && syn != "MATCH" && profile != ""
				m.iconButton(c, "more", icMoreHorizontal, locales.RulesActions()).Size(24, 24).PopoverMenu(func(menu *ui.Menu) {
					if menu.Item(locales.RulesCopy()).Chosen() {
						mygo.Clipboard.WriteText(line)
						c.Toast(locales.CommonCopied())
					}
					if group && menu.Item(locales.RulesGoGroup()).Chosen() {
						m.proxies.focus = r.Proxy
						m.goTo("proxies", "")
					}
					if draftable && menu.Item(locales.RulesOverride()).Chosen() {
						d := [2]string{syn, r.Payload}
						m.openExt(profile, profile, locales.ProfilesExtRules()+" · "+m.state.ProfileName, "rules", &d)
					}
				})
			})
		})
	}).Grow(1)
}

func (m *mainUI) ruleProviders(c *ui.Context, reload func()) {
	s, p, a := &m.rules, m.pal, m.a
	providers := s.view.Providers
	if len(providers) == 0 {
		m.empty(c, locales.RulesNoProviders(), nil)
		return
	}
	update := func(name string) {
		s.busy[name] = true
		m.run(locales.RulesUpdateFailed(), func() error { return Rules{a}.UpdateProvider(context.Background(), name) }, func(error) {
			delete(s.busy, name)
			reload()
		})
	}
	ui.Scroll(c.Key("providers")).Grow(1).Padding(0, 22, 22).Gap(12).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icInfo).FontSize(13).TextColor(p.TextMuted)
			ui.Text(c, locales.RulesProvidersHint()).FontSize(12).TextColor(p.TextMuted).Wrap().Grow(1).Basis(0)
			if m.button(c, "update-all", locales.ProxiesUpdateAll(), icRefreshCw, "").Clicked() {
				for _, pr := range providers {
					update(pr.Name)
				}
			}
		})
		m.cardGrid(c, "grid", 280, len(providers), func(i int) {
			pr := providers[i]
			ui.Row(c).Padding(12, 14).Gap(8).Radius(tileRadius).Background(p.Card).AlignItems(ui.Center).Grow(1).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, pr.Name).FontWeight(620).SingleLine()
					ui.Text(c, pr.Behavior+" · "+cmp.Or(pr.Format, "yaml")+" · "+pr.VehicleType+" · "+locales.RulesRuleCount(fmt.Sprint(pr.RuleCount))).FontSize(12).TextColor(p.TextMuted).SingleLine()
					updated := "—"
					if pr.UpdatedAt != nil {
						updated = m.relTime(*pr.UpdatedAt)
					}
					ui.Text(c, locales.ProfilesUpdatedAgo(updated)).FontSize(11.5).TextColor(p.Faint)
				})
				if m.spinButton(c, "update", icRefreshCw, locales.ProxiesUpdateProvider(), s.busy[pr.Name]).Clicked() {
					update(pr.Name)
				}
			})
		})
	})
}

// ruleTestDialog answers which rule a connection to a domain, an address
// or a URL would hit, and the proxy it would go through.
func (m *mainUI) ruleTestDialog(c *ui.Context, profile string) {
	s, p, a := &m.rules, m.pal, m.a
	if !s.testing {
		return
	}
	run := func() {
		if strings.TrimSpace(s.target) == "" || s.testBusy {
			return
		}
		s.testBusy, s.testErr = true, ""
		req := coreapi.MatchRequest{Target: s.target, Network: []string{"tcp", "udp"}[s.net]}
		win := m.window()
		go func() {
			res, err := (Rules{a}).Test(context.Background(), req)
			win.Update(func() {
				s.testBusy = false
				if err != nil {
					s.result, s.testErr = nil, err.Error()
					return
				}
				s.result = &res
			})
		}()
	}
	m.modal(c, "rule-test", &s.testing, func() {
		ui.Column(c).Width(620).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icCrosshair).FontSize(16).TextColor(p.TextMuted)
				ui.Text(c, locales.RulesTest()).FontSize(15).FontWeight(650)
			})
			ui.Text(c, locales.RulesTestIntro()).FontSize(12).TextColor(p.TextMuted).Wrap()
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if ui.TextInput(c.Key("target"), &s.target).Placeholder(locales.RulesTestPlaceholder()).AutoFocus().Grow(1).Submitted() {
					run()
				}
				if i := m.segmented(c, "net", s.net, "TCP", "UDP"); i >= 0 {
					s.net = i
				}
				if s.testBusy {
					ui.Spinner(c)
				} else if m.button(c, "run", locales.RulesTestRun(), nil, "primary").Disabled(strings.TrimSpace(s.target) == "").Clicked() {
					run()
				}
			})
			if s.testErr != "" {
				m.errorLine(c, s.testErr)
			}
			r := s.result
			if r == nil {
				return
			}
			if r.Source == "mode" {
				key := "rules.test.modeDirect"
				if r.Mode == "global" {
					key = "rules.test.modeGlobal"
				}
				m.banner(c, "info", m.T(key), nil)
			}
			m.kv(c, func() {
				m.kvRow(c, locales.RulesTestRule(), func() {
					switch r.Source {
					case "rule":
						m.badge(c, ruleTypeName(r.RuleType, r.Payload), "")
						ui.Text(c, cmp.Or(r.Payload, "—")).FontSize(12.5).Font("ui-monospace").SingleLine().Selectable().Shrink(1)
						ui.Text(c, "#"+strconv.Itoa(r.Index+1)).FontSize(12).TextColor(p.Faint)
						if m.button(c, "locate", locales.RulesTestLocate(), nil, "ghost").Height(24).Clicked() {
							s.search, s.typ, s.status, s.byHits, s.desc, s.tab = "", "", 0, false, false, 0
							s.testing, s.scrollTo = false, r.Index
						}
					case "none":
						ui.Text(c, locales.RulesTestNone()).FontSize(12.5).TextColor(p.TextMuted)
					default:
						ui.Text(c, "—").TextColor(p.TextMuted)
					}
				})
				m.kvRow(c, locales.RulesTestPolicy(), func() { ui.Text(c, r.Policy).FontSize(12.5).FontWeight(600) })
				m.kvRow(c, locales.RulesTestChain(), func() {
					ui.Row(c).Gap(4).AlignItems(ui.Center).Wrap().Justify(ui.End).Shrink(1).Children(func() {
						for i, h := range r.Chain {
							if i > 0 {
								ui.Icon(c, icChevronRight).FontSize(13).TextColor(p.Faint)
							}
							last := i == len(r.Chain)-1
							ui.Row(c.Key(i)).Gap(4).Padding(2, 7).Radius(6).Background(map[bool]ui.Color{true: p.AccentSoft, false: p.Surface2}[last]).Tooltip(h.Type).Children(func() {
								ui.Text(c, h.Name).FontSize(12)
								ui.Text(c, h.Type).FontSize(11).TextColor(p.Faint)
							})
						}
					})
				})
				m.kvRow(c, locales.RulesTestIps(), func() {
					if len(r.IPs) == 0 {
						ui.Text(c, locales.RulesTestNoIPs()).FontSize(12.5).TextColor(p.Faint)
					} else {
						ui.Text(c, strings.Join(r.IPs, "  ")).FontSize(12.5).Font("ui-monospace").Selectable().Wrap()
					}
				})
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				foot := fmt.Sprintf("%s:%d · %s", r.Host, r.Port, strings.ToUpper(r.Network))
				if r.Disabled > 0 {
					foot += " · " + locales.RulesTestSkipped(fmt.Sprint(r.Disabled))
				}
				ui.Text(c, foot).FontSize(11.5).TextColor(p.Faint).Grow(1)
				if strings.TrimSpace(s.target) != "" && m.button(c, "add-rule", locales.RulesAddAsRule(), icSquarePlus, "ghost").Tooltip(locales.RulesQuickTip()).Clicked() {
					d := draftOf(s.target)
					s.testing = false
					m.openExt("Rules", profile, locales.RulesQuickRules(), "rules", &d)
				}
			})
		})
	})
}

// ruleSetsDialog manages the rule sets the user added: the catalog's
// presets by category, a set of their own, and the ones in place.
func (m *mainUI) ruleSetsDialog(c *ui.Context) {
	s, p := &m.rules, m.pal
	if !s.sets {
		return
	}
	sets := m.st.RuleSets
	save := func(next []config.RuleSet) {
		m.st.RuleSets = next // at once; the settings confirm
		m.patch(map[string]any{"ruleSets": next})
	}
	newID := func() string { return strconv.FormatInt(time.Now().UnixNano(), 36) }
	policies := []string{"PROXY", "DIRECT", "REJECT"}
	policyLabels := []string{locales.RulesetsPolicyProxy(), "DIRECT", "REJECT"}
	presetName := func(pr rulePreset) string {
		if pr.nameKey != "" {
			return m.T(pr.nameKey)
		}
		return cmp.Or(pr.name, pr.id)
	}
	host := func(u string) string {
		if v, err := url.Parse(u); err == nil && v.Host != "" {
			return v.Host
		}
		return u
	}
	m.modal(c, "rulesets", &s.sets, func() {
		ui.Column(c).Width(640).MaxHeight(640).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icLayers2).FontSize(16).TextColor(p.TextMuted)
				ui.Text(c, locales.RulesetsTitle()).FontSize(15).FontWeight(650)
			})
			ui.Text(c, locales.RulesetsHint()).FontSize(12).TextColor(p.TextMuted).Wrap()
			for i := range sets {
				x := sets[i]
				ui.Row(c.Key(x.ID)).Padding(8, 10).Gap(10).Radius(12).Background(p.Surface2).AlignItems(ui.Center).Children(func() {
					on := x.Enabled
					if ui.Switch(c, &on).Changed() {
						next := slices.Clone(sets)
						next[i].Enabled = on
						save(next)
					}
					ui.Row(c).Gap(7).AlignItems(ui.Center).Grow(1).Basis(0).MinWidth(0).Children(func() {
						ui.Text(c, x.Name).FontWeight(600).SingleLine().Shrink(1)
						m.badge(c, x.Behavior, "")
						ui.Text(c, host(x.URL)).FontSize(12).TextColor(p.Faint).SingleLine().Shrink(1)
					})
					pol := policyLabels[max(0, slices.Index(policies, x.Policy))]
					if ui.Select(c.Key("pol"), &pol, policyLabels).Width(150).Changed() {
						next := slices.Clone(sets)
						next[i].Policy = policies[slices.Index(policyLabels, pol)]
						save(next)
					}
					if m.iconButton(c, "del", icTrash2, locales.CommonDelete()).Clicked() {
						save(slices.Delete(slices.Clone(sets), i, i+1))
					}
				})
			}
			ui.Scroll(c.Key("catalog")).Grow(1).Basis(0).MinHeight(160).Gap(12).Children(func() {
				for _, g := range ruleCatalog {
					ui.Column(c.Key(g.key)).Gap(2).Children(func() {
						ui.Text(c, m.T(g.key)).FontSize(12).FontWeight(600).TextColor(p.TextMuted).PaddingY(4)
						for _, pr := range g.items {
							inPlace := slices.ContainsFunc(sets, func(x config.RuleSet) bool { return x.URL == pr.url })
							ui.Row(c.Key(pr.id)).Padding(5, 8).Gap(8).AlignItems(ui.Center).Children(func() {
								ui.Text(c, presetName(pr)).SingleLine().Grow(1).Basis(0)
								ui.Text(c, policyLabels[slices.Index(policies, pr.policy)]).FontSize(11).TextColor(p.Faint)
								if inPlace {
									m.badge(c, locales.RulesetsInPlace(), "")
								} else if m.button(c, "add", locales.CommonAdd(), icPlus, "ghost").Height(26).Clicked() {
									save(append(slices.Clone(sets), config.RuleSet{ID: newID(), Name: presetName(pr), URL: pr.url, Behavior: pr.behavior, Format: pr.format, Policy: pr.policy, Enabled: true}))
								}
							})
						}
					})
				}
			})
			if !s.custom {
				ui.Row(c).Children(func() {
					if m.button(c, "custom", locales.RulesetsCustom(), icPlus, "ghost").Clicked() {
						s.custom, s.setName, s.setURL, s.setBeh, s.setFmt, s.setPol = true, "", "", "domain", "mrs", "PROXY"
					}
				})
				return
			}
			valid := strings.HasPrefix(strings.TrimSpace(s.setURL), "http://") || strings.HasPrefix(strings.TrimSpace(s.setURL), "https://")
			ui.Row(c).Gap(8).Children(func() {
				ui.TextInput(c.Key("name"), &s.setName).Placeholder(locales.RulesetsName()).Width(150)
				ui.TextInput(c.Key("url"), &s.setURL).Placeholder(locales.RulesetsUrl()).Grow(1)
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Select(c.Key("beh"), &s.setBeh, []string{"domain", "ipcidr", "classical"}).Width(130)
				ui.Select(c.Key("fmt"), &s.setFmt, []string{"mrs", "yaml", "text"}).Width(110)
				pol := policyLabels[max(0, slices.Index(policies, s.setPol))]
				if ui.Select(c.Key("pol"), &pol, policyLabels).Width(180).Changed() {
					s.setPol = policies[slices.Index(policyLabels, pol)]
				}
				if m.button(c, "add", locales.CommonAdd(), icPlus, "primary").Disabled(!valid).Clicked() {
					u := strings.TrimSpace(s.setURL)
					name := strings.TrimSpace(s.setName)
					if name == "" {
						name = u[strings.LastIndex(u, "/")+1:]
					}
					save(append(slices.Clone(sets), config.RuleSet{ID: newID(), Name: cmp.Or(name, u), URL: u, Behavior: s.setBeh, Format: s.setFmt, Policy: s.setPol, Enabled: true}))
					s.custom = false
				}
				if m.button(c, "cancel", locales.CommonCancel(), nil, "ghost").Clicked() {
					s.custom = false
				}
			})
		})
	})
}
