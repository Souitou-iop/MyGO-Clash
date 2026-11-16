package app

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
)

// tailscalePage is what the Tailscale page keeps between frames.
type tailscalePage struct {
	sys      *coreapi.TailscaleStatus
	sysMode  string
	loginURL string
	authKey  string
	share    bool
	busy     string
	shared   *bool
	sharedAt string
	shareKey string
	online   int // online, all
	search   string
	pings    map[string]*coreapi.TailscalePingResult
	pinging  map[string]bool
	groups   []string
	groupsAt int
	host     string
	control  string
	port     string
	edited   bool
	qr       *ui.SVG
	qrFor    string
	qrWait   time.Time // since when the login card waits for its link
	qrBusy   bool      // a new link is being asked for
	qrAsked  bool      // one was asked for as the card first showed
	pingedAt time.Time // when the home page's sheet last pinged the peers
}

// qrWaitMax is how long the login card holds the place of a QR code whose
// link has not come.
const qrWaitMax = 15 * time.Second

func (m *mainUI) tailscaleView(c *ui.Context) {
	s, p, a := &m.tsPage, m.pal, m.a
	if s.pings == nil {
		s.pings, s.pinging, s.share = map[string]*coreapi.TailscalePingResult{}, map[string]bool{}, true
	}
	mode := cmp.Or(m.st.Tailscale.Mode, "off")
	if s.sysMode != mode {
		s.sysMode = mode
		win := m.window()
		go func() {
			st := (Tailscale{a}).SystemStatus(context.Background())
			win.Update(func() { s.sys = &st })
		}()
	}
	if !s.edited {
		s.edited, s.host, s.control, s.port = true, m.st.Tailscale.Hostname, m.st.Tailscale.ControlURL, strconv.Itoa(m.st.Tailscale.ShareProxyPort)
	}
	var st *coreapi.TailscaleStatus
	if m.ts.Source == mode {
		v := m.ts
		st = &v
	}
	setMode := func(v string) {
		m.st.Tailscale.Mode = v
		m.patch(map[string]any{"tailscale": map[string]any{"mode": v}})
	}
	m.header(c, "Tailscale", func() {
		// No state, as of a system app not found: no badge, rather than
		// the name of the missing state's text.
		if st != nil && st.BackendState != "" {
			tone := ""
			switch st.BackendState {
			case "Running":
				tone = "success"
			case "NeedsLogin", "NeedsMachineAuth", "Starting":
				tone = "warning"
			case "Stopped":
				tone = "danger"
			}
			m.stateBadge(c, "state", cmp.Or(m.T("ts.state."+st.BackendState), st.BackendState), tone)
		}
		if i := m.segmented(c, "mode", slices.Index(tsModes, mode), locales.TsModeOff(), locales.TsModeEmbedded(), locales.TsModeSystem()); i >= 0 {
			setMode(tsModes[i])
		}
	})
	coreRunning := m.state.Core.Status == coremgr.StatusRunning
	// The page of the mode chosen slides in from the side of its segment,
	// and within a mode the cards of each state crossfade as it goes.
	ui.Scroll(c.Key("ts")).TrackScroll(m.keepScroll("ts")).Grow(1).Padding(4, 22, 22).Children(func() {
		m.switchPane(c, "mode", slices.Index(tsModes, mode), 12, func() {
			switch mode {
			case "off":
				m.tsChoose(c, setMode)
			case "embedded":
				if !coreRunning {
					m.banner(c, "warning", locales.CommonCoreNotRunning(), func() {
						if m.button(c, "start", locales.CoreStart(), icRotateCw, "").Clicked() {
							m.run(locales.CommonFailed(), func() error { return Core{a}.Restart(context.Background()) }, nil)
						}
					})
				}
				state := ""
				switch {
				case st != nil && (st.BackendState == "NeedsLogin" || st.BackendState == "NeedsMachineAuth"):
					state = "login"
				case st != nil && st.BackendState == "Starting":
					// Only as the node says so. Before its first status
					// (st is nil: the page was just chosen) the card would
					// show for a moment and go as the status that comes,
					// stopped or running, takes its place, with the
					// settings below it jumping up and down.
					state = "starting"
				case st != nil && st.BackendState == "Running":
					state = "running"
				}
				m.swapPane(c, "state", state, 12, func() {
					switch state {
					case "login":
						m.tsLogin(c, st)
					case "starting":
						m.card(c, "starting", locales.TsStarting(), icSettings2, nil, func() {
							ui.Text(c, locales.TsStartingHint()).FontSize(12.5).TextColor(p.TextMuted)
						})
					case "running":
						m.tsRunning(c, st, mode)
					}
				})
				m.tsEmbeddedSettings(c)
			case "system":
				state := "running"
				switch {
				case st == nil:
					// Not heard from yet, as the page is just chosen: no
					// card of a missing app to give way to the real one.
					state = ""
				case !st.Available:
					state = "missing"
				case st.BackendState != "Running":
					state = "stopped"
				}
				m.swapPane(c, "state", state, 12, func() {
					switch state {
					case "":
					case "missing":
						m.card(c, "missing", locales.TsNotDetected(), icAppWindow, nil, func() {
							ui.Text(c, locales.TsInstallHint()).FontSize(12.5).TextColor(p.TextMuted).Wrap()
							if st != nil && st.Error != "" {
								ui.Text(c, st.Error).FontSize(11.5).Font("ui-monospace").TextColor(p.Faint).Wrap()
							}
							ui.Row(c).Children(func() {
								if m.button(c, "download", locales.TsDownload(), icExternalLink, "").Clicked() {
									go mygo.Shell.OpenExternal("https://tailscale.com/download")
								}
							})
						})
					case "stopped":
						m.banner(c, "warning", locales.TsSystemNotRunning(m.T("ts.state."+st.BackendState)), func() {
							if m.button(c, "connect", locales.TsConnect(), nil, "").Clicked() {
								m.run(locales.CommonFailed(), func() error { return Tailscale{a}.SetRunning(context.Background(), true) }, nil)
							}
						})
					default:
						m.tsRunning(c, st, mode)
					}
				})
				if st != nil {
					m.tsSystemSettings(c, st)
				}
			}
		})
	})
}

