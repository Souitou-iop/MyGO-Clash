package app

import (
	"context"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

// nativeMain reports whether the main window is native UI, which MyGo
// draws itself, rather than the web page. While both exist,
// MYGO_CLASH_UI=web keeps the page.
func nativeMain() bool { return os.Getenv("MYGO_CLASH_UI") != "web" }

// pages are the pages of the main window, in the order of the navigation.
var pages = []string{"home", "proxies", "profiles", "connections", "stats", "rules", "logs", "tailscale", "connectivity", "settings"}

const trafficLen = 120 // samples of the traffic kept, one a second

// mainUI is the main window drawn by MyGo: the navigation, and the page
// it shows. Its fields below mu are the view's, read and changed on the
// main thread only (Window.Update).
type mainUI struct {
	a *App

	// fillCard has the card being built fill the cell it is in: set
	// while the home page builds its cards, whose cells are buttons.
	fillCard bool

	mu     sync.Mutex
	win    *mygo.Window
	cancel context.CancelFunc

	pal *palette

	frame     uint64 // frames built, for the motion that spans them
	modals    map[any]*modalState
	collapses map[string]*collapseState
	glides    map[string]*glideState
	st        config.Settings
	state     AppState
	pv        ProfilesView
	ts        coreapi.TailscaleStatus
	sy        SyncStatus
	info      AppInfo

	page string
	tab  string // of the settings

	now      coreapi.Traffic
	up, down []int64 // the last trafficLen seconds
	sampleAt time.Time
	memory   int64
	conns    int

	// runtime counts applied configurations and selection the selections
	// made elsewhere, for pages to load again.
	runtime, selection int
	notices            []Notice

	home     homePage
	proxies  proxiesPage
	logs     logsPage
	connPage connsPage
	profs    profilesPage
	rules    rulesPage
	stats    statsPage
	cty      connectivityPage
	tsPage   tailscalePage
	viz      vizState
	settings settingsPage
	syncPg   syncPage
	dash     dashState
	nav      navState
	onb      onboarding
	tour     tourInvite
	connList []coreapi.Connection
	// The last trafficLen seconds of the core's memory and connections, and
	// the last routeLen of what each route (a chain's last proxy) brings
	// down, for the sheets' trails.
	memHist   []int64
	connHist  []int
	routeHist map[string][]int64
	upd       *UpdateState
}

func newMainUI(a *App) *mainUI {
	return &mainUI{a: a, page: "home", tab: "general", up: make([]int64, trafficLen), down: make([]int64, trafficLen)}
}

func (m *mainUI) window() *mygo.Window {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.win != nil && !m.win.IsDestroyed() {
		return m.win
	}
	return nil
}

// attach makes win the window the view shows in, and follows the core's
// streams for it until it is destroyed.
func (m *mainUI) attach(win *mygo.Window) {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	ctx, cancel := context.WithCancel(m.a.ctx)
	m.win, m.cancel = win, cancel
	m.mu.Unlock()
	m.refresh()
	go func() {
		info, _ := AppService{m.a}.Info(ctx)
		win.Update(func() { m.info = info })
	}()
	go follow(ctx, m.a.traffic, func(t coreapi.Traffic) {
		win.Update(func() {
			m.now, m.sampleAt = t, time.Now()
			m.up = append(m.up[1:], t.Up)
			m.down = append(m.down[1:], t.Down)
		})
	})
	go follow(ctx, m.a.memory, func(v coreapi.Memory) {
		mygo.RunOnMain(func() {
			m.memHist = keepLast(append(m.memHist, int64(v.Inuse)), trafficLen)
			if m.memory != int64(v.Inuse) {
				m.memory = int64(v.Inuse)
				win.Invalidate()
			}
		})
	})
	go follow(ctx, m.a.conns, func(v coreapi.Connections) {
		mygo.RunOnMain(func() {
			m.connList = v.Connections
			m.connHist = keepLast(append(m.connHist, len(v.Connections)), trafficLen)
			m.noteRoutes(v.Connections)
			if n := len(v.Connections); m.conns != n || m.page == "home" {
				m.conns = n
				win.Invalidate()
			}
		})
	})
}

// routeLen is how many seconds of each route's speed are kept.
const routeLen = 40

// noteRoutes adds a second of what each route brings down, from the
// connections now: every route seen keeps going, at nothing once idle,
// until it has been idle all its routeLen seconds.
func (m *mainUI) noteRoutes(conns []coreapi.Connection) {
	now := map[string]int64{}
	for _, cn := range conns {
		if len(cn.Chains) > 0 {
			now[cn.Chains[0]] += cn.DownloadSpeed + cn.UploadSpeed
		}
	}
	if m.routeHist == nil {
		m.routeHist = map[string][]int64{}
	}
	for r := range now {
		if _, ok := m.routeHist[r]; !ok {
			m.routeHist[r] = nil
		}
	}
	for r, h := range m.routeHist {
		h = keepLast(append(h, now[r]), routeLen)
		idle := true
		for _, v := range h {
			idle = idle && v == 0
		}
		if idle && len(h) == routeLen {
			delete(m.routeHist, r)
			continue
		}
		m.routeHist[r] = h
	}
}

// keepLast is the last n of s.
func keepLast[T any](s []T, n int) []T {
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}

// follow calls fn with the values of a hub until ctx ends.
func follow[T any](ctx context.Context, h *hub[T], fn func(T)) {
	if h == nil {
		return
	}
	ch, stop := h.Subscribe()
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return
		case v, ok := <-ch:
			if !ok {
				return
			}
			fn(v)
		}
	}
}

