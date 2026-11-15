package app

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/enhance"
	"github.com/mygo-clash/mygo-clash/internal/profiles"
)

// profilesPage is what the profiles page keeps between frames.
type profilesPage struct {
	url       string
	importing bool
	updating  bool
	busy      map[string]string // what a profile does, by UID
	errOpen   map[string]bool

	// The profile dialog: a new profile, or one's properties.
	dialog    bool
	dlgUID    string
	dlgType   int // remote, local
	dlgName   string
	dlgDesc   string
	dlgURL    string
	dlgOpt    profiles.Option
	dlgIntvl  string
	dlgTO     string
	dlgBusy   bool
	dlgSuggst int

	// The text editor: a profile's YAML, an extension's YAML or JavaScript.
	edit     bool
	editUID  string
	editName string
	editRO   bool
	code     *editor.State
	codeLogs []enhance.LogEntry
	codeBusy bool

	// The editor of a rules, proxies or groups extension.
	seq      bool
	seqUID   string
	seqProf  string
	seqKind  string
	seqName  string
	seqPatch *enhance.SeqPatch
	seqOrig  string
	seqNames NameLists
	seqOwn   []string
	seqTab   int
	seqQ     string
	seqBusy  bool
	rule     [4]string // type, payload, target, no-resolve
	links    string
	group    [4]string // name, type, url, filter
	members  []string

	runtime  bool
	logs     bool
	logsData map[string][]enhance.LogEntry

	sub      bool
	subBase  string
	subItems []profiles.SubStoreItem
	subBusy  bool
	subAdd   string
}

var ruleTypes = []string{"DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "DOMAIN-REGEX", "DOMAIN-WILDCARD", "GEOSITE", "IP-CIDR", "IP-CIDR6",
	"IP-SUFFIX", "IP-ASN", "GEOIP", "SRC-GEOIP", "SRC-IP-CIDR", "SRC-PORT", "DST-PORT", "IN-PORT", "IN-TYPE", "NETWORK", "PROCESS-NAME",
	"PROCESS-PATH", "PROCESS-NAME-REGEX", "UID", "DSCP", "RULE-SET", "MATCH"}

var noResolveTypes = []string{"IP-CIDR", "IP-CIDR6", "IP-SUFFIX", "IP-ASN", "GEOIP", "RULE-SET"}

var groupKinds = []string{"select", "url-test", "fallback", "load-balance", "relay"}

func (m *mainUI) profilesView(c *ui.Context) {
	s, p, a := &m.profs, m.pal, m.a
	if s.busy == nil {
		s.busy, s.errOpen = map[string]string{}, map[string]bool{}
	}
	m.header(c, locales.NavProfiles(), func() {
		if m.button(c, "logs", locales.ProfilesEnhanceLogs(), icScrollText, "ghost").Clicked() {
			s.logs, s.logsData = true, nil
			win := m.window()
			go func() {
				l, _ := (Profiles{a}).Logs(context.Background())
				win.Update(func() {
					if l == nil {
						l = map[string][]enhance.LogEntry{}
					}
					s.logsData = l
				})
			}()
		}
		if m.button(c, "runtime", locales.ProfilesRuntime(), icFileStack, "ghost").Clicked() {
			m.openRuntime()
		}
		m.busySwap(c, "update-all-busy", s.updating, func() {
			if m.button(c, "update-all", locales.ProfilesUpdateAll(), icRefreshCw, "").Clicked() {
				s.updating = true
				win := m.window()
				go func() {
					failed, err := (Profiles{a}).UpdateAll(context.Background())
					win.Update(func() {
						s.updating = false
						switch {
						case err != nil:
							m.notices = append(m.notices, Notice{Level: "error", Message: locales.ProfilesUpdateFailed(), Detail: err.Error()})
						case len(failed) > 0:
							m.notices = append(m.notices, Notice{Level: "warning", Message: locales.ProfilesSomeFailed(fmt.Sprint(len(failed)))})
						default:
							m.notices = append(m.notices, Notice{Level: "success", Message: locales.ProfilesAllUpdated()})
						}
					})
				}()
			}
		})
	})
	ui.Scroll(c.Key("profiles")).TrackScroll(m.keepScroll("profiles")).Grow(1).Padding(4, 22, 22).Gap(14).Children(func() {
		if m.state.ConfigError != "" {
			m.banner(c, "danger", m.state.ConfigError, nil)
		}
		// Importing: a URL, a file, a new profile, or Sub-Store.
		ui.Row(c).Padding(10).Gap(8).Radius(tileRadius).Background(p.Card).AlignItems(ui.Center).Children(func() {
			in := ui.TextInput(c.Key("url"), &s.url).Placeholder(locales.ProfilesUrlPlaceholder()).Grow(1).Basis(0).Height(34)
			if in.Submitted() {
				m.importURL()
			}
			m.busySwap(c, "import-busy", s.importing, func() {
				if m.button(c, "import", locales.ProfilesImport(), nil, "primary").Height(34).Disabled(strings.TrimSpace(s.url) == "").Clicked() {
					m.importURL()
				}
			})
			if m.iconButton(c, "file", icFileUp, locales.ProfilesFromFile()).Size(34, 34).Clicked() {
				m.run(locales.ProfilesImportFailed(), func() error { _, err := (Profiles{a}).ImportFile(context.Background()); return err }, nil)
			}
			if m.iconButton(c, "new", icFilePlus2, locales.ProfilesNew()).Size(34, 34).Clicked() {
				m.openProfileDialog(nil)
			}
			if m.iconButton(c, "substore", icBoxes, locales.SubstoreImport()).Size(34, 34).Clicked() {
				s.sub, s.subBase, s.subItems = true, m.st.Subscriptions.SubStore, nil
				if s.subBase != "" {
					m.loadSubStore()
				}
			}
		})
		items := m.pv.Items
		// The first profile in, or the last out, crossfades with the
		// empty page.
		m.swapPane(c, "list-full", map[bool]string{true: "empty", false: "full"}[len(items) == 0], 0, func() {
			if len(items) == 0 {
				m.empty(c, locales.ProfilesEmpty(), func() {
					ui.Text(c, locales.ProfilesEmptyHint()).FontSize(12.5).TextColor(p.TextMuted)
				})
				return
			}
			m.cardGridOf(c, "list", 290, len(items), func(i int) any { return items[i].UID }, func(i int) { m.profileTile(c, &items[i]) })
		})
		ui.Text(c, locales.ProfilesGlobalExt()).FontSize(12).FontWeight(600).TextColor(p.TextMuted).PaddingY(8)
		globals := []struct {
			uid, title, desc string
			icon             *ui.SVG
		}{{"Merge", locales.ProfilesGlobalMerge(), locales.ProfilesGlobalMergeDesc(), icMerge}, {"Script", locales.ProfilesGlobalScript(), locales.ProfilesGlobalScriptDesc(), icCode2}}
		m.cardGrid(c, "globals", 290, len(globals), func(i int) {
			g := globals[i]
			ui.Row(c).Padding(14).Gap(12).Radius(tileRadius).Background(p.Card).AlignItems(ui.Start).Grow(1).Children(func() {
				ui.Box(c).Size(34, 34).Radius(10).Background(p.AccentSoft).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
					ui.Icon(c, g.icon).FontSize(16).TextColor(p.Accent)
				})
				ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
					ui.Text(c, g.title).FontWeight(620)
					ui.Text(c, g.desc).FontSize(12).TextColor(p.TextMuted).Wrap()
				})
				if m.button(c, "edit", locales.CommonEdit(), icPencil, "").Clicked() {
					m.openEditor(g.uid, g.title, false)
				}
			})
		})
	})
	m.profileDialog(c)
	m.editorDialog(c)
	m.seqDialog(c)
	m.runtimeDialog(c)
	m.enhanceLogsDialog(c)
	m.subStoreDialog(c)
}

