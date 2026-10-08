// Package app is MyGO-Clash itself: it ties the core, profiles, settings,
// the system proxy, Tailscale and sync together, and gives them to the
// interface (a web page that calls the services bound here) and to the
// tray, the global shortcuts and the native quick panel.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/corehost"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/logx"
	"github.com/mygo-clash/mygo-clash/internal/paths"
	"github.com/mygo-clash/mygo-clash/internal/profiles"
	"github.com/mygo-clash/mygo-clash/internal/secure"
	"github.com/mygo-clash/mygo-clash/internal/sysproxy"
	"github.com/mygo-clash/mygo-clash/internal/usage"
	"github.com/mygo-clash/mygo-clash/internal/webrtc"
)

// Version is the app's version, set by main.
var Version = "dev"

// App is the running app.
type App struct {
	name  string
	slug  string
	dirs  paths.Dirs
	ready chan struct{}
	ctx   context.Context
	stop  context.CancelFunc

	sealer   *secure.Sealer
	secrets  *secure.Secrets
	settings *config.Store
	profiles *profiles.Manager
	core     *coremgr.Manager
	coreLog  *logx.RotatingFile

	guard sysproxy.Guard
	pac   sysproxy.PACServer

	traffic *hub[coreapi.Traffic]
	memory  *hub[coreapi.Memory]
	conns   *hub[coreapi.Connections]
	usage   *usage.Store
	logs    *logRing

	ts      *tailscaleManager
	syncer  *syncer
	tray    *trayUI
	panel   *quickPanel
	updates *updateChecker

	outdatedTold atomic.Bool // the user heard that the service needs updating

	applyMu sync.Mutex // one configuration at a time

	mu          sync.Mutex
	state       AppState
	runtime     *profiles.Runtime
	win         *mygo.Window
	lightTimer  *time.Timer
	quitting    bool
	cleaned     bool
	pendingURLs []string
	pendingPage string // for the main window being created; see navigate
	hotkeys     []string
}

// Main runs the app. It returns when the app quits.
func Main() {
	corehost.AppVersion = Version
	a := &App{ready: make(chan struct{})}
	a.ctx, a.stop = context.WithCancel(context.Background())
	a.bind()
	if !mygo.App.RequestSingleInstanceLock() {
		return // the running instance takes over
	}
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		for _, arg := range args {
			if isDeepLink(arg) {
				a.handleURL(arg)
				return
			}
		}
		a.showMain()
	})
	mygo.App.OnOpenURL(a.handleURL)
	mygo.App.OnWindowAllClosed(func() {}) // live on in the tray
	mygo.App.OnActivate(func(visible bool) {
		if !visible {
			a.showMain()
		}
	})
	mygo.App.OnBeforeQuit(a.beforeQuit)
	useUpdater()
	mygo.App.WhenReady(a.start)
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func isDeepLink(s string) bool {
	return strings.HasPrefix(s, "clash://") || strings.HasPrefix(s, "mygo-clash://")
}

