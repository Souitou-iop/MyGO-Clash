package profiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/enhance"
	"github.com/mygo-clash/mygo-clash/internal/secure"
	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// ErrNotFound is returned for items that do not exist.
var ErrNotFound = errors.New("no such profile")

// Index is the list of items and the current profile.
type Index struct {
	Current string    `json:"current"`
	Items   []Profile `json:"items"`
}

// Manager keeps the profiles in a directory, sealed.
type Manager struct {
	dir    string
	sealer *secure.Sealer

	mu    sync.Mutex
	index Index

	// ProxyAddr returns the address of the core's mixed proxy, for
	// downloads through it ("" when the core is not running).
	ProxyAddr func() string
	// OnChange is called after the profiles changed; content reports a
	// change of what an item holds, which matters to the runtime when the
	// item belongs to the current profile.
	OnChange func(uid string, content bool)

	updating sync.Map // uid → struct{}: updates in flight
}

const indexLabel = "profiles/index"

func contentLabel(uid string) string { return "profiles/item/" + uid }

// Open loads the profiles of dir.
func Open(dir string, sealer *secure.Sealer) (*Manager, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	m := &Manager{dir: dir, sealer: sealer}
	data, err := sealer.ReadFile(m.indexPath(), indexLabel)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, fmt.Errorf("read the profiles: %w", err)
	default:
		if err := json.Unmarshal(data, &m.index); err != nil {
			return nil, fmt.Errorf("read the profiles: %w", err)
		}
	}
	changed := false
	for _, g := range []struct{ uid, kind, name string }{
		{GlobalMerge, TypeMerge, "Global Merge"},
		{GlobalScript, TypeScript, "Global Script"},
	} {
		if m.find(g.uid) < 0 {
			m.index.Items = append(m.index.Items, Profile{UID: g.uid, Type: g.kind, Name: g.name, Updated: time.Now().Unix()})
			if err := m.writeContent(g.uid, []byte(template(g.kind))); err != nil {
				return nil, err
			}
			changed = true
		}
	}
	if changed {
		if err := m.saveIndex(); err != nil {
			return nil, err
		}
	}
	return m, nil
}

func (m *Manager) indexPath() string             { return filepath.Join(m.dir, "index.bin") }
func (m *Manager) contentPath(uid string) string { return filepath.Join(m.dir, uid+".bin") }

func (m *Manager) find(uid string) int {
	return slices.IndexFunc(m.index.Items, func(p Profile) bool { return p.UID == uid })
}

func (m *Manager) saveIndex() error {
	data, err := json.Marshal(m.index)
	if err != nil {
		return err
	}
	return m.sealer.WriteFile(m.indexPath(), data, indexLabel)
}

func (m *Manager) writeContent(uid string, data []byte) error {
	return m.sealer.WriteFile(m.contentPath(uid), data, contentLabel(uid))
}

func (m *Manager) changed(uid string, content bool) {
	if m.OnChange != nil {
		go m.OnChange(uid, content)
	}
}

// List returns the index.
func (m *Manager) List() Index {
	m.mu.Lock()
	defer m.mu.Unlock()
	return Index{Current: m.index.Current, Items: slices.Clone(m.index.Items)}
}

// Get returns an item.
func (m *Manager) Get(uid string) (Profile, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.find(uid)
	if i < 0 {
		return Profile{}, ErrNotFound
	}
	return m.index.Items[i], nil
}

// Current returns the current profile, if any.
func (m *Manager) Current() (Profile, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.find(m.index.Current)
	if i < 0 {
		return Profile{}, false
	}
	return m.index.Items[i], true
}

// Content returns what an item holds.
func (m *Manager) Content(uid string) ([]byte, error) {
	m.mu.Lock()
	known := m.find(uid) >= 0
	m.mu.Unlock()
	if !known {
		return nil, ErrNotFound
	}
	data, err := m.sealer.ReadFile(m.contentPath(uid), contentLabel(uid))
	if errors.Is(err, secure.ErrNotSealed) {
		return data, nil
	}
	return data, err
}