// cardGrid lays n cards out in as many columns of at least min DIPs as fit.
func (m *mainUI) cardGrid(c *ui.Context, key any, least float32, n int, card func(i int)) {
	m.cardGridOf(c, key, least, n, func(i int) any { return i }, card)
}

// cardGridOf is a cardGrid whose cards are known by id: they glide to
// their new places as they are sorted, added or removed, and as the grid
// gains or loses columns.
func (m *mainUI) cardGridOf(c *ui.Context, key any, least float32, n int, id func(i int) any, card func(i int)) {
	// The columns come from the width the grid had, and their widths from
	// the one it has, so that resizing the window follows it at once.
	m.flipGrid(c, key, 12, func(w float32) int { return int((w + 12) / (least + 12)) }, &reflowOut, n, id,
		func(i int, box ui.Element) { box.Children(func() { card(i) }) })
}

func (m *mainUI) importURL() {
	s, a := &m.profs, m.a
	u := strings.TrimSpace(s.url)
	if !strings.HasPrefix(strings.ToLower(u), "http://") && !strings.HasPrefix(strings.ToLower(u), "https://") {
		m.notices = append(m.notices, Notice{Level: "warning", Message: locales.ProfilesBadUrl()})
		return
	}
	s.importing = true
	m.run(locales.ProfilesImportFailed(), func() error {
		pr, err := (Profiles{a}).Create(context.Background(), profiles.NewProfile{Type: "remote", URL: u})
		if err == nil {
			m.notice(Notice{Level: "success", Message: locales.ProfilesImported(), Detail: pr.Name})
		}
		return err
	}, func(err error) {
		s.importing = false
		if err == nil {
			s.url = ""
		}
	})
}

