package app

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/uwp"
)

// settingsPage is what the settings keep between frames.
type settingsPage struct {
	dialog string  // the dialog open, by name
	tabDir float32 // 1: the tab shown came from further along; -1 back

	// Drafts of the dialogs, saved together.
	bindings  map[string]string
	recording string
	font      string
	sp        config.SystemProxy
	spNow     *string // the system's proxy as it is, described
	spBypass  string
	pacOpen   bool
	pac       *editor.State
	tun       config.Tun
	hijack    string
	exclude   string
	mtu       string
	guard     string
	lan       config.LANAuth
	lanEnable bool
	lanShown  bool
	clash     config.Clash
	ports     map[string]string
	inUse     map[string]bool
	ctrl      config.Controller
	origins   string
	uis       []string
	newUI     string
	dns       *editor.State
	testURL   string
	urlSet    bool
	nums      map[string]string // numbers being typed, by setting

	ifaces   []NetIface
	geo      []GeoFile
	loaded   bool
	geoBusy  bool
	busy     string
	uwpApps  []uwp.StoreApp
	uwpErr   string
	uwpOn    map[string]bool
	uwpQ     string
	autoOpen bool
	autoKnow bool
}

var settingsTabs = []string{"general", "network", "clash", "sync", "advanced"}

// patchPath turns "ui.trafficGraph" and a value into the nested patch the
// settings take.
func patchPath(path string, v any) map[string]any {
	parts := strings.Split(path, ".")
	var out any = v
	for i := len(parts) - 1; i >= 0; i-- {
		out = map[string]any{parts[i]: out}
	}
	return out.(map[string]any)
}

// mergePatch applies a patch to settings as the app will, so that the view
// shows it at once, before the app answers.
func mergePatch(st *config.Settings, patch map[string]any) {
	cur, err := json.Marshal(st)
	if err != nil {
		return
	}
	var m map[string]any
	if json.Unmarshal(cur, &m) != nil {
		return
	}
	var merge func(dst, src map[string]any)
	merge = func(dst, src map[string]any) {
		for k, v := range src {
			if sv, ok := v.(map[string]any); ok {
				if dv, ok := dst[k].(map[string]any); ok {
					merge(dv, sv)
					continue
				}
			}
			dst[k] = v
		}
	}
	// Values of other types (structs, slices of structs) merge as JSON does.
	data, _ := json.Marshal(patch)
	var p map[string]any
	_ = json.Unmarshal(data, &p)
	merge(m, p)
	if data, err := json.Marshal(m); err == nil {
		var next config.Settings
		if json.Unmarshal(data, &next) == nil {
			*st = next
		}
	}
}

// set changes one setting.
func (m *mainUI) set(path string, v any) { m.patch(patchPath(path, v)) }

// section is a titled box of settings.
func (m *mainUI) section(c *ui.Context, key, title string, rows func()) {
	ui.Column(c.Key(key)).Gap(8).Children(func() {
		ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(m.pal.TextMuted).PaddingStart(2)
		m.rows(c, "rows", rows)
	})
}

// swRow is a setting turned on and off.
func (m *mainUI) swRow(c *ui.Context, key, label, desc string, on bool, path string) {
	m.setting(c, key, label, desc, func() {
		v := on
		if ui.Switch(c.Key("sw"), &v).Label(label).Changed() {
			m.set(path, v)
		}
	})
}

// selRow is a setting chosen from a few, values shown by their labels.
func selRow[T comparable](m *mainUI, c *ui.Context, key, label, desc string, cur T, values []T, labels []string, path string, width float32) {
	m.setting(c, key, label, desc, func() {
		m.selectOf(c, cur, values, labels, width, func(v T) { m.set(path, v) })
	})
}

// selectOf is a drop-down of values shown by their labels.
func (m *mainUI) selectOf(c *ui.Context, cur any, values any, labels []string, width float32, pick any) {
	vals := toAnySlice(values)
	i := slices.IndexFunc(vals, func(v any) bool { return v == cur })
	shown := ""
	if i >= 0 {
		shown = labels[i]
	} else if len(labels) > 0 {
		shown = labels[0]
	}
	if ui.Select(c.Key("sel"), &shown, labels).Width(width).Changed() {
		if j := slices.Index(labels, shown); j >= 0 {
			callPick(pick, vals[j])
		}
	}
}

func toAnySlice(v any) []any {
	switch s := v.(type) {
	case []string:
		out := make([]any, len(s))
		for i := range s {
			out[i] = s[i]
		}
		return out
	case []int:
		out := make([]any, len(s))
		for i := range s {
			out[i] = s[i]
		}
		return out
	case []bool:
		out := make([]any, len(s))
		for i := range s {
			out[i] = s[i]
		}
		return out
	}
	return nil
}

func callPick(pick, v any) {
	switch f := pick.(type) {
	case func(string):
		f(v.(string))
	case func(int):
		f(v.(int))
	case func(bool):
		f(v.(bool))
	}
}

// numberIn is a field of a whole number in a range, saved when it is left
// or Enter is pressed.
func (m *mainUI) numberIn(c *ui.Context, key string, cur, lo, hi int, width float32, save func(int)) {
	s := &m.settings
	if s.nums == nil {
		s.nums = map[string]string{}
	}
	text, ok := s.nums[key]
	if !ok {
		text = strconv.Itoa(cur)
	}
	in := ui.TextInput(c.Key("num-"+key), &text).Width(width).FontFeatures("tnum")
	s.nums[key] = text
	if !in.Focused() && !in.Submitted() {
		if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && n != cur && n >= lo && n <= hi {
			save(n)
		}
		if !in.Focused() {
			delete(s.nums, key)
		}
		return
	}
	if in.Submitted() {
		if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil && n >= lo && n <= hi && n != cur {
			save(n)
		}
	}
}

func (m *mainUI) settingsView(c *ui.Context) {
	s, p := &m.settings, m.pal
	conflicts := len(m.sy.Conflicts)
	m.header(c, locales.NavSettings(), nil)
	tab := max(0, slices.Index(settingsTabs, m.tab))
	labels := []string{locales.SettingsTabGeneral(), locales.SettingsTabNetwork(), locales.SettingsTabClash(), locales.SettingsTabSync(), locales.SettingsTabAdvanced()}
	if conflicts > 0 {
		labels[3] += fmt.Sprintf(" (%d)", conflicts)
	}
	// The tabs line up with the column of settings under them.
	ui.Row(c).Padding(0, 22, 6).Shrink(0).Justify(ui.Center).Children(func() {
		ui.Row(c).Grow(1).MaxWidth(formWidth).Children(func() {
			was := tab
			if m.tabs(c, "tabs", &tab, labels...) {
				m.tab = settingsTabs[tab]
				s.tabDir = 1
				if tab < was {
					s.tabDir = -1
				}
			}
		})
	})
	// A tab comes in from the side of the tabs it lies on.
	dx := 14 * s.tabDir
	if c.Direction() == ui.RightToLeft {
		dx = -dx
	}
	host := ui.Box(c.Key("settings-tabs")).Grow(1).Basis(0).MinHeight(0).ChildExit(ui.Motion{X: -dx})
	host.Children(func() {
		ui.Scroll(c.Key("settings-"+m.tab)).TrackScroll(m.keepScroll("settings-"+m.tab)).Absolute().Left(0).Top(0).Right(0).Bottom(0).Padding(8, 22, 26).
			Transition(ui.ElementTransition{Enter: &ui.Motion{X: dx}, Exit: &ui.Motion{X: -dx}, Duration: pageMotion, Ease: easeOut}).Children(func() {
			formColumn(c).Children(func() {
				switch m.tab {
				case "network":
					m.networkTab(c)
				case "clash":
					m.clashTab(c)
				case "sync":
					m.syncTab(c)
				case "advanced":
					m.advancedTab(c)
				default:
					m.generalTab(c)
				}
			})
		})
	})
	_ = p
}

// ---- General ----

// accents are MyGO's own, which follows the theme, then one per member,
// toned to read in both themes.
var accents = []struct{ value, name string }{
	{"", "accent.mygo"}, {"#4fa3d1", "accent.tomori"}, {"#f07891", "accent.anon"},
	{"#4fbf6b", "accent.raana"}, {"#e8b84a", "accent.soyo"}, {"#7d7bc0", "accent.taki"},
}