// refresh shows the app's state as it is now. From any goroutine.
func (m *mainUI) refresh() {
	win := m.window()
	if win == nil {
		return
	}
	a := m.a
	st, state, pv := a.settings.Get(), a.snapshot(), a.profilesView()
	var ts coreapi.TailscaleStatus
	if a.ts != nil {
		ts = a.ts.current()
	}
	var sy SyncStatus
	if a.syncer != nil {
		sy = a.syncer.statusSnapshot()
	}
	go a.noteHistory(state, pv)
	win.Update(func() { m.st, m.state, m.pv, m.ts, m.sy = st, state, pv, ts, sy })
}

// applied tells the pages that a configuration was applied.
func (m *mainUI) applied() {
	if win := m.window(); win != nil {
		win.Update(func() { m.runtime++ })
	}
}

// selected tells the pages that a group's selection changed elsewhere.
func (m *mainUI) selected() {
	if win := m.window(); win != nil {
		win.Update(func() { m.selection++ })
	}
}

// notice shows a message as a toast.
func (m *mainUI) notice(n Notice) {
	if win := m.window(); win != nil {
		win.Update(func() { m.notices = append(m.notices, n) })
	}
}

// navigate shows a page: "proxies", "logs" (a tab of activity), or
// "settings/sync" for a tab of the settings; "home/<tile>" opens a tile's
// sheet, "home/" closes it; "back" goes back.
func (m *mainUI) navigate(target string) {
	page, tab, _ := strings.Cut(target, "/")
	if pl, _ := placeOf(page); pl == "" && page != "back" {
		return
	}
	set := func() {
		switch page {
		case "back":
			m.goBack()
			return
		case "home":
			m.goTo(page, "")
			// home/<tile>+page opens a tile's sheet and grows it on into its
			// page.
			key, grow := strings.CutSuffix(tab, "+page")
			if key == "nodes" {
				key = "exit"
			}
			if m.dash.sheet = key; key != "" {
				m.dash.sheetKey, m.dash.growAfter = key, grow
			}
			return
		}
		m.goTo(page, tab)
	}
	if win := m.window(); win != nil {
		win.Update(set)
	} else {
		set() // before the window exists: the first frame reads it
	}
}

// goTo is navigate from the view: a page, by the name links know it by,
// and for the settings a tab of theirs.
func (m *mainUI) goTo(page, tab string) {
	pl, sub := placeOf(page)
	if pl == "" {
		return
	}
	m.show(pl, sub, "", false)
	if pl == "settings" && tab != "" {
		m.tab = tab
	}
}