// profileTile is a profile: what it is, how much of it is used, and what
// can be done with it.
func (m *mainUI) profileTile(c *ui.Context, pr *profiles.Profile) {
	s, p, a := &m.profs, m.pal, m.a
	current := pr.UID == m.pv.Current
	uid := pr.UID
	var used, total int64
	if pr.Usage != nil {
		used, total = pr.Usage.Upload+pr.Usage.Download, pr.Usage.Total
	}
	expired := pr.Usage != nil && pr.Usage.Expire != 0 && time.Unix(pr.Usage.Expire, 0).Before(time.Now())
	act := func(key, failure, success string, fn func() error) {
		s.busy[uid] = key
		m.run(failure, fn, func(err error) {
			delete(s.busy, uid)
			if err == nil && success != "" {
				m.notices = append(m.notices, Notice{Level: "success", Message: success})
			}
		})
	}
	update := func(viaProxy *bool) {
		act("update", locales.ProfilesUpdateFailed(), locales.ProfilesUpdated(), func() error {
			_, err := (Profiles{a}).Update(context.Background(), uid, viaProxy)
			return err
		})
	}
	activate := func() {
		if !current {
			act("activate", locales.ProfilesActivateFailed(), "", func() error { return Profiles{a}.Activate(context.Background(), uid) })
		}
	}
	card := ui.Column(c.Key(uid)).Grow(1).Padding(14).Gap(10).Radius(tileRadius).Background(p.Card)
	// The pointer over it lifts it a little.
	if card.Hovered() {
		card.Background(p.Surface2).Shadow(0, 4, 14, 0, ui.RGBA(0, 0, 0, 0.08))
	} else {
		card.Shadow(0, 0, 0, 0, ui.RGBA(0, 0, 0, 0))
	}
	card.Transition(hoverFade)
	if current {
		card.Background(p.AccentSoft.Over(p.Card))
	}
	if card.DoubleClicked() {
		activate()
	}
	card.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
				ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, pr.Name).FontSize(14).FontWeight(650).SingleLine().Shrink(1)
					if current {
						ui.Icon(c, icCheckCircle2).FontSize(15).TextColor(p.AccentFg)
					}
				})
				ui.Row(c).Gap(5).AlignItems(ui.Center).Wrap().Children(func() {
					if pr.Type == "remote" {
						m.badge(c, locales.ProfilesRemote(), "info")
					} else {
						m.badge(c, locales.ProfilesLocal(), "")
					}
					if pr.Converted {
						m.badge(c, locales.ProfilesConverted(), "")
					}
					if m.pv.DNS[uid] {
						m.badge(c, "DNS", "info")
					}
					if pr.Proxies > 0 {
						ui.Text(c, locales.ProfilesNodes(fmt.Sprint(pr.Proxies))).FontSize(11.5).TextColor(p.Faint)
					}
				})
			})
			more := m.iconButton(c, "more", icMoreHorizontal, "")
			more.PopoverMenu(func(menu *ui.Menu) {
				if menu.Item(locales.ProfilesEditInfo()).Chosen() {
					m.openProfileDialog(pr)
				}
				if menu.Item(locales.ProfilesEditFile()).Chosen() {
					m.openEditor(uid, pr.Name, false)
				}
				menu.Separator()
				for _, k := range []string{"rules", "proxies", "groups", "merge", "script"} {
					if menu.Item(m.T("profiles.ext." + k)).Chosen() {
						m.openExtension(pr, k)
					}
				}
				menu.Separator()
				if pr.Type == "remote" {
					if menu.Item(locales.ProfilesUpdate()).Chosen() {
						update(nil)
					}
					if menu.Item(locales.ProfilesUpdateViaProxy()).Chosen() {
						t := true
						update(&t)
					}
					if menu.Item(locales.ProfilesCopyUrl()).Chosen() {
						mygo.Clipboard.WriteText(pr.URL)
						c.Toast(locales.CommonCopied())
					}
					if pr.Prev != nil && menu.Item(locales.ProfilesRollback(fmtDate(pr.Prev.Updated))).Chosen() {
						m.confirm(locales.ProfilesRollbackTitle(), locales.ProfilesRollbackMsg(fmt.Sprint(pr.Prev.Proxies)), locales.ProfilesRollbackOk(), func() {
							act("update", locales.CommonFailed(), locales.ProfilesRolledBack(), func() error { _, err := (Profiles{a}).Rollback(context.Background(), uid); return err })
						})
					}
				}
				if pr.Home != "" && menu.Item(locales.ProfilesHome()).Chosen() {
					home := pr.Home
					go mygo.Shell.OpenExternal(home)
				}
				dns := m.pv.DNS[uid]
				label := locales.ProfilesDnsOn()
				if dns {
					label = locales.ProfilesDnsOff()
				}
				if menu.Item(label).Chosen() {
					set := func() {
						m.run(locales.CommonFailed(), func() error { return Profiles{a}.SetDNSOverride(context.Background(), uid, !dns) }, nil)
					}
					if !dns && (Profiles{a}).HasDNS(context.Background(), uid) {
						m.confirm(locales.ProfilesDnsWarnTitle(), locales.ProfilesDnsWarn(), locales.ProfilesDnsOn(), set)
					} else {
						set()
					}
				}
				if menu.Item(locales.ProfilesExport()).Chosen() {
					m.run(locales.CommonFailed(), func() error { _, err := (Profiles{a}).Export(context.Background(), uid); return err }, nil)
				}
				menu.Separator()
				if menu.Item(locales.CommonDelete()).Chosen() {
					msg := locales.ProfilesDeleteMsg(pr.Name)
					if current {
						msg += " " + locales.ProfilesDeleteCurrent()
					}
					m.confirm(locales.ProfilesDeleteTitle(), msg, locales.CommonDelete(), func() {
						m.run(locales.CommonFailed(), func() error { return Profiles{a}.Delete(context.Background(), uid) }, nil)
					})
				}
				if i := slices.IndexFunc(m.pv.Items, func(x profiles.Profile) bool { return x.UID == uid }); len(m.pv.Items) > 1 {
					menu.Separator()
					move := func(d int) {
						order := make([]string, len(m.pv.Items))
						for k, it := range m.pv.Items {
							order[k] = it.UID
						}
						order[i], order[i+d] = order[i+d], order[i]
						m.run(locales.CommonFailed(), func() error { return Profiles{a}.Reorder(context.Background(), order) }, nil)
					}
					if i > 0 && menu.Item(locales.ProfilesMoveUp()).Chosen() {
						move(-1)
					}
					if i < len(m.pv.Items)-1 && menu.Item(locales.ProfilesMoveDown()).Chosen() {
						move(1)
					}
				}
			})
		})
		if pr.Desc != "" {
			ui.Text(c, pr.Desc).FontSize(12).TextColor(p.TextMuted).SingleLine()
		}
		if total > 0 {
			pct := fmtPercent(used, total)
			if expired {
				pct = 100
			}
			m.progress(c, pct)
			ui.Row(c).Children(func() {
				ui.Text(c, fmtBytes(used)+" / "+fmtBytes(total)).FontSize(11.5).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll)
				ui.Spacer(c)
				if pr.Usage.Expire != 0 {
					if expired {
						ui.Text(c, locales.ProfilesExpired()).FontSize(11.5).TextColor(p.Danger)
					} else {
						ui.Text(c, locales.ProfilesExpires(fmtDate(pr.Usage.Expire))).FontSize(11.5).TextColor(p.TextMuted)
					}
				}
			})
		}
		if pr.LastError != "" {
			ui.Row(c).Padding(6, 8).Gap(8).Radius(6).Background(p.DangerSoft).AlignItems(ui.Center).Children(func() {
				txt := ui.ButtonBase(c.Key("err")).Grow(1).Basis(0).MinWidth(0)
				pressable(txt)
				txt.Children(func() {
					t := ui.Text(c, locales.ProfilesLastError()+": "+pr.LastError).FontSize(12).TextColor(p.Danger)
					if s.errOpen[uid] {
						t.Wrap().Selectable()
					} else {
						t.SingleLine().Tooltip(pr.LastError)
					}
				})
				if txt.Clicked() {
					s.errOpen[uid] = !s.errOpen[uid]
				}
				if s.busy[uid] == "update" {
					ui.Spinner(c)
				} else if m.button(c, "retry", locales.ProfilesRetry(), nil, "ghost").Height(24).Clicked() {
					update(nil)
				}
			})
		}
		ui.Spacer(c)
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			when := m.relTime(time.Unix(pr.Updated, 0))
			if pr.Type != "remote" {
				when = locales.ProfilesEditedAgo(when)
			}
			if next, ok := m.pv.NextUpdates[uid]; ok && !next.IsZero() {
				when += " · " + locales.ProfilesNextShort(m.relTime(next))
			}
			ui.Text(c, when).FontSize(11.5).TextColor(p.Faint).SingleLine().Grow(1).Basis(0)
			if pr.Type == "remote" && m.spinButton(c, "update", icRefreshCw, locales.ProfilesUpdate(), s.busy[uid] == "update").Clicked() {
				update(nil)
			}
			switch {
			case current:
				m.badge(c, locales.ProfilesInUse(), "info")
			case s.busy[uid] == "activate":
				ui.Spinner(c)
			default:
				if m.button(c, "use", locales.ProfilesUse(), nil, "").Height(26).Clicked() {
					activate()
				}
			}
		})
	})
}

// ---- The profile dialog ----

func (m *mainUI) openProfileDialog(pr *profiles.Profile) {
	s := &m.profs
	s.dialog, s.dlgBusy = true, false
	if pr == nil {
		s.dlgUID, s.dlgType, s.dlgName, s.dlgDesc, s.dlgURL, s.dlgOpt, s.dlgSuggst = "", 0, "", "", "", profiles.Option{}, 0
	} else {
		s.dlgUID, s.dlgName, s.dlgDesc, s.dlgURL, s.dlgOpt, s.dlgSuggst = pr.UID, pr.Name, pr.Desc, pr.URL, pr.Option, pr.Suggested
		s.dlgType = 0
		if pr.Type != "remote" {
			s.dlgType = 1
		}
	}
	s.dlgIntvl = strconv.Itoa(s.dlgOpt.UpdateInterval)
	s.dlgTO = strconv.Itoa(cmp.Or(s.dlgOpt.TimeoutSeconds, 60))
}

