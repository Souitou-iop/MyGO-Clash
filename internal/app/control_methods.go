package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/agent"
	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/profiles"
	"github.com/mygo-clash/mygo-clash/internal/usage"
)

type controlHandler func(ctx context.Context, a *App, args json.RawMessage) (any, error)

// controlHandlers serve agent.Methods, by name; a test keeps them in step.
// Set in init: the handlers reach the settings, whose changes reach the
// control socket.
var controlHandlers map[string]controlHandler

func init() {
	controlHandlers = map[string]controlHandler{
		"status":            ctlStatus,
		"proxies.list":      ctlProxies,
		"proxies.select":    ctlSelect,
		"proxies.delay":     ctlDelay,
		"connections.list":  ctlConnections,
		"connections.close": ctlCloseConnections,
		"traffic":           ctlTraffic,
		"stats":             ctlStats,
		"profiles.list":     ctlProfiles,
		"profiles.import":   ctlImport,
		"profiles.update":   ctlUpdateProfiles,
		"profiles.activate": ctlActivate,
		"profiles.delete":   ctlDeleteProfile,
		"settings.get":      ctlSettings,
		"settings.patch":    ctlPatchSettings,
		"sysproxy.set":      ctlSysProxy,
		"tun.set":           ctlTun,
		"mode.set":          ctlMode,
		"rules.match":       ctlMatch,
		"logs.tail":         ctlLogs,
		"tailscale.status":  ctlTailscale,
		"core.restart":      ctlRestart,
	}
}

// --- status

type agentExit struct {
	// Chain is the groups from the main one to the node.
	Chain []string `json:"chain"`
	Node  string   `json:"node"`
	Type  string   `json:"type,omitempty"`
	Delay int      `json:"delay"` // ms; 0 failed; -1 untested
}

type agentStatus struct {
	App              string        `json:"app"`
	Version          string        `json:"version"`
	Core             coremgr.State `json:"core"`
	SystemProxy      bool          `json:"systemProxy"`
	SystemProxyError string        `json:"systemProxyError,omitempty"`
	Tun              bool          `json:"tun"`
	TunAvailable     bool          `json:"tunAvailable"`
	Mode             string        `json:"mode"`
	Profile          agentRef      `json:"profile"`
	MixedPort        int           `json:"mixedPort"`
	Exit             *agentExit    `json:"exit,omitempty"`
	Speed            *agentSpeed   `json:"speed,omitempty"`
	ConfigError      string        `json:"configError,omitempty"`
	Lightweight      bool          `json:"lightweight"`
}

type agentRef struct {
	UID  string `json:"uid,omitempty"`
	Name string `json:"name,omitempty"`
}

type agentSpeed struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

func ctlStatus(ctx context.Context, a *App, _ json.RawMessage) (any, error) {
	s := a.snapshot()
	st := agentStatus{
		App: a.name, Version: Version, Core: a.core.State(),
		SystemProxy: s.SystemProxy, SystemProxyError: s.SystemProxyError, Tun: s.Tun, TunAvailable: s.TunAvailable,
		Mode: s.Mode, Profile: agentRef{s.ProfileUID, s.ProfileName}, MixedPort: s.MixedPort,
		ConfigError: agent.ScrubURLs(s.ConfigError), Lightweight: s.Lightweight,
	}
	if t, ok := a.traffic.Last(); ok {
		st.Speed = &agentSpeed{t.Up, t.Down}
	}
	if v, err := (Proxies{a}).View(ctx); err == nil {
		st.Exit = exitOf(v)
	}
	return st, nil
}

// exitOf follows the selections from the main group (GLOBAL in global
// mode, else the profile's first group) to the node they end at.
func exitOf(v ProxiesView) *agentExit {
	if v.Mode == "direct" {
		return &agentExit{Chain: []string{}, Node: "DIRECT", Delay: -1}
	}
	groups := map[string]ProxyGroup{}
	for _, g := range v.Groups {
		groups[g.Name] = g
	}
	var start *ProxyGroup
	if v.Global != nil {
		groups[v.Global.Name] = *v.Global
		if v.Mode == "global" {
			start = v.Global
		}
	}
	if start == nil {
		for i := range v.Groups {
			if !v.Groups[i].Hidden {
				start = &v.Groups[i]
				break
			}
		}
	}
	if start == nil {
		return nil
	}
	e := &agentExit{Chain: []string{start.Name}, Delay: -1}
	g := *start
	for range 16 {
		e.Node = g.Now
		for _, m := range g.All {
			if m.Name == g.Now {
				e.Type, e.Delay = m.Type, m.Delay
			}
		}
		next, ok := groups[g.Now]
		if !ok || slices.Contains(e.Chain, next.Name) {
			break
		}
		e.Chain = append(e.Chain, next.Name)
		g = next
	}
	return e
}

