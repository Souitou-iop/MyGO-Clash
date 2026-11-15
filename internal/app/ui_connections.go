package app

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/usage"
)

// connsPage is what the connections page keeps between frames.
type connsPage struct {
	started  bool
	snap     coreapi.Connections
	closed   []coreapi.Connection
	ends     map[string]time.Time
	total    int
	paused   bool
	search   string
	searched string // what the closed connections were loaded for
	net      int    // all, tcp, udp
	view     int    // active, closed
	sort     string
	desc     bool
	detail   string // the ID shown
	list     ui.ListState
	asking   bool
	ask      func()
	askText  [3]string
	loadAt   time.Time
	table    bool // the active connections as a table, not as a flow
}

// ruleSyntax spells the core's rule types as configurations write them.
var ruleSyntax = map[string]string{
	"Domain": "DOMAIN", "DomainSuffix": "DOMAIN-SUFFIX", "DomainKeyword": "DOMAIN-KEYWORD", "DomainRegex": "DOMAIN-REGEX",
	"DomainWildcard": "DOMAIN-WILDCARD", "GeoSite": "GEOSITE", "GeoIP": "GEOIP", "SrcGeoIP": "SRC-GEOIP", "IPASN": "IP-ASN",
	"SrcIPASN": "SRC-IP-ASN", "IPCIDR": "IP-CIDR", "SrcIPCIDR": "SRC-IP-CIDR", "IPSuffix": "IP-SUFFIX", "SrcIPSuffix": "SRC-IP-SUFFIX",
	"SrcPort": "SRC-PORT", "DstPort": "DST-PORT", "InPort": "IN-PORT", "InUser": "IN-USER", "InName": "IN-NAME", "InType": "IN-TYPE",
	"ProcessName": "PROCESS-NAME", "ProcessPath": "PROCESS-PATH", "ProcessNameRegex": "PROCESS-NAME-REGEX",
	"ProcessPathRegex": "PROCESS-PATH-REGEX", "ProcessNameWildcard": "PROCESS-NAME-WILDCARD",
	"ProcessPathWildcard": "PROCESS-PATH-WILDCARD", "Match": "MATCH", "RuleSet": "RULE-SET", "Network": "NETWORK",
	"DSCP": "DSCP", "Uid": "UID", "SubRules": "SUB-RULE",
}

func ruleTypeName(t, payload string) string {
	s, ok := ruleSyntax[t]
	if !ok {
		s = strings.ToUpper(t)
	}
	if s == "IP-CIDR" && strings.Contains(payload, ":") {
		return "IP-CIDR6"
	}
	return s
}

func connHost(c *coreapi.Connection) string {
	m := c.Metadata
	return cmp.Or(m.Host, m.SniffHost, m.DestinationIP) + ":" + m.DestinationPort
}

func connChain(c *coreapi.Connection) string {
	ch := slices.Clone(c.Chains)
	slices.Reverse(ch)
	return strings.Join(ch, " → ")
}

func fromClosed(c usage.Closed) coreapi.Connection {
	out := coreapi.Connection{ID: c.ID, Start: c.Start, Upload: c.Up, Download: c.Down, Chains: c.Chains, Rule: c.Rule, RulePayload: c.RulePayload}
	out.Metadata = coreapi.ConnectionMetadata{Network: c.Network, Type: c.Type, SourceIP: c.SourceIP, DestinationIP: c.DestinationIP,
		DestinationPort: c.DestinationPort, Host: c.Host, Process: c.Process, ProcessPath: c.ProcessPath}
	if c.Country != "" {
		out.Metadata.DestinationGeoIP = []string{c.Country}
	}
	return out
}

// confirm asks a question in an alert before doing something that can't
// be undone.
func (m *mainUI) confirm(title, message, action string, do func()) {
	s := &m.connPage
	s.asking, s.ask, s.askText = true, do, [3]string{title, message, action}
}