// field is a labelled control, with a hint or an error under it.
func (m *mainUI) field(c *ui.Context, label, hint, errText string, control func()) {
	p := m.pal
	ui.Column(c).Gap(5).Grow(1).Basis(0).MinWidth(0).Children(func() {
		if label != "" {
			ui.Text(c, label).FontSize(12.5).FontWeight(550).TextColor(p.Text2)
		}
		control()
		// A mistake shows in place of the hint, fading across.
		switch {
		case errText != "":
			ui.Text(c.Key("err"), errText).FontSize(11.5).TextColor(p.Danger).Wrap().Transition(noticeIn)
		case hint != "":
			ui.Text(c.Key("hint"), hint).FontSize(11.5).TextColor(p.TextMuted).Wrap().Transition(noticeIn)
		}
	})
}

// setting is a line of a list of settings: what it is, what it does, and
// its control at the end.
func (m *mainUI) setting(c *ui.Context, key any, label, desc string, control func()) {
	p := m.pal
	ui.Row(c.Key(key)).Padding(10, 14).Gap(14).MinHeight(48).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, label).FontSize(13).FontWeight(550)
			if desc != "" {
				ui.Text(c, desc).FontSize(12).TextColor(p.TextMuted).Wrap()
			}
		})
		control()
	})
}

// rows is a list of settings in a box, a hairline between them.
func (m *mainUI) rows(c *ui.Context, key any, fn func()) {
	p := m.pal
	ui.Column(c.Key(key)).Radius(tileRadius).Background(p.Card).Dividers(1, p.Border.Alpha(0.6)).Children(fn)
}

func (m *mainUI) profileDialog(c *ui.Context) {
	s, p, a := &m.profs, m.pal, m.a
	if !s.dialog {
		return
	}
	editing := s.dlgUID != ""
	remote := s.dlgType == 0
	u := strings.TrimSpace(s.dlgURL)
	validURL := !remote || (strings.HasPrefix(strings.ToLower(u), "http://") || strings.HasPrefix(strings.ToLower(u), "https://")) && !strings.ContainsAny(u, " \t\n")
	m.modal(c, "profile-dialog", &s.dialog, func() {
		ui.Column(c).Width(560).Gap(14).Children(func() {
			title := locales.ProfilesNew()
			if editing {
				title = locales.ProfilesEditInfo()
			}
			ui.Text(c, title).FontSize(15).FontWeight(650)
			if !editing {
				m.segmentedOf(c, "type", &s.dlgType, true, locales.ProfilesRemote(), locales.ProfilesLocal())
			}
			ui.Row(c).Gap(12).Children(func() {
				hint, ph := "", ""
				if remote && !editing {
					hint, ph = locales.ProfilesNameHint(), locales.ProfilesNameAuto()
				}
				m.field(c, locales.ProfilesName(), hint, "", func() { ui.TextInput(c.Key("name"), &s.dlgName).Placeholder(ph) })
				m.field(c, locales.ProfilesDesc(), "", "", func() { ui.TextInput(c.Key("desc"), &s.dlgDesc) })
			})
			// The ways to fetch a remote profile fold in and away as the
			// type changes, the dialog following.
			m.collapse(c, "profile-remote", remote, func() {
				errText := ""
				if u != "" && !validURL {
					errText = locales.ProfilesBadUrl()
				}
				m.field(c, locales.ProfilesUrl(), "", errText, func() {
					ui.TextArea(c.Key("url"), &s.dlgURL).Placeholder("https://").Font("ui-monospace").Height(64)
				})
				ui.Row(c).Gap(12).Children(func() {
					m.field(c, locales.ProfilesUserAgent(), locales.ProfilesUserAgentHint(), "", func() {
						ui.TextInput(c.Key("ua"), &s.dlgOpt.UserAgent).Placeholder("mihomo/1.19 clash-verge/v2.4 MyGO-Clash")
					})
					hint := locales.ProfilesIntervalHint()
					if s.dlgIntvl == "0" && s.dlgSuggst > 0 {
						hint = locales.ProfilesIntervalFollow(fmt.Sprint(s.dlgSuggst))
					}
					m.field(c, locales.ProfilesInterval(), hint, "", func() {
						ui.TextInput(c.Key("interval"), &s.dlgIntvl).Disabled(s.dlgOpt.NoAutoUpdate).Width(140)
					})
				})
				ui.Row(c).Gap(12).Children(func() {
					m.field(c, locales.ProfilesTimeout(), "", "", func() { ui.TextInput(c.Key("timeout"), &s.dlgTO).Width(140) })
					ui.Box(c).Grow(1).Basis(0)
				})
				m.rows(c, "opts", func() {
					m.setting(c, "proxy", locales.ProfilesWithProxy(), locales.ProfilesWithProxyHint(), func() { ui.Switch(c, &s.dlgOpt.WithProxy) })
					auto := !s.dlgOpt.NoAutoUpdate
					m.setting(c, "auto", locales.ProfilesAutoUpdate(), locales.ProfilesAutoUpdateHint(), func() {
						if ui.Switch(c, &auto).Changed() {
							s.dlgOpt.NoAutoUpdate = !auto
						}
					})
					ui.Row(c.Key("insecure")).Padding(10, 14).Gap(14).AlignItems(ui.Center).Children(func() {
						ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
							ui.Text(c, locales.ProfilesInsecure()).FontSize(13).FontWeight(550)
							ui.Text(c, locales.ProfilesInsecureHint()).FontSize(12).TextColor(p.Warning).Wrap()
						})
						ui.Switch(c, &s.dlgOpt.Insecure)
					})
				})
			})
		})
		ui.Row(c).Gap(8).Justify(ui.End).PaddingY(4).Children(func() {
			if m.button(c, "cancel", locales.CommonCancel(), nil, "").Clicked() {
				s.dialog = false
			}
			label := locales.CommonCreate()
			switch {
			case editing:
				label = locales.CommonSave()
			case remote:
				label = locales.ProfilesImport()
			}
			if s.dlgBusy {
				ui.Spinner(c)
			} else if m.button(c, "save", label, nil, "primary").Disabled(!validURL).Clicked() {
				opt := s.dlgOpt
				opt.UpdateInterval, _ = strconv.Atoi(strings.TrimSpace(s.dlgIntvl))
				opt.TimeoutSeconds, _ = strconv.Atoi(strings.TrimSpace(s.dlgTO))
				name, desc, url, uid := s.dlgName, s.dlgDesc, s.dlgURL, s.dlgUID
				typ := "remote"
				if !remote {
					typ = "local"
				}
				failure := locales.ProfilesImportFailed()
				if editing {
					failure = locales.ProfilesSaveFailed()
				}
				s.dlgBusy = true
				m.run(failure, func() error {
					if uid != "" {
						patch := profiles.Patch{Name: &name, Desc: &desc, Option: &opt}
						if typ == "remote" {
							patch.URL = &url
						}
						_, err := (Profiles{a}).Patch(context.Background(), uid, patch)
						return err
					}
					_, err := (Profiles{a}).Create(context.Background(), profiles.NewProfile{Type: typ, Name: name, Desc: desc, URL: url, Option: opt})
					return err
				}, func(err error) {
					s.dlgBusy = false
					if err == nil {
						s.dialog = false
					}
				})
			}
		})
	})
}

