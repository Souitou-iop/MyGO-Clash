// Package config holds the app's settings: what the user chooses in the
// app, saved as settings.json in the data directory. Secrets (passwords,
// keys) are not settings; they live sealed in the secret store.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"

	"github.com/mygo-clash/mygo-clash/internal/secure"
)

// Settings is everything the user sets in the app.
type Settings struct {
	// Language is the language of the interface: zh-CN, en, or "" to
	// follow the system.
	Language string `json:"language"`
	// Theme is system, light or dark.
	Theme string `json:"theme"`
	// Accent is the accent color, #RRGGBB, or "" for the app's.
	Accent     string `json:"accent"`
	FontFamily string `json:"fontFamily"`
	// CustomCSS is added to the pages of the app.
	CustomCSS string `json:"customCss"`
	// StartPage is the page the window opens on.
	StartPage string `json:"startPage"`
	// SilentStart starts the app in the tray, without its window.
	SilentStart bool `json:"silentStart"`
	// TrayClick is what clicking the tray icon does: menu, window or panel.
	TrayClick string `json:"trayClick"`
	// CopyEnvType is the shell of "Copy environment": posix, cmd,
	// powershell, fish or nushell.
	CopyEnvType   string `json:"copyEnvType"`
	Notifications bool   `json:"notifications"`
	// AutoCloseConnections closes connections when the selected proxy or
	// the mode changes, so that they reconnect through the new route.
	AutoCloseConnections bool `json:"autoCloseConnections"`
	// BuiltinEnhanced makes old profiles work with mihomo.
	BuiltinEnhanced bool `json:"builtinEnhanced"`
	AutoCheckUpdate bool `json:"autoCheckUpdate"`
	// CoreMode chooses how the core runs: auto (the service when installed,
	// else a child process), sidecar or service.
	CoreMode string `json:"coreMode"`
	// WebUIs are dashboards to open, with %host, %port and %secret.
	WebUIs []string `json:"webUis"`

	UI          UI          `json:"ui"`
	SystemProxy SystemProxy `json:"systemProxy"`
	Tun         Tun         `json:"tun"`
	Clash       Clash       `json:"clash"`
	DNS         DNS         `json:"dns"`
	Latency     Latency     `json:"latency"`
	Hotkeys     Hotkeys     `json:"hotkeys"`
	Tray        Tray        `json:"tray"`
	Lightweight Lightweight `json:"lightweight"`
	Logs        Logs        `json:"logs"`
	Tailscale   Tailscale   `json:"tailscale"`
	Sync        Sync        `json:"sync"`
	Backup      Backup      `json:"backup"`
}

// UI is how the interface looks.
type UI struct {
	CollapseNav   bool       `json:"collapseNav"`
	TrafficGraph  bool       `json:"trafficGraph"`
	MemoryUsage   bool       `json:"memoryUsage"`
	GroupIcons    bool       `json:"groupIcons"`
	ToastPosition string     `json:"toastPosition"` // top-right, top-left, bottom-right, bottom-left
	ProxyColumns  int        `json:"proxyColumns"`  // 0: as many as fit
	ProxyLayout   string     `json:"proxyLayout"`   // card, list
	PauseOnBlur   bool       `json:"pauseOnBlur"`
	HomeCards     []HomeCard `json:"homeCards"`
	// Nav lists the pages of the sidebar in order; pages missing from it
	// are hidden.
	Nav []string `json:"nav"`
}

// HomeCard is a card of the home page.
type HomeCard struct {
	ID      string `json:"id"`
	Visible bool   `json:"visible"`
}

// normalizeHomeCards drops unknown cards and adds missing ones, hidden.
// Layouts from before the control card, which had separate network and
// mode cards, start over from the default order and keep what was shown.
func normalizeHomeCards(cards, defaults []HomeCard) []HomeCard {
	legacy := false
	shown := map[string]bool{}
	for _, c := range cards {
		if c.ID == "network" || c.ID == "mode" {
			legacy = true
			shown["control"] = shown["control"] || c.Visible
			continue
		}
		shown[c.ID] = c.Visible
	}
	if legacy {
		cards = nil
		for _, c := range defaults {
			if v, ok := shown[c.ID]; ok {
				c.Visible = v
			}
			cards = append(cards, c)
		}
		return cards
	}
	out := make([]HomeCard, 0, len(defaults))
	for _, c := range cards {
		known := slices.ContainsFunc(defaults, func(h HomeCard) bool { return h.ID == c.ID })
		dup := slices.ContainsFunc(out, func(h HomeCard) bool { return h.ID == c.ID })
		if known && !dup {
			out = append(out, c)
		}
	}
	for _, c := range defaults {
		if !slices.ContainsFunc(out, func(h HomeCard) bool { return h.ID == c.ID }) {
			out = append(out, HomeCard{ID: c.ID, Visible: false})
		}
	}
	return out
}

