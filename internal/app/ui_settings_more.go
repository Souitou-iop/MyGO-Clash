package app

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/cloudsync"
)

// syncPage is what the sync tab keeps between frames.
type syncPage struct {
	devices    []DeviceInfo
	acts       []SyncActivity
	hist       []cloudsync.Event
	backups    []cloudsync.BackupInfo
	backupsErr string
	loadedFor  string
	busy       string
	from       string // the device whose backups show
	scope      int    // the activity of all devices, or of this one
	who        string
	more       bool

	// The setup.
	setup   bool
	step    int
	su      SyncSetup
	probe   *SyncProbe
	pass2   string
	suBusy  bool
	removed []DeviceInfo
	revoke  bool
	rmPass  string
	rmPass2 string

	// A question that wants a word: a password, a name.
	asking   bool
	askTitle string
	askLabel string
	askText  string
	askPass  bool
	askDo    func(string)
}

// ask asks for a word in a dialog, a password when secret.
func (m *mainUI) ask(title, label, value string, secret bool, do func(string)) {
	s := &m.syncPg
	s.asking, s.askTitle, s.askLabel, s.askText, s.askPass, s.askDo = true, title, label, value, secret, do
}

func (m *mainUI) askDialog(c *ui.Context) {
	s := &m.syncPg
	if !s.asking {
		return
	}
	m.modal(c, "ask", &s.asking, func() {
		ui.Column(c).Width(400).Gap(12).Children(func() {
			ui.Text(c, s.askTitle).FontSize(15).FontWeight(650)
			submit := func() {
				s.asking = false
				if s.askDo != nil {
					s.askDo(s.askText)
				}
			}
			m.field(c, s.askLabel, "", "", func() {
				in := ui.TextInput(c.Key("in"), &s.askText).AutoFocus()
				if s.askPass {
					in.Password()
				}
				if in.Submitted() {
					submit()
				}
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if m.button(c, "cancel", locales.CommonCancel(), nil, "").Clicked() {
					s.asking = false
				}
				if m.button(c, "ok", locales.CommonOk(), nil, "primary").Clicked() {
					submit()
				}
			})
		})
	})
}

// strength scores a passphrase from 0 to 4.
func strength(p string) int {
	if len(p) < 8 {
		return 0
	}
	score := 1
	if len(p) >= 12 {
		score = 2
	}
	lower, upper, digit, other := false, false, false, false
	for _, r := range p {
		switch {
		case r >= 'a' && r <= 'z':
			lower = true
		case r >= 'A' && r <= 'Z':
			upper = true
		case r >= '0' && r <= '9':
			digit = true
		case r != '_':
			other = true
		}
	}
	if lower && upper {
		score++
	}
	if digit && other {
		score++
	}
	if len(p) >= 20 {
		score++
	}
	return min(4, score)
}

func (m *mainUI) loadSync() {
	s, a := &m.syncPg, m.a
	key := fmt.Sprint(m.sy.Configured, m.sy.LastSync)
	if s.loadedFor == key {
		return
	}
	s.loadedFor = key
	win, configured := m.window(), m.sy.Configured
	go func() {
		ctx := context.Background()
		var devices []DeviceInfo
		var acts []SyncActivity
		if configured {
			devices, _ = (Sync{a}).Devices(ctx)
			acts, _ = (Sync{a}).Activity(ctx)
		}
		hist, _ := (Sync{a}).History(ctx)
		backups, err := (Sync{a}).Backups(ctx)
		win.Update(func() {
			s.devices, s.acts, s.hist, s.backups = devices, acts, hist, backups
			s.backupsErr = ""
			if err != nil {
				s.backupsErr = err.Error()
			}
		})
	}()
}