// --- proxies

func ctlProxies(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Group  string `json:"group"`
		Hidden bool   `json:"hidden"`
	}](raw)
	if err != nil {
		return nil, err
	}
	v, err := (Proxies{a}).View(ctx)
	if err != nil {
		return nil, err
	}
	all := v.Groups
	if v.Global != nil {
		all = append([]ProxyGroup{*v.Global}, all...)
	}
	if args.Group != "" {
		for _, g := range all {
			if g.Name == args.Group {
				return g, nil
			}
		}
		return nil, fmt.Errorf("no group %q", args.Group)
	}
	out := struct {
		Mode   string       `json:"mode"`
		Groups []ProxyGroup `json:"groups"`
	}{Mode: v.Mode, Groups: []ProxyGroup{}}
	for _, g := range v.Groups {
		if !g.Hidden || args.Hidden {
			out.Groups = append(out.Groups, g)
		}
	}
	if v.Global != nil && (v.Mode == "global" || args.Hidden) {
		out.Groups = append([]ProxyGroup{*v.Global}, out.Groups...)
	}
	return out, nil
}

func ctlSelect(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Group string `json:"group"`
		Node  string `json:"node"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if args.Group == "" || args.Node == "" {
		return nil, errors.New("group and node are required")
	}
	if err := a.selectProxy(ctx, args.Group, args.Node); err != nil {
		return nil, err
	}
	return map[string]string{"group": args.Group, "now": args.Node}, nil
}

func ctlDelay(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Node  string `json:"node"`
		Group string `json:"group"`
		URL   string `json:"url"`
	}](raw)
	if err != nil {
		return nil, err
	}
	switch {
	case args.Group != "":
		d, err := (Proxies{a}).GroupDelay(ctx, args.Group, args.URL)
		if err != nil {
			// The core fails the whole test when no member answers: that
			// is a result too, every member at 0.
			g, gerr := ctlProxies(ctx, a, json.RawMessage(`{"group":`+strconv.Quote(args.Group)+`}`))
			if gerr != nil {
				return nil, err
			}
			d = map[string]int{}
			for _, m := range g.(ProxyGroup).All {
				d[m.Name] = 0
			}
		}
		return map[string]any{"group": args.Group, "delays": d}, nil
	case args.Node != "":
		d, err := (Proxies{a}).Delay(ctx, args.Node, args.URL)
		if err != nil {
			return nil, err
		}
		return map[string]any{"node": args.Node, "delay": d}, nil
	}
	return nil, errors.New("node or group is required")
}

// --- connections and traffic

type agentConn struct {
	ID       string    `json:"id"`
	Network  string    `json:"network"`
	Host     string    `json:"host"`
	Port     string    `json:"port"`
	Process  string    `json:"process,omitempty"`
	Rule     string    `json:"rule"`
	Payload  string    `json:"rulePayload,omitempty"`
	Chains   []string  `json:"chains"`
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Start    time.Time `json:"start"`
}

func ctlConnections(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Filter string `json:"filter"`
		Limit  *int   `json:"limit"`
	}](raw)
	if err != nil {
		return nil, err
	}
	c, err := a.core.Must()
	if err != nil {
		return nil, err
	}
	snap, err := c.Connections(ctx)
	if err != nil {
		return nil, err
	}
	limit := 50
	if args.Limit != nil {
		limit = *args.Limit
	}
	conns := filterConns(snap.Connections, args.Filter)
	out := struct {
		Total       int         `json:"total"`
		Matched     int         `json:"matched"`
		Connections []agentConn `json:"connections"`
	}{Total: len(snap.Connections), Matched: len(conns), Connections: []agentConn{}}
	if limit > 0 && len(conns) > limit {
		conns = conns[:limit]
	}
	out.Connections = conns
	return out, nil
}

// filterConns keeps the connections that mention text, newest first.
func filterConns(all []coreapi.Connection, text string) []agentConn {
	text = strings.ToLower(text)
	out := []agentConn{}
	for _, c := range all {
		m := c.Metadata
		host := m.Host
		if host == "" {
			host = m.SniffHost
		}
		if host == "" {
			host = m.DestinationIP
		}
		ac := agentConn{ID: c.ID, Network: m.Network, Host: host, Port: m.DestinationPort, Process: m.Process,
			Rule: c.Rule, Payload: c.RulePayload, Chains: c.Chains, Upload: c.Upload, Download: c.Download, Start: c.Start}
		if ac.Chains == nil {
			ac.Chains = []string{}
		}
		if text != "" {
			hay := strings.ToLower(strings.Join(append([]string{host, m.DestinationIP, m.Process, c.Rule, c.RulePayload}, c.Chains...), "\n"))
			if !strings.Contains(hay, text) {
				continue
			}
		}
		out = append(out, ac)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Start.After(out[j].Start) })
	return out
}

func ctlCloseConnections(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		IDs []string `json:"ids"`
		All bool     `json:"all"`
	}](raw)
	if err != nil {
		return nil, err
	}
	switch {
	case args.All:
		if err := (Connections{a}).CloseAll(ctx); err != nil {
			return nil, err
		}
		return map[string]any{"closed": "all"}, nil
	case len(args.IDs) > 0:
		if err := (Connections{a}).Close(ctx, args.IDs); err != nil {
			return nil, err
		}
		return map[string]any{"closed": len(args.IDs)}, nil
	}
	return nil, errors.New("ids or all=true is required")
}

func ctlTraffic(ctx context.Context, a *App, _ json.RawMessage) (any, error) {
	c, err := a.core.Must()
	if err != nil {
		return nil, err
	}
	out := struct {
		Up          int64  `json:"up"`   // bytes per second
		Down        int64  `json:"down"` // bytes per second
		UpTotal     int64  `json:"upTotal"`
		DownTotal   int64  `json:"downTotal"`
		Connections int    `json:"connections"`
		Memory      uint64 `json:"memory"`
	}{}
	// A sample of the stream: its first value may be an old one.
	ch, done := a.traffic.Subscribe()
	timeout := time.After(1500 * time.Millisecond)
	n := 0
wait:
	for n < 2 {
		select {
		case t, ok := <-ch:
			if !ok {
				break wait
			}
			out.Up, out.Down, n = t.Up, t.Down, n+1
		case <-timeout:
			break wait
		case <-ctx.Done():
			break wait
		}
	}
	done()
	if snap, err := c.Connections(ctx); err == nil {
		out.UpTotal, out.DownTotal, out.Connections, out.Memory = snap.UploadTotal, snap.DownloadTotal, len(snap.Connections), snap.Memory
	}
	return out, nil
}

func ctlStats(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Range string `json:"range"`
		Top   int    `json:"top"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if args.Range == "" {
		args.Range = "today"
	}
	if !slices.Contains([]string{"today", "7d", "30d", "all"}, args.Range) {
		return nil, fmt.Errorf("range is today, 7d, 30d or all, not %q", args.Range)
	}
	if args.Top <= 0 {
		args.Top = 10
	}
	r, err := (Stats{a}).Query(ctx, usage.StatsQuery{Range: args.Range, Top: args.Top})
	if err != nil {
		return nil, err
	}
	return r, nil
}