// waitReady waits until the app started, for bound methods called early.
func (a *App) waitReady(ctx context.Context) error {
	select {
	case <-a.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *App) start() {
	if err := a.init(); err != nil {
		log.Printf("start: %v", err)
		mygo.Dialog.Error("MyGO-Clash cannot start", err.Error())
		mygo.App.Exit(1)
		return
	}
	close(a.ready)
	st := a.settings.Get()
	a.applyTheme(st)
	log.Printf("switch: at start, system proxy %s, TUN %s, mode %s", onOff(st.SystemProxy.Enabled), onOff(st.Tun.Enabled), st.Clash.Mode)
	a.tray = newTrayUI(a)
	a.panel = newQuickPanel(a)
	a.updates = newUpdateChecker(a)
	if !st.SilentStart && !mygo.App.WasOpenedAtLogin() {
		a.showMain()
	} else if runtime.GOOS == "darwin" {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}
	a.registerHotkeys(st)
	go a.startCore(context.Background())
	go a.profiles.RunScheduler(a.ctx, a.onProfileUpdated)
	go a.background()
	a.syncer.start()
	a.startDebug()
	a.mu.Lock()
	urls := a.pendingURLs
	a.pendingURLs = nil
	a.mu.Unlock()
	for _, u := range urls {
		go a.handleURL(u)
	}
}

func (a *App) init() error {
	a.name = mygo.App.Name()
	if a.name == "" {
		a.name = "MyGO-Clash"
	}
	a.slug = paths.Slug(a.name)
	data, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	logs, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		logs = filepath.Join(data, "logs")
	}
	cache, err := mygo.App.Path(mygo.PathCache)
	if err != nil {
		cache = filepath.Join(data, "cache")
	}
	if a.dirs, err = paths.New(data, logs, cache); err != nil {
		return err
	}
	// mygo's updater asks GitHub with the default client.
	http.DefaultClient.Transport = newGitHubTransport(a)
	appLog := &logx.RotatingFile{Path: filepath.Join(logs, "app.log"), MaxSize: 4 << 20, MaxFiles: 3}
	log.SetOutput(io.MultiWriter(os.Stderr, appLog))
	log.Printf("MyGO-Clash %s (mihomo %s) on %s/%s", Version, corehost.MihomoVersion(), runtime.GOOS, runtime.GOARCH)

	if a.sealer, err = secure.Open(a.name, data); err != nil {
		return fmt.Errorf("open the key store: %w", err)
	}
	log.Printf("secrets are kept in %s", a.sealer.Keyring().Name())
	if a.secrets, err = secure.OpenSecrets(filepath.Join(data, "secrets.bin"), a.sealer); err != nil {
		return err
	}
	if a.settings, err = config.Load(filepath.Join(data, "settings.json")); err != nil {
		return err
	}
	st := a.settings.Get()
	a.coreLog = &logx.RotatingFile{Path: filepath.Join(logs, "core.log"), MaxSize: int64(st.Logs.MaxSizeMB) << 20, MaxFiles: st.Logs.MaxFiles}
	if a.profiles, err = profiles.Open(a.dirs.Profiles, a.sealer); err != nil {
		return err
	}
	a.profiles.ProxyAddr = a.mixedAddr
	a.profiles.OnChange = a.onProfilesChanged

	a.core = &coremgr.Manager{
		Bootstrap: a.bootstrap,
		OnState:   a.onCoreState,
		OnReady:   a.onCoreReady,
	}
	a.traffic = newHub(func(ctx context.Context, emit func(coreapi.Traffic)) error {
		c, err := a.core.Must()
		if err != nil {
			return err
		}
		return c.StreamTraffic(ctx, emit)
	})
	a.memory = newHub(func(ctx context.Context, emit func(coreapi.Memory)) error {
		c, err := a.core.Must()
		if err != nil {
			return err
		}
		return c.StreamMemory(ctx, emit)
	})
	a.conns = newHub(a.pollConnections)
	a.usage = usage.Open(filepath.Join(a.dirs.Data, "usage.json"))
	a.logs = newLogRing(2000)
	a.ts = newTailscaleManager(a)
	a.syncer = newSyncer(a)
	a.state = AppState{Mode: st.Clash.Mode, MixedPort: st.Clash.MixedPort, Service: ServiceState{Supported: serviceSupported()}}
	logx.CleanOld(logs, st.Logs.AutoCleanDays)
	return nil
}

// mixedAddr returns the address of the core's mixed proxy, "" when the
// core is not running.
func (a *App) mixedAddr() string {
	if a.core == nil || a.core.Client() == nil {
		return ""
	}
	st := a.settings.Get()
	return fmt.Sprintf("127.0.0.1:%d", st.Clash.MixedPort)
}

// proxyURL is the proxy for requests that go through the core.
func (a *App) proxyURL() string {
	if addr := a.mixedAddr(); addr != "" {
		return "http://" + addr
	}
	return ""
}

// ---- State ----

func (a *App) updateState(fn func(*AppState)) {
	a.mu.Lock()
	fn(&a.state)
	s := a.state
	a.mu.Unlock()
	_ = StateEvent.Broadcast(s)
	if a.tray != nil {
		a.tray.refresh()
	}
	if a.panel != nil {
		a.panel.invalidate()
	}
}