func (m *mainUI) syncTab(c *ui.Context) {
	s, p, a, sy, st := &m.syncPg, m.pal, m.a, m.sy, m.st
	m.loadSync()
	c.After(15 * time.Second)
	if !sy.Configured && sy.RemovedBy != "" {
		msg := locales.SyncRemovedBanner(sy.RemovedBy)
		if sy.RemovedBy == "?" {
			msg = locales.SyncRemovedBannerAnon()
		}
		m.banner(c, "warning", msg, nil)
	}
	if !sy.Configured {
		ui.Row(c).Padding(18).Gap(16).Radius(tileRadius).Background(p.Card).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(44, 44).Radius(12).Background(p.AccentSoft).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
				ui.Icon(c, icCloud).FontSize(20).TextColor(p.Accent)
			})
			ui.Column(c).Grow(1).Basis(0).Gap(4).Children(func() {
				ui.Text(c, locales.SyncHeroTitle()).FontSize(16).FontWeight(650)
				ui.Text(c, locales.SyncHeroDesc()).FontSize(12.5).TextColor(p.TextMuted).Wrap()
				ui.Text(c, strings.Join([]string{locales.SyncPoint1(), locales.SyncPoint2(), locales.SyncPoint3()}, " · ")).FontSize(12).TextColor(p.Faint).Wrap()
			})
			if m.button(c, "setup", locales.SyncSetUp(), nil, "primary").Clicked() {
				m.openSyncSetup()
			}
		})
	} else {
		switch {
		case sy.NeedPassphrase:
			m.banner(c, "warning", locales.SyncKeyReplaced(), func() {
				if m.button(c, "rejoin", locales.SyncEnterNewPass(), nil, "").Clicked() {
					m.ask(locales.SyncEnterNewPass(), locales.SyncPassphrase(), "", true, func(v string) {
						m.run(locales.CommonFailed(), func() error { return Sync{a}.Rejoin(context.Background(), v) }, func(err error) {
							if err == nil {
								m.notices = append(m.notices, Notice{Level: "success", Message: locales.SyncRejoined()})
							}
						})
					})
				}
			})
		case sy.LastError != "":
			m.banner(c, "danger", sy.LastError, nil)
		}
		icon := icCloudOff
		if sy.Enabled {
			icon = icCloud
		}
		m.card(c, "status", locales.SyncTitle(), icon, func() {
			if m.button(c, "edit", locales.SyncEdit(), nil, "ghost").Clicked() {
				m.openSyncSetup()
			}
			if s.busy == "now" || sy.Running {
				ui.Spinner(c)
			} else if m.button(c, "now", locales.SyncNow(), icRefreshCw, "primary").Clicked() {
				s.busy = "now"
				win := m.window()
				go func() {
					r, err := (Sync{a}).Now(context.Background())
					win.Update(func() {
						s.busy = ""
						if err != nil {
							m.notices = append(m.notices, Notice{Level: "error", Message: locales.SyncFailed(), Detail: err.Error()})
							return
						}
						m.notices = append(m.notices, Notice{Level: "success", Message: locales.SyncDone(), Detail: locales.SyncReport(fmt.Sprint(len(r.Pushed)), fmt.Sprint(len(r.Pulled)), fmt.Sprint(len(r.Merged)))})
						s.loadedFor = ""
					})
				}()
			}
		}, func() {
			ui.Row(c).Gap(14).Children(func() {
				stat := func(label, value string) {
					ui.Column(c.Key(label)).Grow(1).Basis(0).Gap(3).Children(func() {
						ui.Text(c, label).FontSize(12).TextColor(p.TextMuted)
						ui.Text(c, value).FontWeight(620).SingleLine()
					})
				}
				status := locales.SyncPaused()
				switch {
				case sy.Running:
					status = locales.SyncRunning()
				case sy.Enabled && sy.Pending:
					status = locales.SyncPending()
				case sy.Enabled:
					status = locales.SyncUpToDate()
				}
				stat(locales.SyncStatus(), status)
				last := locales.SyncNever()
				if !sy.LastSync.IsZero() {
					last = m.relTime(sy.LastSync)
				}
				stat(locales.SyncLast(), last)
				next := "—"
				if !sy.NextSync.IsZero() {
					next = m.relTime(sy.NextSync)
				}
				stat(locales.SyncNext(), next)
				n := sy.Devices
				if len(s.devices) > 0 {
					n = len(s.devices)
				}
				stat(locales.SyncDevices(), fmt.Sprint(n))
			})
			if r := sy.Report; r != nil {
				ui.Text(c, locales.SyncReport(fmt.Sprint(len(r.Pushed)), fmt.Sprint(len(r.Pulled)), fmt.Sprint(len(r.Merged)))).FontSize(12).TextColor(p.TextMuted)
			}
		})
		if len(sy.Conflicts) > 0 {
			m.section(c, "conflicts", locales.SyncConflicts(fmt.Sprint(len(sy.Conflicts))), func() {
				for _, cf := range sy.Conflicts {
					path := cf.Path
					m.setting(c, path, m.pathLabel(path, sy.Labels[path]), locales.SyncConflictDesc(m.relTime(cf.LocalModified), cmp.Or(cf.RemoteName, cf.RemoteDevice), m.relTime(cf.RemoteModified)), func() {
						for _, ch := range []struct{ key, label, variant string }{{"local", locales.SyncKeepLocal(), ""}, {"remote", locales.SyncKeepRemote(), ""}, {"both", locales.SyncKeepBoth(), "primary"}} {
							if m.button(c, ch.key, ch.label, nil, ch.variant).Clicked() {
								choice := ch.key
								m.run(locales.CommonFailed(), func() error { return Sync{a}.Resolve(context.Background(), path, choice) }, nil)
							}
						}
					})
				}
			})
		}
		m.section(c, "options", locales.SyncOptions(), func() {
			m.swRow(c, "enabled", locales.SyncEnabled(), locales.SyncEnabledDesc(), st.Sync.Enabled, "sync.enabled")
			mins := []int{0, 15, 30, 60, 180, 720}
			var labels []string
			for _, n := range mins {
				if n == 0 {
					labels = append(labels, locales.SyncManual())
				} else {
					labels = append(labels, locales.SyncEveryMinutes(fmt.Sprint(n)))
				}
			}
			selRow(m, c, "interval", locales.SyncInterval(), "", st.Sync.IntervalMinutes, mins, labels, "sync.intervalMinutes", 150)
			m.swRow(c, "onchange", locales.SyncOnChange(), locales.SyncOnChangeDesc(), st.Sync.OnChange, "sync.onChange")
			desc := locales.SyncTailnetDesc()
			if st.Tailscale.Mode == "off" || st.Tailscale.Mode == "" {
				desc = locales.SyncTailnetOff()
			}
			m.swRow(c, "tailnet", locales.SyncTailnet(), desc, st.Sync.Tailnet, "sync.tailnet")
			pols := []string{"ask", "newest", "local", "remote"}
			labels = nil
			for _, pl := range pols {
				labels = append(labels, m.T("sync.policy."+pl))
			}
			selRow(m, c, "policy", locales.SyncPolicy(), locales.SyncPolicyDesc(), st.Sync.ConflictPolicy, pols, labels, "sync.conflictPolicy", 170)
			desc = locales.SyncWhatDesc()
			if slices.Contains(st.Sync.Categories, "tailscale") {
				desc += " · " + locales.SyncTsNote()
			}
			m.setting(c, "what", locales.SyncWhat(), desc, func() {
				ui.Row(c).Gap(6).Wrap().Children(func() {
					for _, cat := range []string{"profiles", "settings", "dns", "tailscale", "stats"} {
						on := slices.Contains(st.Sync.Categories, cat)
						if m.chip(c, cat, m.T("sync.cat."+cat), on).Clicked() {
							next := slices.DeleteFunc(slices.Clone(st.Sync.Categories), func(x string) bool { return x == cat })
							if !on {
								next = append(next, cat)
							}
							m.set("sync.categories", next)
						}
					}
				})
			})
		})
		m.section(c, "security", locales.SyncSecurity(), func() {
			if sy.Plain {
				m.setting(c, "enc", locales.SyncEncryption(), locales.SyncEncryptionPlainDesc(), func() { m.badge(c, locales.SyncPlainBadge(), "warning") })
			} else {
				m.setting(c, "enc", locales.SyncEncryption(), locales.SyncEncryptionDesc(), func() {
					ui.Box(c).Tooltip("key " + sy.KeyID).Children(func() { m.badge(c, "E2EE", "success") })
				})
			}
			tr, tone, label := locales.SyncTransportTls(), "success", "TLS"
			switch {
			case st.Sync.AllowInsecure:
				tr, tone, label = locales.SyncTransportHttp(), "warning", "HTTP"
			case st.Sync.PinnedKey != "":
				tr, label = locales.SyncTransportPinned(), "TLS · pinned"
			}
			m.setting(c, "transport", locales.SyncTransport(), tr, func() { m.stateBadge(c, "tr", label, tone) })
			if !sy.Plain {
				m.setting(c, "pass", locales.SyncChangePass(), locales.SyncChangePassDesc(), func() {
					if m.button(c, "change", locales.CommonChange(), nil, "").Clicked() {
						m.ask(locales.SyncChangePass(), locales.SyncNewPass(), "", true, func(p1 string) {
							if strength(p1) < 1 {
								m.notices = append(m.notices, Notice{Level: "warning", Message: locales.SyncTooShort()})
								return
							}
							m.ask(locales.SyncChangePass(), locales.SyncPassphrase2(), "", true, func(p2 string) {
								if p1 != p2 {
									m.notices = append(m.notices, Notice{Level: "warning", Message: locales.SyncMismatch()})
									return
								}
								m.run(locales.CommonFailed(), func() error { return Sync{a}.ChangePassphrase(context.Background(), p1) }, func(err error) {
									if err == nil {
										m.notices = append(m.notices, Notice{Level: "success", Message: locales.SyncPassChanged()})
									}
								})
							})
						})
					}
				})
			}
			m.setting(c, "disconnect", locales.SyncDisconnect(), locales.SyncDisconnectDesc(), func() {
				if m.button(c, "disconnect", locales.SyncDisconnect(), nil, "danger").Clicked() {
					m.confirm(locales.SyncDisconnect(), locales.SyncDisconnectMsg(), locales.SyncDisconnect(), func() {
						m.run(locales.CommonFailed(), func() error { return Sync{a}.Disable(context.Background(), true) }, nil)
					})
				}
			})
		})
		if len(s.devices) > 0 {
			m.deviceList(c)
		}
		m.activityList(c)
	}
	m.backupsSection(c)
	m.syncSetupDialog(c)
	m.removeDevicesDialog(c)
	m.askDialog(c)
}