// ---- The text editor ----

func (m *mainUI) openEditor(uid, name string, readOnly bool) {
	s, a := &m.profs, m.a
	s.edit, s.editUID, s.editName, s.editRO, s.code, s.codeLogs = true, uid, name, readOnly, nil, nil
	win := m.window()
	go func() {
		text, err := (Profiles{a}).Content(context.Background(), uid)
		logs, _ := (Profiles{a}).Logs(context.Background())
		win.Update(func() {
			if err != nil {
				s.edit = false
				m.notices = append(m.notices, Notice{Level: "error", Message: locales.ProfilesReadFailed(), Detail: err.Error()})
				return
			}
			s.code = newCode(text, langOf(uid, text), readOnly)
			s.codeLogs = logs[uid]
		})
	}()
}

// langOf guesses whether an item is a script, by its UID or what it holds.
func langOf(uid, text string) string {
	if uid == "Script" || strings.Contains(text, "function main") {
		return "script.js"
	}
	return "config.yaml"
}

func newCode(text, file string, readOnly bool) *editor.State {
	buf := editor.NewBuffer(text)
	buf.SetLanguage(editor.LanguageFor(file))
	return &editor.State{Buffer: buf, ReadOnly: readOnly, TabSize: 2, SoftWrap: true}
}

func (m *mainUI) editorDialog(c *ui.Context) {
	s, p, a := &m.profs, m.pal, m.a
	if !s.edit {
		return
	}
	dirty := s.code != nil && s.code.Buffer.Modified()
	save := func() {
		if s.code == nil || !dirty || s.codeBusy {
			return
		}
		s.codeBusy = true
		uid, text, buf := s.editUID, s.code.Buffer.String(), s.code.Buffer
		win := m.window()
		go func() {
			err := Profiles{a}.SaveContent(context.Background(), uid, text)
			logs, _ := (Profiles{a}).Logs(context.Background())
			win.Update(func() {
				s.codeBusy = false
				if err != nil {
					m.notices = append(m.notices, Notice{Level: "error", Message: locales.ProfilesSaveFailed(), Detail: err.Error()})
					return
				}
				buf.MarkSaved()
				s.codeLogs = logs[uid]
				m.notices = append(m.notices, Notice{Level: "success", Message: locales.ProfilesSaved()})
			})
		}()
	}
	closeIt := func() {
		if dirty {
			m.confirm(locales.ProfilesDiscardTitle(), locales.ProfilesDiscard(), locales.ProfilesDiscardOk(), func() { s.edit = false })
			return
		}
		s.edit = false
	}
	open := true
	modal := m.modal(c, "editor", &open, func() {
		ui.Column(c).Width(900).Height(600).Gap(10).Children(func() {
			ui.Text(c, s.editName).FontSize(15).FontWeight(650).SingleLine()
			box := ui.Column(c).Grow(1).Basis(0).Radius(8).Border(1, p.Border).Clip()
			box.Children(func() {
				if s.code == nil {
					ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Spinner(c) })
					return
				}
				editor.View(c.Key(s.editUID), s.code).Grow(1).FillWidth().AutoFocus()
			})
			if len(s.codeLogs) > 0 {
				ui.Scroll(c).MaxHeight(130).Gap(4).Children(func() { m.enhanceLog(c, s.codeLogs) })
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				hint := "⌘S " + locales.CommonSave()
				switch {
				case s.editRO:
					hint = locales.ProfilesReadOnly()
				case dirty:
					hint = locales.ProfilesUnsaved()
				}
				ui.Text(c, hint).FontSize(12).TextColor(p.TextMuted).Grow(1)
				if m.button(c, "close", locales.CommonClose(), nil, "").Clicked() {
					closeIt()
				}
				if !s.editRO {
					if s.codeBusy {
						ui.Spinner(c)
					} else if m.button(c, "save", locales.CommonSave(), nil, "primary").Disabled(!dirty).Clicked() {
						save()
					}
				}
			})
		})
	})
	c.OnShortcut(ui.Cmd, ui.KeyS, save)
	if modal.Dismissed() || !open {
		closeIt()
	}
}

// enhanceLog lists what an extension left in its log.
func (m *mainUI) enhanceLog(c *ui.Context, logs []enhance.LogEntry) {
	for i, l := range logs {
		tone := "info"
		switch l.Level {
		case "exception", "error":
			tone = "danger"
		case "warn":
			tone = "warning"
		}
		ui.Row(c.Key(i)).Gap(8).AlignItems(ui.Start).Children(func() {
			m.badge(c, l.Level, tone)
			ui.Text(c, l.Message).FontSize(12).Font("ui-monospace").Wrap().Selectable().Grow(1).Basis(0)
		})
	}
}

// ---- Extensions ----

func (m *mainUI) openExtension(pr *profiles.Profile, kind string) {
	m.openExt(pr.UID, pr.UID, m.T("profiles.ext."+kind)+" · "+pr.Name, kind, nil)
}

