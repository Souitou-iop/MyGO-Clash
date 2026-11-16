package app

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/usage"
)

const statsOther = "\x00other"

var statsRanges = []string{"today", "7d", "30d", "all", "custom"}

// statsPage is what the statistics page keeps between frames.
type statsPage struct {
	rng      int
	from, to time.Time
	picked   []string // devices; none is this device alone
	devices  []StatsDevice
	filter   string
	applied  string
	where    map[string]string
	byConns  bool
	expanded map[string]bool
	report   *usage.StatsReport
	key      string // what the report was asked for
	loadAt   time.Time
	loading  bool
	started  bool

	settings bool
	exclude  string
}

// countryName names a country in the interface's language.
func (m *mainUI) countryName(cc string) string {
	switch cc {
	case "LAN":
		return locales.StatsLan()
	case "—", "":
		return locales.StatsUnknownCountry()
	}
	r, err := language.ParseRegion(cc)
	if err != nil {
		return cc
	}
	if name := display.Regions(language.Make(i18n.Locale())).Name(r); name != "" {
		return name
	}
	return cc
}

var ccRe = regexp.MustCompile(`^[A-Z]{2}$`)

func (m *mainUI) statsView(c *ui.Context) {
	s, p, a := &m.stats, m.pal, m.a
	if !s.started {
		s.started, s.where, s.expanded = true, map[string]string{}, map[string]bool{}
		s.to = time.Now()
		s.from = s.to.AddDate(0, 0, -6)
		win := m.window()
		go func() {
			d, _ := (Stats{a}).Devices(context.Background())
			win.Update(func() { s.devices = d })
		}()
	}
	if s.filter != s.applied && time.Since(s.loadAt) > 250*time.Millisecond {
		s.applied = strings.TrimSpace(s.filter)
	}
	top := 10
	if len(s.expanded) > 0 {
		top = 100
	}
	q := usage.StatsQuery{Range: statsRanges[s.rng], Top: top, Filter: s.applied, Where: s.where, SortBy: map[bool]string{true: "conns", false: "traffic"}[s.byConns], Devices: s.picked}
	if s.rng == 4 {
		q.From, q.To = s.from.Format("2006-01-02"), s.to.Format("2006-01-02")
	}
	key := fmt.Sprint(q)
	if (key != s.key || time.Since(s.loadAt) > 10*time.Second) && !s.loading {
		s.key, s.loadAt, s.loading = key, time.Now(), true
		win := m.window()
		go func() {
			r, err := (Stats{a}).Query(context.Background(), q)
			win.Update(func() {
				s.loading = false
				if err == nil {
					s.report = &r
				}
			})
		}()
	}
	c.After(10 * time.Second)
	me := ""
	for _, d := range s.devices {
		if d.This {
			me = d.ID
		}
	}
	onlyHere := len(s.picked) == 0 || (len(s.picked) == 1 && s.picked[0] == me)

	m.header(c, locales.NavStats(), func() {
		if len(s.devices) > 1 {
			m.devicePicker(c, me)
		}
		labels := []string{locales.StatsToday(), locales.StatsDays7(), locales.StatsDays30(), locales.StatsAll(), locales.StatsCustom()}
		if i := m.segmented(c, "range", s.rng, labels...); i >= 0 {
			if i == 4 && s.report != nil {
				if f, err := time.ParseInLocation("2006-01-02", s.report.From, time.Local); err == nil {
					s.from = f
				}
				if t, err := time.ParseInLocation("2006-01-02", s.report.To, time.Local); err == nil {
					s.to = t
				}
			}
			s.rng = i
		}
		m.iconButton(c, "more", icMoreHorizontal, locales.StatsMore()).PopoverMenuButton(func(menu *ui.Menu) {
			exportAs := func(format string) {
				from, to := "", ""
				if s.report != nil {
					from, to = s.report.From, s.report.To
				}
				picked := s.picked
				m.run(locales.CommonFailed(), func() error {
					path, err := (Stats{a}).Export(context.Background(), format, from, to, picked)
					if err == nil && path != "" {
						m.notice(Notice{Level: "success", Message: locales.StatsExported(), Detail: path})
					}
					return err
				}, nil)
			}
			if menu.Item(locales.StatsExportCsv()).Chosen() {
				exportAs("csv")
			}
			if menu.Item(locales.StatsExportJson()).Disabled(!onlyHere).Chosen() {
				exportAs("json")
			}
			if menu.Item(locales.StatsImport()).Chosen() {
				win := m.window()
				go func() {
					n, err := (Stats{a}).Import(context.Background())
					win.Update(func() {
						if err != nil {
							m.notices = append(m.notices, Notice{Level: "error", Message: locales.CommonFailed(), Detail: err.Error()})
						} else if n > 0 {
							m.notices = append(m.notices, Notice{Level: "success", Message: locales.StatsImported(fmt.Sprint(n))})
							s.key = ""
						}
					})
				}()
			}
			menu.Separator()
			if menu.Item(locales.StatsSettings()).Chosen() {
				s.settings, s.exclude = true, strings.Join(m.st.Stats.Exclude, "\n")
			}
			if menu.Item(locales.StatsClear()).Chosen() {
				m.confirm(locales.StatsClearTitle(), locales.StatsClearMsg(), locales.StatsClear(), func() {
					m.run(locales.CommonFailed(), func() error { return Stats{a}.Clear(context.Background()) }, func(error) { s.key = "" })
				})
			}
		}, menuOptions(c))
	})
	ui.Scroll(c.Key("stats")).TrackScroll(m.keepScroll("stats")).Grow(1).Padding(4, 22, 22).Gap(14).Children(func() {
		// The days to choose fold in as the range becomes custom.
		m.collapse(c, "stats-custom", s.rng == 4, func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
				ui.DateInput(c.Key("from"), &s.from).Width(150)
				ui.Text(c, "–").TextColor(p.Faint)
				ui.DateInput(c.Key("to"), &s.to).Width(150)
				for _, n := range []int{7, 14, 30, 90} {
					if m.chip(c, n, locales.StatsLastDays(fmt.Sprint(n)), false).Clicked() {
						s.to = time.Now()
						s.from = s.to.AddDate(0, 0, -(n - 1))
					}
				}
			})
		})
		r := s.report
		narrowed := len(s.where) > 0 || s.applied != ""
		empty := r == nil || (r.Up+r.Down == 0 && !narrowed)
		// Empty and the report crossfade.
		m.swapPane(c, "stats-full", map[bool]string{true: "empty", false: "full"}[empty], 14, func() {
			if empty {
				m.empty(c, locales.StatsEmpty(), func() {
					ui.Text(c, locales.StatsEmptyHint()).FontSize(12.5).TextColor(p.TextMuted)
				})
				return
			}
			ui.Row(c).Gap(12).Children(func() {
				tile := func(key, label string, dot ui.Color, value string, extra func()) {
					ui.Column(c.Key(key)).Grow(1).Basis(0).MinWidth(0).Padding(14).Gap(6).Radius(tileRadius).Background(p.Card).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							if dot != (ui.Color{}) {
								ui.Box(c).Size(8, 8).Radius(2).Background(dot)
							}
							ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
							if extra != nil {
								extra()
							}
						})
						ui.Text(c, value).FontSize(22).FontWeight(650).FontFeatures("tnum").ContentTransition(numRoll).SingleLine()
					})
				}
				tile("total", locales.StatsTotal(), ui.Color{}, fmtBytes(r.Up+r.Down), func() {
					// Against next to nothing, a percentage says nothing.
					if r.Prev == nil || r.Prev.Up+r.Prev.Down < 1<<20 {
						return
					}
					prev := r.Prev.Up + r.Prev.Down
					pct := int(math.Round(float64(r.Up+r.Down-prev) / float64(prev) * 100))
					if pct > 999 || pct < -999 {
						return
					}
					icon, col := icArrowUpRight, p.Warning
					if pct < 0 {
						icon, col, pct = icArrowDownRight, p.Success, -pct
					}
					ui.Row(c).Gap(2).AlignItems(ui.Center).Tooltip(locales.StatsPrevTip(fmtBytes(prev))).Children(func() {
						ui.Icon(c, icon).FontSize(12).TextColor(col)
						ui.Text(c, fmt.Sprintf("%d%%", pct)).FontSize(11.5).TextColor(col).FontFeatures("tnum").ContentTransition(numRoll)
					})
				})
				tile("down", locales.CommonDownload(), p.Accent, fmtBytes(r.Down), nil)
				tile("up", locales.CommonUpload(), p.Accent2, fmtBytes(r.Up), nil)
				tile("conns", locales.StatsConns(), p.TextMuted, fmt.Sprint(r.Conns), nil)
			})
			hourly := r.From == r.To
			title := locales.StatsByDay()
			points := r.Days
			if hourly {
				title, points = locales.StatsByHour(), r.Hours
			}
			m.card(c, "bars", title, nil, func() {
				if i := m.segmented(c, "by", map[bool]int{true: 1, false: 0}[s.byConns], locales.StatsByTraffic(), locales.StatsByConns()); i >= 0 {
					s.byConns = i == 1
				}
			}, func() {
				m.statsBars(c, points, hourly)
				if r.Partial {
					ui.Text(c, locales.StatsPartial()).FontSize(11.5).TextColor(p.Faint)
				}
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
				ui.SearchField(c.Key("filter"), &s.filter).Placeholder(locales.StatsFilter()).Width(260)
				for _, dim := range []string{"app", "site", "node", "policy", "rule", "country"} {
					name, ok := s.where[dim]
					if !ok {
						continue
					}
					chip := ui.ButtonBase(c.Key("w-"+dim)).Row().Height(26).Padding(0, 10).Gap(5).Radius(13).AlignItems(ui.Center).Background(p.AccentSoft)
					pressable(chip)
					chip.Children(func() {
						m.icon(c, icListFilter, 12, p.AccentFg, chip.Hovered())
						ui.Text(c, m.T("stats.dim."+dim)).FontSize(12).TextColor(p.TextMuted)
						ui.Text(c, m.statsLabel(dim, name)).FontSize(12).TextColor(p.AccentFg).SingleLine()
						m.icon(c, icX, 12, p.AccentFg, chip.Hovered())
					})
					if chip.Clicked() {
						delete(s.where, dim)
					}
				}
				if len(s.where) > 1 && m.chip(c, "clear", locales.StatsClearFilters(), false).Clicked() {
					clear(s.where)
				}
				if narrowed && onlyHere {
					ui.Spacer(c)
					if m.button(c, "to-conns", locales.StatsToConnections(), nil, "ghost").Clicked() {
						q := ""
						for _, k := range []string{"site", "app"} {
							if v := s.where[k]; v != "" && v != statsOther && q == "" {
								q = v
							}
						}
						m.connPage.search, m.connPage.view = cmpOr(q, s.applied), 1
						m.goTo("connections", "")
					}
				}
			})
			dims := []struct {
				dim, title string
				items      []usage.StatsItem
			}{{"app", locales.StatsApps(), r.Apps}, {"site", locales.StatsSites(), r.Sites}, {"node", locales.StatsNodes(), r.Nodes},
				{"policy", locales.StatsPolicies(), r.Policies}, {"rule", locales.StatsRules(), r.Rules}, {"country", locales.StatsCountries(), r.Countries}}
			m.cardGrid(c, "ranks", 300, len(dims), func(i int) {
				d := dims[i]
				m.ranking(c, d.dim, d.title, d.items, narrowed)
			})
		})
	})
	m.statsSettings(c)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// chip is a small rounded button, lit while on.