// SystemProxy is the proxy the app sets for the operating system.
type SystemProxy struct {
	Enabled bool `json:"enabled"`
	// Host is the address applications reach the proxy at.
	Host string `json:"host"`
	// Bypass lists hosts that skip the proxy, separated by commas,
	// semicolons or new lines.
	Bypass           string `json:"bypass"`
	UseDefaultBypass bool   `json:"useDefaultBypass"`
	// PAC sets a proxy auto-configuration script instead of a proxy.
	PAC       bool   `json:"pac"`
	PACScript string `json:"pacScript"`
	// Guard sets the proxy again when another app changes it.
	Guard         bool `json:"guard"`
	GuardInterval int  `json:"guardInterval"` // seconds
}

// Tun is the virtual network interface, which captures all traffic.
type Tun struct {
	Enabled             bool     `json:"enabled"`
	Stack               string   `json:"stack"` // system, gvisor, mixed
	Device              string   `json:"device"`
	AutoRoute           bool     `json:"autoRoute"`
	StrictRoute         bool     `json:"strictRoute"`
	AutoDetectInterface bool     `json:"autoDetectInterface"`
	AutoRedirect        bool     `json:"autoRedirect"` // Linux
	DNSHijack           []string `json:"dnsHijack"`
	MTU                 int      `json:"mtu"`
	RouteExcludeAddress []string `json:"routeExcludeAddress"`
}

// Clash is the part of the core's configuration the app owns.
type Clash struct {
	Mode            string     `json:"mode"` // rule, global, direct
	MixedPort       int        `json:"mixedPort"`
	SocksPort       int        `json:"socksPort"`
	SocksEnabled    bool       `json:"socksEnabled"`
	HTTPPort        int        `json:"httpPort"`
	HTTPEnabled     bool       `json:"httpEnabled"`
	RedirPort       int        `json:"redirPort"`
	RedirEnabled    bool       `json:"redirEnabled"`
	TProxyPort      int        `json:"tproxyPort"`
	TProxyEnabled   bool       `json:"tproxyEnabled"`
	AllowLAN        bool       `json:"allowLan"`
	IPv6            bool       `json:"ipv6"`
	LogLevel        string     `json:"logLevel"` // debug, info, warning, error, silent
	UnifiedDelay    bool       `json:"unifiedDelay"`
	TCPConcurrent   bool       `json:"tcpConcurrent"`
	FindProcessMode string     `json:"findProcessMode"` // "", always, strict, off
	Interface       string     `json:"interface"`
	Controller      Controller `json:"controller"`
}

// Controller is the core's REST API on a TCP port, for dashboards.
type Controller struct {
	Enabled             bool     `json:"enabled"`
	Address             string   `json:"address"`
	Secret              string   `json:"secret"`
	AllowOrigins        []string `json:"allowOrigins"`
	AllowPrivateNetwork bool     `json:"allowPrivateNetwork"`
}

// DNS overrides the DNS of profiles.
type DNS struct {
	// DefaultOverride applies the override to profiles the user has not
	// decided for.
	DefaultOverride bool `json:"defaultOverride"`
	// Profiles records the decision for each profile, by uid.
	Profiles map[string]bool `json:"profiles"`
	// Config is the override, YAML with dns and hosts.
	Config string `json:"config"`
}

// Latency is how delays are tested.
type Latency struct {
	URL              string `json:"url"`
	TimeoutMs        int    `json:"timeoutMs"`
	AutoCheck        bool   `json:"autoCheck"`
	AutoCheckMinutes int    `json:"autoCheckMinutes"`
}

// Hotkeys are global keyboard shortcuts, by action.
type Hotkeys struct {
	Enabled  bool              `json:"enabled"`
	Bindings map[string]string `json:"bindings"`
}