// patch changes settings from the view, and reports a failure.
func (m *mainUI) patch(patch map[string]any) {
	mergePatch(&m.st, patch) // at once; the settings confirm
	go func() {
		if _, err := (SettingsService{m.a}).Patch(context.Background(), patch); err != nil {
			m.notice(Notice{Level: "error", Message: locales.CommonFailed(), Detail: err.Error()})
		}
	}()
}

// run does work off the main thread and reports its failure in a toast;
// done, if any, runs on the main thread after it.
func (m *mainUI) run(failure string, fn func() error, done func(err error)) {
	win := m.window()
	go func() {
		err := fn()
		if err != nil && failure != "" {
			m.notice(Notice{Level: "error", Message: failure, Detail: err.Error()})
		}
		if done != nil && win != nil {
			win.Update(func() { done(err) })
		}
	}()
}

// ---- The view ----

func (m *mainUI) view(c *ui.Context) {
	m.frame++
	m.vizFrame(c)
	// The logs stream only while their page shows; it loads the recent
	// ones again when it comes back.
	if !m.showing("logs") && m.logs.started {
		m.logs.stop()
		m.logs.started, m.logs.logs = false, nil
	}
	dark := c.Theme().Dark
	switch m.st.Theme {
	case "light":
		dark = false
	case "dark":
		dark = true
	}
	m.pal = mainPalette(dark, m.st.OLED, m.st.Accent, m.st.FontFamily)
	c.SetTheme(m.pal.Theme)
	for _, n := range m.notices {
		m.toast(c, n)
	}
	m.notices = nil
	m.toasts(c)
	defer m.confirmDialog(c)

	// Back: Cmd+[ (Alt+← elsewhere), a mouse's back button, a swipe.
	back := func() { m.goBack() }
	c.OnShortcut(0, ui.KeyBack, back)
	if runtime.GOOS == "darwin" {
		c.OnShortcut(ui.Super, ui.KeyBracketLeft, back)
	} else {
		c.OnShortcut(ui.Alt, ui.KeyLeft, back)
	}

	ui.Row(c).Fill().AlignItems(ui.Stretch).Children(func() {
		m.sidebar(c)
		// A page comes in from the side of the navigation it lies on, and
		// the one it replaces goes the other way; one a sheet grew into
		// takes the sheet's place, fading in.
		n := &m.nav
		in, out := ui.Motion{Y: 12 * n.dir}, ui.Motion{Y: -12 * n.dir}
		if n.instant {
			in, out = ui.Motion{}, ui.Motion{}
		}
		host := ui.Box(c.Key("pages")).Grow(1).Basis(0).MinWidth(0).ChildExit(out)
		host.Children(func() {
			page := ui.Column(c.Key("page-" + m.page)).Absolute().Left(0).Top(0).Right(0).Bottom(0)
			page.Transition(ui.ElementTransition{Enter: &in, Exit: &out, Duration: pageMotion, Ease: easeOut})
			page.Children(func() {
				switch m.page {
				case "home":
					m.homeView(c)
				case "proxies":
					m.tabbed(c, "proxies", map[string]func(){
						"groups": func() { m.proxiesView(c) }, "providers": func() { m.providersView(c) }, "chains": func() { m.chainsView(c) },
					})
				case "activity":
					m.tabbed(c, "activity", map[string]func(){
						"connections": func() { m.connectionsView(c) }, "stats": func() { m.statsView(c) }, "logs": func() { m.logsView(c) },
					})
				case "connectivity":
					m.connectivityView(c)
				case "tailscale":
					m.tailscaleView(c)
				case "profiles":
					m.profilesView(c)
				case "rules":
					m.rulesView(c)
				case "settings":
					m.settingsView(c)
				default:
					m.header(c, m.T("nav."+m.page), nil)
					m.empty(c, m.T("nav."+m.page), nil)
				}
			})
		})
	})
	m.onboardingView(c)
}