func (m *mainUI) chip(c *ui.Context, key any, label string, on bool) ui.Element {
	p := m.pal
	chip := ui.ButtonBase(c.Key(key)).Height(26).Padding(0, 10).Radius(13).AlignItems(ui.Center).Border(1, p.Border)
	pressable(chip)
	switch {
	case on:
		chip.Background(p.AccentSoft).Border(1, p.Accent.Alpha(0.5))
	case chip.Hovered():
		chip.Background(p.SurfaceHover)
	}
	chip.Children(func() { ui.Text(c, label).FontSize(12).SingleLine() })
	return chip
}

func (m *mainUI) statsLabel(dim, name string) string {
	switch {
	case name == statsOther:
		return locales.StatsOther()
	case dim == "country":
		return m.countryName(name)
	case dim == "rule":
		typ, rest, ok := strings.Cut(name, ",")
		if ok {
			return ruleTypeName(typ, rest) + " " + rest
		}
		return ruleTypeName(typ, "")
	case dim == "app" && name == "—":
		return locales.StatsUnknownApp()
	}
	return name
}

// statsBars is the traffic over time, stacked down and up, or the
// connections; each bar grows in as the range changes.
func (m *mainUI) statsBars(c *ui.Context, points []usage.StatsPoint, hourly bool) {
	s, p := &m.stats, m.pal
	value := func(pt usage.StatsPoint) float64 {
		if s.byConns {
			return float64(pt.Conns)
		}
		return float64(pt.Up + pt.Down)
	}
	peak := 1.0
	for _, pt := range points {
		peak = math.Max(peak, value(pt))
	}
	every := 1
	switch {
	case hourly:
		every = 3
	case len(points) > 62:
		every = 14
	case len(points) > 14:
		every = 5
	}
	ui.Row(c).Height(170).Gap(2).AlignItems(ui.Stretch).Children(func() {
		for i, pt := range points {
			label := pt.Label
			if hourly {
				label += ":00"
			} else if len(label) > 5 {
				label = label[5:]
			}
			tip := fmt.Sprintf("%s  ↓ %s  ↑ %s  %s", cmpOr(map[bool]string{true: label}[hourly], pt.Label), fmtBytes(pt.Down), fmtBytes(pt.Up), locales.StatsConnsN(fmt.Sprint(pt.Conns)))
			col := ui.Column(c.Key(pt.Label)).Grow(1).Basis(0).MinWidth(0).Gap(4).Tooltip(tip)
			share := float32(value(pt) / peak)
			grow := col.Animate("h", share, 450*time.Millisecond)
			hover := col.AnimateTo("hover", b2f(col.Hovered()), ui.Animation{Duration: 120 * time.Millisecond, NonMotion: true})
			col.Children(func() {
				ui.Box(c).Grow(1).Draw(func(pt2 *ui.Painter, r ui.Rect) {
					h := r.H * grow
					y := r.Y + r.H - h
					if hover > 0 {
						pt2.Fill(ui.Rect{X: r.X - 1, Y: r.Y, W: r.W + 2, H: r.H}, p.SurfaceHover.Alpha(0.6*hover), 3)
					}
					if s.byConns {
						pt2.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: h}, p.TextMuted.Alpha(0.7), 2)
						return
					}
					total := float32(pt.Up + pt.Down)
					if total == 0 {
						return
					}
					up := h * float32(pt.Up) / total
					pt2.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: h}, p.Accent, 2)
					pt2.Fill(ui.Rect{X: r.X, Y: y, W: r.W, H: up}, p.Accent2, 2)
				})
				t := ""
				if i%every == 0 {
					t = label
				}
				ui.Text(c, t).FontSize(10).TextColor(p.Faint).SingleLine().Height(12).NoWrap()
			})
		}
	})
}

