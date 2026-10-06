package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"rsc.io/qr"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/corehost"
	"github.com/mygo-clash/mygo-clash/internal/secure"
	"github.com/mygo-clash/mygo-clash/internal/service"
	"github.com/mygo-clash/mygo-clash/internal/sysproxy"
	"github.com/mygo-clash/mygo-clash/internal/tools"
)

// AppInfo describes the app.
type AppInfo struct {
	Name          string `json:"name"`
	Version       string `json:"version"`
	CoreVersion   string `json:"coreVersion"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	DataDir       string `json:"dataDir"`
	CoreDir       string `json:"coreDir"`
	LogsDir       string `json:"logsDir"`
	Hostname      string `json:"hostname"`
	Locale        string `json:"locale"`
	IsDev         bool   `json:"isDev"`
	Keyring       string `json:"keyring"`
	KeyringSecure bool   `json:"keyringSecure"`
	StartedAt     string `json:"startedAt"`
}

var startedAt = time.Now()

// AppService is the app as a whole.
type AppService struct{ a *App }

// Info describes the app.
func (s AppService) Info(ctx context.Context) (AppInfo, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return AppInfo{}, err
	}
	host, _ := os.Hostname()
	return AppInfo{
		Name: s.a.name, Version: Version, CoreVersion: corehost.MihomoVersion(), OS: runtime.GOOS, Arch: runtime.GOARCH,
		DataDir: s.a.dirs.Data, CoreDir: s.a.dirs.Core, LogsDir: s.a.dirs.Logs, Hostname: host, Locale: mygo.App.Locale(),
		IsDev: mygo.IsDev(), Keyring: s.a.sealer.Keyring().Name(), KeyringSecure: s.a.sealer.Keyring().Secure(),
		StartedAt: startedAt.Format(time.RFC3339),
	}, nil
}

// State returns the app's state.
func (s AppService) State(ctx context.Context) (AppState, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return AppState{}, err
	}
	return s.a.snapshot(), nil
}

// TakePage returns, once, the page a window just created should show
// (the target of a notification or a tray item), or "".
func (s AppService) TakePage() string {
	s.a.mu.Lock()
	defer s.a.mu.Unlock()
	p := s.a.pendingPage
	s.a.pendingPage = ""
	return p
}

// OpenDir opens a directory of the app: data, core or logs.
func (s AppService) OpenDir(kind string) error {
	dir := map[string]string{"data": s.a.dirs.Data, "core": s.a.dirs.Core, "logs": s.a.dirs.Logs}[kind]
	if dir == "" {
		return fmt.Errorf("unknown directory %q", kind)
	}
	return mygo.Shell.OpenPath(dir)
}

// OpenURL opens a URL in the browser.
func (s AppService) OpenURL(u string) error {
	if !strings.HasPrefix(u, "https://") && !strings.HasPrefix(u, "http://") {
		return errors.New("only web URLs open")
	}
	return mygo.Shell.OpenExternal(u)
}

// CopyText puts text on the clipboard.
func (s AppService) CopyText(text string) { mygo.Clipboard.WriteText(text) }

// EnvCommand returns the shell command that points programs at the proxy,
// for the shell of the settings.
func (s AppService) EnvCommand() string { return envCommand(s.a.settings.Get()) }

func envCommand(st config.Settings) string {
	host := st.SystemProxy.Host
	if host == "" {
		host = "127.0.0.1"
	}
	http := fmt.Sprintf("http://%s:%d", host, st.Clash.MixedPort)
	socks := fmt.Sprintf("socks5://%s:%d", host, st.Clash.MixedPort)
	switch st.CopyEnvType {
	case "cmd":
		return fmt.Sprintf("set http_proxy=%s & set https_proxy=%s", http, http)
	case "powershell":
		return fmt.Sprintf(`$env:HTTP_PROXY="%s"; $env:HTTPS_PROXY="%s"`, http, http)
	case "fish":
		return fmt.Sprintf("set -x http_proxy %s; set -x https_proxy %s; set -x all_proxy %s", http, http, socks)
	case "nushell":
		return fmt.Sprintf(`load-env {http_proxy: "%s", https_proxy: "%s", all_proxy: "%s"}`, http, http, socks)
	}
	return fmt.Sprintf("export https_proxy=%s http_proxy=%s all_proxy=%s", http, http, socks)
}

// CopyEnv copies the environment command.
func (s AppService) CopyEnv() string {
	cmd := envCommand(s.a.settings.Get())
	mygo.Clipboard.WriteText(cmd)
	return cmd
}

// EnterLightweight closes the window, keeping the core and the tray.
func (s AppService) EnterLightweight() { go s.a.enterLightweight() }

// Quit quits the app.
func (s AppService) Quit() { go s.a.quit() }

// Relaunch restarts the app.
func (s AppService) Relaunch() { go mygo.App.Relaunch() }

// QR returns an SVG of a QR code of text.
func (s AppService) QR(text string) (string, error) { return qrSVG(text) }

func qrSVG(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	n := code.Size
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="-2 -2 %d %d" shape-rendering="crispEdges"><rect x="-2" y="-2" width="%d" height="%d" fill="#fff"/><path fill="#000" d="`, n+4, n+4, n+4, n+4)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x, y)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String(), nil
}

