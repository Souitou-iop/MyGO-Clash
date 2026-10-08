package enhance

import (
	"fmt"
	"slices"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// Base is the part of the configuration the app owns: settings the user
// makes in the app, which win over what profiles and extensions say.
type Base struct {
	Mode       string // rule, global, direct
	MixedPort  int
	SocksPort  int // 0: off
	HTTPPort   int // 0: off
	RedirPort  int // 0: off
	TProxyPort int // 0: off
	AllowLAN   bool
	// LANAuth is the "user:pass" logins of the proxy ports for other
	// devices; empty asks for none.
	LANAuth       []string
	IPv6          bool
	LogLevel      string
	UnifiedDelay  bool
	TCPConcurrent bool
	// FindProcessMode is always, strict or off; "" keeps the profile's.
	FindProcessMode string
	// Interface binds outgoing connections to a network interface; "" keeps
	// the profile's.
	Interface string
	// ExternalController is the TCP address of the REST API for
	// dashboards; "" turns it off.
	ExternalController string
	Secret             string
	CORSOrigins        []string
	CORSPrivateNetwork bool
	Tun                Tun
}

// Tun is the TUN section the app manages.
type Tun struct {
	Enable              bool
	Stack               string // gvisor, system, mixed
	Device              string
	AutoRoute           bool
	StrictRoute         bool
	AutoDetectInterface bool
	AutoRedirect        bool // Linux
	DNSHijack           []string
	MTU                 int
	RouteExcludeAddress []string
}

// controlKeys are the top-level keys of Base, which extensions cannot change.
var controlKeys = []string{
	"external-controller", "external-controller-cors", "secret",
	"mixed-port", "socks-port", "port", "redir-port", "tproxy-port",
	"mode", "allow-lan", "log-level", "ipv6", "unified-delay",
}

// tunKeys are the keys of the tun section the app owns.
var tunKeys = []string{
	"enable", "stack", "device", "auto-route", "strict-route", "auto-detect-interface",
	"auto-redirect", "dns-hijack", "mtu", "route-exclude-address",
}

func strList(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// lanSkipAuth are the sources let in without a login: this device, so that
// the system proxy, TUN and local apps keep working.
var lanSkipAuth = []any{"127.0.0.1/8", "::1/128"}

// ApplyLANAuth makes the proxy ports ask other devices for a login, when
// allow-lan is on. It runs after the extensions, which cannot lift it. The
// profile's own logins and skipped ranges give way.
func ApplyLANAuth(cfg *yamlx.Map, users []string) {
	if allow, _ := cfg.Bool("allow-lan"); !allow || len(users) == 0 {
		return
	}
	cfg.Set("authentication", strList(users))
	cfg.Set("skip-auth-prefixes", slices.Clone(lanSkipAuth))
}

func setPort(cfg *yamlx.Map, key string, port int) {
	if port > 0 {
		cfg.Set(key, int64(port))
	} else {
		cfg.Delete(key)
	}
}

// ApplyBase writes the app's settings over the profile.
func ApplyBase(cfg *yamlx.Map, b Base) {
	cfg.Set("mode", b.Mode)
	setPort(cfg, "mixed-port", b.MixedPort)
	setPort(cfg, "socks-port", b.SocksPort)
	setPort(cfg, "port", b.HTTPPort)
	setPort(cfg, "redir-port", b.RedirPort)
	setPort(cfg, "tproxy-port", b.TProxyPort)
	cfg.Set("allow-lan", b.AllowLAN)
	cfg.Set("ipv6", b.IPv6)
	cfg.Set("log-level", b.LogLevel)
	cfg.Set("unified-delay", b.UnifiedDelay)
	cfg.Set("tcp-concurrent", b.TCPConcurrent)
	if b.FindProcessMode != "" {
		cfg.Set("find-process-mode", b.FindProcessMode)
	}
	if b.Interface != "" {
		cfg.Set("interface-name", b.Interface)
	}
	cfg.Set("external-controller", b.ExternalController)
	if b.ExternalController != "" {
		cfg.Set("secret", b.Secret)
		cors := yamlx.NewMap()
		cors.Set("allow-origins", strList(b.CORSOrigins))
		cors.Set("allow-private-network", b.CORSPrivateNetwork)
		cfg.Set("external-controller-cors", cors)
	} else {
		cfg.Delete("secret")
		cfg.Delete("external-controller-cors")
	}
	// The core serves its API on a private socket it is given at start;
	// the profile's own sockets are of no use.
	cfg.Delete("external-controller-unix")
	cfg.Delete("external-controller-pipe")
	cfg.Delete("external-controller-tls")
	cfg.Delete("external-ui")
	cfg.Delete("external-ui-url")

	tun := cfg.EnsureMap("tun")
	t := b.Tun
	tun.Set("enable", t.Enable)
	tun.Set("stack", t.Stack)
	if t.Device != "" {
		tun.Set("device", t.Device)
	}
	tun.Set("auto-route", t.AutoRoute)
	tun.Set("strict-route", t.StrictRoute)
	tun.Set("auto-detect-interface", t.AutoDetectInterface)
	if t.AutoRedirect {
		tun.Set("auto-redirect", true)
	} else {
		tun.Delete("auto-redirect")
	}
	tun.Set("dns-hijack", strList(t.DNSHijack))
	if t.MTU > 0 {
		tun.Set("mtu", int64(t.MTU))
	}
	if len(t.RouteExcludeAddress) > 0 {
		tun.Set("route-exclude-address", strList(t.RouteExcludeAddress))
	}
	if t.Enable {
		ensureTunDNS(cfg, b.IPv6)
	}
}

// ensureTunDNS turns on the DNS server that TUN needs to hijack DNS, with
// fake-ip unless the profile chose another mode.
func ensureTunDNS(cfg *yamlx.Map, ipv6 bool) {
	dns := cfg.EnsureMap("dns")
	mode := dns.String("enhanced-mode")
	if mode != "" && mode != "fake-ip" {
		dns.Set("enable", true)
		return
	}
	dns.Set("enable", true)
	dns.Set("ipv6", ipv6)
	if mode == "" {
		dns.Set("enhanced-mode", "fake-ip")
	}
	if !dns.Has("fake-ip-range") {
		dns.Set("fake-ip-range", "198.18.0.1/16")
	}
	if ipv6 && dns.String("fake-ip-range6") == "" {
		dns.Set("fake-ip-range6", "2001:2::/64")
	}
	// mihomo refuses a DNS server without upstreams; profiles written for
	// system proxy mode often have none.
	if len(dns.Slice("nameserver")) == 0 && dns.Map("nameserver-policy").Len() == 0 {
		dns.Set("nameserver", []any{"system", "https://doh.pub/dns-query", "https://dns.alidns.com/dns-query"})
		if len(dns.Slice("default-nameserver")) == 0 {
			dns.Set("default-nameserver", []any{"system", "223.5.5.5", "119.29.29.29"})
		}
	}
}

// ApplyDNSOverride merges the user's DNS override (a mapping with dns and
// hosts) into the configuration and returns what it applied, which the app
// then owns. Empty values do not override.
func ApplyDNSOverride(cfg, override *yamlx.Map) *yamlx.Map {
	applied := yamlx.NewMap()
	if override == nil {
		return applied
	}
	if hosts := override.Map("hosts"); hosts.Len() > 0 {
		cfg.Set("hosts", hosts.Clone())
		applied.Set("hosts", hosts.Clone())
	}
	dnsOverride := override.Map("dns")
	if dnsOverride == nil && !override.Has("dns") && !override.Has("hosts") {
		dnsOverride = override // a bare dns mapping
	}
	if dnsOverride.Len() == 0 {
		return applied
	}
	set := yamlx.NewMap()
	dnsOverride.Range(func(k string, v any) bool {
		if isSet(v) {
			set.Set(k, yamlx.CloneValue(v))
		}
		return true
	})
	if set.Len() == 0 {
		return applied
	}
	dns := cfg.EnsureMap("dns")
	set.Range(func(k string, v any) bool {
		dns.Set(k, yamlx.CloneValue(v))
		return true
	})
	if ipv6, _ := dns.Bool("ipv6"); ipv6 {
		mode := dns.String("enhanced-mode")
		if (mode == "" || mode == "fake-ip") && dns.String("fake-ip-range6") == "" {
			dns.Set("fake-ip-range6", "2001:2::/64")
		}
	}
	applied.Set("dns", set)
	return applied
}

func isSet(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case *yamlx.Map:
		return v.Len() > 0
	default:
		return true
	}
}

// Owned is a snapshot of the fields the app owns, taken once it wrote
// them and before extensions run, so that it can tell which fields an
// extension tried to change and write them back.
type Owned struct {
	control *yamlx.Map
	tun     *yamlx.Map
	dns     *yamlx.Map
	hosts   any
}

// Capture snapshots the owned fields of cfg. dnsApplied is what
// ApplyDNSOverride returned.
func Capture(cfg *yamlx.Map, dnsApplied *yamlx.Map) Owned {
	o := Owned{control: yamlx.NewMap(), tun: yamlx.NewMap(), dns: yamlx.NewMap()}
	for _, k := range controlKeys {
		if v, ok := cfg.Get(k); ok {
			o.control.Set(k, yamlx.CloneValue(v))
		}
	}
	if tun := cfg.Map("tun"); tun != nil {
		for _, k := range tunKeys {
			if v, ok := tun.Get(k); ok {
				o.tun.Set(k, yamlx.CloneValue(v))
			}
		}
	}
	if d := dnsApplied.Map("dns"); d != nil {
		o.dns = d.Clone()
	}
	if h, ok := dnsApplied.Get("hosts"); ok {
		o.hosts = yamlx.CloneValue(h)
	}
	return o
}

// Changed returns the owned keys whose values in cfg differ from the
// snapshot: the writes Enforce will discard.
func (o Owned) Changed(cfg *yamlx.Map) []string {
	var keys []string
	for _, k := range controlKeys {
		want, had := o.control.Get(k)
		got, has := cfg.Get(k)
		if had != has || (had && !yamlx.Equal(want, got)) {
			keys = append(keys, k)
		}
	}
	tun := cfg.Map("tun")
	for _, k := range o.tun.Keys() {
		if got, ok := tun.Get(k); !ok || !yamlx.Equal(o.tun.Value(k), got) {
			keys = append(keys, "tun."+k)
		}
	}
	dns := cfg.Map("dns")
	for _, k := range o.dns.Keys() {
		if got, ok := dns.Get(k); !ok || !yamlx.Equal(o.dns.Value(k), got) {
			keys = append(keys, "dns."+k)
		}
	}
	if o.hosts != nil && !yamlx.Equal(o.hosts, cfg.Value("hosts")) {
		keys = append(keys, "hosts")
	}
	return keys
}

// Enforce writes the snapshot back over cfg.
func (o Owned) Enforce(cfg *yamlx.Map) {
	for _, k := range controlKeys {
		if v, ok := o.control.Get(k); ok {
			cfg.Set(k, yamlx.CloneValue(v))
		} else {
			cfg.Delete(k)
		}
	}
	if o.tun.Len() > 0 {
		tun := cfg.EnsureMap("tun")
		o.tun.Range(func(k string, v any) bool {
			tun.Set(k, yamlx.CloneValue(v))
			return true
		})
	}
	if o.dns.Len() > 0 {
		dns := cfg.EnsureMap("dns")
		o.dns.Range(func(k string, v any) bool {
			dns.Set(k, yamlx.CloneValue(v))
			return true
		})
	}
	if o.hosts != nil {
		cfg.Set("hosts", yamlx.CloneValue(o.hosts))
	}
}

// DiscardedNotes turns the keys Changed found into notes for the
// extension's log.
func DiscardedNotes(keys []string) []LogEntry {
	slices.Sort(keys)
	notes := make([]LogEntry, 0, len(keys))
	for _, k := range keys {
		notes = append(notes, LogEntry{"warn", fmt.Sprintf("`%s` is managed in Settings; the value written here was discarded", k)})
	}
	return notes
}