// tsModes are the Tailscale page's modes, in the order of their segments.
var tsModes = []string{"off", "embedded", "system"}

// tsChoose is the page while Tailscale is off: what it is, and the two
// ways to have it.
func (m *mainUI) tsChoose(c *ui.Context, setMode func(string)) {
	s, p := &m.tsPage, m.pal
	ui.Column(c).Gap(14).MaxWidth(900).Children(func() {
		ui.Row(c).Padding(18).Gap(16).Radius(tileRadius).Background(p.Card).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(52, 52).Radius(14).Background(p.AccentSoft).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, icWaypoints).FontSize(26).TextColor(p.Accent)
			})
			ui.Column(c).Grow(1).Basis(0).Gap(4).Children(func() {
				ui.Text(c, locales.TsHeroTitle()).FontSize(17).FontWeight(650)
				ui.Text(c, locales.TsHeroDesc()).FontSize(12.5).TextColor(p.TextMuted).Wrap()
			})
		})
		choice := func(key string, icon *ui.SVG, iconCol ui.Color, title string, badge func(), desc string, points []string, button func()) {
			ui.Column(c.Key(key)).Grow(1).Padding(16).Gap(10).Radius(tileRadius).Background(p.Card).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Icon(c, icon).FontSize(18).TextColor(iconCol)
					ui.Text(c, title).FontSize(14).FontWeight(650)
					badge()
				})
				ui.Text(c, desc).FontSize(12.5).TextColor(p.TextMuted).Wrap()
				ui.Column(c).Gap(5).Children(func() {
					for _, pt := range points {
						ui.Row(c.Key(pt)).Gap(8).AlignItems(ui.Start).Children(func() {
							ui.Icon(c, icCheck).FontSize(13).TextColor(p.Success)
							ui.Text(c, pt).FontSize(12.5).Wrap().Grow(1).Basis(0)
						})
					}
				})
				ui.Spacer(c)
				ui.Row(c).Children(button)
			})
		}
		m.cardGrid(c, "choices", 300, 2, func(i int) {
			if i == 0 {
				choice("embedded", icActivity, p.AccentFg, locales.TsModeEmbedded(), func() { m.badge(c, locales.TsRecommended(), "info") },
					locales.TsEmbeddedDesc(), []string{locales.TsEmbeddedP1(), locales.TsEmbeddedP2(), locales.TsEmbeddedP3()}, func() {
						if m.button(c, "use", locales.TsUseEmbedded(), nil, "primary").Clicked() {
							setMode("embedded")
						}
					})
				return
			}
			choice("system", icAppWindow, p.Accent, locales.TsModeSystem(), func() {
				if s.sys != nil && s.sys.Available {
					m.badge(c, locales.TsDetected(), "success")
				} else {
					m.badge(c, locales.TsNotDetected(), "")
				}
			}, locales.TsSystemDesc(), []string{locales.TsSystemP1(), locales.TsSystemP2(), locales.TsSystemP3()}, func() {
				if m.button(c, "use", locales.TsUseSystem(), nil, "").Clicked() {
					setMode("system")
				}
			})
		})
	})
}