// NewProfile describes a profile to create.
type NewProfile struct {
	Type    string `json:"type"` // remote or local
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	URL     string `json:"url"`
	Content string `json:"content"` // a local profile's YAML; "" for the template
	Option  Option `json:"option"`
}

// Create creates a profile: it downloads a remote one first.
func (m *Manager) Create(ctx context.Context, np NewProfile) (Profile, error) {
	p := Profile{UID: newUID(np.Type), Type: np.Type, Name: strings.TrimSpace(np.Name), Desc: np.Desc, Option: np.Option}
	var content []byte
	switch np.Type {
	case TypeRemote:
		f, err := Fetch(ctx, np.URL, m.fetchOptions(np.Option))
		if err != nil && !np.Option.WithProxy && m.proxyAddr() != "" {
			// Subscriptions are often blocked where the proxy is needed.
			opt := np.Option
			opt.WithProxy = true
			if f2, err2 := Fetch(ctx, np.URL, m.fetchOptions(opt)); err2 == nil {
				f, err = f2, nil
			}
		}
		if err != nil {
			return Profile{}, err
		}
		p.URL = strings.TrimSpace(np.URL)
		applyFetched(&p, f)
		if p.Name == "" {
			p.Name = f.Name
		}
		if p.Option.UpdateInterval == 0 {
			p.Option.UpdateInterval = f.UpdateInterval
			if p.Option.UpdateInterval == 0 {
				p.Option.UpdateInterval = 24 * 60
			}
		}
		content = f.Content
	case TypeLocal:
		content = []byte(np.Content)
		if strings.TrimSpace(np.Content) == "" {
			content = []byte(template(TypeLocal))
		} else {
			normalized, converted, err := Normalize(content)
			if err != nil {
				return Profile{}, err
			}
			content, p.Converted = normalized, converted
		}
		if p.Name == "" {
			p.Name = "Local profile"
		}
		p.Updated = time.Now().Unix()
		p.Size = len(content)
		if cfg, err := yamlx.Parse(content); err == nil {
			p.Proxies, p.Groups = countOf(cfg)
		}
	default:
		return Profile{}, fmt.Errorf("cannot create a profile of type %q", np.Type)
	}
	if err := m.writeContent(p.UID, content); err != nil {
		return Profile{}, err
	}
	m.mu.Lock()
	m.index.Items = append(m.index.Items, p)
	first := m.index.Current == ""
	if first {
		m.index.Current = p.UID
	}
	err := m.saveIndex()
	m.mu.Unlock()
	if err != nil {
		return Profile{}, err
	}
	m.changed(p.UID, first)
	return p, nil
}

func applyFetched(p *Profile, f *Fetched) {
	p.Updated = time.Now().Unix()
	p.Usage = f.Usage
	if f.Home != "" {
		p.Home = f.Home
	}
	p.Size = len(f.Content)
	p.Proxies, p.Groups = f.Proxies, f.Groups
	p.Converted = f.Converted
	p.LastError = ""
}

func (m *Manager) proxyAddr() string {
	if m.ProxyAddr == nil {
		return ""
	}
	return m.ProxyAddr()
}

func (m *Manager) fetchOptions(o Option) FetchOptions {
	fo := FetchOptions{UserAgent: o.UserAgent, Insecure: o.Insecure, Timeout: time.Duration(o.TimeoutSeconds) * time.Second}
	if o.WithProxy {
		if addr := m.proxyAddr(); addr != "" {
			fo.Proxy = "http://" + addr
		}
	}
	return fo
}