// --- profiles

type agentProfile struct {
	UID       string          `json:"uid"`
	Name      string          `json:"name"`
	Type      string          `json:"type"`
	Current   bool            `json:"current"`
	URL       string          `json:"url,omitempty"` // masked
	Updated   time.Time       `json:"updated,omitzero"`
	NextAt    time.Time       `json:"nextUpdate,omitzero"`
	Usage     *profiles.Usage `json:"usage,omitempty"`
	Proxies   int             `json:"proxies"`
	Groups    int             `json:"groups"`
	LastError string          `json:"lastError,omitempty"`
}

func agentProfileOf(p profiles.Profile, current string, next time.Time) agentProfile {
	ap := agentProfile{UID: p.UID, Name: p.Name, Type: p.Type, Current: p.UID == current, URL: agent.MaskURL(p.URL),
		Usage: p.Usage, Proxies: p.Proxies, Groups: p.Groups, LastError: agent.ScrubURLs(p.LastError), NextAt: next}
	if p.Updated > 0 {
		ap.Updated = time.Unix(p.Updated, 0)
	}
	return ap
}

func ctlProfiles(ctx context.Context, a *App, _ json.RawMessage) (any, error) {
	v := a.profilesView()
	out := struct {
		Current  string         `json:"current"`
		Profiles []agentProfile `json:"profiles"`
	}{Current: v.Current, Profiles: []agentProfile{}}
	for _, p := range v.Items {
		out.Profiles = append(out.Profiles, agentProfileOf(p, v.Current, v.NextUpdates[p.UID]))
	}
	return out, nil
}