// qrImage is a QR code of text, kept while the text stays the same.
func (m *mainUI) qrImage(text string) *ui.SVG {
	s := &m.tsPage
	if s.qrFor != text {
		s.qrFor, s.qr = text, nil
		if svg, err := qrSVG(text); err == nil {
			s.qr, _ = ui.ParseSVG([]byte(svg))
		}
	}
	return s.qr
}

func (m *mainUI) tsLogin(c *ui.Context, st *coreapi.TailscaleStatus) {
	s, p, a := &m.tsPage, m.pal, m.a
	shown := cmp.Or(s.loginURL, st.AuthURL)
	m.card(c, "login", locales.TsSignIn(), icLogIn, nil, func() {
		if pr := st.ControlProbe; pr != nil && !pr.Reachable {
			ui.Column(c).Padding(10, 12).Gap(4).Radius(8).Background(p.WarningSoft).Children(func() {
				ui.Text(c, locales.TsControlUnreachable()).FontSize(12.5).TextColor(p.Warning).FontWeight(600)
				ui.Text(c, locales.TsControlUnreachableHint()).FontSize(12).TextColor(p.TextMuted).Wrap()
				if pr.Error != "" {
					ui.Text(c, pr.Error).FontSize(11.5).Font("ui-monospace").TextColor(p.Faint).Wrap().Selectable()
				}
			})
		}
		ui.Row(c).Gap(24).AlignItems(ui.Start).Wrap().Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(260).Gap(12).Children(func() {
				hint := locales.TsSignInHint()
				if st.BackendState == "NeedsMachineAuth" {
					hint = locales.TsNeedsApproval()
				}
				ui.Text(c, hint).FontSize(12.5).TextColor(p.TextMuted).Wrap()
				ui.Row(c).Children(func() {
					if s.busy == "browser" {
						ui.Spinner(c)
					} else if m.button(c, "browser", locales.TsSignInBrowser(), icGlobe, "primary").Clicked() {
						s.busy = "browser"
						win := m.window()
						go func() {
							u, err := (Tailscale{a}).Login(context.Background(), "")
							win.Update(func() {
								s.busy = ""
								if err != nil {
									m.notices = append(m.notices, Notice{Level: "error", Message: locales.TsLoginFailed(), Detail: err.Error()})
									return
								}
								if u != "" {
									s.loginURL = u
									go mygo.Shell.OpenExternal(u)
								}
							})
						}()
					}
				})
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Height(1).Grow(1).Background(p.Border)
					ui.Text(c, locales.CommonOr()).FontSize(11.5).TextColor(p.Faint)
					ui.Box(c).Height(1).Grow(1).Background(p.Border)
				})
				// field takes its share of a row's width (Basis(0)); set
				// straight in this column it gets no height of its own, and
				// the card ends at its label until the QR code beside it
				// stretches the column.
				ui.Row(c).Children(func() {
					m.field(c, locales.TsAuthKey(), locales.TsAuthKeyHint(), "", func() {
						ui.Row(c).Gap(8).Children(func() {
							ui.TextInput(c.Key("key"), &s.authKey).Password().Placeholder("tskey-auth-…").Grow(1)
							if s.busy == "key" {
								ui.Spinner(c)
							} else if m.button(c, "use-key", locales.TsUseKey(), icKeyRound, "").Disabled(strings.TrimSpace(s.authKey) == "").Clicked() {
								s.busy = "key"
								key, share := s.authKey, s.share
								m.run(locales.TsLoginFailed(), func() error {
									if _, err := (Tailscale{a}).Login(context.Background(), key); err != nil {
										return err
									}
									if share {
										return Tailscale{a}.ShareAuthKey(context.Background(), key)
									}
									return nil
								}, func(error) { s.busy, s.authKey = "", "" })
							}
						})
						ui.Row(c).Gap(8).AlignItems(ui.Center).PaddingY(4).Children(func() {
							ui.Switch(c.Key("share"), &s.share)
							ui.Text(c, locales.TsShareKey()).FontSize(12).TextColor(p.TextMuted).Wrap().Grow(1).Basis(0)
						})
					})
				})
			})
			// The QR code of the link to sign in by. Its place stays while
			// the node waits to be signed in: breathing while a link is on its
			// way (one is asked for as the card first shows, without opening
			// the browser), the code once it comes, and the way to ask for
			// another where none came. A press asks for a new one any time.
			if st.BackendState != "NeedsLogin" {
				return
			}
			if shown != "" {
				s.qrWait = time.Time{}
			} else if s.qrWait.IsZero() {
				s.qrWait = c.Now()
			}
			refresh := func() {
				if s.qrBusy {
					return
				}
				s.qrBusy, s.qrWait = true, time.Now()
				win := m.window()
				go func() {
					u, err := (Tailscale{a}).Login(context.Background(), "")
					win.Update(func() {
						s.qrBusy, s.qrWait = false, time.Now()
						if err != nil {
							m.notices = append(m.notices, Notice{Level: "error", Message: locales.TsLoginFailed(), Detail: err.Error()})
							return
						}
						if u != "" {
							s.loginURL = u
						}
					})
				}()
			}
			if shown == "" && !s.qrAsked {
				s.qrAsked = true
				refresh()
			}
			reachable := st.ControlProbe == nil || st.ControlProbe.Reachable
			waiting := shown == "" && (s.qrBusy || (reachable && c.Now().Sub(s.qrWait) < qrWaitMax))
			if waiting {
				c.After(qrWaitMax - c.Now().Sub(s.qrWait))
			}
			ui.Column(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Box(c.Key("qr")).Size(168, 168).Radius(8).Clip().Children(func() {
					state := "code"
					switch {
					case shown != "" && !s.qrBusy:
					case waiting || s.qrBusy:
						state = "wait"
					default:
						state = "none"
					}
					ui.Box(c.Key(state)).Absolute().Left(0).Top(0).Right(0).Bottom(0).AlignItems(ui.Center).Justify(ui.Center).Transition(revealIn).Children(func() {
						switch state {
						case "code":
							if q := m.qrImage(shown); q != nil {
								ui.Image(c, q).Fill()
							}
						case "wait":
							m.skeleton(c, 168).Width(168)
						default:
							ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).Background(p.Surface2)
							if m.iconButton(c, "again", icRefreshCw, locales.CommonRefresh()).Size(44, 44).Clicked() {
								refresh()
							}
						}
					})
				})
				ui.Text(c, locales.TsScanHint()).FontSize(11.5).TextColor(p.Faint).TextAlign(ui.Center).MaxWidth(180).Wrap()
				ui.Row(c).Gap(6).Children(func() {
					if m.button(c, "open", locales.CommonOpen(), icExternalLink, "").Disabled(shown == "").Clicked() {
						go mygo.Shell.OpenExternal(shown)
					}
					if m.button(c, "copy", locales.CommonCopy(), icCopy, "").Disabled(shown == "").Clicked() {
						mygo.Clipboard.WriteText(shown)
						c.Toast(locales.CommonCopied())
					}
					if m.spinButton(c, "refresh", icRefreshCw, locales.CommonRefresh(), s.qrBusy).Clicked() {
						refresh()
					}
				})
			})
		})
	})
}