// Update downloads a remote profile again. withProxy forces the download
// through the core's proxy (true), directly (false), or as the profile
// says (nil).
func (m *Manager) Update(ctx context.Context, uid string, withProxy *bool) (Profile, error) {
	if _, busy := m.updating.LoadOrStore(uid, struct{}{}); busy {
		return Profile{}, errors.New("the profile is updating already")
	}
	defer m.updating.Delete(uid)
	p, err := m.Get(uid)
	if err != nil {
		return Profile{}, err
	}
	if p.Type != TypeRemote {
		return Profile{}, errors.New("only remote profiles update")
	}
	opt := p.Option
	if withProxy != nil {
		opt.WithProxy = *withProxy
	}
	f, err := Fetch(ctx, p.URL, m.fetchOptions(opt))
	if err != nil && withProxy == nil && !opt.WithProxy && m.proxyAddr() != "" {
		opt.WithProxy = true
		if f2, err2 := Fetch(ctx, p.URL, m.fetchOptions(opt)); err2 == nil {
			f, err = f2, nil
		}
	}
	if err != nil {
		m.mu.Lock()
		if i := m.find(uid); i >= 0 {
			m.index.Items[i].LastError = err.Error()
			_ = m.saveIndex()
		}
		m.mu.Unlock()
		m.changed(uid, false)
		return Profile{}, err
	}
	if err := m.writeContent(uid, f.Content); err != nil {
		return Profile{}, err
	}
	m.mu.Lock()
	i := m.find(uid)
	if i < 0 {
		m.mu.Unlock()
		return Profile{}, ErrNotFound
	}
	applyFetched(&m.index.Items[i], f)
	p = m.index.Items[i]
	err = m.saveIndex()
	m.mu.Unlock()
	m.changed(uid, true)
	return p, err
}

// Due returns the remote profiles whose scheduled update is due.
func (m *Manager) Due(now time.Time) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var due []string
	for _, p := range m.index.Items {
		if p.Type != TypeRemote || p.Option.NoAutoUpdate || p.Option.UpdateInterval <= 0 {
			continue
		}
		next := time.Unix(p.Updated, 0).Add(time.Duration(p.Option.UpdateInterval) * time.Minute)
		if p.LastError != "" {
			// Retry failed updates sooner, but not in a tight loop.
			next = time.Unix(p.Updated, 0).Add(min(time.Duration(p.Option.UpdateInterval)*time.Minute, 30*time.Minute))
		}
		if !now.Before(next) {
			due = append(due, p.UID)
		}
	}
	return due
}

// NextUpdate returns when a profile updates next, or zero.
func (m *Manager) NextUpdate(uid string) time.Time {
	p, err := m.Get(uid)
	if err != nil || p.Type != TypeRemote || p.Option.NoAutoUpdate || p.Option.UpdateInterval <= 0 {
		return time.Time{}
	}
	return time.Unix(p.Updated, 0).Add(time.Duration(p.Option.UpdateInterval) * time.Minute)
}