// confirmDialog shows the question confirm asked.
func (m *mainUI) confirmDialog(c *ui.Context) {
	s := &m.connPage
	if ui.AlertDialog(c.Key("confirm"), &s.asking, s.askText[0], s.askText[1], locales.CommonCancel(), s.askText[2]) == 1 && s.ask != nil {
		s.ask()
	}
}

func (m *mainUI) connectionsView(c *ui.Context) {
	s, p, a := &m.connPage, m.pal, m.a
	running := m.state.Core.Status == coremgr.StatusRunning
	if !s.started {
		s.started, s.sort, s.desc, s.ends = true, "start", true, map[string]time.Time{}
		win := m.window()
		go follow(a.ctx, a.conns, func(v coreapi.Connections) {
			win.Update(func() {
				if !s.paused {
					s.snap = v
				}
			})
		})
	}
	// The closed connections come from the history the statistics keep,
	// which goes on while the page is closed; the search goes to it too.
	every := 15 * time.Second
	if s.view == 1 {
		every = 3 * time.Second
	}
	if (s.search != s.searched && time.Since(s.loadAt) > 250*time.Millisecond) || (time.Since(s.loadAt) > every && !s.paused) {
		s.loadAt, s.searched = time.Now(), s.search
		q, win := strings.TrimSpace(s.search), m.window()
		go func() {
			page, err := (Stats{a}).Closed(context.Background(), usage.ClosedQuery{Filter: q, Limit: 500})
			if err != nil {
				return
			}
			win.Update(func() {
				s.closed = s.closed[:0]
				for _, it := range page.Items {
					s.closed = append(s.closed, fromClosed(it))
					s.ends[it.ID] = it.End
				}
				s.total = page.Total
			})
		}()
	}
	c.After(every)

	src := s.snap.Connections
	if s.view == 1 {
		src = s.closed
	}
	q := strings.ToLower(strings.TrimSpace(s.search))
	nets := []string{"", "tcp", "udp"}
	list := make([]*coreapi.Connection, 0, len(src))
	for i := range src {
		cn := &src[i]
		if s.net > 0 && cn.Metadata.Network != nets[s.net] {
			continue
		}
		if q != "" && !slices.ContainsFunc([]string{connHost(cn), cn.Metadata.Process, ruleTypeName(cn.Rule, cn.RulePayload), cn.RulePayload, connChain(cn), cn.Metadata.SourceIP},
			func(v string) bool { return strings.Contains(strings.ToLower(v), q) }) {
			continue
		}
		list = append(list, cn)
	}
	slices.SortStableFunc(list, func(x, y *coreapi.Connection) int {
		var r int
		switch s.sort {
		case "host":
			r = strings.Compare(connHost(x), connHost(y))
		case "down":
			r = cmp.Compare(x.DownloadSpeed, y.DownloadSpeed)
		case "up":
			r = cmp.Compare(x.UploadSpeed, y.UploadSpeed)
		case "downTotal":
			r = cmp.Compare(x.Download, y.Download)
		case "upTotal":
			r = cmp.Compare(x.Upload, y.Upload)
		default:
			r = x.Start.Compare(y.Start)
		}
		if s.desc {
			return -r
		}
		return r
	})

	m.header(c, locales.NavConnections(), func() {
		if s.snap.DownloadTotal > 0 {
			ui.Text(c, "↓ "+fmtBytes(s.snap.DownloadTotal)+" · ↑ "+fmtBytes(s.snap.UploadTotal)).FontSize(12).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll)
		}
		icon, txt := icPause, locales.CommonPause()
		if s.paused {
			icon, txt = icPlay, locales.CommonResume()
		}
		if m.button(c, "pause", txt, icon, "ghost").Clicked() {
			s.paused = !s.paused
		}
		if s.view == 1 {
			if m.button(c, "clear", locales.ConnClearClosed(), icTrash2, "danger").Disabled(s.total == 0).Clicked() {
				m.confirm(locales.ConnClearClosedTitle(), locales.ConnClearClosedMsg(), locales.CommonClear(), func() {
					m.run(locales.CommonFailed(), func() error { return Stats{a}.ClearClosed(context.Background()) }, func(error) { s.loadAt = time.Time{} })
				})
			}
		} else {
			filtered := s.search != "" || s.net > 0
			label := locales.ConnCloseAll()
			if filtered {
				label = locales.ConnCloseFiltered()
			}
			if m.button(c, "close", label, icXCircle, "danger").Disabled(len(list) == 0).Clicked() {
				msg := locales.ConnCloseAllMsg()
				ids := make([]string, len(list))
				for i, cn := range list {
					ids[i] = cn.ID
				}
				if filtered {
					msg = locales.ConnCloseSomeMsg(fmt.Sprint(len(list)))
				}
				m.confirm(locales.ConnCloseAllTitle(), msg, locales.ConnClose(), func() {
					m.run(locales.CommonFailed(), func() error {
						if !filtered {
							return Connections{a}.CloseAll(context.Background())
						}
						return Connections{a}.Close(context.Background(), ids)
					}, nil)
				})
			}
		}
	})
	ui.Row(c).Padding(0, 22, 10).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		if i := m.segmented(c, "view", s.view, fmt.Sprintf("%s %d", locales.ConnActive(), len(s.snap.Connections)), fmt.Sprintf("%s %d", locales.ConnClosed(), s.total)); i >= 0 {
			s.view = i
			s.loadAt = time.Time{}
		}
		if i := m.segmented(c, "net", s.net, locales.CommonAll(), "TCP", "UDP"); i >= 0 {
			s.net = i
		}
		ui.SearchField(c.Key("search"), &s.search).Placeholder(locales.ConnSearch()).Width(280)
		ui.Spacer(c)
		// The shapes are those of the active connections alone.
		m.collapseInline(c, "conn-shape", s.view == 0, func() {
			cur := 0
			if s.table {
				cur = 1
			}
			if i := m.segmentedIcons(c, "shape", cur, icWaypoints, icList); i >= 0 {
				s.table = i == 1
			}
		})
	})
	// The views slide the way the segments go (the flow, the table, the
	// closed), and a view crossfades between empty and full.
	view := 2
	if s.view == 0 {
		view = map[bool]int{false: 0, true: 1}[s.table]
	}
	m.swapFill(c, "conn-state", map[bool]string{true: "up", false: "down"}[running], func() {
		if !running {
			m.coreDown(c)
			return
		}
		m.switchFill(c, "conn-view", view, func() {
			m.swapFill(c, "conn-full", map[bool]string{true: "empty", false: "full"}[len(list) == 0], func() {
				switch {
				case len(list) == 0:
					m.empty(c, locales.ConnEmpty(), nil)
				case view == 0:
					ui.Column(c).Grow(1).MinHeight(0).Margin(0, 22, 22).Padding(6).Background(m.pal.Card).Radius(tileRadius).Clip().Children(func() { m.flowView(c, list) })
				default:
					ui.Column(c).Grow(1).MinHeight(0).Margin(0, 22, 22).Background(m.pal.Card).Radius(tileRadius).Clip().Children(func() { m.connTable(c, list) })
				}
			})
		})
	})
	m.connDetail(c)
}