func (m *mainUI) tsRunning(c *ui.Context, st *coreapi.TailscaleStatus, mode string) {
	ui.Row(c).Gap(12).AlignItems(ui.Stretch).Wrap().Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(320).Children(func() { m.tsSelf(c, st, mode) })
		ui.Column(c).Grow(1).Basis(0).MinWidth(280).Children(func() { m.tsExitNode(c, st, mode) })
	})
	m.tsRouting(c, st, mode)
	m.tsPeers(c, st, mode)
}

func (m *mainUI) tsSelf(c *ui.Context, st *coreapi.TailscaleStatus, mode string) {
	s, p, a := &m.tsPage, m.pal, m.a
	self := st.Self
	if self == nil {
		return
	}
	if mode == "embedded" && s.sharedAt != st.BackendState {
		s.sharedAt = st.BackendState
		win := m.window()
		go func() {
			v := (Tailscale{a}).AuthKeyShared()
			win.Update(func() { s.shared = &v })
		}()
	}
	m.card(c, "self", locales.TsThisDevice(), icMonitor, func() {
		if m.button(c, "admin", locales.TsAdmin(), icExternalLink, "ghost").Height(26).Clicked() {
			go func() { _ = Tailscale{a}.OpenAdmin() }()
		}
		if mode == "embedded" && m.button(c, "logout", locales.TsLogout(), icLogOut, "danger").Height(26).Clicked() {
			m.confirm(locales.TsLogoutTitle(), locales.TsLogoutMsg(), locales.TsLogout(), func() {
				m.run(locales.CommonFailed(), func() error { return Tailscale{a}.Logout(context.Background(), true) }, nil)
			})
		}
	}, func() {
		m.kv(c, func() {
			m.kvText(c, locales.TsDeviceName(), self.HostName)
			m.kvRow(c, locales.TsAddress(), func() {
				for _, ip := range self.TailscaleIPs {
					if m.chip(c, ip, ip, false).Tooltip(locales.CommonCopy()).Clicked() {
						mygo.Clipboard.WriteText(ip)
						c.Toast(locales.TsIpCopied())
					}
				}
			})
			m.kvText(c, "MagicDNS", cmp.Or(self.DNSName, "—")).Font("ui-monospace").Selectable()
			user := "—"
			if st.User != nil {
				user = st.User.LoginName
			}
			m.kvText(c, locales.TsAccount(), user)
			m.kvText(c, locales.TsTailnet(), cmp.Or(st.TailnetName, "—"))
			if st.ShareProxy != "" {
				m.kvText(c, locales.TsSharedProxy(), st.ShareProxy).Font("ui-monospace")
			}
		})
		if mode == "embedded" && s.shared != nil {
			if !*s.shared {
				ui.Row(c).Gap(8).Children(func() {
					ui.TextInput(c.Key("share-key"), &s.shareKey).Password().Placeholder("tskey-auth-…").Grow(1)
					if m.button(c, "share", locales.TsShareBtn(), icKeyRound, "").Disabled(strings.TrimSpace(s.shareKey) == "").Clicked() {
						key := s.shareKey
						m.run(locales.CommonFailed(), func() error { return Tailscale{a}.ShareAuthKey(context.Background(), key) }, func(error) { s.shareKey, s.sharedAt = "", "" })
					}
				})
			} else {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, locales.TsKeyShared()).FontSize(12).TextColor(p.TextMuted).Grow(1)
					if m.button(c, "stop", locales.TsStopSharing(), nil, "").Height(26).Clicked() {
						m.run(locales.CommonFailed(), func() error { return Tailscale{a}.StopSharingAuthKey(context.Background()) }, func(error) { s.sharedAt = "" })
					}
				})
			}
		}
		for i, h := range st.Health {
			ui.Text(c.Key(i), "⚠ "+h).FontSize(12).TextColor(p.Warning).Wrap()
		}
		ui.Spacer(c)
	}).Grow(1)
}