var languageNames = []struct{ value, label string }{
	{"en", "English"}, {"zh-CN", "简体中文"}, {"zh-TW", "繁體中文"}, {"ja", "日本語"}, {"ko", "한국어"}, {"ru", "Русский"},
	{"es", "Español"}, {"pt-BR", "Português (Brasil)"}, {"de", "Deutsch"}, {"fr", "Français"}, {"tr", "Türkçe"},
	{"id", "Bahasa Indonesia"}, {"vi", "Tiếng Việt"}, {"fa", "فارسی"}, {"ar", "العربية"},
}

func (m *mainUI) generalTab(c *ui.Context) {
	s, p, a, st := &m.settings, m.pal, m.a, m.st
	m.section(c, "interface", locales.SettingsInterface(), func() {
		langs := []string{""}
		names := []string{locales.SettingsFollowSystem()}
		for _, l := range languageNames {
			langs, names = append(langs, l.value), append(names, l.label)
		}
		selRow(m, c, "language", locales.SettingsLanguage(), "", st.Language, langs, names, "language", 190)
		m.setting(c, "theme", locales.SettingsTheme(), "", func() {
			themes := []string{"system", "light", "dark"}
			if i := m.segmented(c, "theme", max(0, slices.Index(themes, st.Theme)), locales.SettingsFollowSystem(), locales.SettingsLight(), locales.SettingsDark()); i >= 0 {
				m.set("theme", themes[i])
			}
		})
		m.setting(c, "oled", locales.SettingsOled(), locales.SettingsOledDesc(), func() {
			v := st.OLED
			if ui.Switch(c.Key("sw"), &v).Disabled(st.Theme == "light").Changed() {
				m.set("oled", v)
			}
		})
		m.setting(c, "accent", locales.SettingsAccent(), "", func() {
			ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
				for _, ac := range accents {
					col := p.Accent
					if v, ok := parseHex(ac.value); ok {
						col = v
					} else {
						col = mainPalette(p.Dark, false, "", "").Accent
					}
					on := strings.EqualFold(st.Accent, ac.value)
					sw := ui.ButtonBase(c.Key(ac.name)).Size(22, 22).Radius(11).Background(col).Tooltip(m.T(ac.name)).Label(m.T(ac.name))
					pressable(sw)
					if on {
						sw.Border(2, p.Text)
					} else if sw.Hovered() {
						sw.Scale(1.1, 1.1)
					}
					if sw.Clicked() {
						m.set("accent", ac.value)
					}
				}
				cur, _ := parseHex(cmp.Or(st.Accent, "#0b88bb"))
				pick := cur
				if ui.ColorWell(c.Key("custom"), &pick).Tooltip(locales.AccentCustom()).Changed() {
					m.set("accent", fmt.Sprintf("#%02x%02x%02x", pick.R, pick.G, pick.B))
				}
			})
		})
		names = nil
		for _, pg := range pages {
			names = append(names, m.T("nav."+pg))
		}
		selRow(m, c, "start", locales.SettingsStartPage(), "", st.StartPage, pages, names, "startPage", 150)
		m.setting(c, "layout", locales.SettingsLayout(), locales.SettingsLayoutDesc(), func() {
			if m.button(c, "edit", locales.CommonEdit(), nil, "").Clicked() {
				s.dialog = "layout"
			}
		})
		m.setting(c, "font", locales.SettingsFont(), locales.SettingsAppearanceDesc(), func() {
			if s.dialog != "font" {
				s.font = st.FontFamily
			}
			in := ui.TextInput(c.Key("font"), &s.font).Placeholder("Inter, system-ui").Width(220)
			if in.Focused() {
				s.dialog = "font"
			} else if s.dialog == "font" {
				s.dialog = ""
				if s.font != st.FontFamily {
					m.set("fontFamily", s.font)
				}
			}
		})
	})
	m.section(c, "startup", locales.SettingsStartup(), func() {
		if !s.autoKnow {
			s.autoKnow, s.autoOpen = true, mygo.App.OpenAtLogin()
		}
		m.setting(c, "autolaunch", locales.SettingsAutoLaunch(), locales.SettingsAutoLaunchDesc(), func() {
			v := s.autoOpen
			if ui.Switch(c.Key("sw"), &v).Changed() {
				s.autoOpen = v
				m.run(locales.SettingsSaveFailed(), func() error { return mygo.App.SetOpenAtLogin(v) }, func(err error) {
					if err != nil {
						s.autoOpen = mygo.App.OpenAtLogin()
					}
				})
			}
		})
		m.swRow(c, "silent", locales.SettingsSilentStart(), locales.SettingsSilentStartDesc(), st.SilentStart, "silentStart")
		m.swRow(c, "notify", locales.SettingsNotifications(), locales.SettingsNotificationsDesc(), st.Notifications, "notifications")
	})
	m.section(c, "subs", locales.SubsTitle(), func() {
		m.swRow(c, "quota", locales.SubsQuotaAlert(), locales.SubsQuotaAlertDesc(), st.Subscriptions.QuotaAlert, "subscriptions.quotaAlert")
		m.collapseRows(c, "quota-more", st.Subscriptions.QuotaAlert, func() {
			selRow(m, c, "pct", locales.SubsQuotaPercent(), "", st.Subscriptions.QuotaPercent, []int{5, 10, 20, 30}, []string{"5%", "10%", "20%", "30%"}, "subscriptions.quotaPercent", 110)
			days := []int{1, 3, 7, 14}
			var labels []string
			for _, d := range days {
				labels = append(labels, locales.SubsDays(fmt.Sprint(d)))
			}
			selRow(m, c, "expire", locales.SubsExpireDays(), "", st.Subscriptions.ExpireDays, days, labels, "subscriptions.expireDays", 110)
		})
		m.swRow(c, "rollback", locales.SubsRollback(), locales.SubsRollbackDesc(), st.Subscriptions.Rollback, "subscriptions.rollback")
	})
	m.section(c, "tray", locales.SettingsTray(), func() {
		selRow(m, c, "click", locales.SettingsTrayClick(), "", st.TrayClick, []string{"menu", "panel", "window", "none"},
			[]string{locales.SettingsTrayMenu(), locales.SettingsTrayPanel(), locales.SettingsTrayWindow(), locales.CommonNone()}, "trayClick", 170)
		selRow(m, c, "groups", locales.SettingsTrayGroups(), "", st.Tray.Groups, []string{"submenu", "inline", "off"},
			[]string{locales.SettingsTraySubmenu(), locales.SettingsTrayInline(), locales.SettingsTrayOff()}, "tray.groups", 170)
		m.swRow(c, "modes", locales.SettingsTrayModes(), locales.SettingsTrayModesDesc(), st.Tray.InlineModes, "tray.inlineModes")
		if m.info.OS == "darwin" {
			m.swRow(c, "speed", locales.SettingsTraySpeed(), locales.SettingsTraySpeedDesc(), st.Tray.ShowSpeed, "tray.showSpeed")
		}
	})
	m.section(c, "shortcuts", locales.SettingsShortcuts(), func() {
		m.setting(c, "hotkeys", locales.SettingsHotkeysEnabled(), locales.SettingsHotkeysDesc(), func() {
			if m.button(c, "edit", locales.CommonEdit(), nil, "").Clicked() {
				s.dialog, s.bindings, s.recording = "hotkeys", map[string]string{}, ""
				for k, v := range st.Hotkeys.Bindings {
					s.bindings[k] = v
				}
			}
			v := st.Hotkeys.Enabled
			if ui.Switch(c.Key("sw"), &v).Changed() {
				m.set("hotkeys.enabled", v)
			}
		})
		m.setting(c, "env", locales.SettingsCopyEnv(), locales.SettingsCopyEnvDesc(), func() {
			m.selectOf(c, st.CopyEnvType, []string{"posix", "fish", "powershell", "cmd", "nushell"}, []string{"bash / zsh", "fish", "PowerShell", "cmd", "Nushell"}, 140, func(v string) { m.set("copyEnvType", v) })
			if m.button(c, "copy", locales.CommonCopy(), nil, "").Clicked() {
				cmd := (AppService{a}).CopyEnv()
				m.notices = append(m.notices, Notice{Level: "success", Message: locales.CommonCopied(), Detail: cmd})
			}
		})
	})
	m.layoutDialog(c)
	m.hotkeysDialog(c)
}