// Hotkey actions.
const (
	HotkeyToggleWindow      = "toggle-window"
	HotkeyQuickPanel        = "quick-panel"
	HotkeyToggleSystemProxy = "toggle-system-proxy"
	HotkeyToggleTun         = "toggle-tun"
	HotkeyModeRule          = "mode-rule"
	HotkeyModeGlobal        = "mode-global"
	HotkeyModeDirect        = "mode-direct"
	HotkeyLightweight       = "lightweight"
	HotkeyReactivate        = "reactivate-profile"
)

// HotkeyActions lists the actions in the order the app shows them.
var HotkeyActions = []string{
	HotkeyToggleWindow, HotkeyQuickPanel, HotkeyToggleSystemProxy, HotkeyToggleTun,
	HotkeyModeRule, HotkeyModeGlobal, HotkeyModeDirect, HotkeyLightweight, HotkeyReactivate,
}

// Tray is what the tray icon shows.
type Tray struct {
	// ShowSpeed shows the traffic next to the icon (macOS).
	ShowSpeed bool `json:"showSpeed"`
	// Groups shows proxy groups as submenus, inline, or off.
	Groups      string `json:"groups"`
	InlineModes bool   `json:"inlineModes"`
}

// Lightweight mode closes the window to save memory, keeping the core,
// the tray and the quick panel.
type Lightweight struct {
	AutoEnter    bool `json:"autoEnter"`
	DelayMinutes int  `json:"delayMinutes"`
}

// Logs is the app's own logging.
type Logs struct {
	Level         string `json:"level"` // debug, info, warn, error
	MaxSizeMB     int    `json:"maxSizeMb"`
	MaxFiles      int    `json:"maxFiles"`
	AutoCleanDays int    `json:"autoCleanDays"` // 0: never
}

// Tailscale connects the app to a tailnet.
type Tailscale struct {
	// Mode is off; embedded, a node the core runs, which needs no
	// Tailscale app; or system, the Tailscale app installed on the
	// computer.
	Mode string `json:"mode"`

	// Embedded node.
	Hostname         string `json:"hostname"`
	ControlURL       string `json:"controlUrl"`
	Ephemeral        bool   `json:"ephemeral"`
	AcceptRoutes     bool   `json:"acceptRoutes"`
	ExitNode         string `json:"exitNode"`
	ExitNodeAllowLAN bool   `json:"exitNodeAllowLan"`
	// ControlVia is the proxy the node reaches its coordination server
	// and relays through, "" for direct.
	ControlVia string `json:"controlVia"`
	// ProxyName names the node's outbound in configurations.
	ProxyName string `json:"proxyName"`
	// JoinSelector adds the outbound to the first selector group.
	JoinSelector bool `json:"joinSelector"`
	// RouteSubnets sends the subnets peers advertise through the tailnet.
	RouteSubnets bool `json:"routeSubnets"`
	// ShareProxy offers the mixed proxy to the user's other devices on the
	// tailnet, at ShareProxyPort of the node.
	ShareProxy     bool `json:"shareProxy"`
	ShareProxyPort int  `json:"shareProxyPort"`

	// Both modes.
	// MagicDNS resolves tailnet names through Tailscale.
	MagicDNS bool `json:"magicDns"`
	// Coexist keeps the core's TUN and DNS away from the installed
	// Tailscale (system mode).
	Coexist bool `json:"coexist"`
}

// Sync synchronizes settings and profiles between devices through WebDAV,
// encrypted on the device.
type Sync struct {
	Enabled  bool   `json:"enabled"`
	URL      string `json:"url"`
	Username string `json:"username"`
	// Dir is the folder of the app on the server.
	Dir string `json:"dir"`
	// IntervalMinutes syncs periodically; 0 syncs only on demand and on
	// change.
	IntervalMinutes int  `json:"intervalMinutes"`
	OnChange        bool `json:"onChange"`
	// ConflictPolicy resolves a file both sides changed: ask, newest,
	// local or remote.
	ConflictPolicy string `json:"conflictPolicy"`
	// Categories are what syncs: profiles, settings, dns, tailscale.
	Categories []string `json:"categories"`
	// AllowInsecure allows http:// for servers on the local network.
	AllowInsecure bool `json:"allowInsecure"`
	// PinnedKey is the SHA-256 of the server certificate's public key,
	// base64, to trust a self-signed certificate.
	PinnedKey string `json:"pinnedKey"`
}