// ranking lists what carried the traffic, each row filled to its share of
// the busiest; a row narrows the page to it.
func (m *mainUI) ranking(c *ui.Context, dim, title string, all []usage.StatsItem, narrowed bool) {
	s, p := &m.stats, m.pal
	open := s.expanded[dim]
	named := 0
	for _, it := range all {
		if !it.Other {
			named++
		}
	}
	items := all
	if !open && named > 10 { // the top ten, the rest folded into Other
		other := usage.StatsItem{Other: true}
		var kept []usage.StatsItem
		n := 0
		for _, it := range all {
			if !it.Other && n < 10 {
				kept = append(kept, it)
				n++
				continue
			}
			other.Up, other.Down, other.Conns = other.Up+it.Up, other.Down+it.Down, other.Conns+it.Conns
		}
		items = append(kept, other)
	}
	weight := func(it usage.StatsItem) float64 {
		if s.byConns {
			return float64(it.Conns)
		}
		return float64(it.Up + it.Down)
	}
	peak := 1.0
	for _, it := range items {
		peak = math.Max(peak, weight(it))
	}
	m.card(c, dim, title, nil, nil, func() {
		if len(items) == 0 {
			msg := locales.StatsNoData()
			if narrowed {
				msg = locales.StatsNoMatches()
			}
			ui.Text(c, msg).FontSize(12.5).TextColor(p.Faint).PaddingY(8)
		}
		ui.Column(c).Gap(2).Children(func() {
			for _, it := range items {
				key := it.Name
				if it.Other {
					key = statsOther
				}
				on := s.where[dim] == key
				row := ui.ButtonBase(c.Key(key)).Row().Height(30).Padding(0, 8).Gap(8).Radius(6).AlignItems(ui.Center).Disabled(it.Other && !on).
					Tooltip(fmt.Sprintf("↓ %s  ↑ %s  %s", fmtBytes(it.Down), fmtBytes(it.Up), locales.StatsConnsN(fmt.Sprint(it.Conns))))
				row.Transition(hoverFade)
				share := float32(weight(it) / peak)
				fill := row.Animate("fill", share, 450*time.Millisecond)
				// A sliver of a fill would read as an edge stripe.
				row.Draw(func(pt *ui.Painter, r ui.Rect) {
					if share >= 0.04 {
						pt.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W * fill, H: r.H}, p.AccentSofter, 6)
					}
				})
				if on {
					row.Background(p.AccentSoft)
				} else if row.Hovered() && !it.Other {
					row.Background(p.SurfaceHover.Alpha(0.5))
				}
				row.Children(func() {
					muted := it.Other || it.Name == "—"
					if dim == "country" && ccRe.MatchString(it.Name) {
						m.flagSmall(c, it.Name)
					}
					name := ui.Text(c, m.statsLabel(dim, key)).FontSize(12.5).SingleLine().Grow(1).Basis(0)
					if muted {
						name.TextColor(p.TextMuted)
					}
					v := fmtBytes(it.Up + it.Down)
					if s.byConns {
						v = fmt.Sprint(it.Conns)
					}
					ui.Text(c, v).FontSize(12).TextColor(p.Text2).FontFeatures("tnum").ContentTransition(numRoll)
					if on {
						m.icon(c, icX, 12, p.TextMuted, row.Hovered())
					}
				})
				if row.Clicked() {
					if on {
						delete(s.where, dim)
					} else {
						s.where[dim] = key
					}
				}
			}
		})
		if (open && named > 10) || (!open && named >= 10) {
			label := locales.StatsShowAll()
			if open {
				label = locales.StatsShowLess()
			}
			if m.button(c, "more", label, nil, "ghost").Clicked() {
				if open {
					delete(s.expanded, dim)
				} else {
					s.expanded[dim] = true
				}
			}
		}
		if dim == "app" && m.st.Clash.FindProcessMode != "always" && !slices.ContainsFunc(all, func(it usage.StatsItem) bool { return !it.Other && it.Name != "—" }) && len(all) > 0 {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(8).Radius(8).Background(p.Surface2).Children(func() {
				ui.Text(c, locales.StatsAppsHint()).FontSize(12).TextColor(p.TextMuted).Wrap().Grow(1).Basis(0)
				if m.button(c, "enable", locales.StatsAppsEnable(), nil, "").Clicked() {
					m.patch(map[string]any{"clash": map[string]any{"findProcessMode": "always"}})
				}
			})
		}
		ui.Spacer(c)
	}).Grow(1)
}