// pathLabel names a path of the vault for people.
func (m *mainUI) pathLabel(path, label string) string {
	switch {
	case label != "":
		return label
	case path == "settings", path == "dns", path == "tailscale":
		return m.T("sync.cat." + path)
	case path == "tailscale/authkey":
		return locales.SyncPathTsKey()
	case path == "profiles/order":
		return locales.SyncPathOrder()
	case strings.HasPrefix(path, "usage/"):
		return locales.SyncCatStats()
	}
	return path
}

// listOf joins the first few names and says how many more.
func (m *mainUI) listOf(names []string) string {
	var uniq []string
	for _, n := range names {
		if !slices.Contains(uniq, n) {
			uniq = append(uniq, n)
		}
	}
	shown := strings.Join(uniq[:min(3, len(uniq))], locales.SyncListSep())
	if len(uniq) > 3 {
		return locales.SyncAndMore(shown, fmt.Sprint(len(uniq)-3))
	}
	return shown
}

func (m *mainUI) deviceList(c *ui.Context) {
	s, p, a := &m.syncPg, m.pal, m.a
	var dups []DeviceInfo
	for _, d := range s.devices {
		if d.Duplicate {
			dups = append(dups, d)
		}
	}
	osName := map[string]string{"darwin": "macOS", "windows": "Windows", "linux": "Linux"}
	ui.Column(c.Key("devices")).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SyncDeviceList()).FontSize(12).FontWeight(600).TextColor(p.TextMuted).Grow(1)
			if len(dups) > 0 && m.button(c, "clean", locales.SyncCleanDuplicates(fmt.Sprint(len(dups))), icTrash2, "").Clicked() {
				m.openRemove(dups)
			}
		})
		m.rows(c, "rows", func() {
			for _, d := range s.devices {
				icon := icLaptop
				switch d.OS {
				case "windows":
					icon = icMonitor
				case "linux":
					icon = icServer
				}
				stale := !d.This && time.Since(d.LastSync) > 30*24*time.Hour
				row := ui.Row(c.Key(d.ID)).Padding(10, 14).Gap(12).AlignItems(ui.Center)
				if stale || d.Duplicate {
					row.Opacity(0.6)
				}
				row.Children(func() {
					ui.Icon(c, icon).FontSize(16).TextColor(p.TextMuted)
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(2).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, cmp.Or(d.Label, d.Name)).FontWeight(550).SingleLine()
							if d.This {
								m.badge(c, locales.SyncThisDevice(), "info")
							}
							if d.Duplicate {
								ui.Box(c).Tooltip(locales.SyncDuplicateTip()).Children(func() { m.badge(c, locales.SyncDuplicate(), "warning") })
							}
							if d.Tailnet {
								ui.Icon(c, icWaypoints).FontSize(12).TextColor(p.AccentFg).Tooltip(locales.SyncTailnetTip())
							}
						})
						facts := []string{cmp.Or(osName[d.OS], d.OS)}
						if d.Arch != "" {
							facts = append(facts, d.Arch)
						}
						if d.Version != "" {
							facts = append(facts, "v"+d.Version)
						}
						facts = append(facts, locales.SyncLastSeen(m.relTime(d.LastSync)))
						line := strings.Join(facts, " · ")
						if d.Label != "" && d.Label != d.Name {
							line = d.Name + " · " + line
						}
						ui.Text(c, line).FontSize(12).TextColor(p.TextMuted).SingleLine()
					})
					dev := d
					m.iconButton(c, "more", icMoreHorizontal, locales.SyncManage()).PopoverMenuButton(func(menu *ui.Menu) {
						if menu.Item(locales.SyncRename()).Chosen() {
							m.ask(locales.SyncRenameTitle(), locales.SyncRenameLabel(), cmp.Or(dev.Label, dev.Name), false, func(v string) {
								label := strings.TrimSpace(v)
								if label == dev.Name {
									label = ""
								}
								m.run(locales.CommonFailed(), func() error { return Sync{a}.RenameDevice(context.Background(), dev.ID, label) }, func(error) { s.loadedFor = "" })
							})
						}
						if !dev.This {
							menu.Separator()
							if menu.Item(locales.SyncRemove()).Chosen() {
								m.openRemove([]DeviceInfo{dev})
							}
						}
					}, ui.PopoverMenuButtonOptions{At: ui.AnchorBottomEnd, Self: ui.AnchorTopEnd})
				})
			}
		})
	})
}

func (m *mainUI) openRemove(devices []DeviceInfo) {
	s := &m.syncPg
	s.removed, s.revoke, s.rmPass, s.rmPass2 = devices, false, "", ""
}

// removeDevicesDialog takes devices out of sync, and with them, optionally,
// the key they hold.
func (m *mainUI) removeDevicesDialog(c *ui.Context) {
	s, p, a := &m.syncPg, m.pal, m.a
	open := len(s.removed) > 0
	if !open {
		return
	}
	ids := map[string]bool{}
	for _, d := range s.removed {
		ids[d.ID] = true
	}
	var behind []string
	for _, d := range s.devices {
		if !ids[d.ID] && !d.This && !d.HasKey {
			behind = append(behind, cmp.Or(d.Label, d.Name))
		}
	}
	ok := !s.revoke || (len(s.rmPass) >= 8 && s.rmPass == s.rmPass2)
	title := locales.SyncRemoveTitleN(fmt.Sprint(len(s.removed)))
	if len(s.removed) == 1 {
		title = locales.SyncRemoveTitle(cmp.Or(s.removed[0].Label, s.removed[0].Name))
	}
	m.modal(c, "remove", &open, func() {
		ui.Column(c).Width(420).Gap(12).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(650)
			ui.Text(c, locales.SyncRemoveMsg()).FontSize(12.5).TextColor(p.TextMuted).Wrap()
			if !m.sy.Plain {
				ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
						ui.Text(c, locales.SyncRevoke()).FontWeight(550)
						ui.Text(c, locales.SyncRevokeDesc()).FontSize(12).TextColor(p.TextMuted).Wrap()
					})
					ui.Switch(c.Key("revoke"), &s.revoke)
				})
				m.collapse(c, "revoke", s.revoke, func() {
					m.field(c, locales.SyncNewPass(), "", "", func() { ui.TextInput(c.Key("p1"), &s.rmPass).Password() })
					errText := ""
					switch {
					case s.rmPass2 != "" && s.rmPass != s.rmPass2:
						errText = locales.SyncMismatch()
					case s.rmPass != "" && len(s.rmPass) < 8:
						errText = locales.SyncTooShort()
					}
					m.field(c, locales.SyncPassphrase2(), "", errText, func() { ui.TextInput(c.Key("p2"), &s.rmPass2).Password() })
					if len(behind) > 0 {
						m.banner(c, "warning", locales.SyncRevokeBehind(strings.Join(behind, locales.SyncListSep())), nil)
					}
				})
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if m.button(c, "cancel", locales.CommonCancel(), nil, "").Clicked() {
					s.removed = nil
				}
				if s.busy == "remove" {
					ui.Spinner(c)
				} else if m.button(c, "remove", locales.SyncRemove(), nil, "danger").Disabled(!ok).Clicked() {
					var list []string
					for id := range ids {
						list = append(list, id)
					}
					revoke, pass := s.revoke, s.rmPass
					s.busy = "remove"
					m.run(locales.CommonFailed(), func() error { return Sync{a}.RemoveDevices(context.Background(), list, revoke, pass) }, func(err error) {
						s.busy = ""
						if err == nil {
							msg := locales.SyncDevicesRemoved()
							if revoke {
								msg = locales.SyncRevoked()
							}
							m.notices = append(m.notices, Notice{Level: "success", Message: msg})
							s.removed, s.loadedFor = nil, ""
						}
					})
				}
			})
		})
	})
	if !open {
		s.removed = nil
	}
}