func (m *mainUI) layoutDialog(c *ui.Context) {
	s, st := &m.settings, m.st
	open := s.dialog == "layout"
	m.modal(c, "layout", &open, func() {
		ui.Column(c).Width(520).MaxHeight(620).Gap(12).Children(func() {
			ui.Text(c, locales.SettingsLayout()).FontSize(15).FontWeight(650)
			gutter(ui.Scroll(c).Grow(1).Gap(12), 14).Children(func() {
				m.rows(c, "rows", func() {
					m.swRow(c, "graph", locales.SettingsTrafficGraph(), locales.SettingsTrafficGraphDesc(), st.UI.TrafficGraph, "ui.trafficGraph")
					m.swRow(c, "mem", locales.SettingsMemoryUsage(), locales.SettingsMemoryUsageDesc(), st.UI.MemoryUsage, "ui.memoryUsage")
					m.swRow(c, "icons", locales.SettingsGroupIcons(), locales.SettingsGroupIconsDesc(), st.UI.GroupIcons, "ui.groupIcons")
					m.swRow(c, "nav", locales.SettingsCollapseNav(), "", st.UI.CollapseNav, "ui.collapseNav")
					cols := []int{0, 1, 2, 3, 4, 5, 6}
					labels := []string{locales.SettingsAuto(), "1", "2", "3", "4", "5", "6"}
					selRow(m, c, "cols", locales.SettingsProxyColumns(), locales.SettingsProxyColumnsHint(), st.UI.ProxyColumns, cols, labels, "ui.proxyColumns", 110)
					m.swRow(c, "pause", locales.SettingsPauseOnBlur(), locales.SettingsPauseOnBlurDesc(), st.UI.PauseOnBlur, "ui.pauseOnBlur")
				})
				ui.Text(c, locales.SettingsNavItems()).FontSize(12).FontWeight(600).TextColor(m.pal.TextMuted)
				// The navigation shows places; the setting keeps the pages
				// they hold, by the names they always had.
				nav := m.navPages()
				ui.Row(c).Wrap().Gap(6).Children(func() {
					for _, pl := range places {
						on := slices.ContainsFunc(placePages(pl), func(pg string) bool { return slices.Contains(nav, pg) })
						chip := m.chip(c, pl, m.T("nav."+pl), on).Disabled(pl == "settings")
						if chip.Clicked() {
							var next []string
							for _, x := range pages {
								mine, _ := placeOf(x)
								if mine == pl && on {
									continue
								}
								if slices.Contains(nav, x) || mine == pl {
									next = append(next, x)
								}
							}
							m.set("ui.nav", next)
						}
					}
				})
			})
			ui.Row(c).Justify(ui.End).Children(func() {
				if m.button(c, "done", locales.CommonDone(), nil, "primary").Clicked() {
					s.dialog = ""
				}
			})
		})
	})
	if !open && s.dialog == "layout" {
		s.dialog = ""
	}
}

var hotkeyActions = []string{"toggle-window", "quick-panel", "toggle-system-proxy", "toggle-tun", "mode-rule", "mode-global", "mode-direct", "lightweight", "reactivate-profile"}

// keyName names a key as accelerators write it.
func keyName(k ui.Key) string {
	switch {
	case k >= ui.KeyA && k <= ui.KeyA+25:
		return string(rune('A' + int(k-ui.KeyA)))
	case k >= ui.Key0 && k <= ui.Key0+9:
		return string(rune('0' + int(k-ui.Key0)))
	case k >= ui.KeyF1 && k <= ui.KeyF1+11:
		return fmt.Sprintf("F%d", int(k-ui.KeyF1)+1)
	}
	names := map[ui.Key]string{ui.KeySpace: "Space", ui.KeyEnter: "Enter", ui.KeyTab: "Tab", ui.KeyUp: "Up", ui.KeyDown: "Down",
		ui.KeyLeft: "Left", ui.KeyRight: "Right", ui.KeyHome: "Home", ui.KeyEnd: "End", ui.KeyPageUp: "PageUp", ui.KeyPageDown: "PageDown",
		ui.KeyMinus: "-", ui.KeyEqual: "=", ui.KeyComma: ",", ui.KeyPeriod: ".", ui.KeySlash: "/", ui.KeySemicolon: ";",
		ui.KeyQuote: "'", ui.KeyBracketLeft: "[", ui.KeyBracketRight: "]", ui.KeyBackslash: "\\", ui.KeyBackquote: "`"}
	return names[k]
}

func (m *mainUI) hotkeysDialog(c *ui.Context) {
	s, p := &m.settings, m.pal
	open := s.dialog == "hotkeys"
	m.modal(c, "hotkeys", &open, func() {
		ui.Column(c).Width(480).Gap(12).Children(func() {
			ui.Text(c, locales.SettingsHotkeys()).FontSize(15).FontWeight(650)
			ui.Text(c, locales.SettingsHotkeysHint()).FontSize(12.5).TextColor(p.TextMuted).Wrap()
			m.rows(c, "rows", func() {
				for _, act := range hotkeyActions {
					m.setting(c, act, m.T("hotkey."+act), "", func() {
						rec := s.recording == act
						label := cmp.Or(s.bindings[act], locales.SettingsNotSet())
						if rec {
							label = locales.SettingsPressKeys()
						}
						b := ui.ButtonBase(c.Key("rec")).Height(28).MinWidth(140).Padding(0, 10).Radius(14).Border(1, p.Border).AlignItems(ui.Center).Justify(ui.Center).Focusable()
						pressable(b)
						if rec {
							b.Background(p.AccentSoft).Border(1, p.Accent).AutoFocus()
						}
						b.Children(func() { ui.Text(c, label).FontSize(12).Font("ui-monospace") })
						if b.Clicked() {
							s.recording = act
						}
						if rec {
							b.HandleInput(func(ev ui.InputEvent) bool {
								if ev.Kind != ui.InputKeyDown {
									return false
								}
								switch ev.Key {
								case ui.KeyEscape:
									s.recording = ""
									return true
								case ui.KeyBackspace, ui.KeyDelete:
									s.bindings[act], s.recording = "", ""
									return true
								}
								var mods []string
								if ev.Mods&(ui.Super|ui.Ctrl) != 0 {
									mods = append(mods, "CmdOrCtrl")
								}
								if ev.Mods&ui.Alt != 0 {
									mods = append(mods, "Alt")
								}
								if ev.Mods&ui.Shift != 0 {
									mods = append(mods, "Shift")
								}
								name := keyName(ev.Key)
								if len(mods) == 0 || name == "" {
									return true
								}
								s.bindings[act], s.recording = strings.Join(append(mods, name), "+"), ""
								return true
							})
						}
					})
				}
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if m.button(c, "cancel", locales.CommonCancel(), nil, "").Clicked() {
					s.dialog = ""
				}
				if m.button(c, "save", locales.CommonSave(), nil, "primary").Clicked() {
					m.set("hotkeys.bindings", s.bindings)
					s.dialog = ""
				}
			})
		})
	})
	if !open && s.dialog == "hotkeys" {
		s.dialog = ""
	}
}

// ---- Network ----

