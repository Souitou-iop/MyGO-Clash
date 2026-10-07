package profiles

import (
	"context"
	"fmt"

	"github.com/mygo-clash/mygo-clash/internal/enhance"
	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// Inputs are what the runtime configuration depends on besides profiles.
type Inputs struct {
	Base enhance.Base
	// Builtin makes old profiles work with mihomo.
	Builtin bool
	// DNS is the DNS override to apply, or nil.
	DNS *yamlx.Map
	// Tailnet routes a tailnet, or nil.
	Tailnet *enhance.Tailnet
}

// Runtime is a generated configuration.
type Runtime struct {
	ProfileUID  string
	ProfileName string
	// Display is the configuration as the user sees and exports it.
	Display *yamlx.Map
	// Core is what the core runs: the same, with the tailnet's outbound as
	// a placeholder the core swaps for its node.
	Core *yamlx.Map
	// Logs are the notes and console output of each extension, by UID.
	Logs map[string][]enhance.LogEntry
	// OwnDNS reports that the profile configures DNS itself.
	OwnDNS bool
}

// emptyProfile is the configuration without a profile: everything direct.
const emptyProfile = "proxies: []\nproxy-groups: []\nrules:\n  - MATCH,DIRECT\n"

// Generate builds the runtime configuration of the current profile.
func (m *Manager) Generate(ctx context.Context, in Inputs) (*Runtime, error) {
	rt := &Runtime{Logs: map[string][]enhance.LogEntry{}}
	logf := func(uid string, entries ...enhance.LogEntry) {
		if len(entries) > 0 {
			rt.Logs[uid] = append(rt.Logs[uid], entries...)
		}
	}
	content := []byte(emptyProfile)
	cur, ok := m.Current()
	if ok {
		data, err := m.Content(cur.UID)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", cur.Name, err)
		}
		content = data
		rt.ProfileUID, rt.ProfileName = cur.UID, cur.Name
	}
	cfg, err := yamlx.Parse(content)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid YAML: %w", rt.ProfileName, err)
	}
	cfg = enhance.LowercaseKeys(cfg)
	if dns := cfg.Map("dns"); dns.Len() > 0 {
		rt.OwnDNS = true
	}

	// Lists first, on the profile as it came.
	if ok {
		for _, seq := range []struct{ kind, field string }{
			{TypeRules, "rules"}, {TypeProxies, "proxies"}, {TypeGroups, "proxy-groups"},
		} {
			uid := cur.Option.Extension(seq.kind)
			if uid == "" {
				continue
			}
			data, err := m.Content(uid)
			if err != nil {
				continue
			}
			patch, err := enhance.ParseSeq(data)
			if err != nil {
				logf(uid, enhance.LogEntry{Level: "exception", Message: err.Error()})
				continue
			}
			enhance.ApplySeq(cfg, seq.field, patch)
		}
	}

	// The app's settings, which extensions cannot override.
	enhance.ApplyBase(cfg, in.Base)
	if in.Builtin {
		enhance.ApplyBuiltins(cfg)
	}
	applied := enhance.ApplyDNSOverride(cfg, in.DNS)
	owned := enhance.Capture(cfg, applied)

	// Global extensions, then the profile's.
	var exts []struct{ uid, kind string }
	exts = append(exts, struct{ uid, kind string }{GlobalMerge, TypeMerge}, struct{ uid, kind string }{GlobalScript, TypeScript})
	if ok {
		if uid := cur.Option.Merge; uid != "" {
			exts = append(exts, struct{ uid, kind string }{uid, TypeMerge})
		}
		if uid := cur.Option.Script; uid != "" {
			exts = append(exts, struct{ uid, kind string }{uid, TypeScript})
		}
	}
	for _, e := range exts {
		data, err := m.Content(e.uid)
		if err != nil {
			continue
		}
		switch e.kind {
		case TypeMerge:
			patch, err := yamlx.Parse(data)
			if err != nil {
				logf(e.uid, enhance.LogEntry{Level: "exception", Message: err.Error()})
				continue
			}
			if patch.Len() == 0 {
				continue
			}
			enhance.Merge(cfg, patch)
		case TypeScript:
			out, logs, err := enhance.RunScript(ctx, string(data), cfg, rt.ProfileName)
			logf(e.uid, logs...)
			if err != nil {
				continue
			}
			cfg = out
		}
		if changed := owned.Changed(cfg); len(changed) > 0 {
			logf(e.uid, enhance.DiscardedNotes(changed)...)
			owned.Enforce(cfg)
		}
	}
	enhance.EnsureLANBind(cfg)
	enhance.DefaultGeoX(cfg)

	if in.Tailnet != nil {
		logf("tailscale", enhance.ApplyTailscale(cfg, *in.Tailnet)...)
	}
	logf("cleanup", enhance.CleanupGroups(cfg)...)
	rt.Display = enhance.Sort(cfg)
	rt.Core = rt.Display.Clone()
	if t := in.Tailnet; t != nil && t.Mode == "embedded" {
		for _, p := range rt.Core.Slice("proxies") {
			if pm, ok := p.(*yamlx.Map); ok && pm.String("name") == t.ProxyName && pm.String("type") == "tailscale" {
				*pm = *yamlx.MapOf("name", t.ProxyName, "type", "reject")
			}
		}
	}
	return rt, nil
}