func meaningful(path string) bool { return !strings.HasPrefix(path, "usage/") }

func (m *mainUI) activityList(c *ui.Context) {
	s, p := &m.syncPg, m.pal
	describe := func(a SyncActivity) string {
		switch a.Kind {
		case "join":
			return locales.SyncActJoin()
		case "rename":
			if a.Detail != "" {
				return locales.SyncActRename(a.Target, a.Detail)
			}
			return locales.SyncActRenameReset(a.Target)
		case "remove":
			return locales.SyncActRemove(cmp.Or(a.Target, a.Detail))
		case "rotate":
			return locales.SyncActRotate()
		}
		var sent, gone []string
		for _, ch := range a.Changes {
			if !meaningful(ch.Path) {
				continue
			}
			if ch.Deleted {
				gone = append(gone, m.pathLabel(ch.Path, ch.Label))
			} else {
				sent = append(sent, m.pathLabel(ch.Path, ch.Label))
			}
		}
		var parts []string
		if len(sent) > 0 {
			parts = append(parts, locales.SyncActSent(m.listOf(sent)))
		}
		if len(gone) > 0 {
			parts = append(parts, locales.SyncActDeleted(m.listOf(gone)))
		}
		return strings.Join(parts, locales.SyncListSep())
	}
	name := func(path string) string {
		pth, label, _ := strings.Cut(path, "\x00")
		return m.pathLabel(pth, label)
	}
	summary := func(e cloudsync.Event) string {
		keep := func(l []string) []string {
			var out []string
			for _, x := range l {
				if meaningful(x) {
					out = append(out, name(x))
				}
			}
			return out
		}
		var parts []string
		if l := keep(e.Pushed); len(l) > 0 {
			parts = append(parts, locales.SyncHistPushed(m.listOf(l)))
		}
		if l := keep(e.Pulled); len(l) > 0 {
			parts = append(parts, locales.SyncHistPulled(m.listOf(l)))
		}
		if l := keep(e.Merged); len(l) > 0 {
			parts = append(parts, locales.SyncHistMerged(m.listOf(l)))
		}
		if e.Conflicts > 0 {
			parts = append(parts, locales.SyncConflicts(fmt.Sprint(e.Conflicts)))
		}
		return strings.Join(parts, locales.SyncListSep())
	}
	type row struct {
		at       time.Time
		who, txt string
		err      bool
	}
	var rows []row
	people := map[string]string{}
	var order []string
	if s.scope == 0 {
		for _, a := range s.acts {
			if _, ok := people[a.DeviceID]; !ok {
				order = append(order, a.DeviceID)
				people[a.DeviceID] = a.Device
				if a.This {
					people[a.DeviceID] = locales.SyncThisDevice()
				}
			}
			if s.who != "" && a.DeviceID != s.who {
				continue
			}
			if a.Kind == "sync" && !slices.ContainsFunc(a.Changes, func(ch cloudsync.Change) bool { return meaningful(ch.Path) }) {
				continue
			}
			who := a.Device
			if a.This {
				who = locales.SyncThisDevice()
			}
			rows = append(rows, row{a.At, who, describe(a), false})
		}
	} else {
		for _, e := range s.hist {
			txt := summary(e)
			if e.Error == "" && txt == "" {
				continue
			}
			rows = append(rows, row{e.At, "", cmpOr(e.Error, txt), e.Error != ""})
		}
	}
	total := len(rows)
	if !s.more && len(rows) > 8 {
		rows = rows[:8]
	}
	ui.Column(c.Key("activity")).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.SyncActivity()).FontSize(12).FontWeight(600).TextColor(p.TextMuted).Grow(1)
			if s.scope == 0 && len(order) > 1 {
				ids := append([]string{""}, order...)
				labels := []string{locales.SyncEveryDevice()}
				for _, id := range order {
					labels = append(labels, people[id])
				}
				m.selectOf(c, s.who, ids, labels, 150, func(v string) { s.who, s.more = v, false })
			}
			if i := m.segmented(c, "scope", s.scope, locales.SyncActivityAll(), locales.SyncActivityHere()); i >= 0 {
				s.scope, s.more = i, false
			}
		})
		ui.Column(c).Padding(10, 14).Gap(6).Radius(tileRadius).Background(p.Card).Children(func() {
			// The rows of the other scope slide in from its side.
			m.switchPane(c, "rows", s.scope, 6, func() {
				if len(rows) == 0 {
					ui.Text(c, locales.SyncNoActivity()).FontSize(12.5).TextColor(p.Faint)
				}
				for i, r := range rows {
					ui.Row(c.Key(i)).Gap(10).AlignItems(ui.Start).Transition(listRow).Children(func() {
						ui.Text(c, m.relTime(r.at)).FontSize(12).TextColor(p.Faint).Width(90).Shrink(0).Tooltip(fmtDateTime(r.at))
						if r.who != "" {
							ui.Text(c, r.who).FontSize(12.5).FontWeight(550).Shrink(0)
						}
						t := ui.Text(c, r.txt).FontSize(12.5).Wrap().Grow(1).Basis(0)
						if r.err {
							t.TextColor(p.Danger)
						}
					})
				}
				if !s.more && total > 8 {
					if m.button(c, "more", locales.SyncShowMore(fmt.Sprint(total-8)), nil, "ghost").Clicked() {
						s.more = true
					}
				}
			})
		})
	})
}