// openExt opens the kind extension of owner (a profile's UID, or "Rules"
// for the quick rules), whose names are prof's; draft, if any, fills the
// form of a new rule.
func (m *mainUI) openExt(owner, prof, title, kind string, draft *[2]string) {
	s, a := &m.profs, m.a
	win := m.window()
	go func() {
		uid, err := (Profiles{a}).Extension(context.Background(), owner, kind)
		if err != nil {
			m.notice(Notice{Level: "error", Message: locales.CommonFailed(), Detail: err.Error()})
			return
		}
		if kind == "merge" || kind == "script" {
			win.Update(func() { m.openEditor(uid, title, false) })
			return
		}
		seq, err1 := (Profiles{a}).Seq(context.Background(), uid)
		names, err2 := (Profiles{a}).Names(context.Background(), prof)
		var own []string
		var err3 error
		switch kind {
		case "rules":
			own, err3 = (Profiles{a}).RulesOf(context.Background(), prof)
		case "proxies":
			own = names.Proxies
		default:
			own = names.Groups
		}
		if err := cmp.Or(err1, err2, err3); err != nil {
			m.notice(Notice{Level: "error", Message: locales.ProfilesReadFailed(), Detail: err.Error()})
			return
		}
		orig, _ := json.Marshal(seq)
		win.Update(func() {
			s.seq, s.seqUID, s.seqProf, s.seqKind, s.seqName = true, uid, prof, kind, title
			s.seqPatch, s.seqOrig, s.seqNames, s.seqOwn, s.seqTab, s.seqQ = &seq, string(orig), names, own, 0, ""
			s.rule = [4]string{"DOMAIN-SUFFIX", "", "DIRECT", ""}
			if draft != nil {
				s.rule[0], s.rule[1] = draft[0], draft[1]
			}
			s.group = [4]string{"", "select", "https://www.gstatic.com/generate_204", ""}
			s.links, s.members = "", nil
		})
	}()
}

func seqLabel(it any) string {
	switch v := it.(type) {
	case string:
		return v
	case map[string]any:
		if n, ok := v["name"]; ok {
			return fmt.Sprint(n)
		}
		b, _ := json.Marshal(v)
		return string(b)
	}
	return fmt.Sprint(it)
}

func seqSub(it any) string {
	o, ok := it.(map[string]any)
	if !ok {
		return ""
	}
	var parts []string
	if t, ok := o["type"]; ok {
		parts = append(parts, fmt.Sprint(t))
	}
	if sv, ok := o["server"]; ok {
		parts = append(parts, fmt.Sprintf("%v:%v", sv, cmp.Or(fmt.Sprint(o["port"]), "")))
	}
	if ps, ok := o["proxies"].([]any); ok {
		parts = append(parts, fmt.Sprintf("%d members", len(ps)))
	}
	return strings.Join(parts, " · ")
}

func (m *mainUI) seqDialog(c *ui.Context) {
	s, p, a := &m.profs, m.pal, m.a
	if !s.seq || s.seqPatch == nil {
		return
	}
	pt := s.seqPatch
	cur, _ := json.Marshal(pt)
	dirty := string(cur) != s.seqOrig
	leave := func(then func()) {
		if dirty {
			m.confirm(locales.ProfilesDiscardTitle(), locales.ProfilesDiscard(), locales.ProfilesDiscardOk(), then)
			return
		}
		then()
	}
	list := func() *[]any {
		if s.seqTab == 1 {
			return &pt.Append
		}
		return &pt.Prepend
	}
	add := func(items ...any) { l := list(); *l = append(*l, items...) }
	open := true
	modal := m.modal(c, "seq", &open, func() {
		ui.Column(c).Width(640).MaxHeight(640).Gap(12).Children(func() {
			ui.Text(c, s.seqName).FontSize(15).FontWeight(650).SingleLine()
			m.tabs(c, "tabs", &s.seqTab, fmt.Sprintf("%s (%d)", locales.EditorPrepend(), len(pt.Prepend)),
				fmt.Sprintf("%s (%d)", locales.EditorAppend(), len(pt.Append)), fmt.Sprintf("%s (%d)", locales.EditorDelete(), len(pt.Delete)))
			ui.Text(c, m.T("editor."+[]string{"prepend", "append", "delete"}[s.seqTab]+"Hint")).FontSize(12).TextColor(p.TextMuted).Wrap().ContentTransition(ui.ContentTransition{})
			// What deletes is another form than what adds: it slides in.
			formIdx := 0
			if s.seqTab == 2 {
				formIdx = 1
			}
			m.switchPane(c, "seq-form", formIdx, 0, func() {
				form := ui.Column(c).Padding(12).Gap(8).Radius(14).Background(p.Surface2)
				switch {
				case s.seqTab == 2:
					form.Children(func() {
						ui.SearchField(c.Key("q"), &s.seqQ).Placeholder(locales.CommonSearch())
						ui.Scroll(c).MaxHeight(200).Children(func() {
							q := strings.ToLower(s.seqQ)
							n := 0
							for _, name := range s.seqOwn {
								if q != "" && !strings.Contains(strings.ToLower(name), q) {
									continue
								}
								n++
								on := slices.Contains(pt.Delete, name)
								if ui.Checkbox(c.Key(name), &on, name).Changed() {
									if on {
										pt.Delete = append(pt.Delete, name)
									} else {
										pt.Delete = slices.DeleteFunc(pt.Delete, func(x string) bool { return x == name })
									}
								}
							}
							if n == 0 {
								ui.Text(c, locales.EditorNothing()).TextColor(p.TextMuted).Padding(8)
							}
						})
					})
				case s.seqKind == "rules":
					form.Children(func() { m.ruleForm(c, add) })
				case s.seqKind == "proxies":
					form.Children(func() {
						ui.TextArea(c.Key("links"), &s.links).Placeholder(locales.EditorLinksHint()).Font("ui-monospace").Height(80)
						ui.Row(c).Justify(ui.End).Children(func() {
							if m.button(c, "add", locales.EditorAddLinks(), icPlus, "primary").Disabled(strings.TrimSpace(s.links) == "").Clicked() {
								items, err := (Profiles{a}).ParseLinks(s.links)
								if err != nil {
									m.notices = append(m.notices, Notice{Level: "error", Message: locales.EditorParseFailed(), Detail: err.Error()})
									return
								}
								for _, it := range items {
									add(it)
								}
								s.links = ""
								m.notices = append(m.notices, Notice{Level: "success", Message: locales.EditorParsed(fmt.Sprint(len(items)))})
							}
						})
					})
				default:
					form.Children(func() { m.groupForm(c, add) })
				}
			})
			ui.Scroll(c.Key("items")).Grow(1).Basis(0).MinHeight(120).Radius(10).Border(1, p.Border).Children(func() {
				// Each tab's items slide in from its side.
				m.switchPane(c, "seq-items", s.seqTab, 0, func() {
					var items []any
					if s.seqTab == 2 {
						for _, d := range pt.Delete {
							items = append(items, d)
						}
					} else {
						items = *list()
					}
					if len(items) == 0 {
						m.empty(c, locales.EditorEmpty(), nil)
						return
					}
					for i, it := range items {
						ui.Row(c.Key(i)).Padding(6, 10).Gap(8).AlignItems(ui.Center).BorderWidth(0, 0, 1, 0).BorderColor(p.Border.Alpha(0.6)).Children(func() {
							m.badge(c, strconv.Itoa(i+1), "")
							ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
								ui.Text(c, seqLabel(it)).FontSize(12).Font("ui-monospace").SingleLine()
								if sub := seqSub(it); sub != "" {
									ui.Text(c, sub).FontSize(11).TextColor(p.Faint).SingleLine()
								}
							})
							if s.seqTab != 2 {
								l := list()
								if m.iconButton(c, "up", icArrowUp, "").Disabled(i == 0).Clicked() {
									(*l)[i], (*l)[i-1] = (*l)[i-1], (*l)[i]
								}
								if m.iconButton(c, "down", icArrowDown, "").Disabled(i == len(items)-1).Clicked() {
									(*l)[i], (*l)[i+1] = (*l)[i+1], (*l)[i]
								}
							}
							if m.iconButton(c, "del", icTrash2, "").Clicked() {
								if s.seqTab == 2 {
									pt.Delete = slices.Delete(pt.Delete, i, i+1)
								} else {
									l := list()
									*l = slices.Delete(*l, i, i+1)
								}
							}
						})
					}
				})
			})
			if s.seqKind == "rules" && s.seqTab != 2 {
				ui.Text(c, locales.EditorRuleOrder()).FontSize(11.5).TextColor(p.TextMuted).Wrap()
			}
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if m.button(c, "yaml", locales.EditorEditYaml(), icCode2, "ghost").Clicked() {
					uid, name := s.seqUID, s.seqName
					leave(func() {
						s.seq = false
						m.openEditor(uid, name, false)
					})
				}
				ui.Spacer(c)
				if m.button(c, "cancel", locales.CommonCancel(), nil, "").Clicked() {
					leave(func() { s.seq = false })
				}
				if s.seqBusy {
					ui.Spinner(c)
				} else if m.button(c, "save", locales.CommonSave(), nil, "primary").Clicked() {
					s.seqBusy = true
					uid, patch := s.seqUID, *pt
					m.run(locales.ProfilesSaveFailed(), func() error { return Profiles{a}.SaveSeq(context.Background(), uid, patch) }, func(err error) {
						s.seqBusy = false
						if err == nil {
							s.seq = false
							m.notices = append(m.notices, Notice{Level: "success", Message: locales.ProfilesSaved()})
						}
					})
				}
			})
		})
	})
	if modal.Dismissed() || !open {
		leave(func() { s.seq = false })
	}
}