// findProfile finds a profile by uid, else by name (which must be unique).
func (a *App) findProfile(key string) (profiles.Profile, error) {
	if key == "" {
		return profiles.Profile{}, errors.New("a profile uid or name is required")
	}
	var byName []profiles.Profile
	for _, p := range a.profilesView().Items {
		if p.UID == key {
			return p, nil
		}
		if strings.EqualFold(p.Name, key) {
			byName = append(byName, p)
		}
	}
	switch len(byName) {
	case 1:
		return byName[0], nil
	case 0:
		return profiles.Profile{}, fmt.Errorf("no profile %q", key)
	}
	return profiles.Profile{}, fmt.Errorf("%d profiles are named %q: use the uid", len(byName), key)
}

func ctlImport(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		URL      string `json:"url"`
		Name     string `json:"name"`
		Activate bool   `json:"activate"`
	}](raw)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(strings.TrimSpace(args.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errors.New("url must be an http or https URL")
	}
	p, err := (Profiles{a}).Create(ctx, profiles.NewProfile{Type: profiles.TypeRemote, Name: args.Name, URL: u.String()})
	if err != nil {
		return nil, err
	}
	if args.Activate {
		if err := (Profiles{a}).Activate(ctx, p.UID); err != nil {
			return nil, fmt.Errorf("imported %s, but it did not apply: %w", p.UID, err)
		}
	}
	cur, _ := a.profiles.Current()
	return agentProfileOf(p, cur.UID, a.profiles.NextUpdate(p.UID)), nil
}

func ctlUpdateProfiles(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		UID string `json:"uid"`
		All bool   `json:"all"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if args.All {
		failed, err := (Profiles{a}).UpdateAll(ctx)
		if err != nil {
			return nil, err
		}
		for k, v := range failed {
			failed[k] = agent.ScrubURLs(v)
		}
		return map[string]any{"failed": failed}, nil
	}
	p, err := a.findProfile(args.UID)
	if err != nil {
		return nil, err
	}
	if p.Type != profiles.TypeRemote {
		return nil, fmt.Errorf("%s is a local profile: there is nothing to download", p.Name)
	}
	np, err := (Profiles{a}).Update(ctx, p.UID, nil)
	if err != nil {
		return nil, err
	}
	cur, _ := a.profiles.Current()
	return agentProfileOf(np, cur.UID, a.profiles.NextUpdate(np.UID)), nil
}

func ctlActivate(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Profile string `json:"profile"`
	}](raw)
	if err != nil {
		return nil, err
	}
	p, err := a.findProfile(args.Profile)
	if err != nil {
		return nil, err
	}
	if err := (Profiles{a}).Activate(ctx, p.UID); err != nil {
		return nil, err
	}
	return agentRef{p.UID, p.Name}, nil
}

func ctlDeleteProfile(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		UID     string `json:"uid"`
		Confirm bool   `json:"confirm"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if !args.Confirm {
		return nil, errors.New("deleting a profile cannot be undone: pass confirm=true")
	}
	p, err := a.findProfile(args.UID)
	if err != nil {
		return nil, err
	}
	if err := (Profiles{a}).Delete(ctx, p.UID); err != nil {
		return nil, err
	}
	return map[string]string{"deleted": p.UID, "name": p.Name}, nil
}

// --- settings and switches