// Backup makes local backups.
type Backup struct {
	AutoIntervalHours int  `json:"autoIntervalHours"` // 0: off
	OnChange          bool `json:"onChange"`
	Keep              int  `json:"keep"`
}

// Defaults returns the settings of a new installation.
func Defaults() Settings {
	s := Settings{
		Theme:           "system",
		StartPage:       "home",
		TrayClick:       "menu",
		CopyEnvType:     "posix",
		Notifications:   true,
		BuiltinEnhanced: true,
		AutoCheckUpdate: true,
		CoreMode:        "auto",
		WebUIs: []string{
			"https://metacubex.github.io/metacubexd/#/setup?http=true&hostname=%host&port=%port&secret=%secret",
			"https://yacd.metacubex.one/?hostname=%host&port=%port&secret=%secret",
			"https://board.zash.run.place/#/setup?http=true&hostname=%host&port=%port&secret=%secret",
		},
		UI: UI{
			TrafficGraph:  true,
			MemoryUsage:   true,
			GroupIcons:    true,
			ToastPosition: "top-right",
			ProxyLayout:   "card",
			PauseOnBlur:   true,
			HomeCards: []HomeCard{
				{"control", true}, {"traffic", true}, {"proxy", true},
				{"profile", true}, {"ip", true}, {"tailscale", true},
				{"test", true}, {"core", true}, {"system", false},
			},
			Nav: []string{"home", "proxies", "profiles", "connections", "rules", "logs", "tailscale", "unlock", "settings"},
		},
		SystemProxy: SystemProxy{
			Host:             "127.0.0.1",
			UseDefaultBypass: true,
			PACScript:        DefaultPAC,
			GuardInterval:    30,
		},
		Tun: Tun{
			Stack:               "mixed",
			AutoRoute:           true,
			AutoDetectInterface: true,
			DNSHijack:           []string{"any:53"},
			MTU:                 1500,
		},
		Clash: Clash{
			Mode:         "rule",
			MixedPort:    7897,
			SocksPort:    7898,
			HTTPPort:     7899,
			RedirPort:    7895,
			TProxyPort:   7896,
			IPv6:         true,
			LogLevel:     "info",
			UnifiedDelay: true,
			Controller: Controller{
				Address:             "127.0.0.1:9097",
				AllowOrigins:        []string{"https://metacubex.github.io", "https://yacd.metacubex.one", "https://board.zash.run.place"},
				AllowPrivateNetwork: true,
			},
		},
		DNS: DNS{Profiles: map[string]bool{}, Config: DefaultDNSConfig},
		Latency: Latency{
			URL:              "https://www.gstatic.com/generate_204",
			TimeoutMs:        5000,
			AutoCheckMinutes: 10,
		},
		Hotkeys: Hotkeys{Bindings: map[string]string{}},
		Tray:    Tray{Groups: "submenu", ShowSpeed: false},
		Lightweight: Lightweight{
			DelayMinutes: 10,
		},
		Logs: Logs{Level: "info", MaxSizeMB: 8, MaxFiles: 5, AutoCleanDays: 7},
		Tailscale: Tailscale{
			Mode:           "off",
			ProxyName:      "Tailscale",
			AcceptRoutes:   true,
			RouteSubnets:   true,
			MagicDNS:       true,
			Coexist:        true,
			ShareProxyPort: 7890,
		},
		Sync: Sync{
			Dir:             "MyGO-Clash",
			IntervalMinutes: 30,
			OnChange:        true,
			ConflictPolicy:  "ask",
			Categories:      []string{"profiles", "settings", "dns"},
		},
		Backup: Backup{Keep: 10},
	}
	if runtime.GOOS == "windows" {
		s.CopyEnvType = "powershell"
	}
	if runtime.GOOS == "darwin" {
		s.Tray.ShowSpeed = false
	}
	return s
}