// connRowWidth is the least width of the table's rows; narrower, it
// scrolls sideways.
const connRowWidth = 1100

// connTable lists connections in sortable columns.
func (m *mainUI) connTable(c *ui.Context, list []*coreapi.Connection) {
	s, p, a := &m.connPage, m.pal, m.a
	active := s.view == 0
	type col struct {
		title, sort string
		width       float32
		end         bool
	}
	cols := []col{{locales.ConnHost(), "host", 0, false}, {locales.ConnNetwork(), "", 66, false}, {locales.ConnProcess(), "", 120, false},
		{locales.ConnRule(), "", 150, false}, {locales.ConnChain(), "", 150, false}, {locales.ConnDlSpeed(), "down", 84, true},
		{locales.ConnUlSpeed(), "up", 84, true}, {locales.ConnDl(), "downTotal", 78, true}, {locales.ConnUl(), "upTotal", 78, true},
		{locales.ConnTime(), "start", 66, true}, {"", "", 30, false}}
	cell := func(cl col, fn func()) {
		e := ui.Row(c).Gap(4).AlignItems(ui.Center).MinWidth(0)
		if cl.width > 0 {
			e.Width(cl.width).Shrink(0)
		} else {
			e.Grow(1).Basis(0).MinWidth(180)
		}
		if cl.end {
			e.Justify(ui.End)
		}
		e.Children(fn)
	}
	head := ui.Row(c).Clip().Shrink(0).BorderWidth(0, 0, 1, 0).BorderColor(p.Border)
	head.Children(func() {
		ui.Row(c).Padding(0, 22).Height(32).Gap(10).AlignItems(ui.Center).MinWidth(connRowWidth).Grow(1).Children(func() {
			for i, cl := range cols {
				cell(cl, func() {
					if cl.sort == "" {
						ui.Text(c, cl.title).FontSize(11.5).FontWeight(600).TextColor(p.TextMuted).SingleLine()
						return
					}
					b := ui.ButtonBase(c.Key(i)).Row().Gap(3).AlignItems(ui.Center)
					pressable(b)
					b.Children(func() {
						ui.Text(c, cl.title).FontSize(11.5).FontWeight(600).TextColor(p.TextMuted).SingleLine()
						icon, col := icChevronsUpDown, p.Faint
						if s.sort == cl.sort {
							icon, col = icArrowUp, p.Text2
							if s.desc {
								icon = icArrowDown
							}
						}
						ui.Icon(c, icon).FontSize(11).TextColor(col)
					})
					if b.Clicked() {
						if s.sort == cl.sort {
							s.desc = !s.desc
						} else {
							s.sort, s.desc = cl.sort, true
						}
					}
				})
			}
		})
	})
	s.list.Key = func(i int) any { return list[i].ID }
	table := ui.List(c.Key("conns"), &s.list, len(list), func(i int) {
		cn := list[i]
		row := ui.ButtonBase(c.Key(cn.ID)).Row().Padding(0, 22).Height(36).Gap(10).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Border.Alpha(0.5))
		row.Transition(hoverFade)
		if row.Hovered() {
			row.Background(p.SurfaceHover.Alpha(0.6))
		}
		md := cn.Metadata
		text := func(s string, col ui.Color) ui.Element {
			return ui.Text(c, s).FontSize(12).TextColor(col).SingleLine().FontFeatures("tnum")
		}
		row.Children(func() {
			cell(cols[0], func() { text(connHost(cn), p.Text).Font("ui-monospace").Tooltip(connHost(cn)) })
			cell(cols[1], func() {
				tone := ""
				if md.Network == "udp" {
					tone = "warning"
				}
				m.badge(c, strings.ToUpper(md.Network), tone)
			})
			cell(cols[2], func() { text(cmp.Or(md.Process, "—"), p.TextMuted).Tooltip(md.ProcessPath) })
			cell(cols[3], func() {
				ui.RichText(c, ui.Span{Text: ruleTypeName(cn.Rule, cn.RulePayload)}, ui.Span{Text: " " + cn.RulePayload, Color: p.Faint}).FontSize(12).SingleLine().Tooltip(cn.RulePayload)
			})
			cell(cols[4], func() { text(connChain(cn), p.Text).Tooltip(connChain(cn)) })
			dash := func(v int64) string {
				if !active {
					return "—"
				}
				return fmtRate(v)
			}
			cell(cols[5], func() { text(dash(cn.DownloadSpeed), p.Text) })
			cell(cols[6], func() { text(dash(cn.UploadSpeed), p.Text) })
			cell(cols[7], func() { text(fmtBytes(cn.Download), p.TextMuted) })
			cell(cols[8], func() { text(fmtBytes(cn.Upload), p.TextMuted) })
			end := time.Now()
			if !active {
				end = s.ends[cn.ID]
			}
			cell(cols[9], func() { text(fmtDuration(end.Sub(cn.Start)), p.Faint) })
			cell(cols[10], func() {
				if active {
					id := cn.ID
					if m.iconButton(c, "x", icX, locales.ConnClose()).Size(24, 24).Clicked() {
						m.run(locales.CommonFailed(), func() error { return Connections{a}.Close(context.Background(), []string{id}) }, nil)
					}
				}
			})
		})
		if row.Clicked() {
			s.detail = cn.ID
		}
	}).Grow(1).RowWidth(connRowWidth)
	head.ScrollWith(table)
	if active {
		c.After(time.Second) // the durations
	}
}