func splitList(v string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' }) {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

func (m *mainUI) networkTab(c *ui.Context) {
	s, p, a, st, state := &m.settings, m.pal, m.a, m.st, m.state
	if state.SystemProxyError != "" {
		m.banner(c, "danger", state.SystemProxyError, nil)
	}
	m.section(c, "proxy", locales.SettingsProxy(), func() {
		m.setting(c, "sys", locales.SettingsSystemProxy(), locales.SettingsSystemProxyDesc(), func() {
			if m.button(c, "conf", locales.CommonConfigure(), nil, "").Clicked() {
				s.dialog, s.sp, s.spNow = "sysproxy", st.SystemProxy, nil
				s.guard, s.spBypass = strconv.Itoa(st.SystemProxy.GuardInterval), st.SystemProxy.Bypass
				win := m.window()
				go func() {
					cur, err := (SettingsService{a}).SystemProxy()
					text := locales.CommonOff()
					switch {
					case err != nil:
						text = err.Error()
					case cur.Enabled && cur.PAC != "":
						text = locales.CommonOn() + " · PAC " + cur.PAC
					case cur.Enabled:
						text = fmt.Sprintf("%s · %s:%d", locales.CommonOn(), cur.Host, cur.Port)
					}
					win.Update(func() { s.spNow = &text })
				}()
			}
			v := st.SystemProxy.Enabled
			if s.busy == "sys" {
				ui.Spinner(c)
			} else if ui.Switch(c.Key("sw"), &v).Changed() {
				s.busy = "sys"
				m.run(locales.SettingsSaveFailed(), func() error {
					_, err := (SettingsService{a}).Patch(context.Background(), patchPath("systemProxy.enabled", v))
					return err
				}, func(error) { s.busy = "" })
			}
		})
		desc := locales.SettingsTunNeedsService()
		switch {
		case state.TunAvailable:
			desc = locales.SettingsTunDesc()
		case state.Service.Outdated:
			desc = locales.HomeTunServiceOutdated()
		}
		m.setting(c, "tun", locales.SettingsTun(), desc, func() {
			if m.button(c, "conf", locales.CommonConfigure(), nil, "").Clicked() {
				s.dialog, s.tun = "tun", st.Tun
				s.hijack, s.exclude, s.mtu = strings.Join(st.Tun.DNSHijack, ", "), strings.Join(st.Tun.RouteExcludeAddress, "\n"), strconv.Itoa(st.Tun.MTU)
			}
			v := st.Tun.Enabled && state.TunAvailable
			if s.busy == "tun" {
				ui.Spinner(c)
			} else if ui.Switch(c.Key("sw"), &v).Changed() {
				if v && !state.TunAvailable {
					go a.installServiceForTun()
				} else {
					s.busy = "tun"
					m.run(locales.SettingsSaveFailed(), func() error {
						_, err := (SettingsService{a}).Patch(context.Background(), patchPath("tun.enabled", v))
						return err
					}, func(error) { s.busy = "" })
				}
			}
		})
		m.swRow(c, "lan", locales.SettingsAllowLan(), locales.SettingsAllowLanDesc(), st.Clash.AllowLAN, "clash.allowLan")
		m.collapseRows(c, "lanauth", st.Clash.AllowLAN, func() {
			m.setting(c, "lanauth", locales.SettingsLanAuth(), locales.SettingsLanAuthDesc(), func() {
				if m.button(c, "conf", locales.CommonConfigure(), nil, "").Clicked() {
					s.dialog, s.lan, s.lanEnable, s.lanShown = "lanauth", st.Clash.LANAuth, false, false
				}
				v := st.Clash.LANAuth.Enabled
				if ui.Switch(c.Key("sw"), &v).Changed() {
					// Without a login there is nothing to ask for yet.
					if v && (st.Clash.LANAuth.Username == "" || st.Clash.LANAuth.Password == "") {
						s.dialog, s.lan, s.lanEnable, s.lanShown = "lanauth", st.Clash.LANAuth, true, false
					} else {
						m.set("clash.lanAuth.enabled", v)
					}
				}
			})
		})
	})
	m.section(c, "wifi", locales.SettingsWifi(), func() {
		rules := st.WiFiRules
		save := func(next []config.WiFiRule) { m.set("wifiRules", next) }
		for i := range rules {
			r := rules[i]
			ui.Column(c.Key(i)).Padding(10, 14).Gap(8).Children(func() {
				ui.Row(c).Gap(8).Children(func() {
					ssid := r.SSID
					in := ui.TextInput(c.Key("ssid"), &ssid).Placeholder(locales.SettingsWifiSSID()).Grow(1)
					if ssid != r.SSID {
						next := slices.Clone(rules)
						next[i].SSID = ssid
						if in.Submitted() || !in.Focused() {
							save(next)
						} else {
							m.st.WiFiRules = next
						}
					} else if !in.Focused() && ssid != st.WiFiRules[i].SSID {
						save(rules)
					}
					if m.iconButton(c, "del", icTrash2, locales.CommonDelete()).Clicked() {
						save(slices.Delete(slices.Clone(rules), i, i+1))
					}
				})
				ui.Row(c).Gap(8).Wrap().Children(func() {
					uids := []string{""}
					names := []string{locales.SettingsWifiProfile()}
					for _, pr := range m.pv.Items {
						uids, names = append(uids, pr.UID), append(names, pr.Name)
					}
					m.selectOf(c, r.ProfileUID, uids, names, 170, func(v string) {
						next := slices.Clone(rules)
						next[i].ProfileUID, next[i].ProfileName = v, ""
						if j := slices.Index(uids, v); j > 0 {
							next[i].ProfileName = names[j]
						}
						save(next)
					})
					tri := func(key, label string, cur *bool, put func(*config.WiFiRule, *bool)) {
						vals := []string{"keep", "on", "off"}
						labels := []string{label + " · " + locales.SettingsWifiKeep(), label + " · " + locales.SettingsWifiOn(), label + " · " + locales.SettingsWifiOff()}
						now := "keep"
						if cur != nil {
							now = map[bool]string{true: "on", false: "off"}[*cur]
						}
						ui.Box(c.Key(key)).Children(func() {
							m.selectOf(c, now, vals, labels, 190, func(v string) {
								next := slices.Clone(rules)
								var b *bool
								if v != "keep" {
									on := v == "on"
									b = &on
								}
								put(&next[i], b)
								save(next)
							})
						})
					}
					tri("sys", locales.SettingsWifiSys(), r.SysProxy, func(w *config.WiFiRule, b *bool) { w.SysProxy = b })
					tri("tun", locales.SettingsWifiTun(), r.Tun, func(w *config.WiFiRule, b *bool) { w.Tun = b })
					ui.Box(c.Key("mode")).Children(func() {
						m.selectOf(c, r.Mode, []string{"", "rule", "global", "direct"},
							[]string{locales.SettingsWifiMode() + " · " + locales.SettingsWifiKeep(), locales.ModeRule(), locales.ModeGlobal(), locales.ModeDirect()}, 190, func(v string) {
								next := slices.Clone(rules)
								next[i].Mode = v
								save(next)
							})
					})
				})
			})
		}
		m.setting(c, "add", locales.SettingsWifiDesc(), "", func() {
			if m.button(c, "add", locales.SettingsWifiAdd(), icPlus, "ghost").Clicked() {
				save(append(slices.Clone(rules), config.WiFiRule{}))
			}
		})
	})
	if m.info.OS == "windows" {
		m.section(c, "leak", locales.SettingsLeakProtection(), func() {
			m.swRow(c, "dns", locales.SettingsDnsLeak(), locales.SettingsDnsLeakDesc(), st.Tun.StrictRoute, "tun.strictRoute")
			m.swRow(c, "webrtc", locales.SettingsWebrtcGuard(), locales.SettingsWebrtcGuardDesc(), st.WebRTCGuard, "webrtcGuard")
		})
	}
	if svc := state.Service; svc.Supported {
		m.section(c, "service", locales.ServiceTitle(), func() {
			desc := locales.ServiceNotInstalled()
			switch {
			case svc.Installed && svc.Error != "":
				desc = svc.Error
			case svc.Installed && svc.Outdated:
				desc = locales.ServiceOutdated(cmp.Or(svc.Version, "?"))
			case svc.Installed:
				desc = locales.ServiceOk(svc.Version)
			}
			m.setting(c, "status", locales.ServiceStatus(), desc, func() {
				act := func(key, ok, fail string, fn func() error) {
					s.busy = key
					m.run(fail, fn, func(err error) {
						s.busy = ""
						if err == nil {
							m.notices = append(m.notices, Notice{Level: "success", Message: ok})
						}
					})
				}
				if strings.HasPrefix(s.busy, "svc") {
					ui.Spinner(c)
					return
				}
				if !svc.Installed {
					if m.button(c, "install", locales.ServiceInstall(), icShieldCheck, "primary").Clicked() {
						act("svc-install", locales.ServiceInstalled(), locales.ServiceInstallFailed(), func() error { return System{a}.InstallService(context.Background(), false) })
					}
					return
				}
				label := locales.ServiceRepair()
				if svc.Outdated || svc.Behind {
					label = locales.ServiceUpdate()
				}
				if m.button(c, "repair", label, icWrench, "").Clicked() {
					act("svc-repair", locales.ServiceInstalled(), locales.ServiceInstallFailed(), func() error { return System{a}.InstallService(context.Background(), false) })
				}
				if m.button(c, "remove", locales.ServiceUninstall(), nil, "danger").Clicked() {
					m.confirm(locales.ServiceUninstall(), locales.ServiceUninstallMsg(), locales.ServiceUninstall(), func() {
						act("svc-remove", locales.ServiceUninstalled(), locales.ServiceUninstallFailed(), func() error { return System{a}.UninstallService(context.Background()) })
					})
				}
			})
			desc = locales.ServiceCoreInSidecar()
			if state.Core.Mode == "service" {
				desc = locales.ServiceCoreInService()
			}
			m.setting(c, "core", locales.ServiceCoreRuns(), desc, func() {
				if state.Core.Privileged {
					m.stateBadge(c, "priv", locales.ServicePrivileged(), "success")
				} else {
					m.stateBadge(c, "priv", locales.ServiceUnprivileged(), "")
				}
			})
		})
	}
	m.sysProxyDialog(c)
	m.tunDialog(c)
	m.lanAuthDialog(c)
	_ = p
}

// dialogFrame is a dialog of settings: its title, its body, and Cancel and
// Save, or Done alone when save is nil.
func (m *mainUI) dialogFrame(c *ui.Context, key, title string, width float32, body func(), save func() bool) {
	s := &m.settings
	open := s.dialog == key
	m.modal(c, "dlg-"+key, &open, func() {
		ui.Column(c).Width(width).MaxHeight(660).Gap(14).Children(func() {
			ui.Text(c, title).FontSize(15).FontWeight(650)
			// Grow alone: the column is as tall as its content, up to its
			// maximum, and a body told to start from nothing (Basis 0)
			// has no room to grow into there and shows as an empty dialog.
			gutter(ui.Scroll(c).Grow(1).Gap(14), 14).Children(body)
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if save == nil {
					if m.button(c, "done", locales.CommonDone(), nil, "primary").Clicked() {
						s.dialog = ""
					}
					return
				}
				if m.button(c, "cancel", locales.CommonCancel(), nil, "").Clicked() {
					s.dialog = ""
				}
				if m.button(c, "save", locales.CommonSave(), nil, "primary").Clicked() && save() {
					s.dialog = ""
				}
			})
		})
	})
	if !open && s.dialog == key {
		s.dialog = ""
	}
}