func (a *App) snapshot() AppState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

// notify shows a toast while the main window is in front, and otherwise
// errors and warnings as system notifications whose click opens n.Page.
func (a *App) notify(n Notice) {
	if n.Level == "" {
		n.Level = "info"
	}
	if !a.inFront() && (n.Level == "error" || n.Level == "warning" || n.Important) && a.settings != nil && a.settings.Get().Notifications && mygo.NotificationsSupported() {
		title, body := n.Message, n.Detail
		if body == "" {
			title, body = a.name, n.Message
		}
		nt := mygo.NewNotification(mygo.NotificationOptions{Title: title, Body: body})
		page := n.Page
		nt.OnClick(func() { a.navigate(page) })
		// Show waits for the system, and on macOS for the user to allow
		// notifications the first time; a toast stands in when they're not.
		go func() {
			if err := nt.Show(); err != nil {
				log.Printf("notification: %v", err)
				_ = NoticeEvent.Broadcast(n)
			}
		}()
		return
	}
	_ = NoticeEvent.Broadcast(n)
}

// inFront reports whether the main window is visible and focused.
func (a *App) inFront() bool {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	return win != nil && !win.IsDestroyed() && win.IsVisible() && win.IsFocused()
}

// notifyErr reports err, if any; a click on its notification opens page.
func (a *App) notifyErr(page, msg string, err error) {
	if err == nil {
		return
	}
	log.Printf("%s: %v", msg, err)
	a.notify(Notice{Level: "error", Message: msg, Detail: err.Error(), Page: page})
}

// ---- Windows ----

// showMain shows the main window, creating it when it does not exist (in
// lightweight mode, or the first time).
func (a *App) showMain() {
	a.mu.Lock()
	win := a.win
	if a.lightTimer != nil {
		a.lightTimer.Stop()
		a.lightTimer = nil
	}
	wasLight := a.state.Lightweight
	a.mu.Unlock()
	if wasLight {
		a.updateState(func(s *AppState) { s.Lightweight = false })
	}
	if runtime.GOOS == "darwin" {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyRegular)
	}
	if win != nil && !win.IsDestroyed() {
		if win.IsMinimized() {
			win.Restore()
		}
		win.Show()
		win.Focus()
		mygo.App.Focus()
		return
	}
	win = mygo.NewWindow(mygo.WindowOptions{
		Title:           a.name,
		URL:             "/",
		Width:           1120,
		Height:          740,
		MinWidth:        880,
		MinHeight:       580,
		StateKey:        "main",
		Hidden:          true,
		TitleBarStyle:   mygo.TitleBarHidden,
		TitleBarHeight:  44,
		BackgroundColor: windowBackground(a.settings.Get()),
	})
	win.OnReadyToShow(func() {
		win.Show()
		win.Focus()
	})
	win.OnClose(func(e *mygo.CloseEvent) {
		a.mu.Lock()
		quitting := a.quitting
		a.mu.Unlock()
		if quitting {
			return
		}
		// Closing the window keeps the app in the tray.
		e.PreventDefault()
		win.Hide()
		a.afterHide()
	})
	win.Page().OnWillNavigate(func(e *mygo.NavigateEvent) {
		if u, err := url.Parse(e.URL); err == nil && (u.Scheme == "http" || u.Scheme == "https") && !mygo.IsDev() && e.UserInitiated {
			e.PreventDefault()
			go mygo.Shell.OpenExternal(e.URL)
		}
	})
	a.mu.Lock()
	a.win = win
	a.mu.Unlock()
}

