package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/metacubex/mihomo/common/convert"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/enhance"
	"github.com/mygo-clash/mygo-clash/internal/profiles"
	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// ProfilesView is the list of profiles the page shows.
type ProfilesView struct {
	Current string             `json:"current"`
	Items   []profiles.Profile `json:"items"`
	// NextUpdates is when remote profiles update next, by UID.
	NextUpdates map[string]time.Time `json:"nextUpdates"`
	// DNS reports, by UID, whether the DNS override applies.
	DNS map[string]bool `json:"dns"`
}

func (a *App) profilesView() ProfilesView {
	idx := a.profiles.List()
	st := a.settings.Get()
	v := ProfilesView{Current: idx.Current, Items: []profiles.Profile{}, NextUpdates: map[string]time.Time{}, DNS: map[string]bool{}}
	for _, p := range idx.Items {
		if !p.IsProfile() {
			continue
		}
		v.Items = append(v.Items, p)
		if t := a.profiles.NextUpdate(p.UID); !t.IsZero() {
			v.NextUpdates[p.UID] = t
		}
		v.DNS[p.UID] = a.dnsOverride(st, p.UID)
	}
	return v
}

// Profiles manages profiles and their extensions.
type Profiles struct{ a *App }

// List returns the profiles.
func (s Profiles) List(ctx context.Context) (ProfilesView, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return ProfilesView{}, err
	}
	return s.a.profilesView(), nil
}

// Create creates a profile: a remote one downloads first.
func (s Profiles) Create(ctx context.Context, np profiles.NewProfile) (profiles.Profile, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return profiles.Profile{}, err
	}
	p, err := s.a.profiles.Create(ctx, np)
	if err != nil {
		return p, err
	}
	if cur, _ := s.a.profiles.Current(); cur.UID == p.UID {
		s.a.scheduleApply()
	}
	return p, nil
}

// ImportFile asks for a file and creates a local profile from it.
func (s Profiles) ImportFile(ctx context.Context) (*profiles.Profile, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return nil, err
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:  mygo.CallerWindow(ctx),
		Title:   tr(s.a, "importFile"),
		Filters: []mygo.FileFilter{{Name: "YAML", Extensions: []string{"yaml", "yml", "txt"}}},
	})
	if err != nil || len(paths) == 0 {
		return nil, err
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		return nil, err
	}
	name := strings.TrimSuffix(filepath.Base(paths[0]), filepath.Ext(paths[0]))
	p, err := s.a.profiles.Create(ctx, profiles.NewProfile{Type: profiles.TypeLocal, Name: name, Content: string(data)})
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// Update downloads a remote profile again; withProxy chooses the route
// (nil: as the profile says, falling back to the proxy).
func (s Profiles) Update(ctx context.Context, uid string, withProxy *bool) (profiles.Profile, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return profiles.Profile{}, err
	}
	_ = ProfilesEvent.Broadcast(s.a.profilesView())
	return s.a.profiles.Update(ctx, uid, withProxy)
}

// UpdateAll updates every remote profile, and returns those that failed.
func (s Profiles) UpdateAll(ctx context.Context) (map[string]string, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return nil, err
	}
	failed := map[string]string{}
	for _, p := range s.a.profiles.List().Items {
		if p.Type != profiles.TypeRemote {
			continue
		}
		if _, err := s.a.profiles.Update(ctx, p.UID, nil); err != nil {
			failed[p.UID] = err.Error()
		}
	}
	return failed, nil
}

// Patch changes a profile's name, description, URL or options.
func (s Profiles) Patch(ctx context.Context, uid string, p profiles.Patch) (profiles.Profile, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return profiles.Profile{}, err
	}
	return s.a.profiles.Patch(uid, p)
}

// Delete deletes a profile and its extensions.
func (s Profiles) Delete(ctx context.Context, uid string) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	return s.a.profiles.Delete(uid)
}

// Reorder orders the profiles.
func (s Profiles) Reorder(ctx context.Context, uids []string) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	return s.a.profiles.Reorder(uids)
}

// Activate makes a profile current and applies it. When the core rejects
// it, the previous profile stays current.
func (s Profiles) Activate(ctx context.Context, uid string) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	prev, hadPrev := s.a.profiles.Current()
	if err := s.a.profiles.SetCurrent(uid); err != nil {
		return err
	}
	if err := s.a.applyConfig(ctx); err != nil {
		if hadPrev && prev.UID != uid {
			_ = s.a.profiles.SetCurrent(prev.UID)
			_ = s.a.applyConfig(context.Background())
		}
		return err
	}
	if s.a.settings.Get().AutoCloseConnections {
		if c := s.a.core.Client(); c != nil {
			_ = c.CloseAllConnections(ctx)
		}
	}
	s.a.restoreSelections(ctx)
	return nil
}

// Content returns what an item holds: a profile's YAML, an extension's.
func (s Profiles) Content(ctx context.Context, uid string) (string, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return "", err
	}
	data, err := s.a.profiles.Content(uid)
	return string(data), err
}

// SaveContent replaces what an item holds, after checking it: profiles
// are checked by the core.
func (s Profiles) SaveContent(ctx context.Context, uid, content string) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	p, err := s.a.profiles.Get(uid)
	if err != nil {
		return err
	}
	if p.IsProfile() {
		if c := s.a.core.Client(); c != nil {
			if err := c.Validate(ctx, content); err != nil {
				return fmt.Errorf("%s: %w", tr(s.a, "invalidProfile"), err)
			}
		}
	}
	return s.a.profiles.SetContent(uid, []byte(content))
}