// ruleForm adds a rule: its type, what it matches, and where it sends it.
func (m *mainUI) ruleForm(c *ui.Context, add func(...any)) {
	s := &m.profs
	targets := slices.Concat(s.seqNames.Groups, s.seqNames.Builtin, s.seqNames.Proxies)
	match := s.rule[0] == "MATCH"
	submit := func() {
		parts := []string{s.rule[0], strings.TrimSpace(s.rule[1]), s.rule[2]}
		if match {
			parts = []string{s.rule[0], s.rule[2]}
		}
		if s.rule[3] == "1" && slices.Contains(noResolveTypes, s.rule[0]) {
			parts = append(parts, "no-resolve")
		}
		add(strings.Join(parts, ","))
		s.rule[1] = ""
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
		ui.Select(c.Key("type"), &s.rule[0], ruleTypes).Width(170)
		if !match {
			if ui.TextInput(c.Key("payload"), &s.rule[1]).Placeholder(locales.EditorPayload()).Grow(1).MinWidth(160).Submitted() && strings.TrimSpace(s.rule[1]) != "" {
				submit()
			}
		}
		ui.Select(c.Key("target"), &s.rule[2], targets).Width(170)
		if slices.Contains(noResolveTypes, s.rule[0]) {
			on := s.rule[3] == "1"
			if ui.Checkbox(c.Key("nr"), &on, "no-resolve").Changed() {
				s.rule[3] = map[bool]string{true: "1", false: ""}[on]
			}
		}
		if m.button(c, "add", locales.CommonAdd(), icPlus, "primary").Disabled(!match && strings.TrimSpace(s.rule[1]) == "").Clicked() {
			submit()
		}
	})
}

// groupForm adds a proxy group of the members chosen.
func (m *mainUI) groupForm(c *ui.Context, add func(...any)) {
	s, p := &m.profs, m.pal
	tests := s.group[1] == "url-test" || s.group[1] == "fallback" || s.group[1] == "load-balance"
	ui.Row(c).Gap(8).Children(func() {
		ui.TextInput(c.Key("name"), &s.group[0]).Placeholder(locales.EditorGroupName()).Grow(1)
		ui.Select(c.Key("kind"), &s.group[1], groupKinds).Width(150)
	})
	if tests {
		ui.TextInput(c.Key("url"), &s.group[2]).Placeholder("https://www.gstatic.com/generate_204")
	}
	ui.SearchField(c.Key("filter"), &s.group[3]).Placeholder(locales.EditorMembers()).Width(260)
	all := slices.Concat(s.seqNames.Builtin[:min(2, len(s.seqNames.Builtin))], s.seqNames.Groups, s.seqNames.Proxies)
	ui.Scroll(c).MaxHeight(140).Children(func() {
		ui.Row(c).Wrap().Gap(6).Children(func() {
			for _, n := range all {
				if f := strings.ToLower(s.group[3]); f != "" && !strings.Contains(strings.ToLower(n), f) {
					continue
				}
				on := slices.Contains(s.members, n)
				chip := ui.ButtonBase(c.Key(n)).Height(26).Padding(0, 10).Radius(13).AlignItems(ui.Center).Border(1, p.Border)
				pressable(chip)
				if on {
					chip.Background(p.AccentSoft).Border(1, p.Accent.Alpha(0.5))
				} else if chip.Hovered() {
					chip.Background(p.SurfaceHover)
				}
				chip.Children(func() { ui.Text(c, n).FontSize(12).SingleLine() })
				if chip.Clicked() {
					if on {
						s.members = slices.DeleteFunc(s.members, func(x string) bool { return x == n })
					} else {
						s.members = append(s.members, n)
					}
				}
			}
		})
	})
	ui.Row(c).Justify(ui.End).Children(func() {
		if m.button(c, "add", locales.EditorAddGroup(fmt.Sprint(len(s.members))), icPlus, "primary").Disabled(strings.TrimSpace(s.group[0]) == "" || len(s.members) == 0).Clicked() {
			g := map[string]any{"name": strings.TrimSpace(s.group[0]), "type": s.group[1], "proxies": slices.Clone(s.members)}
			if tests {
				g["url"], g["interval"] = s.group[2], 300
			}
			add(g)
			s.group[0], s.members = "", nil
		}
	})
}