// connDetail shows every field of the connection chosen, kept up to date
// while it lasts.
func (m *mainUI) connDetail(c *ui.Context) {
	s, p, a := &m.connPage, m.pal, m.a
	open := s.detail != ""
	if !open {
		return
	}
	var cn *coreapi.Connection
	live := false
	for i := range s.snap.Connections {
		if s.snap.Connections[i].ID == s.detail {
			cn, live = &s.snap.Connections[i], true
		}
	}
	for i := range s.closed {
		if cn == nil && s.closed[i].ID == s.detail {
			cn = &s.closed[i]
		}
	}
	if cn == nil {
		s.detail = ""
		return
	}
	md := cn.Metadata
	inbound := strings.Join(slices.DeleteFunc([]string{md.InboundName, func() string {
		if md.InboundIP == "" {
			return ""
		}
		return md.InboundIP + ":" + md.InboundPort
	}()}, func(s string) bool { return s == "" }), " · ")
	rule := ruleTypeName(cn.Rule, cn.RulePayload)
	if cn.RulePayload != "" {
		rule += " (" + cn.RulePayload + ")"
	}
	rows := [][2]string{
		{locales.ConnHost(), connHost(cn)}, {locales.ConnNetwork(), md.Network + " · " + md.Type},
		{locales.ConnSource(), md.SourceIP + ":" + md.SourcePort}, {locales.ConnDestination(), cmp.Or(md.DestinationIP, "—") + ":" + md.DestinationPort},
		{"SNI", cmp.Or(md.SniffHost, "—")}, {locales.ConnProcess(), cmp.Or(md.Process, "—")}, {locales.ConnProcessPath(), cmp.Or(md.ProcessPath, "—")},
		{locales.ConnRule(), rule}, {locales.ConnChain(), connChain(cn)}, {locales.ConnInbound(), cmp.Or(inbound, "—")},
		{locales.ConnDnsMode(), cmp.Or(md.DNSMode, "—")}, {locales.ConnRemote(), cmp.Or(md.RemoteDestination, "—")},
		{"GeoIP", cmp.Or(strings.Join(md.DestinationGeoIP, ", "), "—")}, {"ASN", cmp.Or(md.DestinationIPASN, "—")},
		{locales.ConnTraffic(), "↑ " + fmtBytes(cn.Upload) + "  ↓ " + fmtBytes(cn.Download)}, {locales.ConnStarted(), fmtDateTime(cn.Start)},
	}
	m.modal(c, "detail", &open, func() {
		ui.Column(c).Width(560).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.ConnDetails()).FontSize(15).FontWeight(650)
				if !live {
					m.badge(c, locales.ConnClosed(), "")
				}
			})
			ui.Column(c).Gap(7).Children(func() {
				for _, r := range rows {
					ui.Row(c.Key(r[0])).Gap(12).AlignItems(ui.Start).Children(func() {
						ui.Text(c, r[0]).FontSize(12.5).TextColor(p.TextMuted).Width(130).Shrink(0)
						ui.Text(c, r[1]).FontSize(12.5).Font("ui-monospace").Wrap().Selectable().Grow(1).Basis(0)
					})
				}
			})
			if live {
				ui.Row(c).Justify(ui.End).Children(func() {
					id := cn.ID
					if m.button(c, "close", locales.ConnClose(), icXCircle, "danger").Clicked() {
						m.run(locales.CommonFailed(), func() error { return Connections{a}.Close(context.Background(), []string{id}) }, nil)
						s.detail = ""
					}
				})
			}
		})
	})
	if !open {
		s.detail = ""
	}
}