// toast shows a notice, with the button of its action if it has one.
func (m *mainUI) toast(c *ui.Context, n Notice) {
	t := ui.Toast{Title: n.Message, Description: n.Detail, Type: n.Level}
	switch n.Level {
	case "error":
		t.Timeout = 9 * time.Second
	case "warning":
		t.Timeout = 7 * time.Second
	}
	if label, do := m.noticeAction(n.Action); label != "" {
		t.Action, t.OnAction = label, do
	}
	c.AddToast(t)
}

// noticeAction is the button of a notice's action, and what it does.
func (m *mainUI) noticeAction(action string) (string, func()) {
	a := m.a
	switch action {
	case "":
		return "", nil
	case "install-service", "repair-service":
		return locales.ServiceInstall(), func() {
			m.run(locales.ServiceInstallFailed(), func() error { return System{a}.InstallService(context.Background(), false) }, nil)
		}
	case "install-tun":
		return locales.ServiceInstall(), func() { go a.installServiceForTun() }
	case "relaunch":
		return locales.ActionRelaunch(), func() { go mygo.App.Relaunch() }
	case "open-sync":
		return locales.CommonOpen(), func() { m.goTo("settings", "sync") }
	}
	return locales.CommonOpen(), func() { m.goTo("settings", "") }
}