func (m *mainUI) sysProxyDialog(c *ui.Context) {
	s, p, a := &m.settings, m.pal, m.a
	m.dialogFrame(c, "sysproxy", locales.SettingsSysproxySettings(), 620, func() {
		ui.Column(c).Padding(12, 14).Gap(6).Radius(8).Background(p.Surface2).Children(func() {
			ui.Text(c, locales.SettingsOsProxyNow()).FontSize(12).FontWeight(600).TextColor(p.TextMuted)
			if s.spNow == nil {
				ui.Spinner(c)
			} else {
				ui.Text(c, *s.spNow).FontSize(12.5).Font("ui-monospace").Wrap()
			}
		})
		ui.Row(c).Gap(12).Children(func() {
			m.rowField(c, locales.SettingsProxyHost(), locales.SettingsProxyHostHint(), "", func() { ui.TextInput(c.Key("host"), &s.sp.Host) })
			m.rowField(c, locales.SettingsGuardInterval(), locales.SettingsGuardHint(), "", func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Switch(c.Key("guard"), &s.sp.Guard)
					ui.TextInput(c.Key("interval"), &s.guard).Width(90)
					ui.Text(c, locales.CommonSeconds()).TextColor(p.TextMuted)
				})
			})
		})
		m.rows(c, "opts", func() {
			m.setting(c, "pac", locales.SettingsPacMode(), locales.SettingsPacDesc(), func() {
				if m.button(c, "edit", locales.SettingsEditPac(), nil, "").Clicked() {
					s.pacOpen = true
					s.pac = newCode(s.sp.PACScript, "proxy.pac.js", false)
				}
				ui.Switch(c.Key("sw"), &s.sp.PAC)
			})
			m.setting(c, "def", locales.SettingsDefaultBypass(), strings.Join((SettingsService{a}).DefaultBypass(), ", "), func() {
				ui.Switch(c.Key("sw"), &s.sp.UseDefaultBypass)
			})
		})
		m.field(c, locales.SettingsBypass(), locales.SettingsBypassHint(), "", func() {
			ui.TextArea(c.Key("bypass"), &s.spBypass).Placeholder("*.example.com, 10.0.0.0/8").Font("ui-monospace").Height(80)
		})
		if s.pacOpen && s.pac != nil {
			m.modal(c, "pac", &s.pacOpen, func() {
				ui.Column(c).Width(640).Height(460).Gap(10).Children(func() {
					ui.Text(c, locales.SettingsEditPac()).FontSize(15).FontWeight(650)
					ui.Column(c).Grow(1).Radius(8).Border(1, p.Border).Clip().Children(func() { editor.View(c.Key("ed"), s.pac).Grow(1).FillWidth() })
					ui.Text(c, locales.SettingsPacPlaceholders()).FontSize(12).TextColor(p.TextMuted).Wrap()
					ui.Row(c).Justify(ui.End).Children(func() {
						if m.button(c, "done", locales.CommonDone(), nil, "primary").Clicked() {
							s.sp.PACScript, s.pacOpen = s.pac.Buffer.String(), false
						}
					})
				})
			})
		}
	}, func() bool {
		sp := s.sp
		sp.Enabled = m.st.SystemProxy.Enabled
		sp.Bypass = s.spBypass
		if n, err := strconv.Atoi(strings.TrimSpace(s.guard)); err == nil && n > 0 {
			sp.GuardInterval = n
		}
		m.set("systemProxy", sp)
		return true
	})
}

func (m *mainUI) tunDialog(c *ui.Context) {
	s, p := &m.settings, m.pal
	m.dialogFrame(c, "tun", locales.SettingsTunSettings(), 620, func() {
		m.field(c, locales.SettingsStack(), locales.SettingsStackHint(), "", func() {
			stacks := []string{"mixed", "gvisor", "system"}
			ui.Row(c).Children(func() {
				if i := m.segmented(c, "stack", max(0, slices.Index(stacks, s.tun.Stack)), "Mixed", "gVisor", "System"); i >= 0 {
					s.tun.Stack = stacks[i]
				}
			})
		})
		ui.Row(c).Gap(12).Children(func() {
			ph := "mygo0"
			switch m.info.OS {
			case "darwin":
				ph = "utun1024"
			case "windows":
				ph = "MyGO"
			}
			m.rowField(c, locales.SettingsDevice(), locales.SettingsDeviceHint(), "", func() { ui.TextInput(c.Key("dev"), &s.tun.Device).Placeholder(ph) })
			m.rowField(c, "MTU", "", "", func() { ui.TextInput(c.Key("mtu"), &s.mtu).Width(120) })
		})
		m.rows(c, "opts", func() {
			m.setting(c, "route", locales.SettingsAutoRoute(), locales.SettingsAutoRouteDesc(), func() { ui.Switch(c.Key("sw"), &s.tun.AutoRoute) })
			m.setting(c, "strict", locales.SettingsStrictRoute(), locales.SettingsStrictRouteDesc(), func() { ui.Switch(c.Key("sw"), &s.tun.StrictRoute) })
			m.setting(c, "detect", locales.SettingsAutoDetect(), locales.SettingsAutoDetectDesc(), func() { ui.Switch(c.Key("sw"), &s.tun.AutoDetectInterface) })
			if m.info.OS == "linux" {
				m.setting(c, "redirect", locales.SettingsAutoRedirect(), locales.SettingsAutoRedirectDesc(), func() { ui.Switch(c.Key("sw"), &s.tun.AutoRedirect) })
			}
		})
		m.field(c, locales.SettingsDnsHijack(), locales.SettingsDnsHijackHint(), "", func() { ui.TextInput(c.Key("hijack"), &s.hijack).Font("ui-monospace") })
		m.field(c, locales.SettingsExcludeRoutes(), locales.SettingsExcludeRoutesHint(), "", func() {
			ui.TextArea(c.Key("exclude"), &s.exclude).Placeholder("192.168.0.0/16").Font("ui-monospace").Height(70)
		})
		_ = p
	}, func() bool {
		t := s.tun
		t.Enabled = m.st.Tun.Enabled
		t.DNSHijack, t.RouteExcludeAddress = splitList(s.hijack), splitList(s.exclude)
		if n, err := strconv.Atoi(strings.TrimSpace(s.mtu)); err == nil && n >= 576 && n <= 65535 {
			t.MTU = n
		}
		m.set("tun", t)
		return true
	})
}