// DefaultDNSConfig is the DNS override offered to new installations.
const DefaultDNSConfig = `dns:
  enable: true
  listen: :53
  ipv6: true
  enhanced-mode: fake-ip
  fake-ip-range: 198.18.0.1/16
  fake-ip-filter-mode: blacklist
  fake-ip-filter:
    - '*.lan'
    - '*.local'
    - '*.arpa'
    - time.*.com
    - ntp.*.com
    - '+.market.xiaomi.com'
    - localhost.ptlogin2.qq.com
    - '*.msftncsi.com'
    - www.msftconnecttest.com
  default-nameserver:
    - system
    - 223.6.6.6
    - 8.8.8.8
  nameserver:
    - 8.8.8.8
    - https://doh.pub/dns-query
    - https://dns.alidns.com/dns-query
  fallback: []
  nameserver-policy: {}
  proxy-server-nameserver:
    - https://doh.pub/dns-query
    - https://dns.alidns.com/dns-query
  direct-nameserver: []
  direct-nameserver-follow-policy: false
  respect-rules: false
  use-hosts: false
  use-system-hosts: false
hosts: {}
`

// DefaultPAC is the proxy auto-configuration script offered to new
// installations; %mixed-port% and %proxy-host% are filled in.
const DefaultPAC = `function FindProxyForURL(url, host) {
  return "PROXY %proxy-host%:%mixed-port%; SOCKS5 %proxy-host%:%mixed-port%; DIRECT;";
}
`

var (
	hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	oneOf    = func(v string, opts ...string) bool { return slices.Contains(opts, v) }
)