// sidebar is the navigation: the brand, the pages, and the traffic. It
// stands on the window itself, no slab of its own: a rail of icons whose
// names show as tips, or with the names beside them when unfolded.
//
// It folds and unfolds in one continuous move: everything in it follows
// a single progress, so the icons glide to and from their places in the
// rail, each item's soft fill narrows to its icon and widens back, and
// the names and the traffic graph fade as the edge passes over them,
// rather than the layout flipping to the other state at the start.
func (m *mainUI) sidebar(c *ui.Context) {
	p, bar := m.pal, c.TitleBar()
	collapsed := m.st.UI.CollapseNav
	const wideW = float32(212)
	railW := max(68, bar.Left+9)
	dots := map[string]bool{
		"settings":  m.state.ConfigError != "" || (m.state.Service.Installed && m.state.Service.Outdated) || len(m.sy.Conflicts) > 0,
		"tailscale": m.ts.BackendState == "NeedsLogin",
		"profiles":  m.state.ConfigError != "",
	}
	side := ui.Column(c.Key("sidebar"))
	// k is how far folded it is: 0 unfolded, 1 a rail.
	k := side.AnimateTo("fold", b2f(collapsed), ui.Animation{Duration: navFoldDur, Ease: easeStd})
	rail := collapsed && k > 0.999 // folded all the way
	// The names fade out over the first part of folding, as the edge
	// comes to them, and back in over the last part of unfolding.
	fade := max(0, 1-k*1.7)
	folding := k > 0.0001 && k < 0.9999
	side.Width(lerp(wideW, railW, k)).Shrink(0).Clip()
	side.Children(func() {
		ui.Box(c).Height(max(bar.Height, 44)).DragWindow()
		// Brand: the compass stays where the rail centers it.
		ui.Row(c.Key("brand")).Height(40).Gap(10).AlignItems(ui.Center).DragWindow().Clip().
			PaddingStart(lerp(18, (railW-30)/2, k)).Children(func() {
			// A press spins it.
			mark := ui.ButtonBase(c.Key("mark")).Size(30, 30).Shrink(0).Label("MyGO-Clash")
			pressable(mark)
			turn := compassSpin(mark)
			inviting := m.tourInviting(c)
			if inviting {
				m.tourTip(c, mark)
				if mark.Clicked() {
					m.playTour()
				}
				turn += m.tourNudge(c)
				mark.Children(func() {
					// Cut out of the ring by the window behind it.
					ui.Box(c.Key("tour")).Size(10, 10).Radius(5).Background(p.Accent).Border(2, p.Theme.Background).Absolute().Top(-2).End(-2)
				})
			}
			mark.Draw(func(pt *ui.Painter, r ui.Rect) {
				drawCompass(pt, r.X+r.W/2, r.Y+r.H/2, 34, compassLook{Ring: true, Turn: turn})
			})
			if !rail {
				ui.RichText(c, ui.Span{Text: "MyGO", Weight: 700, Color: p.AccentFg}, ui.Span{Text: "-Clash", Weight: 700}).
					FontSize(15).SingleLine().Shrink(0).Opacity(fade)
			}
		})
		main, foot := m.navPlaces()
		// The selection's soft fill slides from the place left to the one
		// chosen.
		thumb := segThumb{side.ID()}
		itemH := lerp(38, 42, k)
		item := func(id string) {
			active := m.navSelected() == id
			b := ui.ButtonBase(c.Key(id)).Row().Height(itemH).PaddingStart(lerp(12, (42-19)/2, k)).Gap(12).
				Radius(12).AlignItems(ui.Center).Justify(ui.Start).Clip().Label(m.T("nav." + id))
			if rail {
				b.Tooltip(m.T("nav." + id))
			}
			fg := p.TextMuted
			switch {
			case active:
				fg = p.AccentFg
			case b.Hovered():
				b.Background(p.SurfaceHover)
				fg = p.Text2
			}
			b.Children(func() {
				if active {
					fill := ui.Box(c.Key("thumb")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Radius(12).Background(p.AccentSoft)
					// While the bar folds the fill keeps to its item, which
					// narrows or widens with it, rather than chasing it.
					if !folding {
						fill.Transition(ui.ElementTransition{Match: thumb, Animation: &segSlide})
					}
				}
				m.icon(c, placeIcons[id], 19, fg, active || b.Hovered()).ZIndex(1).Shrink(0)
				if dots[id] {
					ui.Box(c).Size(7, 7).Radius(4).Background(p.Danger).Absolute().Top(6).End(6).ZIndex(2)
				}
				if !rail {
					txt := ui.Text(c, m.T("nav."+id)).FontSize(13.5).TextColor(fg).SingleLine().Shrink(0).ZIndex(1).Opacity(fade)
					if active {
						txt.FontWeight(600)
					}
				}
			})
			pressable(b)
			if b.Clicked() {
				m.show(id, "", "", false)
			}
		}
		ui.Column(c).PaddingY(18).PaddingX(lerp(12, (railW-42)/2, k)).Gap(4).Grow(1).Children(func() {
			for _, id := range main {
				item(id)
			}
			ui.Spacer(c)
			for _, id := range foot {
				item(id)
			}
		})
		// Traffic, the eye and the way to fold the navigation. The lines
		// keep to the bar's edge either way, and the two buttons glide from
		// it to the middle of the rail, from places worked out rather than
		// measured, so that nothing jerks as the numbers change.
		ui.Column(c.Key("foot")).Padding(10, 0, 14).Children(func() {
			if m.st.UI.TrafficGraph && !rail {
				// It keeps its width, the edge passing over it, and gives
				// up its height as it goes.
				ui.Box(c.Key("graph")).Height((34 + 8) * (1 - k)).Clip().PaddingX(14).Opacity(fade).Children(func() {
					m.trafficGraph(c, 40, 34, true).Width(wideW - 28).Shrink(0)
				})
			}
			// Keyed: the graph above comes and goes with the fold, and a
			// column known only by its place would be a new one then, its
			// rates' glides starting over from an unmeasured width.
			ui.Column(c.Key("rates")).Gap(8).Children(func() {
				// Folded, each rate is centered on the column of the icons
				// above; as its digits change its width, its place glides
				// rather than jumps.
				rate := func(key string, icon *ui.SVG, col ui.Color, v int64) {
					row := ui.Row(c.Key(key))
					row.Children(func() {
						in := ui.Row(c.Key("in")).Gap(5).AlignItems(ui.Center).Shrink(0)
						center := row.AnimateTo("center", (railW-in.LayoutBounds().W)/2, ui.Animation{Duration: 200 * time.Millisecond, Ease: easeStd})
						row.PaddingStart(lerp(14, max(0, center), k))
						in.Children(func() {
							ui.Icon(c, icon).FontSize(12).TextColor(col).Shrink(0)
							// Folded, the rate is said short. The two ways of
							// saying it swap at once rather than roll into each
							// other; within each the digits roll.
							s, form := fmtRate(v), "long"
							if collapsed {
								s, form = shortRate(v), "short"
							}
							ui.Text(c.Key(form), s).FontSize(11.5).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll).SingleLine().Shrink(0)
						})
					})
				}
				rate("up", icArrowUp, p.Accent2, m.now.Up)
				rate("down", icArrowDown, p.Accent, m.now.Down)
				// The two buttons side by side at the bar's edge, unfolded;
				// one above the other on the icons' column, folded, gliding
				// between the two as it folds.
				const btn, gap = float32(28), float32(4)
				mid := (railW - btn) / 2
				ui.Box(c.Key("buttons")).Height(lerp(btn, 2*btn+gap, k)).Shrink(0).Children(func() {
					icon, tip := icPanelLeftClose, locales.NavCollapse()
					if collapsed {
						icon, tip = icPanelLeftOpen, locales.NavExpand()
					}
					fold := m.iconButton(c, "fold", icon, tip).Absolute().Left(lerp(14, mid, k)).Top(0)
					if fold.Clicked() {
						m.st.UI.CollapseNav = !collapsed // at once; the settings confirm
						m.patch(map[string]any{"ui": map[string]any{"collapseNav": !collapsed}})
					}
					eye := m.eyeButton(c).Absolute().Left(lerp(14+btn+gap, mid, k)).Top(lerp(0, btn+gap, k))
					if eye.Clicked() {
						v := !m.reveal()
						m.st.UI.RevealPrivate = v
						m.patch(map[string]any{"ui": map[string]any{"revealPrivate": v}})
					}
				})
			})
		})
	})
}