// RunScheduler updates due profiles every minute until ctx ends. report
// is told about each update.
func (m *Manager) RunScheduler(ctx context.Context, report func(uid string, err error)) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		for _, uid := range m.Due(time.Now()) {
			uctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			_, err := m.Update(uctx, uid, nil)
			cancel()
			if report != nil {
				report(uid, err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Patch describes changes to an item.
type Patch struct {
	Name   *string `json:"name,omitempty"`
	Desc   *string `json:"desc,omitempty"`
	URL    *string `json:"url,omitempty"`
	Option *Option `json:"option,omitempty"`
}

// Patch changes an item's properties.
func (m *Manager) Patch(uid string, p Patch) (Profile, error) {
	m.mu.Lock()
	i := m.find(uid)
	if i < 0 {
		m.mu.Unlock()
		return Profile{}, ErrNotFound
	}
	it := &m.index.Items[i]
	if p.Name != nil {
		it.Name = strings.TrimSpace(*p.Name)
	}
	if p.Desc != nil {
		it.Desc = *p.Desc
	}
	if p.URL != nil && it.Type == TypeRemote {
		it.URL = strings.TrimSpace(*p.URL)
	}
	if p.Option != nil {
		o := *p.Option
		// The extensions are the app's to link, not the patch's.
		for _, k := range ExtensionKinds {
			o.setExtension(k, it.Option.Extension(k))
		}
		it.Option = o
	}
	out := *it
	err := m.saveIndex()
	m.mu.Unlock()
	m.changed(uid, false)
	return out, err
}

// SetContent replaces what an item holds, after checking it.
func (m *Manager) SetContent(uid string, content []byte) error {
	p, err := m.Get(uid)
	if err != nil {
		return err
	}
	if err := Check(p.Type, content); err != nil {
		return err
	}
	if err := m.writeContent(uid, content); err != nil {
		return err
	}
	m.mu.Lock()
	if i := m.find(uid); i >= 0 {
		it := &m.index.Items[i]
		it.Updated = time.Now().Unix()
		it.Size = len(content)
		if it.IsProfile() {
			if cfg, err := yamlx.Parse(content); err == nil {
				it.Proxies, it.Groups = countOf(cfg)
			}
		}
	}
	err = m.saveIndex()
	m.mu.Unlock()
	m.changed(uid, true)
	return err
}

// Check checks the content of an item of a type: valid YAML of the right
// shape, or a script that compiles.
func Check(kind string, content []byte) error {
	switch kind {
	case TypeScript:
		return enhance.CheckScript(string(content))
	case TypeRules, TypeProxies, TypeGroups:
		_, err := enhance.ParseSeq(content)
		return err
	default:
		_, err := yamlx.Parse(content)
		return err
	}
}

// Extension returns the UID of a profile's extension of a kind, creating
// it from its template the first time.
func (m *Manager) Extension(uid, kind string) (string, error) {
	if !slices.Contains(ExtensionKinds, kind) {
		return "", fmt.Errorf("unknown extension %q", kind)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.find(uid)
	if i < 0 {
		return "", ErrNotFound
	}
	if ext := m.index.Items[i].Option.Extension(kind); ext != "" && m.find(ext) >= 0 {
		return ext, nil
	}
	ext := Profile{UID: newUID(kind), Type: kind, Name: m.index.Items[i].Name + " · " + kind, Updated: time.Now().Unix()}
	content := []byte(template(kind))
	ext.Size = len(content)
	if err := m.writeContent(ext.UID, content); err != nil {
		return "", err
	}
	m.index.Items = append(m.index.Items, ext)
	m.index.Items[i].Option.setExtension(kind, ext.UID)
	return ext.UID, m.saveIndex()
}

// Delete deletes an item and its extensions. Deleting the current profile
// makes the next one current.
func (m *Manager) Delete(uid string) error {
	if uid == GlobalMerge || uid == GlobalScript {
		return errors.New("the global extensions cannot be deleted")
	}
	m.mu.Lock()
	i := m.find(uid)
	if i < 0 {
		m.mu.Unlock()
		return ErrNotFound
	}
	gone := []string{uid}
	for _, k := range ExtensionKinds {
		if ext := m.index.Items[i].Option.Extension(k); ext != "" {
			gone = append(gone, ext)
		}
	}
	m.index.Items = slices.DeleteFunc(m.index.Items, func(p Profile) bool { return slices.Contains(gone, p.UID) })
	wasCurrent := m.index.Current == uid
	if wasCurrent {
		m.index.Current = ""
		for _, p := range m.index.Items {
			if p.IsProfile() {
				m.index.Current = p.UID
				break
			}
		}
	}
	err := m.saveIndex()
	m.mu.Unlock()
	for _, g := range gone {
		_ = os.Remove(m.contentPath(g))
	}
	if wasCurrent {
		m.changed("", true) // another profile (or none) runs now
	} else {
		m.changed(uid, false)
	}
	return err
}

// Reorder puts the profiles in the order of uids; others keep theirs, after.
func (m *Manager) Reorder(uids []string) error {
	m.mu.Lock()
	pos := map[string]int{}
	for i, u := range uids {
		pos[u] = i
	}
	slices.SortStableFunc(m.index.Items, func(a, b Profile) int {
		ia, oka := pos[a.UID]
		ib, okb := pos[b.UID]
		switch {
		case oka && okb:
			return ia - ib
		case oka:
			return -1
		case okb:
			return 1
		}
		return 0
	})
	err := m.saveIndex()
	m.mu.Unlock()
	m.changed("", false)
	return err
}

// SetCurrent makes a profile current.
func (m *Manager) SetCurrent(uid string) error {
	m.mu.Lock()
	i := m.find(uid)
	if i < 0 || !m.index.Items[i].IsProfile() {
		m.mu.Unlock()
		return ErrNotFound
	}
	m.index.Current = uid
	err := m.saveIndex()
	m.mu.Unlock()
	m.changed(uid, true)
	return err
}

// RecordSelection remembers the selection of a group in the current
// profile.
func (m *Manager) RecordSelection(group, now string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i := m.find(m.index.Current)
	if i < 0 {
		return
	}
	sel := m.index.Items[i].Selected
	if j := slices.IndexFunc(sel, func(s Selected) bool { return s.Name == group }); j >= 0 {
		if sel[j].Now == now {
			return
		}
		sel[j].Now = now
	} else {
		sel = append(sel, Selected{Name: group, Now: now})
	}
	m.index.Items[i].Selected = sel
	_ = m.saveIndex()
}

// Bundle is every item with its content, for backups and sync.
type Bundle struct {
	Index    Index             `json:"index"`
	Contents map[string][]byte `json:"contents"`
}

// Export returns every item with its content.
func (m *Manager) Export() (Bundle, error) {
	idx := m.List()
	b := Bundle{Index: idx, Contents: map[string][]byte{}}
	for _, p := range idx.Items {
		data, err := m.Content(p.UID)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return Bundle{}, fmt.Errorf("%s: %w", p.Name, err)
		}
		b.Contents[p.UID] = data
	}
	return b, nil
}

// Import replaces every item with those of a bundle.
func (m *Manager) Import(b Bundle) error {
	for uid, data := range b.Contents {
		if strings.ContainsAny(uid, `/\.`) {
			return fmt.Errorf("invalid item %q", uid)
		}
		if err := m.writeContent(uid, data); err != nil {
			return err
		}
	}
	m.mu.Lock()
	old := m.index.Items
	m.index = b.Index
	err := m.saveIndex()
	m.mu.Unlock()
	for _, p := range old {
		if _, keep := b.Contents[p.UID]; !keep {
			_ = os.Remove(m.contentPath(p.UID))
		}
	}
	m.changed(b.Index.Current, true)
	return err
}

// UpsertItem adds or replaces one item with its content, as sync does.
// A nil content deletes the item.
func (m *Manager) UpsertItem(p *Profile, uid string, content []byte) error {
	if strings.ContainsAny(uid, `/\.`) {
		return fmt.Errorf("invalid item %q", uid)
	}
	if p == nil {
		m.mu.Lock()
		m.index.Items = slices.DeleteFunc(m.index.Items, func(q Profile) bool { return q.UID == uid })
		if m.index.Current == uid {
			m.index.Current = ""
		}
		err := m.saveIndex()
		m.mu.Unlock()
		_ = os.Remove(m.contentPath(uid))
		m.changed(uid, true)
		return err
	}
	if err := m.writeContent(uid, content); err != nil {
		return err
	}
	m.mu.Lock()
	if i := m.find(uid); i >= 0 {
		m.index.Items[i] = *p
	} else {
		m.index.Items = append(m.index.Items, *p)
	}
	err := m.saveIndex()
	m.mu.Unlock()
	m.changed(uid, true)
	return err
}

// SetCurrentQuiet sets the current profile without checks, as sync does.
func (m *Manager) SetCurrentQuiet(uid string) error {
	m.mu.Lock()
	m.index.Current = uid
	err := m.saveIndex()
	m.mu.Unlock()
	m.changed(uid, true)
	return err
}