// Normalize fixes what can be fixed and returns an error for the rest.
func (s *Settings) Normalize() error {
	d := Defaults()
	var errs []error
	bad := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if !oneOf(s.Theme, "system", "light", "dark") {
		s.Theme = d.Theme
	}
	if s.Accent != "" && !hexColor.MatchString(s.Accent) {
		bad("the accent color %q is not #RRGGBB", s.Accent)
	}
	if !oneOf(s.TrayClick, "menu", "window", "panel", "none") {
		s.TrayClick = d.TrayClick
	}
	if !oneOf(s.CopyEnvType, "posix", "cmd", "powershell", "fish", "nushell") {
		s.CopyEnvType = d.CopyEnvType
	}
	if !oneOf(s.CoreMode, "auto", "sidecar", "service") {
		s.CoreMode = d.CoreMode
	}
	if !oneOf(s.UI.ToastPosition, "top-right", "top-left", "bottom-right", "bottom-left") {
		s.UI.ToastPosition = d.UI.ToastPosition
	}
	if !oneOf(s.UI.ProxyLayout, "card", "list") {
		s.UI.ProxyLayout = d.UI.ProxyLayout
	}
	if s.UI.ProxyColumns < 0 || s.UI.ProxyColumns > 6 {
		s.UI.ProxyColumns = 0
	}
	if len(s.UI.Nav) == 0 {
		s.UI.Nav = d.UI.Nav
	} else if !slices.Contains(s.UI.Nav, "settings") {
		s.UI.Nav = append(s.UI.Nav, "settings") // settings cannot be hidden
	}
	s.UI.HomeCards = normalizeHomeCards(s.UI.HomeCards, d.UI.HomeCards)

	sp := &s.SystemProxy
	sp.Host = strings.TrimSpace(sp.Host)
	if sp.Host == "" {
		sp.Host = d.SystemProxy.Host
	} else if net.ParseIP(sp.Host) == nil && !validHostname(sp.Host) {
		bad("the proxy host %q is not an address", sp.Host)
	}
	if sp.GuardInterval < 1 {
		sp.GuardInterval = d.SystemProxy.GuardInterval
	}
	if strings.TrimSpace(sp.PACScript) == "" {
		sp.PACScript = DefaultPAC
	}

	t := &s.Tun
	if !oneOf(t.Stack, "system", "gvisor", "mixed") {
		t.Stack = d.Tun.Stack
	}
	if t.MTU != 0 && (t.MTU < 576 || t.MTU > 65535) {
		bad("the MTU %d is not between 576 and 65535", t.MTU)
	}
	t.DNSHijack = cleanList(t.DNSHijack)
	t.RouteExcludeAddress = cleanList(t.RouteExcludeAddress)
	for _, a := range t.RouteExcludeAddress {
		if _, err := netip.ParsePrefix(a); err != nil {
			bad("%q is not a CIDR range, such as 192.168.0.0/16", a)
		}
	}

	c := &s.Clash
	if !oneOf(c.Mode, "rule", "global", "direct") {
		c.Mode = d.Clash.Mode
	}
	if !oneOf(c.LogLevel, "debug", "info", "warning", "error", "silent") {
		c.LogLevel = d.Clash.LogLevel
	}
	if !oneOf(c.FindProcessMode, "", "always", "strict", "off") {
		c.FindProcessMode = ""
	}
	ports := map[int]string{}
	checkPort := func(name string, port int, on bool) {
		if !on {
			return
		}
		if port < 1 || port > 65535 {
			bad("the %s port %d is not between 1 and 65535", name, port)
			return
		}
		if other, ok := ports[port]; ok {
			bad("the %s port %d is also the %s port", name, port, other)
		}
		ports[port] = name
	}
	checkPort("mixed", c.MixedPort, true)
	checkPort("SOCKS", c.SocksPort, c.SocksEnabled)
	checkPort("HTTP", c.HTTPPort, c.HTTPEnabled)
	checkPort("redir", c.RedirPort, c.RedirEnabled && runtime.GOOS != "windows")
	checkPort("TProxy", c.TProxyPort, c.TProxyEnabled && runtime.GOOS == "linux")
	if c.Controller.Enabled {
		host, port, err := net.SplitHostPort(c.Controller.Address)
		if err != nil || port == "" {
			bad("the controller address %q is not host:port", c.Controller.Address)
		} else if host != "" && host != "127.0.0.1" && host != "localhost" && host != "::1" && c.Controller.Secret == "" {
			bad("a controller reachable from the network needs a secret")
		}
	}
	c.Controller.AllowOrigins = cleanList(c.Controller.AllowOrigins)

	if s.DNS.Profiles == nil {
		s.DNS.Profiles = map[string]bool{}
	}
	if s.Latency.URL == "" {
		s.Latency.URL = d.Latency.URL
	} else if u, err := url.Parse(s.Latency.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		bad("the test URL %q is not an http(s) URL", s.Latency.URL)
	}
	if s.Latency.TimeoutMs < 100 || s.Latency.TimeoutMs > 60000 {
		s.Latency.TimeoutMs = d.Latency.TimeoutMs
	}
	if s.Latency.AutoCheckMinutes < 1 {
		s.Latency.AutoCheckMinutes = d.Latency.AutoCheckMinutes
	}
	if s.Hotkeys.Bindings == nil {
		s.Hotkeys.Bindings = map[string]string{}
	}
	for action := range s.Hotkeys.Bindings {
		if !slices.Contains(HotkeyActions, action) {
			delete(s.Hotkeys.Bindings, action)
		}
	}
	if !oneOf(s.Tray.Groups, "submenu", "inline", "off") {
		s.Tray.Groups = d.Tray.Groups
	}
	if s.Lightweight.DelayMinutes < 1 {
		s.Lightweight.DelayMinutes = d.Lightweight.DelayMinutes
	}
	if !oneOf(s.Logs.Level, "debug", "info", "warn", "error") {
		s.Logs.Level = d.Logs.Level
	}
	if s.Logs.MaxSizeMB < 1 {
		s.Logs.MaxSizeMB = d.Logs.MaxSizeMB
	}
	if s.Logs.MaxFiles < 1 {
		s.Logs.MaxFiles = d.Logs.MaxFiles
	}

	ts := &s.Tailscale
	if !oneOf(ts.Mode, "off", "embedded", "system") {
		ts.Mode = "off"
	}
	ts.ProxyName = strings.TrimSpace(ts.ProxyName)
	if ts.ProxyName == "" {
		ts.ProxyName = d.Tailscale.ProxyName
	}
	if ts.Hostname != "" && !validHostname(ts.Hostname) {
		bad("the device name %q may only have letters, digits and hyphens", ts.Hostname)
	}
	if ts.ControlURL != "" {
		if u, err := url.Parse(ts.ControlURL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			bad("the control server %q is not a URL", ts.ControlURL)
		}
	}
	if ts.ShareProxyPort < 1 || ts.ShareProxyPort > 65535 {
		ts.ShareProxyPort = d.Tailscale.ShareProxyPort
	}

	sy := &s.Sync
	if !oneOf(sy.ConflictPolicy, "ask", "newest", "local", "remote") {
		sy.ConflictPolicy = d.Sync.ConflictPolicy
	}
	sy.Dir = strings.Trim(strings.TrimSpace(sy.Dir), "/")
	if sy.Dir == "" {
		sy.Dir = d.Sync.Dir
	}
	if sy.IntervalMinutes < 0 {
		sy.IntervalMinutes = 0
	}
	if sy.URL != "" {
		u, err := url.Parse(sy.URL)
		switch {
		case err != nil || u.Host == "":
			bad("the WebDAV address %q is not a URL", sy.URL)
		case u.Scheme == "http" && !sy.AllowInsecure:
			bad("the WebDAV address uses http://, which sends the password in the clear; use https:// or allow insecure connections")
		case u.Scheme != "http" && u.Scheme != "https":
			bad("the WebDAV address %q is not http(s)", sy.URL)
		}
	}
	if sy.Enabled && sy.URL == "" {
		bad("sync needs a WebDAV address")
	}
	sy.Categories = cleanList(sy.Categories)
	if s.Backup.Keep < 1 {
		s.Backup.Keep = d.Backup.Keep
	}
	if s.Backup.AutoIntervalHours < 0 {
		s.Backup.AutoIntervalHours = 0
	}
	return errors.Join(errs...)
}