// reveal reports whether the addresses, places and providers the app
// hides behind dots show: the eye in the navigation, for the whole app.
func (m *mainUI) reveal() bool { return m.st.UI.RevealPrivate }

// eyeButton is the navigation's eye, which shows and hides what is
// private across the app; its eye opens and shuts as it turns.
func (m *mainUI) eyeButton(c *ui.Context) ui.Element {
	p, shown := m.pal, m.reveal()
	icon, tip := icEye, locales.CommonShow()
	if shown {
		icon, tip = icEyeOff, locales.CommonHide()
	}
	b := ui.ButtonBase(c.Key("eye")).Size(28, 28).Radius(8).AlignItems(ui.Center).Justify(ui.Center).Label(tip).Tooltip(tip).Shrink(0)
	if b.Hovered() {
		b.Background(p.SurfaceHover)
	}
	pressable(b)
	hot := b.Hovered()
	b.Children(func() {
		ui.Box(c).Size(15, 15).Children(func() {
			ui.Box(c.Key(shown)).Absolute().Left(0).Top(0).Right(0).Bottom(0).AlignItems(ui.Center).Justify(ui.Center).
				Transition(eyeTurn).Children(func() { m.icon(c, icon, 15, p.TextMuted, hot) })
		})
	})
	return b
}

// eyeTurn is how the eye's open and shut icons turn into each other.
var eyeTurn = ui.ElementTransition{
	Enter: &ui.Motion{Scale: 0.5}, Exit: &ui.Motion{Scale: 0.5},
	Colors: true, Duration: 220 * time.Millisecond, ExitDuration: 140 * time.Millisecond, Ease: easeOut,
}

// navFoldDur is how long the navigation takes to fold or unfold.
const navFoldDur = 380 * time.Millisecond

// header is the top of a page: its title, where the window drags, and
// the page's actions.
func (m *mainUI) header(c *ui.Context, title string, actions func()) {
	bar := c.TitleBar()
	end := float32(20)
	if runtime.GOOS != "darwin" {
		end += bar.Right
	}
	ui.Row(c).Height(max(bar.Height, 44) + 8).PaddingStart(22).PaddingEnd(end).Gap(8).AlignItems(ui.Center).DragWindow().Shrink(0).Children(func() {
		// A page shown as a tab leaves its title's place to the tabs.
		if w := m.nav.tabsW; w > 0 && m.nav.inTabs {
			ui.Box(c).Width(w - 6).Shrink(0)
		} else {
			ui.Text(c, title).FontSize(18).FontWeight(650).SingleLine()
		}
		ui.Spacer(c)
		if actions != nil {
			actions()
		}
	})
}