// maskSettings hides the secrets of the settings, as the diagnostics do.
func maskSettings(st config.Settings) config.Settings {
	st.Clash.Controller.Secret = redact(st.Clash.Controller.Secret)
	st.Clash.LANAuth.Password = redact(st.Clash.LANAuth.Password)
	st.Sync.URL, st.Sync.Username = redact(st.Sync.URL), redact(st.Sync.Username)
	st.Sync.PinnedKey = redact(st.Sync.PinnedKey)
	st.Tailscale.ControlURL = redact(st.Tailscale.ControlURL)
	return st
}

func ctlSettings(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Path string `json:"path"`
	}](raw)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(maskSettings(a.settings.Get()))
	var doc any
	_ = json.Unmarshal(data, &doc)
	if args.Path == "" {
		return doc, nil
	}
	return lookupPath(doc, args.Path)
}

// lookupPath finds a dot-separated key in a decoded JSON document; list
// items are numbered.
func lookupPath(doc any, path string) (any, error) {
	cur := doc
	for _, k := range strings.Split(path, ".") {
		switch v := cur.(type) {
		case map[string]any:
			next, ok := v[k]
			if !ok {
				return nil, fmt.Errorf("no setting %q", path)
			}
			cur = next
		case []any:
			i, err := strconv.Atoi(k)
			if err != nil || i < 0 || i >= len(v) {
				return nil, fmt.Errorf("no setting %q", path)
			}
			cur = v[i]
		default:
			return nil, fmt.Errorf("no setting %q", path)
		}
	}
	return cur, nil
}

func ctlPatchSettings(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Patch map[string]any `json:"patch"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if len(args.Patch) == 0 {
		return nil, errors.New("patch is required: a JSON object of the settings to change")
	}
	if agent.HasRedacted(args.Patch) {
		return nil, fmt.Errorf("the patch holds %q: masked values cannot be written back; leave those keys out", agent.Redacted)
	}
	// Refuse unknown keys rather than drop them in silence.
	known, _ := json.Marshal(a.settings.Get())
	var doc any
	_ = json.Unmarshal(known, &doc)
	if err := unknownKeys(doc, args.Patch, ""); err != nil {
		return nil, err
	}
	data, _ := json.Marshal(args.Patch)
	st, err := a.changeSettings(ctx, data)
	if err != nil {
		return nil, err
	}
	out, _ := json.Marshal(maskSettings(st))
	var masked any
	_ = json.Unmarshal(out, &masked)
	return masked, nil
}

// unknownKeys reports a key of the patch that the settings do not have.
// Maps of the settings (by uid, by name) take any key.
func unknownKeys(doc any, patch map[string]any, prefix string) error {
	dm, ok := doc.(map[string]any)
	if !ok {
		return nil
	}
	for k, v := range patch {
		dv, ok := dm[k]
		if !ok {
			if isOpenMap(prefix) {
				continue
			}
			return fmt.Errorf("no setting %q", strings.TrimPrefix(prefix+"."+k, "."))
		}
		if pm, ok := v.(map[string]any); ok {
			if err := unknownKeys(dv, pm, strings.TrimPrefix(prefix+"."+k, ".")); err != nil {
				return err
			}
		}
	}
	return nil
}

// isOpenMap reports the settings that are maps of the user's keys.
func isOpenMap(path string) bool {
	return path == "dns.profiles" || path == "hotkeys.bindings" || strings.HasPrefix(path, "ui.")
}

func ctlSysProxy(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		On *bool `json:"on"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if args.On == nil {
		return nil, errors.New("on is required")
	}
	if _, err := a.updateSettings(ctx, func(s *config.Settings) { s.SystemProxy.Enabled = *args.On }); err != nil {
		return nil, err
	}
	s := a.snapshot()
	return map[string]any{"systemProxy": s.SystemProxy, "error": s.SystemProxyError}, nil
}

func ctlTun(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		On *bool `json:"on"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if args.On == nil {
		return nil, errors.New("on is required")
	}
	if *args.On && !a.snapshot().TunAvailable {
		return nil, errors.New("TUN needs the privileged service, which is not installed or not usable: install it from the app (Settings → Network), which asks for an administrator's password")
	}
	if _, err := a.updateSettings(ctx, func(s *config.Settings) { s.Tun.Enabled = *args.On }); err != nil {
		return nil, err
	}
	return map[string]any{"tun": a.settings.Get().Tun.Enabled}, nil
}

func ctlMode(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Mode string `json:"mode"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if !slices.Contains([]string{"rule", "global", "direct"}, args.Mode) {
		return nil, fmt.Errorf("mode is rule, global or direct, not %q", args.Mode)
	}
	if err := (Core{a}).SetMode(ctx, args.Mode); err != nil {
		return nil, err
	}
	return map[string]string{"mode": args.Mode}, nil
}