func (m *mainUI) backupsSection(c *ui.Context) {
	s, p, a, st := &m.syncPg, m.pal, m.a, m.st
	act := func(key, ok string, fn func() error) {
		s.busy = key
		m.run(locales.CommonFailed(), fn, func(err error) {
			s.busy, s.loadedFor = "", ""
			if err == nil && ok != "" {
				m.notices = append(m.notices, Notice{Level: "success", Message: ok})
			}
		})
	}
	exportOne := func(b *cloudsync.BackupInfo) {
		m.ask(locales.BackupExportTitle(), locales.BackupPassword(), "", true, func(pw string) {
			if len(pw) < 8 {
				m.notices = append(m.notices, Notice{Level: "warning", Message: locales.SyncTooShort()})
				return
			}
			name, remote := "", false
			if b != nil {
				name, remote = b.Name, b.Remote
			}
			act("export", locales.BackupExported(), func() error {
				_, err := (Sync{a}).ExportBackup(context.Background(), name, remote, pw)
				return err
			})
		})
	}
	var owners []string
	for _, b := range s.backups {
		if b.Device != "" && !slices.Contains(owners, b.Device) {
			owners = append(owners, b.Device)
		}
	}
	slices.Sort(owners)
	ui.Column(c.Key("backups")).Gap(8).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Wrap().Children(func() {
			ui.Text(c, locales.BackupTitle()).FontSize(12).FontWeight(600).TextColor(p.TextMuted).Grow(1)
			if len(owners) > 1 {
				m.selectOf(c, s.from, append([]string{""}, owners...), append([]string{locales.SyncEveryDevice()}, owners...), 150, func(v string) { s.from = v })
			}
			if s.busy == "local" {
				ui.Spinner(c)
			} else if m.button(c, "local", locales.BackupLocal(), icHardDrive, "").Clicked() {
				act("local", locales.BackupCreated(), func() error { _, err := (Sync{a}).CreateBackup(context.Background(), false); return err })
			}
			if m.sy.Configured {
				if s.busy == "remote" {
					ui.Spinner(c)
				} else if m.button(c, "remote", locales.BackupRemote(), icUpload, "").Clicked() {
					act("remote", locales.BackupUploaded(), func() error { _, err := (Sync{a}).CreateBackup(context.Background(), true); return err })
				}
			}
			if m.button(c, "export", locales.BackupExportNow(), icDownload, "").Clicked() {
				exportOne(nil)
			}
			if m.button(c, "import", locales.BackupImport(), icFileUp, "").Clicked() {
				m.ask(locales.BackupImportTitle(), locales.BackupPassword(), "", true, func(pw string) {
					act("import", locales.BackupRestored(), func() error { return Sync{a}.ImportBackup(context.Background(), pw) })
				})
			}
		})
		m.rows(c, "list", func() {
			shown := 0
			for i := range s.backups {
				b := s.backups[i]
				if s.from != "" && b.Device != s.from {
					continue
				}
				shown++
				ui.Row(c.Key(fmt.Sprint(b.Remote, b.Name))).Padding(10, 14).Gap(10).AlignItems(ui.Center).Children(func() {
					icon := icHardDrive
					if b.Remote {
						icon = icCloud
					}
					ui.Icon(c, icon).FontSize(16).TextColor(p.TextMuted)
					ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
						ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
							ui.Text(c, fmtDateTime(b.Created)).FontWeight(550)
							if b.Remote {
								m.badge(c, "WebDAV", "info")
							} else {
								m.badge(c, locales.BackupDevice(), "")
							}
						})
						ui.Text(c, b.Device+" · "+fmtBytes(b.Size)).FontSize(12).TextColor(p.TextMuted)
					})
					if m.button(c, "restore", locales.BackupRestore(), icRotateCcw, "").Clicked() {
						m.confirm(locales.BackupRestoreTitle(), locales.BackupRestoreMsg(), locales.BackupRestore(), func() {
							act("restore", locales.BackupRestored(), func() error { return Sync{a}.RestoreBackup(context.Background(), b.Name, b.Remote) })
						})
					}
					if m.iconButton(c, "export", icDownload, locales.BackupExport()).Clicked() {
						exportOne(&b)
					}
					if m.iconButton(c, "delete", icTrash2, locales.CommonDelete()).Clicked() {
						m.confirm(locales.BackupDeleteTitle(), fmtDateTime(b.Created), locales.CommonDelete(), func() {
							act("delete", "", func() error { return Sync{a}.DeleteBackup(context.Background(), b.Name, b.Remote) })
						})
					}
				})
			}
			if shown == 0 {
				ui.Row(c.Key("none")).Padding(16).Justify(ui.Center).Children(func() {
					ui.Text(c, locales.BackupEmpty()).FontSize(12.5).TextColor(p.TextMuted)
				})
			}
			if s.backupsErr != "" {
				ui.Row(c.Key("err")).Padding(10, 14).Children(func() { ui.Text(c, s.backupsErr).FontSize(12).TextColor(p.Danger).Wrap() })
			}
		})
	})
	m.section(c, "auto", locales.BackupAuto(), func() {
		hours := []int{0, 6, 12, 24, 72, 168}
		var labels []string
		for _, h := range hours {
			switch {
			case h == 0:
				labels = append(labels, locales.CommonOff())
			case h < 24:
				labels = append(labels, locales.BackupEveryHours(fmt.Sprint(h)))
			default:
				labels = append(labels, locales.BackupEveryDays(fmt.Sprint(h/24)))
			}
		}
		selRow(m, c, "interval", locales.BackupInterval(), locales.BackupIntervalDesc(), st.Backup.AutoIntervalHours, hours, labels, "backup.autoIntervalHours", 150)
		m.setting(c, "keep", locales.BackupKeep(), "", func() {
			m.numberIn(c, "keep", st.Backup.Keep, 1, 100, 80, func(n int) { m.set("backup.keep", n) })
		})
	})
}

var syncPresets = []struct{ name, url string }{
	{"Nextcloud", "https://cloud.example.com/remote.php/dav/files/USER/"},
	{"坚果云 Jianguoyun", "https://dav.jianguoyun.com/dav/"},
	{"InfiniCLOUD", "https://ogi.teracloud.jp/dav/"},
}

func (m *mainUI) openSyncSetup() {
	s, st := &m.syncPg, m.st.Sync
	s.setup, s.step, s.probe, s.pass2, s.suBusy = true, 1, nil, "", false
	s.su = SyncSetup{URL: st.URL, Username: st.Username, Dir: st.Dir, AllowInsecure: st.AllowInsecure, PinnedKey: st.PinnedKey}
}