func (m *mainUI) tsExitNode(c *ui.Context, st *coreapi.TailscaleStatus, mode string) {
	s, p, a := &m.tsPage, m.pal, m.a
	var options []coreapi.TailscalePeer
	for _, pe := range st.Peers {
		if pe.ExitNodeOption {
			options = append(options, pe)
		}
	}
	m.card(c, "exit", locales.TsExitNode(), icRouter, nil, func() {
		hint := locales.TsExitHintSystem()
		if mode == "embedded" {
			hint = locales.TsExitHintEmbedded()
		}
		ui.Text(c, hint).FontSize(12.5).TextColor(p.TextMuted).Wrap()
		labels := []string{locales.CommonNone()}
		ids := []string{""}
		for _, pe := range options {
			l := pe.HostName
			if !pe.Online {
				l += " (" + locales.TsOffline() + ")"
			}
			labels = append(labels, l)
			ids = append(ids, pe.ID)
		}
		cur := labels[max(0, slices.Index(ids, st.ExitNodeID))]
		if ui.Select(c.Key("exit"), &cur, labels).Disabled(s.busy == "exit").Changed() {
			id := ids[slices.Index(labels, cur)]
			s.busy = "exit"
			m.run(locales.CommonFailed(), func() error { return Tailscale{a}.SetExitNode(context.Background(), id) }, func(error) { s.busy = "" })
		}
		if len(options) == 0 {
			ui.Text(c, locales.TsNoExitNodes()).FontSize(12).TextColor(p.Faint)
		}
		if mode == "embedded" {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.TsAllowLan()).Grow(1)
				on := m.st.Tailscale.ExitNodeAllowLAN
				if ui.Switch(c.Key("lan"), &on).Changed() {
					v := on
					m.run(locales.CommonFailed(), func() error {
						return Tailscale{a}.SetPrefs(context.Background(), coreapi.TailscalePrefs{ExitNodeAllowLAN: &v})
					}, nil)
				}
			})
		}
		ui.Spacer(c)
	}).Grow(1)
}