// flagSmall is a country's flag beside its name.
// flagMasked is flagSmall for a place that is private: its block stays as
// the eye shuts, a globe crossfading in over the flag.
func (m *mainUI) flagMasked(c *ui.Context, code string, masked bool) {
	box := ui.Box(c.Key("flag")).Size(18, 13.5).Radius(2).Clip().Shrink(0).Background(m.pal.Surface3).AlignItems(ui.Center).Justify(ui.Center)
	s := svgImage("assets/flags/" + strings.ToLower(code) + ".svg")
	box.Children(func() {
		if masked || s == nil {
			ui.Icon(c.Key("globe"), icGlobe).FontSize(9).TextColor(m.pal.Faint).Transition(flagFade)
			return
		}
		ui.Image(c.Key("flag-"+code), s).Fill().Transition(flagFade)
	})
}

func (m *mainUI) flagSmall(c *ui.Context, code string) {
	box := ui.Box(c.Key("flag-"+code)).Size(18, 13.5).Radius(2).Clip().Shrink(0).Background(m.pal.Surface3).Transition(unveil)
	if s := svgImage("assets/flags/" + strings.ToLower(code) + ".svg"); s != nil {
		box.Children(func() { ui.Image(c, s).Fill() })
	}
}

// devicePicker chooses whose statistics show: this device, or any of
// those that sync theirs.
func (m *mainUI) devicePicker(c *ui.Context, me string) {
	s := &m.stats
	ids := s.picked
	if len(ids) == 0 {
		ids = []string{me}
	}
	all := len(ids) == len(s.devices)
	label := locales.StatsNDevices(fmt.Sprint(len(ids)))
	switch {
	case all:
		label = locales.StatsAllDevices()
	case len(ids) == 1 && ids[0] == me:
		label = locales.StatsThisDevice()
	case len(ids) == 1:
		for _, d := range s.devices {
			if d.ID == ids[0] {
				label = d.Name
			}
		}
	}
	m.button(c, "devices", label, icMonitorSmartphone, "").PopoverMenuButton(func(menu *ui.Menu) {
		if menu.Item(locales.StatsAllDevices()).Checked(all).Chosen() {
			if all {
				s.picked = nil
			} else {
				s.picked = nil
				for _, d := range s.devices {
					s.picked = append(s.picked, d.ID)
				}
			}
		}
		menu.Separator()
		for _, d := range s.devices {
			on := slices.Contains(ids, d.ID)
			name := d.Name
			if d.This {
				name += " · " + locales.StatsThisDevice()
			}
			if menu.Item(name).Checked(on).Chosen() {
				next := slices.Clone(ids)
				if on {
					next = slices.DeleteFunc(next, func(x string) bool { return x == d.ID })
				} else {
					next = append(next, d.ID)
				}
				if len(next) == 0 { // one at least
					continue
				}
				if len(next) == 1 && next[0] == me {
					next = nil
				}
				s.picked = next
			}
		}
	}, menuOptions(c, ui.AnchorBottomStart, ui.AnchorTopStart))
}