// afterHide hides the Dock icon on macOS and starts the timer of
// lightweight mode.
func (a *App) afterHide() {
	if runtime.GOOS == "darwin" {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}
	st := a.settings.Get()
	if !st.Lightweight.AutoEnter {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.lightTimer != nil {
		a.lightTimer.Stop()
	}
	a.lightTimer = time.AfterFunc(time.Duration(st.Lightweight.DelayMinutes)*time.Minute, a.enterLightweight)
}

// enterLightweight closes the main window, which frees the web view's
// memory; the core, the tray and the quick panel stay.
func (a *App) enterLightweight() {
	a.mu.Lock()
	win := a.win
	a.win = nil
	a.mu.Unlock()
	if win != nil && !win.IsDestroyed() {
		win.Destroy()
	}
	if runtime.GOOS == "darwin" {
		mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	}
	a.updateState(func(s *AppState) { s.Lightweight = true })
}

func (a *App) toggleMain() {
	a.mu.Lock()
	win := a.win
	a.mu.Unlock()
	if win != nil && !win.IsDestroyed() && win.IsVisible() && win.IsFocused() {
		win.Hide()
		a.afterHide()
		return
	}
	a.showMain()
}

// navigate shows the main window on a page: "proxies", "settings/sync" (a
// tab of the settings), or home when page is empty.
func (a *App) navigate(page string) {
	if page == "" {
		page = "home"
	}
	a.mu.Lock()
	win := a.win
	if win == nil || win.IsDestroyed() {
		// The window showMain creates takes it when its page boots
		// (App.takePage), as events sent before that are lost.
		a.pendingPage = page
		win = nil
	}
	a.mu.Unlock()
	a.showMain()
	if win != nil {
		_ = NavigateEvent.Emit(win, page)
	}
}

// ---- Deep links ----

// handleURL imports a profile from clash://install-config?url=...&name=...
func (a *App) handleURL(raw string) {
	select {
	case <-a.ready:
	default:
		a.mu.Lock()
		a.pendingURLs = append(a.pendingURLs, raw)
		a.mu.Unlock()
		return
	}
	u, err := url.Parse(raw)
	if err != nil {
		return
	}
	a.showMain()
	if u.Host != "install-config" && strings.Trim(u.Path, "/") != "install-config" {
		return
	}
	sub := u.Query().Get("url")
	if sub == "" {
		return
	}
	a.notify(Notice{Level: "info", Message: tr(a, "importing"), Detail: u.Query().Get("name"), Page: "profiles"})
	ctx, cancel := context.WithTimeout(a.ctx, 2*time.Minute)
	defer cancel()
	p, err := a.profiles.Create(ctx, profiles.NewProfile{Type: profiles.TypeRemote, URL: sub, Name: u.Query().Get("name")})
	if err != nil {
		a.notifyErr("profiles", tr(a, "importFailed"), err)
		return
	}
	a.notify(Notice{Level: "success", Message: tr(a, "imported"), Detail: p.Name, Page: "profiles"})
	a.navigate("profiles")
}

// ---- Quitting ----

func (a *App) beforeQuit(e *mygo.QuitEvent) {
	a.mu.Lock()
	if a.cleaned {
		a.mu.Unlock()
		return
	}
	a.quitting = true
	a.mu.Unlock()
	e.PreventDefault()
	go func() {
		a.cleanup()
		a.mu.Lock()
		a.cleaned = true
		a.mu.Unlock()
		mygo.App.Quit()
	}()
}

// cleanup puts the system back as it was: the system proxy off and the
// core stopped (which removes its routes).
func (a *App) cleanup() {
	select {
	case <-a.ready:
	default:
		return
	}
	a.stop()
	if err := a.usage.Flush(); err != nil {
		log.Printf("save the traffic statistics: %v", err)
	}
	a.guard.Stop()
	if a.settings.Get().SystemProxy.Enabled {
		if err := sysproxy.Set(sysproxy.Proxy{Enabled: false}); err != nil {
			log.Printf("turn the system proxy off: %v", err)
		}
	}
	if err := webrtc.Apply(webrtc.Unset, filepath.Join(a.dirs.Data, "webrtc-policy.json")); err != nil {
		log.Printf("webrtc policy: %v", err)
	}
	a.pac.Close()
	a.syncer.stop()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	a.core.Stop(ctx)
	mygo.GlobalShortcut.UnregisterAll()
	log.Printf("bye")
}

// quit quits the app.
func (a *App) quit() { mygo.App.Quit() }

var errNotReady = errors.New("the app is starting")