// ---- The configuration that runs, the logs of extensions, Sub-Store ----

func (m *mainUI) openRuntime() {
	s, a := &m.profs, m.a
	s.runtime, s.code = true, nil
	win := m.window()
	go func() {
		text, err := (Profiles{a}).RuntimeYAML(context.Background())
		win.Update(func() {
			if err != nil {
				s.runtime = false
				m.notices = append(m.notices, Notice{Level: "error", Message: locales.ProfilesReadFailed(), Detail: err.Error()})
				return
			}
			s.code = newCode(text, "runtime.yaml", true)
		})
	}()
}

func (m *mainUI) runtimeDialog(c *ui.Context) {
	s, p := &m.profs, m.pal
	if !s.runtime {
		return
	}
	m.modal(c, "runtime", &s.runtime, func() {
		ui.Column(c).Width(900).Height(600).Gap(10).Children(func() {
			ui.Text(c, locales.ProfilesRuntime()).FontSize(15).FontWeight(650)
			ui.Column(c).Grow(1).Basis(0).Radius(8).Border(1, p.Border).Clip().Children(func() {
				if s.code == nil {
					ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Children(func() { ui.Spinner(c) })
					return
				}
				editor.View(c.Key("rt"), s.code).Grow(1).FillWidth()
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				if m.button(c, "copy", locales.CommonCopy(), icCopy, "").Disabled(s.code == nil).Clicked() {
					mygo.Clipboard.WriteText(s.code.Buffer.String())
					c.Toast(locales.CommonCopied())
				}
			})
		})
	})
}

func (m *mainUI) enhanceLogsDialog(c *ui.Context) {
	s, p := &m.profs, m.pal
	if !s.logs {
		return
	}
	names := map[string]string{"Merge": locales.ProfilesGlobalMerge(), "Script": locales.ProfilesGlobalScript()}
	for _, it := range m.pv.Items {
		names[it.UID] = it.Name
	}
	m.modal(c, "elogs", &s.logs, func() {
		ui.Column(c).Width(640).MaxHeight(560).Gap(12).Children(func() {
			ui.Text(c, locales.ProfilesEnhanceLogs()).FontSize(15).FontWeight(650)
			if s.logsData == nil {
				ui.Spinner(c)
				return
			}
			uids := make([]string, 0, len(s.logsData))
			for uid, l := range s.logsData {
				if len(l) > 0 {
					uids = append(uids, uid)
				}
			}
			slices.Sort(uids)
			if len(uids) == 0 {
				m.empty(c, locales.ProfilesNoLogs(), nil)
				return
			}
			ui.Scroll(c).Grow(1).Gap(14).Children(func() {
				for _, uid := range uids {
					ui.Column(c.Key(uid)).Gap(6).Children(func() {
						ui.Text(c, cmp.Or(names[uid], uid)).FontSize(12).FontWeight(600).TextColor(p.TextMuted)
						m.enhanceLog(c, s.logsData[uid])
					})
				}
			})
		})
	})
}

func (m *mainUI) loadSubStore() {
	s, a := &m.profs, m.a
	base := strings.TrimSpace(s.subBase)
	if base == "" {
		return
	}
	s.subBusy = true
	win := m.window()
	go func() {
		items, err := (Profiles{a}).SubStore(context.Background(), base)
		win.Update(func() {
			s.subBusy = false
			if err != nil {
				m.notices = append(m.notices, Notice{Level: "error", Message: locales.SubstoreFailed(), Detail: err.Error()})
				return
			}
			if items == nil {
				items = []profiles.SubStoreItem{}
			}
			s.subItems = items
		})
	}()
}

func (m *mainUI) subStoreDialog(c *ui.Context) {
	s, p, a := &m.profs, m.pal, m.a
	if !s.sub {
		return
	}
	known := map[string]bool{}
	for _, it := range m.pv.Items {
		known[it.URL] = true
	}
	m.modal(c, "substore", &s.sub, func() {
		ui.Column(c).Width(520).MaxHeight(560).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icBoxes).FontSize(16).TextColor(p.TextMuted)
				ui.Text(c, "Sub-Store").FontSize(15).FontWeight(650)
			})
			ui.Row(c).Gap(8).Children(func() {
				if ui.TextInput(c.Key("base"), &s.subBase).Placeholder("http://127.0.0.1:3000").Font("ui-monospace").Grow(1).Submitted() {
					m.loadSubStore()
				}
				if s.subBusy {
					ui.Spinner(c)
				} else if m.button(c, "connect", locales.SubstoreConnect(), nil, "primary").Disabled(strings.TrimSpace(s.subBase) == "").Clicked() {
					m.loadSubStore()
				}
			})
			switch {
			case s.subItems == nil:
				ui.Text(c, locales.SubstoreHint()).FontSize(12.5).TextColor(p.TextMuted).Wrap()
			case len(s.subItems) == 0:
				m.empty(c, locales.SubstoreEmpty(), nil)
			default:
				ui.Scroll(c).Grow(1).Children(func() {
					m.rows(c, "items", func() {
						for _, it := range s.subItems {
							kind := locales.SubstoreSub()
							if it.Kind == "collection" {
								kind = locales.SubstoreCollection()
							}
							if it.Remark != "" {
								kind += " · " + it.Remark
							}
							m.setting(c, it.URL, cmp.Or(it.DisplayName, it.Name), kind, func() {
								switch {
								case known[it.URL]:
									m.button(c, "added", locales.SubstoreAdded(), icCheck, "").Disabled(true)
								case s.subAdd == it.URL:
									ui.Spinner(c)
								default:
									if m.button(c, "add", locales.CommonAdd(), icPlus, "").Clicked() {
										s.subAdd = it.URL
										item := it
										m.run(locales.ProfilesImportFailed(), func() error {
											pr, err := (Profiles{a}).Create(context.Background(), profiles.NewProfile{Type: "remote", URL: item.URL, Name: cmp.Or(item.DisplayName, item.Name), Desc: "Sub-Store"})
											if err == nil {
												m.notice(Notice{Level: "success", Message: locales.ProfilesImported(), Detail: pr.Name})
											}
											return err
										}, func(error) { s.subAdd = "" })
									}
								}
							})
						}
					})
				})
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				if m.button(c, "close", locales.CommonClose(), nil, "").Clicked() {
					s.sub = false
				}
			})
		})
	})
}
