package app

import (
	"context"
	"fmt"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// ProxyItem is a member of a group.
type ProxyItem struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	UDP      bool   `json:"udp"`
	XUDP     bool   `json:"xudp,omitempty"`
	TFO      bool   `json:"tfo,omitempty"`
	MPTCP    bool   `json:"mptcp,omitempty"`
	Alive    bool   `json:"alive"`
	Delay    int    `json:"delay"` // ms; 0 failed; -1 untested
	Provider string `json:"provider,omitempty"`
	IsGroup  bool   `json:"isGroup"`
	Now      string `json:"now,omitempty"`
	Icon     string `json:"icon,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
}

// ProxyGroup is a group of proxies.
type ProxyGroup struct {
	Name    string      `json:"name"`
	Type    string      `json:"type"`
	Now     string      `json:"now"`
	Fixed   string      `json:"fixed,omitempty"`
	Hidden  bool        `json:"hidden"`
	Icon    string      `json:"icon,omitempty"`
	TestURL string      `json:"testUrl,omitempty"`
	All     []ProxyItem `json:"all"`
}

// ProxiesView is what the proxies page shows.
type ProxiesView struct {
	Mode   string       `json:"mode"`
	Groups []ProxyGroup `json:"groups"`
	// Global is the group of global mode.
	Global *ProxyGroup `json:"global,omitempty"`
	// Direct lists the proxies of the configuration, for direct mode.
	Proxies   []ProxyItem    `json:"proxies"`
	Providers []ProviderView `json:"providers"`
}

// ProviderView is a provider of proxies.
type ProviderView struct {
	Name             string                    `json:"name"`
	VehicleType      string                    `json:"vehicleType"`
	Count            int                       `json:"count"`
	UpdatedAt        *time.Time                `json:"updatedAt,omitempty"`
	SubscriptionInfo *coreapi.SubscriptionInfo `json:"subscriptionInfo,omitempty"`
	TestURL          string                    `json:"testUrl,omitempty"`
}

func lastDelay(p coreapi.Proxy, testURL string) (int, bool) {
	hist := p.History
	alive := p.Alive
	if testURL != "" {
		if st, ok := p.Extra[testURL]; ok {
			hist, alive = st.History, st.Alive
		}
	}
	if len(hist) == 0 {
		return -1, alive
	}
	return hist[len(hist)-1].Delay, alive
}

func itemOf(p coreapi.Proxy, testURL string) ProxyItem {
	d, alive := lastDelay(p, testURL)
	return ProxyItem{
		Name: p.Name, Type: p.Type, UDP: p.UDP, XUDP: p.XUDP, TFO: p.TFO, MPTCP: p.MPTCP,
		Alive: alive, Delay: d, Provider: p.ProviderName, IsGroup: len(p.All) > 0 || p.Now != "",
		Now: p.Now, Icon: p.Icon, Hidden: p.Hidden,
	}
}

func groupOf(p coreapi.Proxy, all map[string]coreapi.Proxy) ProxyGroup {
	g := ProxyGroup{Name: p.Name, Type: p.Type, Now: p.Now, Fixed: p.Fixed, Hidden: p.Hidden, Icon: p.Icon, TestURL: p.TestURL, All: []ProxyItem{}}
	for _, name := range p.All {
		if m, ok := all[name]; ok {
			g.All = append(g.All, itemOf(m, p.TestURL))
		} else {
			g.All = append(g.All, ProxyItem{Name: name, Delay: -1})
		}
	}
	return g
}

// Proxies is the proxies page.
type Proxies struct{ a *App }

// View returns the groups and proxies of the running configuration, in
// the order of the configuration.
func (s Proxies) View(ctx context.Context) (ProxiesView, error) {
	c, err := s.a.core.Must()
	if err != nil {
		return ProxiesView{}, err
	}
	all, err := c.Proxies(ctx)
	if err != nil {
		return ProxiesView{}, err
	}
	v := ProxiesView{Mode: s.a.settings.Get().Clash.Mode, Groups: []ProxyGroup{}, Proxies: []ProxyItem{}, Providers: []ProviderView{}}
	var order []string
	if rt := s.a.runtimeConfig(); rt != nil {
		for _, g := range rt.Core.Slice("proxy-groups") {
			if gm, ok := g.(*yamlx.Map); ok {
				order = append(order, gm.String("name"))
			}
		}
	}
	if g, ok := all["GLOBAL"]; ok {
		gg := groupOf(g, all)
		v.Global = &gg
		if len(order) == 0 {
			order = g.All
		}
	}
	for _, name := range order {
		p, ok := all[name]
		if !ok || len(p.All) == 0 {
			continue
		}
		v.Groups = append(v.Groups, groupOf(p, all))
	}
	names := make([]string, 0, len(all))
	for name, p := range all {
		if len(p.All) == 0 && name != "GLOBAL" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		v.Proxies = append(v.Proxies, itemOf(all[n], ""))
	}
	if provs, err := c.ProxyProviders(ctx); err == nil {
		for name, p := range provs {
			if p.VehicleType == "Compatible" {
				continue // the configuration's own proxies
			}
			v.Providers = append(v.Providers, ProviderView{Name: name, VehicleType: p.VehicleType, Count: len(p.Proxies), UpdatedAt: p.UpdatedAt, SubscriptionInfo: p.SubscriptionInfo, TestURL: p.TestURL})
		}
		sort.Slice(v.Providers, func(i, j int) bool { return v.Providers[i].Name < v.Providers[j].Name })
	}
	return v, nil
}

// Select selects a member of a selector group.
func (s Proxies) Select(ctx context.Context, group, name string) error {
	return s.a.selectProxy(ctx, group, name)
}

func (a *App) selectProxy(ctx context.Context, group, name string) error {
	c, err := a.core.Must()
	if err != nil {
		return err
	}
	if err := c.SelectProxy(ctx, group, name); err != nil {
		return err
	}
	a.profiles.RecordSelection(group, name)
	if a.settings.Get().AutoCloseConnections {
		if snap, err := c.Connections(ctx); err == nil {
			for _, cn := range snap.Connections {
				if slices.Contains(cn.Chains, group) {
					_ = c.CloseConnection(ctx, cn.ID)
				}
			}
		}
	}
	_ = SelectionEvent.Broadcast(Selection{Group: group, Now: name})
	if a.tray != nil {
		go a.tray.rebuild()
	}
	if a.panel != nil {
		a.panel.invalidate()
	}
	return nil
}

// Unfix lets a url-test or fallback group choose again.
func (s Proxies) Unfix(ctx context.Context, group string) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	return c.UnfixProxy(ctx, group)
}

func (a *App) testURL(u string) (string, time.Duration) {
	st := a.settings.Get()
	if u == "" {
		u = st.Latency.URL
	}
	return u, time.Duration(st.Latency.TimeoutMs) * time.Millisecond
}

// Delay tests the delay of a proxy, in ms; 0 when it failed.
func (s Proxies) Delay(ctx context.Context, name, testURL string) (int, error) {
	c, err := s.a.core.Must()
	if err != nil {
		return 0, err
	}
	u, timeout := s.a.testURL(testURL)
	d, err := c.ProxyDelay(ctx, name, u, timeout)
	if err != nil {
		return 0, nil // a failed test is a result
	}
	return d, nil
}

// GroupDelay tests every member of a group at once.
func (s Proxies) GroupDelay(ctx context.Context, group, testURL string) (map[string]int, error) {
	c, err := s.a.core.Must()
	if err != nil {
		return nil, err
	}
	u, timeout := s.a.testURL(testURL)
	return c.GroupDelay(ctx, group, u, timeout)
}

// UpdateProvider downloads a provider again.
func (s Proxies) UpdateProvider(ctx context.Context, name string) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	return c.UpdateProxyProvider(ctx, name)
}

// HealthcheckProvider tests a provider's proxies.
func (s Proxies) HealthcheckProvider(ctx context.Context, name string) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	return c.HealthcheckProxyProvider(ctx, name)
}

// Connections is the connections page.
type Connections struct{ a *App }

// Stream sends the open connections every second.
func (s Connections) Stream(ctx context.Context, ch *mygo.Channel[coreapi.Connections]) error {
	return pump(ctx, s.a.conns, ch.Send)
}

// Close closes connections.
func (s Connections) Close(ctx context.Context, ids []string) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := c.CloseConnection(ctx, id); err != nil && !coreapi.IsNotFound(err) {
			return err
		}
	}
	return nil
}

// CloseAll closes every connection.
func (s Connections) CloseAll(ctx context.Context) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	return c.CloseAllConnections(ctx)
}

// Rules is the rules page.
type Rules struct{ a *App }

// RulesView is the rules and their providers.
type RulesView struct {
	Rules     []coreapi.Rule         `json:"rules"`
	Providers []coreapi.RuleProvider `json:"providers"`
}

// List returns the running rules and rule providers.
func (s Rules) List(ctx context.Context) (RulesView, error) {
	c, err := s.a.core.Must()
	if err != nil {
		return RulesView{}, err
	}
	rules, err := c.Rules(ctx)
	if err != nil {
		return RulesView{}, err
	}
	v := RulesView{Rules: rules, Providers: []coreapi.RuleProvider{}}
	if provs, err := c.RuleProviders(ctx); err == nil {
		for _, p := range provs {
			v.Providers = append(v.Providers, p)
		}
		sort.Slice(v.Providers, func(i, j int) bool { return v.Providers[i].Name < v.Providers[j].Name })
	}
	if v.Rules == nil {
		v.Rules = []coreapi.Rule{}
	}
	return v, nil
}

// UpdateProvider downloads a rule provider again.
func (s Rules) UpdateProvider(ctx context.Context, name string) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	return c.UpdateRuleProvider(ctx, name)
}

// SetDisabled disables or enables a rule until the next reload.
func (s Rules) SetDisabled(ctx context.Context, index int, disabled bool) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	return c.DisableRules(ctx, map[int]bool{index: disabled})
}

// Logs is the logs page.
type Logs struct{ a *App }

// Recent returns the logs kept, oldest first.
func (s Logs) Recent(ctx context.Context) ([]coreapi.LogEvent, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return nil, err
	}
	return s.a.logs.recent(), nil
}

// Stream sends new logs as they come.
func (s Logs) Stream(ctx context.Context, ch *mygo.Channel[coreapi.LogEvent]) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	if s.a.logs.hub == nil {
		return nil
	}
	return pump(ctx, s.a.logs.hub, ch.Send)
}

// Clear forgets the logs kept.
func (s Logs) Clear() { s.a.logs.clear() }

// Core is the core itself.
type Core struct{ a *App }

// Traffic sends the traffic every second.
func (s Core) Traffic(ctx context.Context, ch *mygo.Channel[coreapi.Traffic]) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	return pump(ctx, s.a.traffic, ch.Send)
}

// Memory sends the core's memory use every second.
func (s Core) Memory(ctx context.Context, ch *mygo.Channel[coreapi.Memory]) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	return pump(ctx, s.a.memory, ch.Send)
}

// Restart restarts the core.
func (s Core) Restart(ctx context.Context) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	go s.a.startCore(context.Background())
	return nil
}

// SetMode switches the mode: rule, global or direct.
func (s Core) SetMode(ctx context.Context, mode string) error {
	_, err := s.a.updateSettings(ctx, func(st *config.Settings) { st.Clash.Mode = mode })
	return err
}

// Reapply generates and applies the configuration again.
func (s Core) Reapply(ctx context.Context) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	return s.a.applyConfig(ctx)
}

// UpdateGeo downloads the GeoIP, GeoSite and ASN databases again.
func (s Core) UpdateGeo(ctx context.Context) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	return c.UpdateGeo(ctx)
}

// FlushFakeIP empties the fake-ip cache.
func (s Core) FlushFakeIP(ctx context.Context) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	return c.FlushFakeIP(ctx)
}

// FlushDNS empties the DNS cache.
func (s Core) FlushDNS(ctx context.Context) error {
	c, err := s.a.core.Must()
	if err != nil {
		return err
	}
	return c.FlushDNS(ctx)
}

// DNSQuery resolves a name with the core's DNS.
func (s Core) DNSQuery(ctx context.Context, name, typ string) (coreapi.DNSQueryResult, error) {
	c, err := s.a.core.Must()
	if err != nil {
		return coreapi.DNSQueryResult{}, err
	}
	return c.DNSQuery(ctx, name, typ)
}

// WebUIURL fills a dashboard's URL with the controller's host, port and
// secret.
func (s Core) WebUIURL(template string) (string, error) {
	st := s.a.settings.Get()
	ctl := st.Clash.Controller
	if !ctl.Enabled {
		return "", fmt.Errorf("%s", tr(s.a, "controllerOff"))
	}
	host, port, err := net.SplitHostPort(ctl.Address)
	if err != nil {
		return "", err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return strings.NewReplacer("%host", host, "%port", port, "%secret", ctl.Secret).Replace(template), nil
}

// OpenWebUI opens a dashboard in the browser.
func (s Core) OpenWebUI(template string) error {
	u, err := s.WebUIURL(template)
	if err != nil {
		return err
	}
	return mygo.Shell.OpenExternal(u)
}

// PortInUse reports whether a TCP port of the loopback is taken by
// another program.
func (s Core) PortInUse(port int) bool {
	if port == s.a.settings.Get().Clash.MixedPort && s.a.core.Client() != nil {
		return false // ours
	}
	ln, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(port))
	if err != nil {
		return true
	}
	ln.Close()
	return false
}