// --- rules, logs, Tailscale, the core

func ctlMatch(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[coreapi.MatchRequest](raw)
	if err != nil {
		return nil, err
	}
	if args.Target == "" {
		return nil, errors.New("target is required")
	}
	return (Rules{a}).Test(ctx, args)
}

var agentLogLevels = []string{"debug", "info", "warning", "error"}

func ctlLogs(ctx context.Context, a *App, raw json.RawMessage) (any, error) {
	args, err := decodeArgs[struct {
		Lines int    `json:"lines"`
		Level string `json:"level"`
	}](raw)
	if err != nil {
		return nil, err
	}
	if args.Lines <= 0 {
		args.Lines = 50
	}
	min := 0
	if args.Level != "" {
		if min = slices.Index(agentLogLevels, args.Level); min < 0 {
			return nil, fmt.Errorf("level is debug, info, warning or error, not %q", args.Level)
		}
	}
	return tailLogs(a.logs.recent(), args.Lines, min), nil
}

func tailLogs(all []coreapi.LogEvent, n, min int) []coreapi.LogEvent {
	out := []coreapi.LogEvent{}
	for _, l := range all {
		if slices.Index(agentLogLevels, l.Type) >= min {
			l.Payload = agent.ScrubURLs(l.Payload)
			out = append(out, l)
		}
	}
	return out[max(0, len(out)-n):]
}

type agentPeer struct {
	Name     string   `json:"name"`
	IPs      []string `json:"ips"`
	OS       string   `json:"os,omitempty"`
	Online   bool     `json:"online"`
	ExitNode bool     `json:"exitNode,omitempty"`
	CanExit  bool     `json:"canExit,omitempty"`
}

func ctlTailscale(ctx context.Context, a *App, _ json.RawMessage) (any, error) {
	st := a.settings.Get().Tailscale
	s, err := (Tailscale{a}).Status(ctx)
	if err != nil {
		return nil, err
	}
	out := struct {
		Mode         string      `json:"mode"`
		Source       string      `json:"source"`
		Available    bool        `json:"available"`
		BackendState string      `json:"backendState"`
		NeedsLogin   bool        `json:"needsLogin"`
		Tailnet      string      `json:"tailnet,omitempty"`
		Self         *agentPeer  `json:"self,omitempty"`
		Peers        []agentPeer `json:"peers"`
		ExitNode     string      `json:"exitNode,omitempty"`
		MagicDNS     bool        `json:"magicDns"`
		Health       []string    `json:"health,omitempty"`
		Error        string      `json:"error,omitempty"`
	}{Mode: st.Mode, Source: s.Source, Available: s.Available, BackendState: s.BackendState,
		NeedsLogin: s.BackendState == "NeedsLogin", Tailnet: s.TailnetName, MagicDNS: s.MagicDNS, Health: s.Health,
		Error: agent.ScrubURLs(s.Error), Peers: []agentPeer{}}
	if s.Self != nil {
		p := peerOf(*s.Self)
		out.Self = &p
	}
	for _, p := range s.Peers {
		ap := peerOf(p)
		if p.ExitNode || (s.ExitNodeID != "" && p.ID == s.ExitNodeID) {
			ap.ExitNode = true
			out.ExitNode = ap.Name
		}
		out.Peers = append(out.Peers, ap)
	}
	return out, nil
}

func peerOf(p coreapi.TailscalePeer) agentPeer {
	ips := p.TailscaleIPs
	if ips == nil {
		ips = []string{}
	}
	name := p.DNSName
	if name == "" {
		name = p.HostName
	}
	return agentPeer{Name: strings.TrimSuffix(name, "."), IPs: ips, OS: p.OS, Online: p.Online, CanExit: p.ExitNodeOption}
}

func ctlRestart(ctx context.Context, a *App, _ json.RawMessage) (any, error) {
	if err := (Core{a}).Restart(ctx); err != nil {
		return nil, err
	}
	return map[string]bool{"restarting": true}, nil
}