func (m *mainUI) syncSetupDialog(c *ui.Context) {
	s, p, a := &m.syncPg, m.pal, m.a
	if !s.setup {
		return
	}
	su := &s.su
	pr := s.probe
	untrusted := pr != nil && pr.Fingerprint != "" && !pr.Trusted
	canNext := pr != nil && pr.Reachable && (!untrusted || su.PinnedKey == pr.Fingerprint)
	newVault := pr == nil || !pr.HasVault
	unencrypted := su.Plain
	if !newVault {
		unencrypted = pr.Plain
	}
	st := strength(su.Passphrase)
	canFinish := unencrypted || (newVault && st >= 1 && su.Passphrase == s.pass2) || (!newVault && len(su.Passphrase) >= 8)
	changed := func() { s.probe = nil }
	m.modal(c, "sync-setup", &s.setup, func() {
		ui.Column(c).Width(600).MaxHeight(660).Gap(14).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icCloud).FontSize(18).TextColor(p.AccentFg)
				ui.Text(c, locales.SyncSetupTitle()).FontSize(15).FontWeight(650)
			})
			step2 := locales.SyncStepPassphrase()
			if newVault || unencrypted {
				step2 = locales.SyncStepEncrypt()
			}
			ui.Row(c).Gap(10).Children(func() {
				for i, label := range []string{"1 · " + locales.SyncStepServer(), "2 · " + step2} {
					t := ui.Text(c.Key(i), label).FontSize(12.5)
					col := p.TextMuted
					if s.step == i+1 {
						t.FontWeight(650)
						col = p.AccentFg
					}
					t.TextColor(t.AnimateColor("fg", col, ui.Animation{Duration: 200 * time.Millisecond}))
				}
			})
			// The steps slide the way the wizard goes.
			dir := float32(1)
			if s.step == 1 {
				dir = -1
			}
			if c.Direction() == ui.RightToLeft {
				dir = -dir
			}
			gutter(ui.Scroll(c).Grow(1).ChildExit(ui.Motion{X: -16 * dir}), 14).Children(func() {
				ui.Column(c.Key(s.step)).Gap(12).Transition(ui.ElementTransition{
					Enter: &ui.Motion{X: 16 * dir}, Exit: &ui.Motion{X: -16 * dir},
					Colors: true, Duration: 240 * time.Millisecond, Ease: easeStd,
				}).Children(func() {
					if s.step == 1 {
						m.field(c, locales.SyncUrl(), locales.SyncUrlHint(), "", func() {
							if ui.TextInput(c.Key("url"), &su.URL).Placeholder("https://dav.example.com/dav/").Font("ui-monospace").Changed() {
								changed()
							}
						})
						ui.Row(c).Gap(6).Wrap().Children(func() {
							for _, ps := range syncPresets {
								if m.chip(c, ps.name, ps.name, false).Clicked() {
									su.URL = ps.url
									changed()
								}
							}
						})
						ui.Row(c).Gap(12).Children(func() {
							m.rowField(c, locales.SyncUsername(), "", "", func() {
								if ui.TextInput(c.Key("user"), &su.Username).Changed() {
									changed()
								}
							})
							hint := locales.SyncPasswordHint()
							if m.st.Sync.URL != "" {
								hint = locales.SyncPasswordKeep()
							}
							m.rowField(c, locales.SyncPassword(), hint, "", func() {
								if ui.TextInput(c.Key("pass"), &su.Password).Password().Changed() {
									changed()
								}
							})
						})
						m.field(c, locales.SyncFolder(), locales.SyncFolderHint(), "", func() {
							if ui.TextInput(c.Key("dir"), &su.Dir).Changed() {
								changed()
							}
						})
						if strings.HasPrefix(strings.TrimSpace(su.URL), "http://") {
							m.banner(c, "warning", locales.SyncHttpWarn(), func() {
								if ui.Switch(c.Key("insecure"), &su.AllowInsecure).Changed() {
									changed()
								}
							})
						}
						if pr != nil {
							tone, text := "danger", pr.Error
							if pr.Reachable {
								tone, text = "info", locales.SyncNoVault()
								if pr.HasVault {
									text = locales.SyncFoundVault()
									if pr.Plain {
										text = locales.SyncFoundPlain()
									}
								}
							}
							if pr.Fingerprint != "" {
								text += "\nSHA-256 " + pr.Fingerprint
							}
							m.banner(c, tone, text, nil)
						}
						if untrusted {
							m.banner(c, "warning", locales.SyncUntrusted(), func() {
								on := su.PinnedKey == pr.Fingerprint
								if ui.Switch(c.Key("pin"), &on).Changed() {
									su.PinnedKey = ""
									if on {
										su.PinnedKey = pr.Fingerprint
									}
								}
							})
						}
						return
					}
					if newVault {
						mode := 0
						if su.Plain {
							mode = 1
						}
						if m.segmentedOf(c, "mode", &mode, true, locales.SyncEncrypt(), locales.SyncPlain()) {
							su.Plain = mode == 1
						}
						if su.Plain {
							m.banner(c, "warning", locales.SyncPlainWarn(), nil)
						}
					} else if unencrypted {
						m.banner(c, "info", locales.SyncJoinPlainHint(), nil)
					}
					// The passphrase folds away as the vault goes plain, and back.
					m.collapse(c, "sync-secret", !unencrypted, func() {
						hint := locales.SyncExistingPassHint()
						if newVault {
							hint = locales.SyncNewPassHint()
						}
						m.banner(c, "info", hint, nil)
						m.field(c, locales.SyncPassphrase(), "", "", func() { ui.TextInput(c.Key("pp"), &su.Passphrase).Password().AutoFocus() })
						if newVault {
							ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
								cols := []ui.Color{p.Danger, p.Danger, p.Warning, p.Success, p.Success}
								for i := range 4 {
									bar := ui.Box(c.Key(i)).Height(4).Grow(1).Radius(2).Background(p.Surface3).Transition(ui.ElementTransition{Colors: true, Duration: 240 * time.Millisecond})
									if i < st {
										bar.Background(cols[st])
									}
								}
								ui.Text(c, m.T(fmt.Sprintf("sync.strength.%d", st))).FontSize(11.5).TextColor(p.TextMuted).PaddingStart(6).ContentTransition(ui.ContentTransition{})
							})
							errText := ""
							if s.pass2 != "" && su.Passphrase != s.pass2 {
								errText = locales.SyncMismatch()
							}
							m.field(c, locales.SyncPassphrase2(), "", errText, func() { ui.TextInput(c.Key("pp2"), &s.pass2).Password() })
							ui.Text(c, locales.SyncNoRecovery()).FontSize(12).TextColor(p.TextMuted).Wrap()
						}
					})
				})
			})
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				if s.step == 1 {
					if s.suBusy {
						ui.Spinner(c)
					} else if m.button(c, "test", locales.SyncTest(), nil, "").Disabled(strings.TrimSpace(su.URL) == "").Clicked() {
						s.suBusy = true
						setup, win := *su, m.window()
						go func() {
							probe := (Sync{a}).Probe(context.Background(), setup)
							win.Update(func() { s.suBusy, s.probe = false, &probe })
						}()
					}
					ui.Spacer(c)
					if m.button(c, "cancel", locales.CommonCancel(), nil, "").Clicked() {
						s.setup = false
					}
					if m.button(c, "next", locales.CommonNext(), nil, "primary").Disabled(!canNext).Clicked() {
						s.step = 2
					}
					return
				}
				if m.button(c, "back", locales.CommonBack(), nil, "").Clicked() {
					s.step = 1
				}
				ui.Spacer(c)
				if m.button(c, "cancel", locales.CommonCancel(), nil, "").Clicked() {
					s.setup = false
				}
				label := locales.SyncOpenVault()
				switch {
				case unencrypted && newVault:
					label = locales.SyncCreatePlain()
				case unencrypted:
					label = locales.SyncJoinPlain()
				case newVault:
					label = locales.SyncCreateVault()
				}
				if s.suBusy {
					ui.Spinner(c)
				} else if m.button(c, "finish", label, nil, "primary").Disabled(!canFinish).Clicked() {
					s.suBusy = true
					setup, win := *su, m.window()
					setup.Plain = unencrypted
					go func() {
						created, err := (Sync{a}).Setup(context.Background(), setup)
						win.Update(func() {
							s.suBusy = false
							if err != nil {
								m.notices = append(m.notices, Notice{Level: "error", Message: locales.SyncSetupFailed(), Detail: err.Error()})
								return
							}
							key := "sync.vaultOpened"
							switch {
							case unencrypted && created:
								key = "sync.plainCreated"
							case unencrypted:
								key = "sync.plainOpened"
							case created:
								key = "sync.vaultCreated"
							}
							m.notices = append(m.notices, Notice{Level: "success", Message: m.T(key)})
							s.setup, s.loadedFor = false, ""
						})
					}()
				}
			})
		})
	})
}