func (m *mainUI) statsSettings(c *ui.Context) {
	s, p := &m.stats, m.pal
	m.modal(c, "stats-settings", &s.settings, func() {
		ui.Column(c).Width(420).Gap(14).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icSettings2).FontSize(16).TextColor(p.TextMuted)
				ui.Text(c, locales.StatsSettings()).FontSize(15).FontWeight(650)
			})
			m.field(c, locales.StatsKeep(), "", "", func() {
				days := []int{30, 90, 180, 365, 0}
				var labels []string
				for _, d := range days {
					if d == 0 {
						labels = append(labels, locales.StatsKeepForever())
					} else {
						labels = append(labels, locales.StatsKeepDays(fmt.Sprint(d)))
					}
				}
				cur := labels[max(0, slices.Index(days, m.st.Stats.KeepDays))]
				if dropdown(c.Key("keep"), &cur, labels).Width(160).Changed() {
					v := days[slices.Index(labels, cur)]
					m.st.Stats.KeepDays = v
					m.patch(map[string]any{"stats": map[string]any{"keepDays": v}})
				}
			})
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.StatsExcludeLocal()).FontWeight(550).Grow(1)
				on := m.st.Stats.ExcludeLocal
				if ui.Switch(c, &on).Changed() {
					m.st.Stats.ExcludeLocal = on
					m.patch(map[string]any{"stats": map[string]any{"excludeLocal": on}})
				}
			})
			m.field(c, locales.StatsExclude(), locales.StatsExcludeHint(), "", func() {
				ui.TextArea(c.Key("exclude"), &s.exclude).Placeholder("example.com\n10.0.0.0/8\nprocess:Steam").Font("ui-monospace").Height(110)
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				if m.button(c, "done", locales.CommonDone(), nil, "primary").Clicked() {
					var list []string
					for _, l := range strings.Split(s.exclude, "\n") {
						if l = strings.TrimSpace(l); l != "" {
							list = append(list, l)
						}
					}
					m.patch(map[string]any{"stats": map[string]any{"exclude": list}})
					s.settings = false
				}
			})
		})
	})
}