func (m *mainUI) lanAuthDialog(c *ui.Context) {
	s, a := &m.settings, m.a
	invalid := strings.TrimSpace(s.lan.Username) == "" || s.lan.Password == "" || strings.Contains(s.lan.Username, ":")
	m.dialogFrame(c, "lanauth", locales.SettingsLanAuthTitle(), 440, func() {
		errText := ""
		if strings.Contains(s.lan.Username, ":") {
			errText = ":"
		}
		m.field(c, locales.SettingsLanAuthUser(), "", errText, func() { ui.TextInput(c.Key("user"), &s.lan.Username).Font("ui-monospace") })
		m.field(c, locales.SettingsLanAuthPass(), locales.SettingsLanAuthHint(), "", func() {
			ui.Row(c).Gap(6).Children(func() {
				in := ui.TextInput(c.Key("pass"), &s.lan.Password).Font("ui-monospace").Grow(1)
				if !s.lanShown {
					in.Password()
				}
				icon, tip := icEye, locales.CommonShow()
				if s.lanShown {
					icon, tip = icEyeOff, locales.CommonHide()
				}
				if m.iconButton(c, "eye", icon, tip).Size(32, 32).Clicked() {
					s.lanShown = !s.lanShown
				}
				if m.iconButton(c, "dice", icDices, locales.SettingsLanAuthRandom()).Size(32, 32).Clicked() {
					s.lan.Password, s.lanShown = (SettingsService{a}).RandomSecret(), true
				}
			})
		})
	}, func() bool {
		if invalid {
			return false
		}
		l := s.lan
		l.Username = strings.TrimSpace(l.Username)
		l.Enabled = l.Enabled || s.lanEnable
		m.set("clash.lanAuth", l)
		return true
	})
}

// ---- The core ----

func (m *mainUI) clashTab(c *ui.Context) {
	s, p, a, st := &m.settings, m.pal, m.a, m.st
	cl := st.Clash
	if !s.loaded {
		s.loaded = true
		win := m.window()
		go func() {
			ifaces := (AppService{a}).NetworkInterfaces()
			geo, _ := (Core{a}).GeoFiles(context.Background())
			win.Update(func() { s.ifaces, s.geo = ifaces, geo })
		}()
	}
	act := func(key, ok string, fn func() error) {
		s.busy = key
		m.run(locales.CommonFailed(), fn, func(err error) {
			s.busy = ""
			if err == nil {
				m.notices = append(m.notices, Notice{Level: "success", Message: ok})
			}
		})
	}
	m.section(c, "core", locales.SettingsCore(), func() {
		ports := []string{fmt.Sprintf("mixed %d", cl.MixedPort)}
		if cl.SocksEnabled {
			ports = append(ports, fmt.Sprintf("socks %d", cl.SocksPort))
		}
		if cl.HTTPEnabled {
			ports = append(ports, fmt.Sprintf("http %d", cl.HTTPPort))
		}
		if cl.RedirEnabled && m.info.OS != "windows" {
			ports = append(ports, fmt.Sprintf("redir %d", cl.RedirPort))
		}
		if cl.TProxyEnabled && m.info.OS == "linux" {
			ports = append(ports, fmt.Sprintf("tproxy %d", cl.TProxyPort))
		}
		m.setting(c, "ports", locales.SettingsPorts(), strings.Join(ports, " · "), func() {
			if m.button(c, "edit", locales.CommonEdit(), nil, "").Clicked() {
				s.dialog, s.clash, s.inUse = "ports", cl, map[string]bool{}
				s.ports = map[string]string{"mixed": strconv.Itoa(cl.MixedPort), "socks": strconv.Itoa(cl.SocksPort), "http": strconv.Itoa(cl.HTTPPort),
					"redir": strconv.Itoa(cl.RedirPort), "tproxy": strconv.Itoa(cl.TProxyPort)}
			}
		})
		m.swRow(c, "ipv6", "IPv6", locales.SettingsIpv6Desc(), cl.IPv6, "clash.ipv6")
		m.swRow(c, "unified", locales.SettingsUnifiedDelay(), locales.SettingsUnifiedDelayDesc(), cl.UnifiedDelay, "clash.unifiedDelay")
		m.swRow(c, "tcpc", locales.SettingsTcpConcurrent(), locales.SettingsTcpConcurrentDesc(), cl.TCPConcurrent, "clash.tcpConcurrent")
		selRow(m, c, "find", locales.SettingsFindProcess(), locales.SettingsFindProcessDesc(), cl.FindProcessMode, []string{"", "strict", "always", "off"},
			[]string{locales.SettingsFollowProfile(), locales.SettingsFindProcessStrict(), locales.SettingsFindProcessAlways(), locales.SettingsFindProcessOff()}, "clash.findProcessMode", 150)
		names := []string{""}
		labels := []string{locales.SettingsAuto()}
		for _, i := range s.ifaces {
			names = append(names, i.Name)
			l := i.Name
			if len(i.Addrs) > 0 {
				l += " " + i.Addrs[0]
			}
			labels = append(labels, l)
		}
		selRow(m, c, "iface", locales.SettingsInterface(), locales.SettingsInterfaceDesc(), cl.Interface, names, labels, "clash.interface", 200)
		levels := []string{"debug", "info", "warning", "error", "silent"}
		labels = nil
		for _, l := range levels {
			labels = append(labels, m.T("logs."+l))
		}
		selRow(m, c, "level", locales.SettingsLogLevel(), locales.SettingsLogLevelDesc(), cl.LogLevel, levels, labels, "clash.logLevel", 130)
		desc := locales.SettingsControllerOff()
		if cl.Controller.Enabled {
			desc = cl.Controller.Address
		}
		m.setting(c, "ctrl", locales.SettingsController(), desc, func() {
			if m.button(c, "conf", locales.CommonConfigure(), nil, "").Clicked() {
				s.dialog, s.ctrl, s.origins, s.uis, s.newUI = "controller", cl.Controller, strings.Join(cl.Controller.AllowOrigins, "\n"), slices.Clone(st.WebUIs), ""
			}
		})
	})
	m.section(c, "geo", locales.SettingsGeo(), func() {
		selRow(m, c, "src", locales.SettingsGeoSource(), locales.SettingsGeoSourceDesc(), cl.GeoMirror, []bool{true, false}, []string{locales.SettingsGeoMirror(), locales.SettingsGeoOfficial()}, "clash.geoMirror", 170)
		m.swRow(c, "auto", locales.SettingsGeoAuto(), locales.SettingsGeoAutoDesc(), cl.GeoAutoUpdate, "clash.geoAutoUpdate")
		m.collapseRows(c, "geo-interval", cl.GeoAutoUpdate, func() {
			m.setting(c, "interval", locales.SettingsGeoInterval(), locales.SettingsGeoIntervalDesc(), func() {
				m.numberIn(c, "geo", cmp.Or(cl.GeoUpdateInterval, 24), 1, 168, 110, func(n int) { m.set("clash.geoUpdateInterval", n) })
			})
		})
		var files []string
		for _, f := range s.geo {
			files = append(files, f.Name+" · "+fmtBytes(f.Size)+" · "+m.relTime(time.Unix(f.Updated, 0)))
		}
		desc := locales.SettingsGeoNone()
		if len(files) > 0 {
			desc = strings.Join(files, "  ·  ")
		}
		m.setting(c, "files", locales.SettingsGeoFiles(), desc, func() {
			if s.geoBusy {
				ui.Spinner(c)
			} else if m.button(c, "update", locales.SettingsGeoUpdateNow(), nil, "").Clicked() {
				s.geoBusy = true
				m.run(locales.SettingsGeoUpdateFailed(), func() error { return Core{a}.UpdateGeo(context.Background()) }, func(err error) {
					s.geoBusy, s.loaded = false, false
					if err == nil {
						m.notices = append(m.notices, Notice{Level: "success", Message: locales.SettingsGeoUpdated()})
					}
				})
			}
		})
	})
	m.section(c, "chain", locales.SettingsChain(), func() { m.chainsEditor(c) })
	m.section(c, "dns", "DNS", func() {
		m.setting(c, "override", locales.SettingsDnsOverrideDefault(), locales.SettingsDnsOverrideDesc(), func() {
			if m.button(c, "edit", locales.CommonEdit(), nil, "").Clicked() {
				s.dialog, s.dns = "dns", newCode(st.DNS.Config, "dns.yaml", false)
			}
			v := st.DNS.DefaultOverride
			if ui.Switch(c.Key("sw"), &v).Changed() {
				m.set("dns.defaultOverride", v)
			}
		})
		m.setting(c, "flush", locales.SettingsFlushDns(), locales.SettingsFlushDnsDesc(), func() {
			if s.busy == "dns" {
				ui.Spinner(c)
			} else if m.button(c, "dns", "DNS", nil, "").Clicked() {
				act("dns", locales.SettingsFlushed(), func() error { return Core{a}.FlushDNS(context.Background()) })
			}
			if s.busy == "fakeip" {
				ui.Spinner(c)
			} else if m.button(c, "fakeip", "Fake IP", nil, "").Clicked() {
				act("fakeip", locales.SettingsFlushed(), func() error { return Core{a}.FlushFakeIP(context.Background()) })
			}
		})
	})
	m.section(c, "latency", locales.SettingsLatency(), func() {
		m.setting(c, "url", locales.SettingsTestUrl(), locales.SettingsTestUrlDesc(), func() {
			if !s.urlSet {
				s.urlSet, s.testURL = true, st.Latency.URL
			}
			in := ui.TextInput(c.Key("in"), &s.testURL).Font("ui-monospace").Width(280)
			if s.testURL != st.Latency.URL && (in.Submitted() || !in.Focused()) {
				m.set("latency.url", s.testURL)
			}
		})
		m.setting(c, "timeout", locales.SettingsTestTimeout(), locales.SettingsTestTimeoutDesc(), func() {
			m.numberIn(c, "timeout", st.Latency.TimeoutMs, 100, 60000, 100, func(n int) { m.set("latency.timeoutMs", n) })
			ui.Text(c, "ms").TextColor(p.TextMuted)
		})
		m.setting(c, "auto", locales.SettingsAutoCheck(), locales.SettingsAutoCheckDesc(), func() {
			m.collapseInline(c, "autocheck", st.Latency.AutoCheck, func() {
				m.numberIn(c, "autocheck", st.Latency.AutoCheckMinutes, 1, 1440, 80, func(n int) { m.set("latency.autoCheckMinutes", n) })
				ui.Text(c, locales.CommonMinutes()).TextColor(p.TextMuted).SingleLine()
			})
			v := st.Latency.AutoCheck
			if ui.Switch(c.Key("sw"), &v).Changed() {
				m.set("latency.autoCheck", v)
			}
		})
	})
	m.section(c, "behavior", locales.SettingsBehavior(), func() {
		m.swRow(c, "autoclose", locales.SettingsAutoClose(), locales.SettingsAutoCloseDesc(), st.AutoCloseConnections, "autoCloseConnections")
		m.swRow(c, "builtin", locales.SettingsBuiltin(), locales.SettingsBuiltinDesc(), st.BuiltinEnhanced, "builtinEnhanced")
		if m.info.OS == "windows" {
			m.setting(c, "uwp", locales.SettingsUwp(), locales.SettingsUwpDesc(), func() {
				if m.button(c, "conf", locales.CommonConfigure(), nil, "").Clicked() {
					s.dialog, s.uwpApps, s.uwpErr, s.uwpQ, s.uwpOn = "uwp", nil, "", "", map[string]bool{}
					win := m.window()
					go func() {
						apps, err := (System{a}).UWPApps(context.Background())
						win.Update(func() {
							if err != nil {
								s.uwpErr = err.Error()
								return
							}
							s.uwpApps = cmpSlice(apps)
							for _, ap := range apps {
								if ap.Exempt {
									s.uwpOn[ap.SID] = true
								}
							}
						})
					}()
				}
			})
		}
	})
	m.portsDialog(c)
	m.controllerDialog(c)
	m.dnsDialog(c)
	m.uwpDialog(c)
}

func cmpSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (m *mainUI) portsDialog(c *ui.Context) {
	s, p, a := &m.settings, m.pal, m.a
	type port struct {
		key, label string
		on         *bool
		show       bool
	}
	ports := []port{{"mixed", locales.SettingsMixedPort(), nil, true}, {"socks", locales.SettingsSocksPort(), &s.clash.SocksEnabled, true},
		{"http", locales.SettingsHttpPort(), &s.clash.HTTPEnabled, true}, {"redir", locales.SettingsRedirPort(), &s.clash.RedirEnabled, m.info.OS != "windows"},
		{"tproxy", locales.SettingsTproxyPort(), &s.clash.TProxyEnabled, m.info.OS == "linux"}}
	check := func(key string) {
		n, err := strconv.Atoi(strings.TrimSpace(s.ports[key]))
		if err != nil {
			return
		}
		win := m.window()
		go func() {
			used := (Core{a}).PortInUse(n)
			win.Update(func() { s.inUse[key] = used })
		}()
	}
	m.dialogFrame(c, "ports", locales.SettingsPorts(), 480, func() {
		m.rows(c, "ports", func() {
			for _, pt := range ports {
				if !pt.show {
					continue
				}
				desc := ""
				if s.inUse[pt.key] {
					desc = locales.SettingsPortInUse()
				}
				ui.Row(c.Key(pt.key)).Padding(8, 14).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Column(c).Grow(1).Basis(0).Gap(2).Children(func() {
						ui.Text(c, pt.label).FontWeight(550)
						if desc != "" {
							ui.Text(c, desc).FontSize(12).TextColor(p.Danger)
						}
					})
					v := s.ports[pt.key]
					in := ui.TextInput(c.Key("port"), &v).Width(90).FontFeatures("tnum")
					if v != s.ports[pt.key] {
						s.ports[pt.key] = v
					}
					if in.Submitted() || (!in.Focused() && in.Changed()) {
						check(pt.key)
					}
					if m.iconButton(c, "dice", icDices, locales.SettingsRandomPort()).Clicked() {
						s.ports[pt.key] = strconv.Itoa(20000 + rand.IntN(40000))
						check(pt.key)
					}
					if pt.on != nil {
						ui.Switch(c.Key("on"), pt.on)
					} else {
						ui.Box(c).Width(36)
					}
				})
			}
		})
	}, func() bool {
		cl := s.clash
		set := func(key string, dst *int) {
			if n, err := strconv.Atoi(strings.TrimSpace(s.ports[key])); err == nil && n > 0 && n < 65536 {
				*dst = n
			}
		}
		set("mixed", &cl.MixedPort)
		set("socks", &cl.SocksPort)
		set("http", &cl.HTTPPort)
		set("redir", &cl.RedirPort)
		set("tproxy", &cl.TProxyPort)
		m.set("clash", map[string]any{"mixedPort": cl.MixedPort, "socksPort": cl.SocksPort, "socksEnabled": cl.SocksEnabled, "httpPort": cl.HTTPPort,
			"httpEnabled": cl.HTTPEnabled, "redirPort": cl.RedirPort, "redirEnabled": cl.RedirEnabled, "tproxyPort": cl.TProxyPort, "tproxyEnabled": cl.TProxyEnabled})
		return true
	})
}