// ---- Advanced ----

func (m *mainUI) advancedTab(c *ui.Context) {
	s, p, a, st, info, state := &m.settings, m.pal, m.a, m.st, m.info, m.state
	m.section(c, "core", locales.AdvCore(), func() {
		selRow(m, c, "mode", locales.AdvCoreMode(), locales.AdvCoreModeDesc(), st.CoreMode, []string{"auto", "sidecar", "service"},
			[]string{locales.AdvCoreAuto(), locales.AdvCoreSidecar(), locales.AdvCoreService()}, "coreMode", 190)
		pid := "—"
		if state.Core.PID != 0 {
			pid = fmt.Sprint(state.Core.PID)
		}
		desc := fmt.Sprintf("%s · %s · PID %s · mihomo %s", m.T("core."+state.Core.Status), state.Core.Mode, pid, cmp.Or(state.Core.CoreVersion, info.CoreVersion))
		m.setting(c, "status", locales.CoreStatus(), desc, func() {
			if m.button(c, "restart", locales.CoreRestart(), icRefreshCw, "").Clicked() {
				m.run(locales.CommonFailed(), func() error { return Core{a}.Restart(context.Background()) }, func(err error) {
					if err == nil {
						m.notices = append(m.notices, Notice{Level: "info", Message: locales.AdvRestarting()})
					}
				})
			}
		})
	})
	m.section(c, "light", locales.AdvLightweight(), func() {
		m.setting(c, "auto", locales.AdvLightAuto(), locales.AdvLightAutoDesc(), func() {
			m.collapseInline(c, "light", st.Lightweight.AutoEnter, func() {
				m.numberIn(c, "light", st.Lightweight.DelayMinutes, 1, 1440, 80, func(n int) { m.set("lightweight.delayMinutes", n) })
				ui.Text(c, locales.CommonMinutes()).TextColor(p.TextMuted).SingleLine()
			})
			v := st.Lightweight.AutoEnter
			if ui.Switch(c.Key("sw"), &v).Changed() {
				m.set("lightweight.autoEnter", v)
			}
		})
		m.setting(c, "now", locales.AdvLightNow(), locales.AdvLightNowDesc(), func() {
			if m.button(c, "enter", locales.AdvEnter(), nil, "").Clicked() {
				(AppService{a}).EnterLightweight()
			}
		})
	})
	m.section(c, "agent", locales.AdvAgent(), func() {
		m.swRow(c, "control", locales.AdvAgentControl(), locales.AdvAgentControlDesc(), st.Agent.Enabled, "agent.enabled")
	})
	m.section(c, "logs", locales.AdvLogs(), func() {
		selRow(m, c, "level", locales.AdvAppLogLevel(), locales.AdvAppLogLevelDesc(), st.Logs.Level, []string{"debug", "info", "warn", "error"},
			[]string{locales.LogsDebug(), locales.LogsInfo(), locales.LogsWarning(), locales.LogsError()}, "logs.level", 130)
		m.setting(c, "size", locales.AdvLogSize(), locales.AdvLogSizeDesc(), func() {
			m.numberIn(c, "logsize", st.Logs.MaxSizeMB, 1, 512, 70, func(n int) { m.set("logs.maxSizeMb", n) })
			ui.Text(c, "MB ×").TextColor(p.TextMuted)
			m.numberIn(c, "logfiles", st.Logs.MaxFiles, 1, 50, 60, func(n int) { m.set("logs.maxFiles", n) })
		})
		days := []int{0, 1, 7, 30, 90}
		var labels []string
		for _, d := range days {
			if d == 0 {
				labels = append(labels, locales.AdvNever())
			} else {
				labels = append(labels, locales.AdvKeepDays(fmt.Sprint(d)))
			}
		}
		selRow(m, c, "clean", locales.AdvLogClean(), locales.AdvLogCleanDesc(), st.Logs.AutoCleanDays, days, labels, "logs.autoCleanDays", 150)
	})
	m.section(c, "folders", locales.AdvFolders(), func() {
		for _, f := range []struct{ key, label, path string }{{"data", locales.AdvDataDir(), info.DataDir}, {"core", locales.AdvCoreDir(), info.CoreDir}, {"logs", locales.AdvLogsDir(), info.LogsDir}} {
			m.setting(c, f.key, f.label, f.path, func() {
				if m.button(c, "open", locales.CommonOpen(), icFolderOpen, "").Clicked() {
					kind := f.key
					m.run(locales.CommonFailed(), func() error { return AppService{a}.OpenDir(kind) }, nil)
				}
			})
		}
	})
	m.section(c, "security", locales.AdvSecurity(), func() {
		desc := locales.AdvAtRestFile()
		tone := "warning"
		if info.KeyringSecure {
			desc, tone = locales.AdvAtRestSecure(info.Keyring), "success"
		}
		m.setting(c, "rest", locales.AdvAtRest(), desc, func() { m.badge(c, info.Keyring, tone) })
		m.setting(c, "diag", locales.AdvDiagnostics(), locales.AdvDiagnosticsDesc(), func() {
			if m.button(c, "copy", locales.CommonCopy(), icBug, "").Clicked() {
				m.run(locales.CommonFailed(), func() error {
					text, err := (AppService{a}).Diagnostics(context.Background())
					if err == nil {
						mygo.RunOnMain(func() { mygo.Clipboard.WriteText(text) })
						m.notice(Notice{Level: "success", Message: locales.AdvDiagnosticsCopied()})
					}
					return err
				}, nil)
			}
		})
	})
	m.about(c)
	m.updatesSection(c)
	m.section(c, "more", locales.AdvMore(), func() {
		m.setting(c, "license", locales.AdvLicense(), locales.AdvLicenseDesc(), func() {
			if m.button(c, "mihomo", "mihomo", icExternalLink, "ghost").Clicked() {
				go mygo.Shell.OpenExternal("https://github.com/MetaCubeX/mihomo")
			}
			if m.button(c, "mygo", "MyGo", icExternalLink, "ghost").Clicked() {
				go mygo.Shell.OpenExternal("https://github.com/egoist/mygo")
			}
		})
		m.setting(c, "restart", locales.AdvRestartApp(), locales.AdvRestartAppDesc(), func() {
			if m.button(c, "relaunch", locales.AdvRestart(), nil, "").Clicked() {
				go mygo.App.Relaunch()
			}
			if m.button(c, "quit", locales.AdvQuit(), icLogOut, "danger").Clicked() {
				m.confirm(locales.AdvQuitTitle(), locales.AdvQuitMsg(), locales.AdvQuit(), func() { go a.quit() })
			}
		})
	})
	_ = s
}