// NetIface is a network interface.
type NetIface struct {
	Name  string   `json:"name"`
	MAC   string   `json:"mac"`
	Addrs []string `json:"addrs"`
	Up    bool     `json:"up"`
}

// NetworkInterfaces lists the computer's network interfaces.
func (s AppService) NetworkInterfaces() []NetIface {
	ifaces, _ := net.Interfaces()
	out := []NetIface{}
	for _, i := range ifaces {
		if i.Flags&net.FlagLoopback != 0 {
			continue
		}
		ni := NetIface{Name: i.Name, MAC: i.HardwareAddr.String(), Up: i.Flags&net.FlagUp != 0, Addrs: []string{}}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			ni.Addrs = append(ni.Addrs, a.String())
		}
		if len(ni.Addrs) > 0 {
			out = append(out, ni)
		}
	}
	return out
}

// AutoLaunch reports whether the app starts at login.
func (s AppService) AutoLaunch() bool { return mygo.App.OpenAtLogin() }

// SetAutoLaunch starts the app at login, or not.
func (s AppService) SetAutoLaunch(on bool) error { return mygo.App.SetOpenAtLogin(on) }

// Diagnostics returns a report for bug reports, without secrets, and copies
// it.
func (s AppService) Diagnostics(ctx context.Context) (string, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return "", err
	}
	info, _ := s.Info(ctx)
	st := s.a.settings.Get()
	st.Clash.Controller.Secret = redact(st.Clash.Controller.Secret)
	st.Sync.URL, st.Sync.Username = redact(st.Sync.URL), redact(st.Sync.Username)
	st.Tailscale.ControlURL = redact(st.Tailscale.ControlURL)
	settings, _ := json.MarshalIndent(st, "", "  ")
	state, _ := json.MarshalIndent(s.a.snapshot(), "", "  ")
	logs := s.a.logs.recent()
	var lines []string
	for _, l := range logs[max(0, len(logs)-60):] {
		lines = append(lines, fmt.Sprintf("%s [%s] %s", l.Time.Format("15:04:05"), l.Type, l.Payload))
	}
	report := fmt.Sprintf("# MyGO-Clash diagnostics\n\nVersion: %s (mihomo %s)\nOS: %s/%s\nKeyring: %s (secure: %v)\n\n## State\n%s\n\n## Settings\n%s\n\n## Recent core logs\n%s\n",
		info.Version, info.CoreVersion, info.OS, info.Arch, info.Keyring, info.KeyringSecure, state, settings, strings.Join(lines, "\n"))
	mygo.Clipboard.WriteText(report)
	return report, nil
}

func redact(s string) string {
	if s == "" {
		return ""
	}
	return "[redacted]"
}

// SettingsService reads and changes the settings.
type SettingsService struct{ a *App }

// Get returns the settings.
func (s SettingsService) Get(ctx context.Context) (config.Settings, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return config.Settings{}, err
	}
	return s.a.settings.Get(), nil
}