var hostnameRe = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9.-]{0,251}[A-Za-z0-9])?$`)

func validHostname(s string) bool { return hostnameRe.MatchString(s) }

func cleanList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// Clone returns a deep copy.
func (s Settings) Clone() Settings {
	data, _ := json.Marshal(s)
	var c Settings
	_ = json.Unmarshal(data, &c)
	return c
}

// Store keeps the settings and saves them.
type Store struct {
	mu   sync.Mutex
	path string
	cur  Settings
}

// Load reads the settings in path; a missing file gives the defaults.
// Fields the file does not have keep their defaults.
func Load(path string) (*Store, error) {
	s := &Store{path: path, cur: Defaults()}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		_ = s.cur.Normalize()
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	cur := Defaults()
	if err := json.Unmarshal(data, &cur); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	_ = cur.Normalize() // keep what a newer or hand-edited file has; fix the rest
	s.cur = cur
	return s, nil
}

// Get returns a copy of the settings.
func (s *Store) Get() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.Clone()
}

// Update changes the settings with fn, validates and saves them, and
// returns the settings before and after.
func (s *Store) Update(fn func(*Settings)) (old, cur Settings, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old = s.cur.Clone()
	next := s.cur.Clone()
	fn(&next)
	if err := next.Normalize(); err != nil {
		return old, old, err
	}
	if err := s.save(next); err != nil {
		return old, old, err
	}
	s.cur = next
	return old, next.Clone(), nil
}

// Patch applies a JSON merge patch (RFC 7396) of the settings.
func (s *Store) Patch(patch []byte) (old, cur Settings, err error) {
	var p any
	if err := json.Unmarshal(patch, &p); err != nil {
		return Settings{}, Settings{}, fmt.Errorf("the patch is not JSON: %w", err)
	}
	var applyErr error
	old, cur, err = s.Update(func(st *Settings) {
		var doc any
		data, _ := json.Marshal(st)
		_ = json.Unmarshal(data, &doc)
		doc = MergePatch(doc, p)
		merged, _ := json.Marshal(doc)
		next := Defaults()
		next.DNS.Profiles = nil
		next.Hotkeys.Bindings = nil
		if err := json.Unmarshal(merged, &next); err != nil {
			applyErr = err
			return
		}
		*st = next
	})
	if applyErr != nil {
		return old, old, applyErr
	}
	return old, cur, err
}

// Replace sets all the settings, as restoring a backup does.
func (s *Store) Replace(next Settings) (old, cur Settings, err error) {
	return s.Update(func(st *Settings) { *st = next })
}

func (s *Store) save(st Settings) error {
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return secure.WriteFileAtomic(s.path, data, 0o600)
}

// MergePatch applies a JSON merge patch to a document decoded into any.
func MergePatch(doc, patch any) any {
	pm, ok := patch.(map[string]any)
	if !ok {
		return patch
	}
	dm, ok := doc.(map[string]any)
	if !ok {
		dm = map[string]any{}
	}
	for k, v := range pm {
		if v == nil {
			delete(dm, k)
			continue
		}
		dm[k] = MergePatch(dm[k], v)
	}
	return dm
}