// members are the image colors of the five: Tomori, Anon, Rāna, Soyo, Taki.
var members = []string{"#77BBDD", "#FF8899", "#77DD77", "#FFDD88", "#7777AA"}

// about is the app, its versions, and its motto, with the band name's five
// exclamation marks, one per member.
func (m *mainUI) about(c *ui.Context) {
	p, info := m.pal, m.info
	line := fmt.Sprintf("v%s · mihomo %s · MyGo %s · %s/%s", info.Version, info.CoreVersion, info.MyGoVersion, info.OS, info.Arch)
	ui.Column(c.Key("about")).Gap(8).Children(func() {
		ui.Text(c, locales.AdvAbout()).FontSize(12).FontWeight(600).TextColor(p.TextMuted)
		ui.Row(c).Padding(18).Gap(16).Radius(tileRadius).Background(p.Card).AlignItems(ui.Center).Children(func() {
			// It drifts round slowly, and a press spins it.
			logo := ui.ButtonBase(c.Key("logo")).Size(64, 64).Shrink(0).Label("MyGO-Clash")
			pressable(logo)
			spin := logo.Loop("about", 40*time.Second, ui.Linear)
			turn := compassSpin(logo)
			logo.Draw(func(pt *ui.Painter, r ui.Rect) {
				drawCompass(pt, r.X+r.W/2, r.Y+r.H/2, 72, compassLook{Ring: true, Turn: spin*360 + turn, Glints: 1, Glint: p.Spark})
			})
			ui.Column(c).Grow(1).Basis(0).Gap(5).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.RichText(c, ui.Span{Text: "MyGO", Weight: 700, Color: p.AccentFg}, ui.Span{Text: "-Clash", Weight: 700}).FontSize(18)
					ui.Box(c).Size(5*4.2+4*3.8, 16).Draw(func(pt *ui.Painter, r ui.Rect) {
						w, gap := float32(4.2), float32(3.8)
						for i, hex := range members {
							col := ui.Hex(hex)
							x := r.X + float32(i)*(w+gap)
							pt.Fill(ui.Rect{X: x, Y: r.Y, W: w, H: r.H * 0.66}, col, w/2)
							pt.FillPath((&ui.Path{}).Circle(x+w/2, r.Y+r.H-w/2, w/2), col)
						}
					})
				})
				ui.Text(c, line).FontSize(12).TextColor(p.TextMuted).Selectable()
				ui.Row(c).Gap(8).Wrap().Children(func() {
					ui.Text(c, "迷子でもいい、前へ進め。").FontSize(12.5).Lang("ja")
					ui.Text(c, locales.AboutMotto()).FontSize(12.5).TextColor(p.TextMuted)
				})
			})
			ui.Column(c).Gap(6).AlignItems(ui.End).Shrink(0).Children(func() {
				if m.button(c, "copy", locales.CommonCopy(), icCopy, "").Clicked() {
					mygo.Clipboard.WriteText(fmt.Sprintf("MyGO-Clash v%s (mihomo %s, MyGo %s, %s/%s)", info.Version, info.CoreVersion, info.MyGoVersion, info.OS, info.Arch))
					c.Toast(locales.CommonCopied())
				}
				if m.button(c, "onboarding", locales.SettingsOnboarding(), icPlay, "ghost").Tooltip(locales.SettingsOnboardingReplay()).Clicked() {
					m.replayOnboarding()
				}
			})
		})
		ui.Text(c, locales.AboutHomage()).FontSize(11.5).TextColor(p.Faint).Wrap()
	})
}

// updatesSection checks for new versions of the app, and keeps the choices
// that Sparkle offers on the Mac.
func (m *mainUI) updatesSection(c *ui.Context) {
	p, a, st := m.pal, m.a, m.st
	u := m.upd
	if u == nil {
		if a.updates != nil {
			v := a.updates.state()
			m.upd = &v
			u = &v
		} else {
			return
		}
	}
	status := locales.UpdatesNever()
	switch {
	case !u.Supported && u.Reason == "dev":
		status = locales.UpdatesDev()
	case !u.Supported:
		status = locales.UpdatesPackage()
	case u.Checking:
		status = locales.UpdatesChecking()
	case u.LastCheck > 0:
		status = locales.UpdatesLastCheck(m.relTime(time.UnixMilli(u.LastCheck)))
	}
	m.section(c, "updates", locales.UpdatesTitle(), func() {
		m.setting(c, "check", locales.UpdatesCheck(), status, func() {
			if !u.Supported && u.Reason != "dev" && m.button(c, "releases", locales.UpdatesReleases(), icExternalLink, "ghost").Clicked() {
				go mygo.Shell.OpenExternal((UpdateService{a}).ReleasesURL())
			}
			if m.button(c, "now", locales.UpdatesCheckNow(), nil, "").Clicked() {
				(UpdateService{a}).Check()
			}
		})
		m.collapseRows(c, "update-ready", u.Ready != "", func() {
			m.setting(c, "ready", locales.UpdatesReady(u.Ready), locales.UpdatesReadyDesc(), func() {
				if m.button(c, "relaunch", locales.UpdatesRelaunch(), icRefreshCw, "primary").Clicked() {
					go mygo.App.Relaunch()
				}
			})
		})
		if !u.Supported {
			return
		}
		m.swRow(c, "auto", locales.UpdatesAuto(), locales.UpdatesAutoDesc(), st.Updates.AutoCheck, "updates.autoCheck")
		m.setting(c, "interval", locales.UpdatesInterval(), "", func() {
			vals := []string{"hourly", "daily", "weekly", "monthly"}
			var labels []string
			for _, v := range vals {
				labels = append(labels, m.T("updates."+v))
			}
			ui.Box(c).Opacity(map[bool]float32{true: 1, false: 0.5}[st.Updates.AutoCheck]).Children(func() {
				m.selectOf(c, st.Updates.Interval, vals, labels, 150, func(v string) { m.set("updates.interval", v) })
			})
		})
		m.setting(c, "install", locales.UpdatesAutoInstall(), locales.UpdatesAutoInstallDesc(), func() {
			v := u.AutomaticDownloads
			if ui.Switch(c.Key("sw"), &v).Disabled(!st.Updates.AutoCheck).Changed() {
				m.run(locales.CommonFailed(), func() error { return UpdateService{a}.SetAutomaticDownloads(v) }, func(error) { m.upd = nil })
			}
		})
		_ = p
	})
}