// Patch changes the settings with a JSON merge patch and does what the
// change implies; it returns the settings after.
func (s SettingsService) Patch(ctx context.Context, patch map[string]any) (config.Settings, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return config.Settings{}, err
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return config.Settings{}, err
	}
	return s.a.changeSettings(ctx, data)
}

// Defaults returns the settings of a new installation.
func (s SettingsService) Defaults() config.Settings { return config.Defaults() }

// DefaultBypass returns the hosts that skip the system proxy by default.
func (s SettingsService) DefaultBypass() []string { return sysproxy.DefaultBypass }

// SystemProxy returns the system's proxy setting as it is now.
func (s SettingsService) SystemProxy() (sysproxy.Proxy, error) { return sysproxy.Get() }

// RandomSecret returns a random secret for the controller.
func (s SettingsService) RandomSecret() string {
	return strings.TrimRight(base64.RawURLEncoding.EncodeToString(secure.RandomBytes(18)), "=")
}

// System is what touches the operating system: the service.
type System struct{ a *App }

// ServiceStatus checks the service.
func (s System) ServiceStatus(ctx context.Context) (ServiceState, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return ServiceState{}, err
	}
	return s.a.checkService(ctx), nil
}

// InstallService installs (or repairs, or updates) the service, asking for
// an administrator's authorization, and moves the core to it; with
// enableTun, it then turns TUN mode on.
func (s System) InstallService(ctx context.Context, enableTun bool) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	return s.a.installService(context.WithoutCancel(ctx), enableTun)
}

// UninstallService removes the service; the core moves back to the app,
// without TUN.
func (s System) UninstallService(ctx context.Context) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	if s.a.core.State().Mode == "service" {
		s.a.core.Stop(ctx)
	}
	if err := service.Elevate(ctx, tr(s.a, "servicePrompt"), "service", "uninstall", "--name", s.a.slug); err != nil {
		go s.a.startCore(context.Background())
		return err
	}
	_, _, _ = s.a.settings.Update(func(st *config.Settings) { st.Tun.Enabled = false })
	_ = SettingsEvent.Broadcast(s.a.settings.Get())
	s.a.checkService(ctx)
	go s.a.startCore(context.Background())
	return nil
}

// UWPLoopback lets Windows Store apps reach the proxy on the loopback.
func (s System) UWPLoopback(ctx context.Context) error {
	if runtime.GOOS != "windows" {
		return errors.New("only Windows has this restriction")
	}
	return service.Elevate(ctx, tr(s.a, "uwpPrompt"), "uwp-loopback")
}

// Tools tests the network through the proxy.
type Tools struct{ a *App }

// IPInfo returns the address the world sees, through the proxy, or
// directly when direct is set.
func (s Tools) IPInfo(ctx context.Context, direct bool) (tools.IPInfo, error) {
	proxy := s.a.proxyURL()
	if direct {
		proxy = ""
	}
	return tools.LookupIP(ctx, proxy)
}

// Sites returns the sites the home page tests.
func (s Tools) Sites() []tools.Site { return tools.DefaultSites }

// TestSites tests sites through the proxy, sending each result as it
// arrives.
func (s Tools) TestSites(ctx context.Context, sites []tools.Site, ch *mygo.Channel[tools.SiteResult]) error {
	if len(sites) == 0 {
		sites = tools.DefaultSites
	}
	proxy := s.a.proxyURL()
	results := make(chan tools.SiteResult)
	for _, site := range sites {
		go func() { results <- tools.TestSite(ctx, proxy, site) }()
	}
	for range sites {
		if err := ch.Send(<-results); err != nil {
			return nil
		}
	}
	return nil
}

// UnlockList returns the services the unlock page checks.
func (s Tools) UnlockList() []tools.Unlock { return tools.UnlockList() }

// CheckUnlock checks services through the proxy, sending each result as it
// arrives.
func (s Tools) CheckUnlock(ctx context.Context, ids []string, ch *mygo.Channel[tools.Unlock]) error {
	tools.CheckUnlock(ctx, s.a.proxyURL(), ids, func(u tools.Unlock) { _ = ch.Send(u) })
	return nil
}