func (m *mainUI) controllerDialog(c *ui.Context) {
	s, p, a := &m.settings, m.pal, m.a
	m.dialogFrame(c, "controller", locales.SettingsController(), 640, func() {
		m.rows(c, "en", func() {
			m.setting(c, "en", locales.SettingsControllerEnable(), locales.SettingsControllerDesc(), func() { ui.Switch(c.Key("sw"), &s.ctrl.Enabled) })
		})
		ui.Row(c).Gap(12).Children(func() {
			m.rowField(c, locales.SettingsAddress(), locales.SettingsAddressHint(), "", func() { ui.TextInput(c.Key("addr"), &s.ctrl.Address).Font("ui-monospace") })
			m.rowField(c, locales.SettingsSecret(), "", "", func() {
				ui.Row(c).Gap(6).Children(func() {
					ui.TextInput(c.Key("secret"), &s.ctrl.Secret).Font("ui-monospace").Grow(1)
					if m.iconButton(c, "dice", icDices, locales.SettingsRandomSecret()).Size(32, 32).Clicked() {
						s.ctrl.Secret = (SettingsService{a}).RandomSecret()
					}
				})
			})
		})
		m.field(c, locales.SettingsOrigins(), locales.SettingsOriginsHint(), "", func() {
			ui.TextArea(c.Key("origins"), &s.origins).Font("ui-monospace").Height(64)
		})
		m.rows(c, "pn", func() {
			m.setting(c, "pn", locales.SettingsPrivateNetwork(), locales.SettingsPrivateNetworkDesc(), func() { ui.Switch(c.Key("sw"), &s.ctrl.AllowPrivateNetwork) })
		})
		ui.Text(c, locales.SettingsWebUis()).FontSize(12).FontWeight(600).TextColor(p.TextMuted)
		m.rows(c, "uis", func() {
			for i, u := range s.uis {
				ui.Row(c.Key(i)).Padding(8, 14).Gap(6).AlignItems(ui.Center).Children(func() {
					ui.Text(c, u).FontSize(12).Font("ui-monospace").SingleLine().TruncateMiddle().Grow(1).Basis(0)
					if m.iconButton(c, "open", icExternalLink, locales.CommonOpen()).Disabled(!m.st.Clash.Controller.Enabled).Clicked() {
						tmpl := u
						m.run(locales.CommonFailed(), func() error { return Core{a}.OpenWebUI(tmpl) }, nil)
					}
					if m.iconButton(c, "del", icTrash2, locales.CommonDelete()).Clicked() {
						s.uis = slices.Delete(slices.Clone(s.uis), i, i+1)
					}
				})
			}
			ui.Row(c.Key("new")).Padding(8, 14).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.TextInput(c.Key("url"), &s.newUI).Placeholder("https://…?hostname=%host&port=%port&secret=%secret").Font("ui-monospace").Grow(1)
				ok := strings.HasPrefix(s.newUI, "http://") || strings.HasPrefix(s.newUI, "https://")
				if m.button(c, "add", locales.CommonAdd(), icPlus, "").Disabled(!ok).Clicked() {
					s.uis, s.newUI = append(s.uis, s.newUI), ""
				}
			})
		})
	}, func() bool {
		ctrl := s.ctrl
		ctrl.AllowOrigins = strings.Fields(s.origins)
		m.patch(map[string]any{"clash": map[string]any{"controller": ctrl}, "webUis": s.uis})
		return true
	})
}

func (m *mainUI) dnsDialog(c *ui.Context) {
	s, p := &m.settings, m.pal
	if s.dns == nil {
		return
	}
	m.dialogFrame(c, "dns", locales.SettingsDnsOverride(), 860, func() {
		m.banner(c, "warning", locales.SettingsDnsWarn(), func() {
			if m.button(c, "reset", locales.SettingsResetDefault(), nil, "ghost").Clicked() {
				s.dns.Buffer.Set(config.Defaults().DNS.Config)
			}
		})
		ui.Column(c).Height(420).Radius(8).Border(1, p.Border).Clip().Children(func() { editor.View(c.Key("ed"), s.dns).Grow(1).FillWidth() })
	}, func() bool {
		m.set("dns.config", s.dns.Buffer.String())
		return true
	})
}

// uwpDialog chooses which Microsoft Store apps may reach the proxy on the
// loopback, as Clash Verge's loopback tool does: all, none, or some.
func (m *mainUI) uwpDialog(c *ui.Context) {
	s, p, a := &m.settings, m.pal, m.a
	q := strings.ToLower(strings.TrimSpace(s.uwpQ))
	var shown []uwp.StoreApp
	for _, ap := range s.uwpApps {
		if q == "" || strings.Contains(strings.ToLower(ap.DisplayName), q) || strings.Contains(strings.ToLower(ap.Name), q) {
			shown = append(shown, ap)
		}
	}
	allOn := len(shown) > 0 && !slices.ContainsFunc(shown, func(ap uwp.StoreApp) bool { return !s.uwpOn[ap.SID] })
	m.dialogFrame(c, "uwp", locales.SettingsUwp(), 520, func() {
		ui.Text(c, locales.SettingsUwpHint()).FontSize(12.5).TextColor(p.TextMuted).Wrap()
		ui.Row(c).Gap(8).Children(func() {
			ui.SearchField(c.Key("q"), &s.uwpQ).Placeholder(locales.CommonSearch()).Grow(1)
			label := locales.SettingsUwpAll()
			if allOn {
				label = locales.SettingsUwpNone()
			}
			if m.button(c, "all", label, nil, "").Disabled(len(shown) == 0).Clicked() {
				for _, ap := range shown {
					s.uwpOn[ap.SID] = !allOn
				}
			}
		})
		switch {
		case s.uwpErr != "":
			m.banner(c, "danger", s.uwpErr, nil)
		case s.uwpApps == nil:
			ui.Spinner(c)
		case len(shown) == 0:
			m.empty(c, locales.SettingsUwpEmpty(), nil)
		default:
			ui.Column(c).Gap(2).Children(func() {
				for _, ap := range shown {
					on := s.uwpOn[ap.SID]
					if ui.Checkbox(c.Key(ap.SID), &on, ap.DisplayName).Changed() {
						s.uwpOn[ap.SID] = on
					}
				}
			})
		}
		if s.uwpApps != nil {
			n := 0
			for _, on := range s.uwpOn {
				if on {
					n++
				}
			}
			ui.Text(c, locales.SettingsUwpCount(fmt.Sprint(n), fmt.Sprint(len(s.uwpApps)))).FontSize(12).TextColor(p.TextMuted)
		}
	}, func() bool {
		var sids []string
		for sid, on := range s.uwpOn {
			if on {
				sids = append(sids, sid)
			}
		}
		m.run(locales.CommonFailed(), func() error { return System{a}.SetUWPLoopback(context.Background(), sids) }, func(err error) {
			if err == nil {
				m.notices = append(m.notices, Notice{Level: "success", Message: locales.SettingsUwpDone()})
			}
		})
		return true
	})
}

// chainsEditor lists the proxy chains, each with its switch, what it
// matches and what it goes through, and the way to add one.
func (m *mainUI) chainsEditor(c *ui.Context) {
	chains := m.st.ProxyChains
	save := func(next []config.ProxyChain) { m.set("proxyChains", next) }
	for i := range chains {
		ch := chains[i]
		ui.Row(c.Key(i)).Padding(8, 14).Gap(8).AlignItems(ui.Center).Children(func() {
			on := ch.Enabled
			if ui.Switch(c.Key("on"), &on).Changed() {
				next := slices.Clone(chains)
				next[i].Enabled = on
				save(next)
			}
			match, through := ch.Match, ch.Through
			in1 := ui.TextInput(c.Key("match"), &match).Placeholder(locales.SettingsChainMatch()).Grow(1)
			in2 := ui.TextInput(c.Key("through"), &through).Placeholder(locales.SettingsChainThrough()).Width(190)
			if match != ch.Match || through != ch.Through {
				next := slices.Clone(chains)
				next[i].Match, next[i].Through = match, through
				m.st.ProxyChains = next
				if !in1.Focused() && !in2.Focused() || in1.Submitted() || in2.Submitted() {
					save(next)
				}
			}
			if m.iconButton(c, "del", icTrash2, locales.CommonDelete()).Clicked() {
				save(slices.Delete(slices.Clone(chains), i, i+1))
			}
		})
	}
	m.setting(c, "add", locales.SettingsChainDesc(), "", func() {
		if m.button(c, "add", locales.SettingsChainAdd(), icPlus, "ghost").Clicked() {
			save(append(slices.Clone(chains), config.ProxyChain{Enabled: true}))
		}
	})
}