func (m *mainUI) tsRouting(c *ui.Context, st *coreapi.TailscaleStatus, mode string) {
	p, ts := m.pal, m.st.Tailscale
	via := "DIRECT"
	if mode == "embedded" {
		via = ts.ProxyName
	}
	dns := "*.ts.net"
	if st.MagicDNSSuffix != "" {
		dns += ", *." + st.MagicDNSSuffix
	}
	rows := [][2]string{{"100.64.0.0/10", via}, {"fd7a:115c:a1e0::/48", via}, {dns, via}}
	if mode == "embedded" && ts.RouteSubnets && ts.AcceptRoutes {
		for _, r := range st.Routes {
			rows = append(rows, [2]string{r, via})
		}
	}
	m.card(c, "routing", locales.TsRouting(), icWaypoints, nil, func() {
		hint := locales.TsRoutingSystemOff()
		switch {
		case mode == "embedded":
			hint = locales.TsRoutingEmbedded(ts.ProxyName)
		case ts.Coexist:
			hint = locales.TsRoutingSystem()
		}
		ui.Text(c, hint).FontSize(12.5).TextColor(p.TextMuted).Wrap()
		if mode != "embedded" && !ts.Coexist {
			return
		}
		m.rows(c, "routes", func() {
			for _, r := range rows {
				ui.Row(c.Key(r[0])).Padding(8, 12).Gap(10).AlignItems(ui.Center).Children(func() {
					ui.Text(c, r[0]).FontSize(12.5).Font("ui-monospace").SingleLine().Grow(1).Basis(0)
					ui.Text(c, "→").TextColor(p.Faint)
					tone := "info"
					if r[1] == "DIRECT" {
						tone = ""
					}
					m.badge(c, r[1], tone)
				})
			}
		})
	})
}