// Extension returns the UID of a profile's extension of a kind (rules,
// proxies, groups, merge, script), creating it the first time.
func (s Profiles) Extension(ctx context.Context, uid, kind string) (string, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return "", err
	}
	return s.a.profiles.Extension(uid, kind)
}

// Get returns an item.
func (s Profiles) Get(ctx context.Context, uid string) (profiles.Profile, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return profiles.Profile{}, err
	}
	return s.a.profiles.Get(uid)
}

// RuntimeYAML returns the configuration the core runs.
func (s Profiles) RuntimeYAML(ctx context.Context) (string, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return "", err
	}
	rt := s.a.runtimeConfig()
	if rt == nil {
		return "", errors.New(tr(s.a, "noRuntime"))
	}
	data, err := yamlx.Marshal(rt.Display)
	return string(data), err
}

// Logs returns what the extensions noted and printed at the last apply,
// by UID.
func (s Profiles) Logs(ctx context.Context) (map[string][]enhance.LogEntry, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return nil, err
	}
	rt := s.a.runtimeConfig()
	if rt == nil {
		return map[string][]enhance.LogEntry{}, nil
	}
	return rt.Logs, nil
}

// NameLists are the names of a profile's proxies and groups.
type NameLists struct {
	Proxies []string `json:"proxies"`
	Groups  []string `json:"groups"`
	// Builtin are the policies every configuration has: DIRECT, REJECT...
	Builtin []string `json:"builtin"`
}

// Names returns the names of a profile's proxies and groups, for the
// extension editors.
func (s Profiles) Names(ctx context.Context, uid string) (NameLists, error) {
	data, err := s.a.profiles.Content(uid)
	if err != nil {
		return NameLists{}, err
	}
	cfg, err := yamlx.Parse(data)
	if err != nil {
		return NameLists{}, err
	}
	p, g := enhance.ProxyNames(cfg)
	if p == nil {
		p = []string{}
	}
	if g == nil {
		g = []string{}
	}
	return NameLists{Proxies: p, Groups: g, Builtin: enhance.BuiltinPolicies}, nil
}

// HasDNS reports whether a profile configures DNS itself, which the DNS
// override would replace.
func (s Profiles) HasDNS(ctx context.Context, uid string) bool {
	data, err := s.a.profiles.Content(uid)
	if err != nil {
		return false
	}
	cfg, err := yamlx.Parse(data)
	return err == nil && cfg.Map("dns").Len() > 0
}

// SetDNSOverride turns the DNS override on or off for a profile.
func (s Profiles) SetDNSOverride(ctx context.Context, uid string, on bool) error {
	_, err := s.a.updateSettings(ctx, func(st *config.Settings) { st.DNS.Profiles[uid] = on })
	if err == nil {
		_ = ProfilesEvent.Broadcast(s.a.profilesView())
	}
	return err
}

// Export saves an item's content to a file the user picks. The file is
// not encrypted.
func (s Profiles) Export(ctx context.Context, uid string) (string, error) {
	p, err := s.a.profiles.Get(uid)
	if err != nil {
		return "", err
	}
	data, err := s.a.profiles.Content(uid)
	if err != nil {
		return "", err
	}
	ext := ".yaml"
	if p.Type == profiles.TypeScript {
		ext = ".js"
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: mygo.CallerWindow(ctx), DefaultPath: sanitizeFile(p.Name) + ext})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o600)
}

func sanitizeFile(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
	if s == "" {
		s = "profile"
	}
	return s
}

// Templates returns the templates of new extensions, by kind.
func (s Profiles) Templates() map[string]string {
	return map[string]string{"script": enhance.ScriptTemplate}
}

// Seq reads a rules, proxies or groups extension.
func (s Profiles) Seq(ctx context.Context, uid string) (enhance.SeqPatch, error) {
	data, err := s.a.profiles.Content(uid)
	if err != nil {
		return enhance.SeqPatch{}, err
	}
	p, err := enhance.ParseSeq(data)
	if p.Prepend == nil {
		p.Prepend = []any{}
	}
	if p.Append == nil {
		p.Append = []any{}
	}
	if p.Delete == nil {
		p.Delete = []string{}
	}
	return p, err
}

// SaveSeq writes a rules, proxies or groups extension.
func (s Profiles) SaveSeq(ctx context.Context, uid string, p enhance.SeqPatch) error {
	doc := yamlx.NewMap()
	doc.Set("prepend", yamlx.FromStd(nonNil(p.Prepend)))
	doc.Set("append", yamlx.FromStd(nonNil(p.Append)))
	del := make([]any, len(p.Delete))
	for i, d := range p.Delete {
		del[i] = d
	}
	doc.Set("delete", del)
	data, err := yamlx.Marshal(doc)
	if err != nil {
		return err
	}
	return s.a.profiles.SetContent(uid, data)
}

func nonNil(v []any) []any {
	if v == nil {
		return []any{}
	}
	return v
}

// ParseLinks turns share links (vmess://, ss://, trojan://, vless://,
// hysteria2://, ..., or a base64 list of them) into proxies.
func (s Profiles) ParseLinks(text string) ([]map[string]any, error) {
	proxies, err := convert.ConvertsV2Ray([]byte(strings.TrimSpace(text)))
	if err != nil {
		return nil, err
	}
	return proxies, nil
}

// RulesOf returns the rules a profile has itself, for the rules editor.
func (s Profiles) RulesOf(ctx context.Context, uid string) ([]string, error) {
	data, err := s.a.profiles.Content(uid)
	if err != nil {
		return nil, err
	}
	cfg, err := yamlx.Parse(data)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, r := range cfg.Slice("rules") {
		if str, ok := r.(string); ok {
			out = append(out, str)
		}
	}
	return out, nil
}