func (m *mainUI) tsPeers(c *ui.Context, st *coreapi.TailscaleStatus, mode string) {
	s, p, a := &m.tsPage, m.pal, m.a
	q := strings.ToLower(strings.TrimSpace(s.search))
	online := 0
	var peers []coreapi.TailscalePeer
	for _, pe := range st.Peers {
		if pe.Online {
			online++
		}
		if s.online == 0 && !pe.Online {
			continue
		}
		if q != "" && !slices.ContainsFunc(append([]string{pe.HostName, pe.DNSName, pe.OS, pe.User}, pe.TailscaleIPs...), func(v string) bool { return strings.Contains(strings.ToLower(v), q) }) {
			continue
		}
		peers = append(peers, pe)
	}
	c.After(15 * time.Second)
	m.card(c, "peers", locales.TsPeers(), icNetwork, func() {
		ui.SearchField(c.Key("q"), &s.search).Placeholder(locales.CommonSearch()).Width(180)
		if i := m.segmented(c, "filter", s.online, fmt.Sprintf("%s %d", locales.TsOnline(), online), fmt.Sprintf("%s %d", locales.CommonAll(), len(st.Peers))); i >= 0 {
			s.online = i
		}
	}, func() {
		// Peers filtered in and out fold in and away, the rest gliding
		// to their places.
		m.swapPane(c, "list", map[bool]string{true: "empty", false: "list"}[len(peers) == 0], 0, func() {
			if len(peers) == 0 {
				m.empty(c, locales.TsNoPeers(), nil)
				return
			}
			ui.Column(c).Dividers(1, p.Border.Alpha(0.6)).Gap(1).Children(func() {
				for _, pe := range peers {
					ui.Row(c.Key(pe.ID)).PaddingY(8).Gap(10).AlignItems(ui.Center).Transition(listRow).Children(func() {
						dot := p.Faint
						if pe.Online {
							dot = p.Success
						}
						ui.Box(c).Size(8, 8).Radius(4).Background(dot).Shrink(0)
						os := strings.ToLower(pe.OS)
						icon := icMonitor
						switch {
						case strings.Contains(os, "ios") || strings.Contains(os, "android"):
							icon = icSmartphone
						case strings.Contains(os, "linux"):
							icon = icServer
						}
						ui.Icon(c, icon).FontSize(15).TextColor(p.TextMuted)
						ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
							ui.Row(c).Gap(6).AlignItems(ui.Center).Wrap().Children(func() {
								ui.Text(c, pe.HostName).FontWeight(600).SingleLine()
								if pe.ExitNode {
									m.badge(c, locales.TsUsingExit(), "info")
								} else if pe.ExitNodeOption {
									m.badge(c, locales.TsExitCapable(), "")
								}
								if len(pe.PrimaryRoutes) > 0 {
									m.badge(c, locales.TsSubnets(fmt.Sprint(len(pe.PrimaryRoutes))), "info")
								}
								if pe.Expired {
									m.badge(c, locales.TsKeyExpired(), "danger")
								}
								for _, tag := range pe.Tags {
									m.badge(c, strings.Replace(tag, "tag:", "#", 1), "")
								}
							})
							line := cmp.Or(pe.OS, "—")
							if pe.User != "" {
								line += " · " + pe.User
							}
							switch {
							case pe.Online && pe.Active:
								how := locales.TsRelay() + " " + pe.Relay
								if pe.CurAddr != "" {
									how = locales.TsDirect()
								}
								line += " · " + how + " · ↓" + fmtBytes(pe.RxBytes) + " ↑" + fmtBytes(pe.TxBytes)
							case pe.Online:
								line += " · " + locales.TsIdle()
							case pe.LastSeen != nil:
								line += " · " + locales.TsLastSeen(m.relTime(*pe.LastSeen))
							}
							ui.Text(c, line).FontSize(11.5).TextColor(p.Faint).SingleLine()
						})
						if len(pe.TailscaleIPs) > 0 {
							ip := pe.TailscaleIPs[0]
							if m.chip(c, "ip", ip, false).Tooltip(locales.CommonCopy()).Clicked() {
								mygo.Clipboard.WriteText(ip)
								c.Toast(locales.TsIpCopied())
							}
						}
						if r := s.pings[pe.ID]; r != nil {
							if r.Err != "" {
								ui.Text(c, locales.CommonFailed()).FontSize(12).TextColor(p.Danger)
							} else {
								arrow := "⇢"
								if r.Endpoint != "" {
									arrow = "↔"
								}
								ui.Text(c, fmt.Sprintf("%.0f ms %s", r.LatencyMs, arrow)).FontSize(12).TextColor(p.Success).FontFeatures("tnum").ContentTransition(numRoll).Tooltip(cmp.Or(r.Endpoint, r.DERP))
							}
						}
						id := pe.ID
						can := pe.Online && len(pe.TailscaleIPs) > 0 && mode != "off"
						if m.spinButton(c, "ping", icRadar, locales.TsPing(), s.pinging[id]).Disabled(!can).Clicked() {
							s.pinging[id] = true
							ip := pe.TailscaleIPs[0]
							win := m.window()
							go func() {
								r, err := (Tailscale{a}).Ping(context.Background(), ip)
								if err != nil {
									r = coreapi.TailscalePingResult{Err: err.Error()}
								}
								win.Update(func() {
									delete(s.pinging, id)
									s.pings[id] = &r
								})
							}()
						}
					})
				}
			})
		})
	})
}

// tsSwitch is a line of settings whose switch patches a field of the
// Tailscale settings.
func (m *mainUI) tsSwitch(c *ui.Context, key, label, desc string, on bool) {
	m.setting(c, key, label, desc, func() {
		v := on
		if ui.Switch(c.Key("sw"), &v).Changed() {
			m.patch(map[string]any{"tailscale": map[string]any{key: v}})
		}
	})
}

func (m *mainUI) tsEmbeddedSettings(c *ui.Context) {
	s, p, a, ts := &m.tsPage, m.pal, m.a, m.st.Tailscale
	if s.groupsAt != m.runtime+1 && m.state.Core.Status == coremgr.StatusRunning {
		s.groupsAt = m.runtime + 1
		win := m.window()
		go func() {
			v, err := (Proxies{a}).View(context.Background())
			if err != nil {
				return
			}
			var names []string
			for _, g := range v.Groups {
				names = append(names, g.Name)
			}
			win.Update(func() { s.groups = names })
		}()
	}
	ui.Text(c, locales.TsNodeSettings()).FontSize(12).FontWeight(600).TextColor(p.TextMuted).PaddingY(6)
	m.rows(c, "settings", func() {
		m.setting(c, "hostname", locales.TsDeviceName(), locales.TsDeviceNameHint(), func() {
			in := ui.TextInput(c.Key("in"), &s.host).Placeholder(locales.TsDeviceNameAuto()).Width(200)
			if s.host != ts.Hostname && (in.Submitted() || !in.Focused()) {
				m.patch(map[string]any{"tailscale": map[string]any{"hostname": s.host}})
			}
		})
		m.setting(c, "controlUrl", locales.TsControlUrl(), locales.TsControlUrlHint(), func() {
			in := ui.TextInput(c.Key("in"), &s.control).Placeholder("https://controlplane.tailscale.com").Width(260)
			if s.control != ts.ControlURL && (in.Submitted() || !in.Focused()) {
				m.patch(map[string]any{"tailscale": map[string]any{"controlUrl": s.control}})
			}
		})
		m.setting(c, "controlVia", locales.TsControlVia(), locales.TsControlViaHint(), func() {
			labels := append([]string{locales.TsDirect()}, s.groups...)
			values := append([]string{""}, s.groups...)
			cur := labels[max(0, slices.Index(values, ts.ControlVia))]
			if ui.Select(c.Key("sel"), &cur, labels).Width(200).Changed() {
				m.patch(map[string]any{"tailscale": map[string]any{"controlVia": values[slices.Index(labels, cur)]}})
			}
		})
		m.tsSwitch(c, "acceptRoutes", locales.TsAcceptRoutes(), locales.TsAcceptRoutesHint(), ts.AcceptRoutes)
		m.tsSwitch(c, "routeSubnets", locales.TsRouteSubnets(), locales.TsRouteSubnetsHint(), ts.RouteSubnets)
		m.tsSwitch(c, "magicDns", "MagicDNS", locales.TsMagicDnsHint(), ts.MagicDNS)
		m.tsSwitch(c, "joinSelector", locales.TsJoinSelector(), locales.TsJoinSelectorHint(ts.ProxyName), ts.JoinSelector)
		m.setting(c, "shareProxy", locales.TsShareProxy(), locales.TsShareProxyHint(), func() {
			m.collapseInline(c, "ts-share-port", ts.ShareProxy, func() {
				in := ui.TextInput(c.Key("port"), &s.port).Width(90)
				if n, err := strconv.Atoi(strings.TrimSpace(s.port)); err == nil && n != ts.ShareProxyPort && n > 0 && n < 65536 && (in.Submitted() || !in.Focused()) {
					m.patch(map[string]any{"tailscale": map[string]any{"shareProxyPort": n}})
				}
			})
			v := ts.ShareProxy
			if ui.Switch(c.Key("sw"), &v).Changed() {
				m.patch(map[string]any{"tailscale": map[string]any{"shareProxy": v}})
			}
		})
		m.tsSwitch(c, "ephemeral", locales.TsEphemeral(), locales.TsEphemeralHint(), ts.Ephemeral)
	})
}

func (m *mainUI) tsSystemSettings(c *ui.Context, st *coreapi.TailscaleStatus) {
	p, a, ts := m.pal, m.a, m.st.Tailscale
	ui.Text(c, locales.TsCoexistence()).FontSize(12).FontWeight(600).TextColor(p.TextMuted).PaddingY(6)
	m.rows(c, "system", func() {
		m.setting(c, "connected", locales.TsConnected(), locales.TsConnectedHint(), func() {
			on := st.BackendState == "Running"
			if ui.Switch(c.Key("sw"), &on).Changed() {
				v := on
				m.run(locales.CommonFailed(), func() error { return Tailscale{a}.SetRunning(context.Background(), v) }, nil)
			}
		})
		m.tsSwitch(c, "coexist", locales.TsCoexist(), locales.TsCoexistHint(), ts.Coexist)
		m.tsSwitch(c, "magicDns", "MagicDNS", locales.TsMagicDnsSystemHint(), ts.MagicDNS)
		m.setting(c, "acceptRoutes", locales.TsAcceptRoutes(), locales.TsAcceptRoutesHint(), func() {
			on := st.AcceptRoutes
			if ui.Switch(c.Key("sw"), &on).Changed() {
				v := on
				m.run(locales.CommonFailed(), func() error {
					return Tailscale{a}.SetPrefs(context.Background(), coreapi.TailscalePrefs{AcceptRoutes: &v})
				}, nil)
			}
		})
	})
}
