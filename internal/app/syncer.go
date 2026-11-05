package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/cloudsync"
	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/profiles"
	"github.com/mygo-clash/mygo-clash/internal/secure"
	"github.com/mygo-clash/mygo-clash/internal/webdav"
)

// Secrets of sync, in the sealed secret store.
const (
	secretSyncPassword = "sync.password"
	secretSyncKey      = "sync.key"
	// secretSyncIdentity is the device's key pair, to which other devices
	// seal a new vault key when they replace it.
	secretSyncIdentity = "sync.identity"
)

// usagePrefix is the path of each device's traffic statistics.
const usagePrefix = "usage/"

// SyncStatus is the state of sync.
type SyncStatus struct {
	Configured bool                 `json:"configured"`
	Enabled    bool                 `json:"enabled"`
	Running    bool                 `json:"running"`
	Pending    bool                 `json:"pending"`
	LastSync   time.Time            `json:"lastSync,omitzero"`
	LastError  string               `json:"lastError,omitempty"`
	Conflicts  []cloudsync.Conflict `json:"conflicts"`
	Devices    int                  `json:"devices"`
	DeviceID   string               `json:"deviceId"`
	DeviceName string               `json:"deviceName"`
	KeyID      string               `json:"keyId,omitempty"`
	// Plain is set when the vault on the server is not encrypted.
	Plain    bool              `json:"plain"`
	Report   *cloudsync.Report `json:"report,omitempty"`
	NextSync time.Time         `json:"nextSync,omitzero"`
	Labels   map[string]string `json:"labels"`
	// NeedPassphrase is set when another device replaced the vault's key
	// and this one needs the new passphrase to follow.
	NeedPassphrase bool `json:"needPassphrase,omitempty"`
	// RemovedBy names the device that removed this one from sync, after
	// it was; "?" when it is not known.
	RemovedBy string `json:"removedBy,omitempty"`
}

type syncer struct {
	a *App

	mu       sync.Mutex
	state    *cloudsync.State
	status   SyncStatus
	running  bool
	applying bool
	cancel   context.CancelFunc
	trigger  chan struct{}
	debounce *time.Timer
	ident    *cloudsync.Identity
	// names are the names of the vault's devices at the last sync.
	names map[string]string
}

const syncStateLabel = "sync/state"

func newSyncer(a *App) *syncer {
	s := &syncer{a: a, trigger: make(chan struct{}, 1), state: &cloudsync.State{}}
	data, err := a.sealer.ReadFile(s.statePath(), syncStateLabel)
	if err == nil {
		_ = json.Unmarshal(data, s.state)
	}
	if s.state.DeviceID == "" {
		s.state.DeviceID = strings.ToLower(base64.RawURLEncoding.EncodeToString(secure.RandomBytes(6)))
		s.save()
	}
	return s
}

func (s *syncer) statePath() string { return filepath.Join(s.a.dirs.Sync, "state.bin") }

func (s *syncer) save() {
	data, err := json.Marshal(s.state)
	if err != nil {
		return
	}
	if err := s.a.sealer.WriteFile(s.statePath(), data, syncStateLabel); err != nil {
		log.Printf("save the sync state: %v", err)
	}
}

func (s *syncer) deviceName() string {
	h, _ := os.Hostname()
	h = strings.TrimSuffix(h, ".local")
	if h == "" {
		h = runtime.GOOS
	}
	return h
}

func (s *syncer) configured() bool {
	st := s.a.settings.Get().Sync
	return st.URL != "" && s.a.secrets.Has(secretSyncKey)
}

// statusSnapshot returns the status, up to date.
func (s *syncer) statusSnapshot() SyncStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.a.settings.Get().Sync
	out := s.status
	out.Configured = s.configured()
	out.Enabled = st.Enabled && out.Configured
	out.Running = s.running
	out.LastSync, out.LastError = s.state.LastSync, s.state.LastError
	out.Conflicts = slices.Clone(s.state.Conflicts)
	if out.Conflicts == nil {
		out.Conflicts = []cloudsync.Conflict{}
	}
	out.DeviceID, out.DeviceName, out.KeyID = s.state.DeviceID, s.deviceName(), s.state.KeyID
	if k, err := s.keys(); err == nil {
		out.Plain = k.Plain()
	}
	out.Labels = s.labels()
	if out.Report != nil {
		for p, l := range out.Report.Labels {
			out.Labels[p] = l
		}
	}
	if out.Enabled && st.IntervalMinutes > 0 && !out.LastSync.IsZero() {
		out.NextSync = out.LastSync.Add(time.Duration(st.IntervalMinutes) * time.Minute)
	}
	return out
}

// labels names the paths of conflicts for people.
func (s *syncer) labels() map[string]string {
	out := map[string]string{}
	for _, c := range s.state.Conflicts {
		out[c.Path] = s.label(c.Path)
	}
	return out
}

func (s *syncer) label(path string) string {
	if uid, ok := strings.CutPrefix(path, "profiles/item/"); ok {
		if p, err := s.a.profiles.Get(uid); err == nil {
			return p.Name
		}
	}
	return path
}

func (s *syncer) broadcast() { _ = SyncEvent.Broadcast(s.statusSnapshot()) }

// start runs the scheduler.
func (s *syncer) start() {
	ctx, cancel := context.WithCancel(s.a.ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()
	go s.loop(ctx)
}

func (s *syncer) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func (s *syncer) restart() {
	s.stop()
	s.start()
	s.broadcast()
}

func (s *syncer) loop(ctx context.Context) {
	st := s.a.settings.Get().Sync
	interval := time.Duration(st.IntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	first := time.NewTimer(8 * time.Second)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	defer first.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-first.C:
		case <-tick.C:
			if st.IntervalMinutes <= 0 {
				continue
			}
		case <-s.trigger:
		}
		if !s.a.settings.Get().Sync.Enabled || !s.configured() {
			continue
		}
		if _, err := s.run(ctx); err != nil {
			log.Printf("sync: %v", err)
		}
	}
}

// changed records a local change, syncing shortly after when sync on
// change is on.
func (s *syncer) changed() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.applying {
		return // our own writes from the server
	}
	s.status.Pending = true
	st := s.a.settings.Get().Sync
	if !st.Enabled || !st.OnChange {
		return
	}
	if s.debounce != nil {
		s.debounce.Stop()
	}
	s.debounce = time.AfterFunc(10*time.Second, func() {
		select {
		case s.trigger <- struct{}{}:
		default:
		}
	})
}

// client returns a WebDAV client of the settings.
func (s *syncer) client(st config.Sync, password string) (*webdav.Client, error) {
	return webdav.New(st.URL, st.Username, password, webdav.Options{AllowInsecure: st.AllowInsecure, PinnedKey: st.PinnedKey})
}

// identity returns the device's key pair, made on first use; nil when the
// secret store cannot keep one.
func (s *syncer) identity() *cloudsync.Identity {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ident != nil {
		return s.ident
	}
	if raw, err := base64.StdEncoding.DecodeString(s.a.secrets.Get(secretSyncIdentity)); err == nil && len(raw) > 0 {
		if id, err := cloudsync.IdentityFromRaw(raw); err == nil {
			s.ident = id
			return id
		}
	}
	id := cloudsync.NewIdentity()
	if err := s.a.secrets.Set(secretSyncIdentity, base64.StdEncoding.EncodeToString(id.Raw())); err != nil {
		log.Printf("sync: keep the device key: %v", err)
		return nil
	}
	s.ident = id
	return id
}

// keepKeys saves the vault's keys.
func (s *syncer) keepKeys(k *cloudsync.Keys) error {
	return s.a.secrets.Set(secretSyncKey, base64.StdEncoding.EncodeToString(k.Raw()))
}

// syncLabel names a path for the activity of the vault: profiles by name;
// the interface names the others.
func syncLabel(path string, data []byte) string {
	if strings.HasPrefix(path, itemPrefix) {
		var it itemFile
		if json.Unmarshal(data, &it) == nil {
			return it.Meta.Name
		}
	}
	return ""
}

// keys returns the vault's keys this device keeps.
func (s *syncer) keys() (*cloudsync.Keys, error) {
	raw, err := base64.StdEncoding.DecodeString(s.a.secrets.Get(secretSyncKey))
	if err != nil || len(raw) == 0 {
		return nil, errors.New(tr(s.a, "syncNotSetUp"))
	}
	return cloudsync.KeysFromRaw(raw)
}

func (s *syncer) engine() (*cloudsync.Engine, error) {
	st := s.a.settings.Get()
	c, err := s.client(st.Sync, s.a.secrets.Get(secretSyncPassword))
	if err != nil {
		return nil, err
	}
	keys, err := s.keys()
	if err != nil {
		return nil, err
	}
	self := cloudsync.Device{Version: Version, Arch: runtime.GOARCH, Machine: machineID(), Tailnet: s.a.nudge.address()}
	ident := s.identity()
	if ident != nil {
		self.PubKey = ident.Public()
	}
	return &cloudsync.Engine{
		Client: c, Dir: st.Sync.Dir, Keys: keys, Source: appSource{a: s.a},
		Policy: st.Sync.ConflictPolicy, DeviceName: s.deviceName(),
		Mergeable: func(p string) bool {
			return p == "settings" || p == "dns" || p == "tailscale" || strings.HasPrefix(p, itemPrefix)
		},
		Wanted: appSource{a: s.a}.wanted,
		Self:   self, Identity: ident, Label: syncLabel, OnKeys: s.keepKeys,
	}, nil
}

// withEngine runs fn on an engine with a copy of the state, which replaces
// the state when fn is done, as a sync does; no sync runs meanwhile.
func (s *syncer) withEngine(fn func(eng *cloudsync.Engine) error) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return errors.New(tr(s.a, "syncBusy"))
	}
	s.running = true
	s.mu.Unlock()
	s.broadcast()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		s.broadcast()
	}()
	eng, err := s.engine()
	if err != nil {
		return err
	}
	s.mu.Lock()
	work := cloneState(s.state)
	s.mu.Unlock()
	eng.State = work
	err = fn(eng)
	s.mu.Lock()
	*s.state = *work
	s.save()
	s.mu.Unlock()
	return err
}

// run syncs now.
func (s *syncer) run(ctx context.Context) (cloudsync.Report, error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return cloudsync.Report{}, errors.New(tr(s.a, "syncBusy"))
	}
	s.running = true
	s.mu.Unlock()
	s.broadcast()
	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		s.broadcast()
	}()
	eng, err := s.engine()
	if err != nil {
		return cloudsync.Report{}, err
	}
	// The engine works on a copy, which replaces the state when it is done:
	// the status reads the state meanwhile.
	s.mu.Lock()
	work := cloneState(s.state)
	s.mu.Unlock()
	eng.State = work
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	started := time.Now()
	rep, err := eng.Sync(ctx)
	var removed *cloudsync.RemovedError
	s.mu.Lock()
	*s.state = *work
	ev := cloudsync.Event{At: started, Duration: time.Since(started).Milliseconds()}
	if err != nil {
		s.state.LastError = err.Error()
		ev.Error = err.Error()
		s.status.NeedPassphrase = errors.Is(err, cloudsync.ErrKeyReplaced)
	} else {
		s.status.Pending = false
		s.status.Report = &rep
		s.status.Devices = rep.Devices
		s.status.NeedPassphrase = false
		label := func(p string) string {
			if l := rep.Labels[p]; l != "" {
				return p + "\x00" + l
			}
			return p
		}
		for _, p := range rep.Pushed {
			ev.Pushed = append(ev.Pushed, label(p))
		}
		for _, p := range rep.Pulled {
			ev.Pulled = append(ev.Pulled, label(p))
		}
		for _, p := range rep.Merged {
			ev.Merged = append(ev.Merged, label(p))
		}
		ev.Conflicts = len(rep.Conflicts)
		s.names = map[string]string{}
		for id, d := range eng.Current().Devices {
			s.names[id] = d.DisplayName()
		}
	}
	// A sync that found nothing to do is not worth a line of the history.
	if err != nil || len(ev.Pushed)+len(ev.Pulled)+len(ev.Merged)+ev.Conflicts > 0 {
		s.state.Record(ev)
	}
	s.save()
	s.mu.Unlock()
	if errors.As(err, &removed) {
		s.removed(removed.By)
		return rep, err
	}
	if errors.Is(err, cloudsync.ErrKeyReplaced) {
		s.a.notify(Notice{Level: "warning", Message: tr(s.a, "syncKeyReplaced"), Action: "open-sync", Page: "settings/sync", Important: true})
		return rep, err
	}
	if err != nil {
		return rep, err
	}
	if len(rep.Pushed) > 0 {
		go s.a.nudge.tell(eng)
	}
	if len(rep.Conflicts) > 0 {
		s.a.notify(Notice{Level: "warning", Message: fmt.Sprintf(tr(s.a, "syncConflicts"), len(rep.Conflicts)), Action: "open-sync", Page: "settings/sync"})
	}
	return rep, nil
}

// removed turns sync off after another device removed this one, and
// forgets the vault's keys; the data stays.
func (s *syncer) removed(by string) {
	if by == "" {
		by = "?"
	}
	_, _ = s.a.updateSettings(s.a.ctx, func(cs *config.Settings) { cs.Sync.Enabled = false })
	_ = s.a.secrets.Set(secretSyncKey, "")
	s.mu.Lock()
	id := s.state.DeviceID
	*s.state = cloudsync.State{DeviceID: id}
	s.status.RemovedBy = by
	s.save()
	s.mu.Unlock()
	msg := tr(s.a, "syncRemoved")
	if by != "?" {
		msg = fmt.Sprintf(tr(s.a, "syncRemovedBy"), by)
	}
	s.a.notify(Notice{Level: "warning", Message: msg, Page: "settings/sync", Important: true})
	s.broadcast()
}

// appSource is the app's data, as sync sees it: settings, the DNS
// override, Tailscale's settings, and each profile and extension.
type appSource struct{ a *App }

const (
	pathSettings  = "settings"
	pathDNS       = "dns"
	pathTailscale = "tailscale"
	pathTSKey     = "tailscale/authkey"
	pathOrder     = "profiles/order"
	itemPrefix    = "profiles/item/"
)

// itemFile is a profile or extension as it syncs.
type itemFile struct {
	Meta    profiles.Profile `json:"meta"`
	Content []byte           `json:"content"`
}

func canonical(v any) []byte {
	data, _ := json.Marshal(v)
	var m any
	_ = json.Unmarshal(data, &m)
	out, _ := json.MarshalIndent(m, "", "  ")
	return out
}

func (s appSource) categories() map[string]bool {
	out := map[string]bool{}
	for _, c := range s.a.settings.Get().Sync.Categories {
		out[c] = true
	}
	return out
}

// wanted reports whether the categories of the settings include path.
func (s appSource) wanted(path string) bool {
	cats := s.categories()
	switch {
	case path == pathSettings:
		return cats["settings"]
	case path == pathDNS:
		return cats["dns"]
	case path == pathTailscale || path == pathTSKey:
		return cats["tailscale"]
	case strings.HasPrefix(path, usagePrefix):
		return cats["stats"]
	}
	return cats["profiles"] // the order, the profiles and their extensions
}

func (s appSource) Snapshot(ctx context.Context) (map[string]cloudsync.Item, error) {
	return s.snapshot(s.categories())
}

// snapshot exports the data of the categories.
func (s appSource) snapshot(cats map[string]bool) (map[string]cloudsync.Item, error) {
	out := map[string]cloudsync.Item{}
	st := s.a.settings.Get()
	mtime := time.Now()
	if fi, err := os.Stat(filepath.Join(s.a.dirs.Data, "settings.json")); err == nil {
		mtime = fi.ModTime()
	}
	if cats["settings"] {
		m := syncable(st)
		delete(m, "tailscale")
		out[pathSettings] = cloudsync.Item{Data: canonical(m), Modified: mtime}
	}
	if cats["dns"] {
		out[pathDNS] = cloudsync.Item{Data: canonical(st.DNS), Modified: mtime}
	}
	if cats["tailscale"] {
		out[pathTailscale] = cloudsync.Item{Data: canonical(syncable(st)["tailscale"]), Modified: mtime}
	}
	if cats["tailscale"] {
		if k, ok := s.a.sharedAuthKey(); ok {
			out[pathTSKey] = cloudsync.Item{Data: canonical(k), Modified: time.Unix(k.Updated, 0)}
		}
	}
	if cats["profiles"] {
		b, err := s.a.profiles.Export()
		if err != nil {
			return nil, err
		}
		var order []string
		for _, p := range b.Index.Items {
			order = append(order, p.UID)
			meta := p
			meta.Selected, meta.LastError, meta.Prev = nil, "", nil
			out[itemPrefix+p.UID] = cloudsync.Item{Data: canonical(itemFile{Meta: meta, Content: b.Contents[p.UID]}), Modified: time.Unix(p.Updated, 0)}
		}
		out[pathOrder] = cloudsync.Item{Data: canonical(order), Modified: mtime}
	}
	if cats["stats"] {
		// This device's own, and the others' as they came: a path stays
		// unchanged here unless this device wrote it.
		out[usagePrefix+s.a.syncer.state.DeviceID] = cloudsync.Item{Data: s.a.ownUsageData(), Modified: time.Now().Truncate(time.Hour)}
		for p, data := range s.a.readRemoteUsage() {
			out[p] = cloudsync.Item{Data: data, Modified: mtime}
		}
	}
	return out, nil
}

func (s appSource) Apply(ctx context.Context, changes map[string][]byte) error {
	s.a.syncer.mu.Lock()
	s.a.syncer.applying = true
	s.a.syncer.mu.Unlock()
	defer func() {
		s.a.syncer.mu.Lock()
		s.a.syncer.applying = false
		s.a.syncer.mu.Unlock()
	}()
	var errs []error
	settingsPatch := map[string]any{}
	for path, data := range changes {
		switch {
		case path == pathSettings && data != nil:
			var m map[string]any
			if err := json.Unmarshal(data, &m); err != nil {
				errs = append(errs, err)
				continue
			}
			mergeSyncable(settingsPatch, m)
		case path == pathTailscale && data != nil:
			var m map[string]any
			if err := json.Unmarshal(data, &m); err == nil {
				mergeSyncable(settingsPatch, map[string]any{"tailscale": m})
			}
		case path == pathTSKey:
			if data == nil {
				errs = append(errs, s.a.secrets.Set(secretTSAuthKey, ""))
				continue
			}
			var k sharedKey
			if json.Unmarshal(data, &k) == nil && k.Key != "" {
				errs = append(errs, s.a.secrets.Set(secretTSAuthKey, string(canonical(k))))
				s.a.ts.autoLogin(s.a.ts.current())
			}
		case path == pathDNS && data != nil:
			var d config.DNS
			if err := json.Unmarshal(data, &d); err == nil {
				settingsPatch["dns"] = d
			}
		case strings.HasPrefix(path, usagePrefix):
			if id := strings.TrimPrefix(path, usagePrefix); id != s.a.syncer.state.DeviceID {
				errs = append(errs, s.a.writeRemoteUsage(id, data))
			}
		case strings.HasPrefix(path, itemPrefix):
			uid := strings.TrimPrefix(path, itemPrefix)
			if data == nil {
				errs = append(errs, s.a.profiles.UpsertItem(nil, uid, nil))
				continue
			}
			var it itemFile
			if err := json.Unmarshal(data, &it); err != nil {
				errs = append(errs, err)
				continue
			}
			// The same subscription added on two devices twice exists twice
			// in the vault. Keep one, the same one everywhere, by the winner
			// rule; a copy dropped here is deleted from the server by the
			// next sync, which no longer finds it on this device.
			if it.Meta.Type == profiles.TypeRemote {
				if url := strings.TrimSpace(it.Meta.URL); url != "" {
					dups := s.a.profiles.SameURL(url, uid)
					winner := it.Meta
					for _, d := range dups {
						winner = profiles.SameURLWinner(winner, d)
					}
					if winner.UID != uid {
						continue
					}
					for _, d := range dups {
						errs = append(errs, s.a.profiles.Delete(d.UID))
					}
				}
			}
			if local, err := s.a.profiles.Get(uid); err == nil {
				// Selections and the previous version stay the device's.
				it.Meta.Selected, it.Meta.Prev = local.Selected, local.Prev
			}
			errs = append(errs, s.a.profiles.UpsertItem(&it.Meta, uid, it.Content))
		case path == pathOrder && data != nil:
			var order []string
			if json.Unmarshal(data, &order) == nil {
				errs = append(errs, s.a.profiles.Reorder(order))
			}
		}
	}
	if len(settingsPatch) > 0 {
		data, _ := json.Marshal(settingsPatch)
		if _, err := s.a.changeSettings(from(ctx, "sync"), data); err != nil {
			errs = append(errs, err)
		}
	}
	if _, ok := s.a.profiles.Current(); !ok {
		for _, p := range s.a.profiles.List().Items {
			if p.IsProfile() {
				errs = append(errs, s.a.profiles.SetCurrentQuiet(p.UID))
				break
			}
		}
	}
	return errors.Join(errs...)
}

// mergeSyncable writes the remote values of synced settings into a patch.
func mergeSyncable(dst, src map[string]any) {
	for k, v := range src {
		dst[k] = v
	}
}

func (s appSource) Merge(path string, base, local, remote []byte, preferRemote bool) ([]byte, bool) {
	if strings.HasPrefix(path, itemPrefix) {
		var l, r itemFile
		if json.Unmarshal(local, &l) != nil || json.Unmarshal(remote, &r) != nil {
			return nil, false
		}
		// Two devices refreshed the same subscription: both versions hold
		// the same content, and differ only in what the fetch reports (when
		// it ran, the traffic counted). There is nothing to choose between,
		// so merge the metadata; contents that differ are a real conflict.
		if !bytes.Equal(l.Content, r.Content) {
			return nil, false
		}
		return cloudsync.MergeJSON(base, local, remote, preferRemote)
	}
	switch path {
	case pathSettings, pathDNS, pathTailscale:
		return cloudsync.MergeJSON(base, local, remote, preferRemote)
	}
	return nil, false
}

// KeepBoth keeps the server's version of a profile as a new profile.
func (s appSource) KeepBoth(ctx context.Context, path string, remote []byte, device string) (string, error) {
	uid, ok := strings.CutPrefix(path, itemPrefix)
	if !ok {
		return "", fmt.Errorf("%s cannot be kept twice", path)
	}
	var it itemFile
	if err := json.Unmarshal(remote, &it); err != nil {
		return "", err
	}
	meta := it.Meta
	meta.UID = uid + "c" + strings.ToLower(base64.RawURLEncoding.EncodeToString(secure.RandomBytes(3)))
	meta.Name = fmt.Sprintf("%s (%s %s)", meta.Name, tr(s.a, "conflictFrom"), device)
	// A copy of a profile has no extensions of its own.
	meta.Option.Merge, meta.Option.Script, meta.Option.Rules, meta.Option.Proxies, meta.Option.Groups = "", "", "", "", ""
	if err := s.a.profiles.UpsertItem(&meta, meta.UID, it.Content); err != nil {
		return "", err
	}
	return itemPrefix + meta.UID, nil
}

// ---- Backups ----

// snapshotAll is a full backup of the app's data.
func (a *App) snapshotAll() (cloudsync.Snapshot, error) {
	files, err := appSource{a: a}.snapshot(map[string]bool{"settings": true, "dns": true, "tailscale": true, "profiles": true})
	if err != nil {
		return cloudsync.Snapshot{}, err
	}
	snap := cloudsync.Snapshot{Created: time.Now().UTC(), Device: a.syncer.deviceName(), OS: runtime.GOOS, Version: Version, Files: map[string][]byte{}}
	for p, it := range files {
		snap.Files[p] = it.Data
	}
	return snap, nil
}

// restoreSnapshot replaces the app's data with a backup's.
func (a *App) restoreSnapshot(ctx context.Context, snap cloudsync.Snapshot) error {
	changes := map[string][]byte{}
	for p, d := range snap.Files {
		changes[p] = d
	}
	// Profiles the backup does not have are deleted.
	if _, ok := snap.Files[pathOrder]; ok {
		for _, p := range a.profiles.List().Items {
			if _, keep := snap.Files[itemPrefix+p.UID]; !keep && p.UID != profiles.GlobalMerge && p.UID != profiles.GlobalScript {
				changes[itemPrefix+p.UID] = nil
			}
		}
	}
	if err := (appSource{a: a}).Apply(ctx, changes); err != nil {
		return err
	}
	a.syncer.changed()
	return a.applyConfig(ctx)
}

const localBackupLabel = "backup/local"

func (a *App) localBackups() []cloudsync.BackupInfo {
	entries, _ := os.ReadDir(a.dirs.Backups)
	var out []cloudsync.BackupInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bin") {
			continue
		}
		fi, _ := e.Info()
		t, dev := cloudsync.ParseBackupName(e.Name())
		out = append(out, cloudsync.BackupInfo{Name: e.Name(), Size: fi.Size(), Created: t, Device: dev})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out
}

// createLocalBackup writes a backup to the device, sealed, keeping the
// newest of the settings' count.
func (a *App) createLocalBackup() (string, error) {
	snap, err := a.snapshotAll()
	if err != nil {
		return "", err
	}
	data, _ := json.Marshal(snap)
	name := cloudsync.BackupName(snap.Created, a.syncer.deviceName())
	if err := a.sealer.WriteFile(filepath.Join(a.dirs.Backups, name), data, localBackupLabel); err != nil {
		return "", err
	}
	keep := a.settings.Get().Backup.Keep
	for i, b := range a.localBackups() {
		if i >= keep {
			_ = os.Remove(filepath.Join(a.dirs.Backups, b.Name))
		}
	}
	return name, nil
}

func (a *App) readLocalBackup(name string) (cloudsync.Snapshot, error) {
	if strings.ContainsAny(name, `/\`) {
		return cloudsync.Snapshot{}, errors.New("invalid backup name")
	}
	data, err := a.sealer.ReadFile(filepath.Join(a.dirs.Backups, name), localBackupLabel)
	if err != nil {
		return cloudsync.Snapshot{}, err
	}
	var snap cloudsync.Snapshot
	return snap, json.Unmarshal(data, &snap)
}

// Sync is the sync and backup page.
type Sync struct{ a *App }

// SyncSetup sets sync up.
type SyncSetup struct {
	URL        string `json:"url"`
	Username   string `json:"username"`
	Password   string `json:"password"` // "" keeps the saved one
	Dir        string `json:"dir"`
	Passphrase string `json:"passphrase"`
	// Plain creates a vault that is not encrypted; an existing vault
	// stays as it is.
	Plain         bool   `json:"plain"`
	AllowInsecure bool   `json:"allowInsecure"`
	PinnedKey     string `json:"pinnedKey"`
}

// SyncProbe is what a test of the server found.
type SyncProbe struct {
	Reachable bool `json:"reachable"`
	HasVault  bool `json:"hasVault"`
	// Plain is set when the vault found is not encrypted.
	Plain bool   `json:"plain"`
	Error string `json:"error,omitempty"`
	// Fingerprint and Trusted describe the server's key, for pinning one
	// that certificate authorities do not trust.
	Fingerprint string `json:"fingerprint,omitempty"`
	Trusted     bool   `json:"trusted"`
}

func (s Sync) setupClient(setup SyncSetup) (*webdav.Client, config.Sync, string, error) {
	st := s.a.settings.Get().Sync
	st.URL, st.Username, st.AllowInsecure, st.PinnedKey = strings.TrimSpace(setup.URL), setup.Username, setup.AllowInsecure, setup.PinnedKey
	if d := strings.Trim(strings.TrimSpace(setup.Dir), "/"); d != "" {
		st.Dir = d
	}
	password := setup.Password
	if password == "" {
		password = s.a.secrets.Get(secretSyncPassword)
	}
	c, err := s.a.syncer.client(st, password)
	return c, st, password, err
}

// Probe tests a server without changing anything.
func (s Sync) Probe(ctx context.Context, setup SyncSetup) SyncProbe {
	var p SyncProbe
	if strings.HasPrefix(setup.URL, "https://") {
		p.Fingerprint, p.Trusted, _ = webdav.ServerKey(ctx, setup.URL)
	}
	c, st, _, err := s.setupClient(setup)
	if err != nil {
		p.Error = err.Error()
		return p
	}
	if _, err := c.Stat(ctx, ""); err != nil && !errors.Is(err, webdav.ErrNotFound) {
		p.Error = err.Error()
		return p
	}
	p.Reachable = true
	if meta, err := cloudsync.ReadVault(ctx, c, st.Dir); err == nil {
		p.HasVault, p.Plain = true, meta.Plain()
	} else if !errors.Is(err, cloudsync.ErrNoVault) {
		p.Error = err.Error()
	}
	return p
}

// Setup connects to the server, opens its vault with the passphrase (or
// creates it), saves the credentials and turns sync on. It returns whether
// it created the vault.
func (s Sync) Setup(ctx context.Context, setup SyncSetup) (bool, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return false, err
	}
	c, st, password, err := s.setupClient(setup)
	if err != nil {
		return false, err
	}
	keys, created, err := cloudsync.OpenVault(ctx, c, st.Dir, setup.Passphrase, true, setup.Plain)
	if err != nil {
		return false, err
	}
	if err := s.a.secrets.Set(secretSyncPassword, password); err != nil {
		return false, err
	}
	if err := s.a.syncer.keepKeys(keys); err != nil {
		return false, err
	}
	s.a.syncer.join(ctx, &cloudsync.Engine{Client: c, Dir: st.Dir, Keys: keys})
	if _, err := s.a.updateSettings(ctx, func(cs *config.Settings) {
		cs.Sync.Enabled, cs.Sync.URL, cs.Sync.Username, cs.Sync.Dir = true, st.URL, st.Username, st.Dir
		cs.Sync.AllowInsecure, cs.Sync.PinnedKey = st.AllowInsecure, st.PinnedKey
	}); err != nil {
		return created, err
	}
	go func() {
		if _, err := s.a.syncer.run(s.a.ctx); err != nil {
			s.a.notifyErr("settings/sync", tr(s.a, "syncFailed"), err)
		}
	}()
	return created, nil
}

// join records that the device joins the vault now, which lifts an old
// removal of it. A device that lost its state (the app was installed again)
// takes the place of its old self, found by the computer and its name,
// instead of becoming another device.
func (s *syncer) join(ctx context.Context, eng *cloudsync.Engine) {
	m, _ := eng.Manifest(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	me := s.state.DeviceID
	if _, known := m.Devices[me]; !known && len(s.state.Base) == 0 {
		if mid := machineID(); mid != "" {
			for id, d := range m.Devices {
				if d.Machine == mid && d.Name == s.deviceName() && d.OS == runtime.GOOS {
					log.Printf("sync: this device was %s before", id)
					me = id
					break
				}
			}
		}
	}
	if me == s.state.DeviceID && s.state.KeyID == eng.Keys.ID {
		// The same vault again, as when the server's password changes.
		s.state.Joined = time.Now()
	} else {
		*s.state = cloudsync.State{DeviceID: me, Joined: time.Now(), History: s.state.History}
	}
	s.status.RemovedBy, s.status.NeedPassphrase = "", false
	s.save()
}

// Status returns the state of sync.
func (s Sync) Status(ctx context.Context) (SyncStatus, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return SyncStatus{}, err
	}
	return s.a.syncer.statusSnapshot(), nil
}

// Now syncs now.
func (s Sync) Now(ctx context.Context) (cloudsync.Report, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return cloudsync.Report{}, err
	}
	return s.a.syncer.run(ctx)
}

// Resolve resolves a conflict (local, remote or both) and syncs.
func (s Sync) Resolve(ctx context.Context, path, choice string) error {
	s.a.syncer.mu.Lock()
	if s.a.syncer.running {
		s.a.syncer.mu.Unlock()
		return errors.New(tr(s.a, "syncBusy"))
	}
	err := s.a.syncer.state.Resolve(path, choice)
	s.a.syncer.save()
	s.a.syncer.mu.Unlock()
	if err != nil {
		return err
	}
	_, err = s.a.syncer.run(ctx)
	return err
}

// Disable turns sync off; forget also deletes the saved credentials and
// keys from this device. The server keeps its data.
func (s Sync) Disable(ctx context.Context, forget bool) error {
	if _, err := s.a.updateSettings(ctx, func(cs *config.Settings) { cs.Sync.Enabled = false }); err != nil {
		return err
	}
	if forget {
		_ = s.a.secrets.Set(secretSyncPassword, "")
		_ = s.a.secrets.Set(secretSyncKey, "")
		s.a.syncer.mu.Lock()
		id := s.a.syncer.state.DeviceID
		*s.a.syncer.state = cloudsync.State{DeviceID: id}
		s.a.syncer.save()
		s.a.syncer.mu.Unlock()
	}
	s.a.syncer.broadcast()
	return nil
}

// ChangePassphrase wraps the vault's key with a new passphrase.
func (s Sync) ChangePassphrase(ctx context.Context, passphrase string) error {
	eng, err := s.a.syncer.engine()
	if err != nil {
		return err
	}
	return cloudsync.ChangePassphrase(ctx, eng.Client, eng.Dir, eng.Keys, passphrase)
}

// DeviceInfo is a device that syncs.
type DeviceInfo struct {
	ID string `json:"id"`
	// Name is the device's own name; Label the user's for it, if any.
	Name     string    `json:"name"`
	Label    string    `json:"label,omitempty"`
	OS       string    `json:"os"`
	Arch     string    `json:"arch,omitempty"`
	Version  string    `json:"version,omitempty"`
	LastSync time.Time `json:"lastSync"`
	Joined   time.Time `json:"joined,omitzero"`
	This     bool      `json:"this"`
	// Duplicate is set on an older entry of the same name and OS as
	// another, most likely the same computer before the app was installed
	// again.
	Duplicate bool `json:"duplicate,omitempty"`
	// HasKey reports that the device follows a replaced key by itself;
	// without, it needs the new passphrase.
	HasKey bool `json:"hasKey"`
	// Tailnet is set when the device takes word of changes over Tailscale.
	Tailnet bool `json:"tailnet,omitempty"`
}

// Devices lists the devices that sync with the vault, the latest first.
func (s Sync) Devices(ctx context.Context) ([]DeviceInfo, error) {
	eng, err := s.a.syncer.engine()
	if err != nil {
		return nil, err
	}
	devs, err := eng.Devices(ctx)
	if err != nil {
		return nil, err
	}
	me := s.a.syncer.state.DeviceID
	out := []DeviceInfo{}
	for id, d := range devs {
		out = append(out, DeviceInfo{
			ID: id, Name: d.Name, Label: d.Label, OS: d.OS, Arch: d.Arch, Version: d.Version, LastSync: d.LastSync, Joined: d.Joined,
			This: id == me, HasKey: len(d.PubKey) > 0, Tailnet: d.Tailnet != "",
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].This != out[j].This {
			return out[i].This
		}
		return out[i].LastSync.After(out[j].LastSync)
	})
	seen := map[string]bool{}
	for i := range out {
		k := strings.ToLower(out[i].Name) + "\x00" + out[i].OS
		if seen[k] && !out[i].This {
			out[i].Duplicate = true
		}
		seen[k] = true
	}
	return out, nil
}

// RenameDevice names a device for every device of the vault; "" gives it
// back its own name.
func (s Sync) RenameDevice(ctx context.Context, id, label string) error {
	return s.a.syncer.withEngine(func(eng *cloudsync.Engine) error { return eng.RenameDevice(ctx, id, label) })
}

// RemoveDevices takes devices out of the vault, with their statistics.
// With revoke, the vault's key and passphrase are replaced too, so that
// they can no longer read what it holds; the devices that stay follow by
// themselves (or, for older versions, with the new passphrase).
func (s Sync) RemoveDevices(ctx context.Context, ids []string, revoke bool, passphrase string) error {
	var paths []string
	for _, id := range ids {
		if id == s.a.syncer.state.DeviceID {
			return errors.New(tr(s.a, "syncRemoveSelf"))
		}
		paths = append(paths, usagePrefix+id)
	}
	err := s.a.syncer.withEngine(func(eng *cloudsync.Engine) error {
		if !revoke {
			return eng.RemoveDevice(ctx, ids, paths)
		}
		// Bring the vault up to date first: what the rotation encrypts
		// again is what the server has.
		if _, err := eng.Sync(ctx); err != nil {
			return err
		}
		_, err := eng.Rotate(ctx, passphrase, ids, paths)
		return err
	})
	if err != nil {
		return err
	}
	go func() {
		if _, err := s.a.syncer.run(s.a.ctx); err != nil {
			log.Printf("sync after removing devices: %v", err)
		}
	}()
	return nil
}

// Rejoin follows a vault whose key another device replaced, with the new
// passphrase, and syncs.
func (s Sync) Rejoin(ctx context.Context, passphrase string) error {
	err := s.a.syncer.withEngine(func(eng *cloudsync.Engine) error {
		keys, err := eng.Rejoin(ctx, passphrase)
		if err != nil {
			return err
		}
		return s.a.syncer.keepKeys(keys)
	})
	if err != nil {
		return err
	}
	s.a.syncer.mu.Lock()
	s.a.syncer.status.NeedPassphrase = false
	s.a.syncer.mu.Unlock()
	_, err = s.a.syncer.run(ctx)
	return err
}

// SyncActivity is something a device did to the vault.
type SyncActivity struct {
	At       time.Time          `json:"at"`
	DeviceID string             `json:"deviceId"`
	Device   string             `json:"device"`
	This     bool               `json:"this"`
	Kind     string             `json:"kind"`
	Changes  []cloudsync.Change `json:"changes"`
	Target   string             `json:"target,omitempty"`
	Detail   string             `json:"detail,omitempty"`
}

// Activity lists what the devices did to the vault, the latest first.
func (s Sync) Activity(ctx context.Context) ([]SyncActivity, error) {
	eng, err := s.a.syncer.engine()
	if err != nil {
		return nil, err
	}
	m, err := eng.Manifest(ctx)
	if err != nil {
		return nil, err
	}
	me := s.a.syncer.state.DeviceID
	out := make([]SyncActivity, 0, len(m.Activity))
	for i := len(m.Activity) - 1; i >= 0; i-- {
		ac := m.Activity[i]
		it := SyncActivity{At: ac.At, DeviceID: ac.Device, Device: m.DeviceName(ac.Device), This: ac.Device == me, Kind: ac.Kind, Changes: ac.Changes, Detail: ac.Detail}
		if it.Changes == nil {
			it.Changes = []cloudsync.Change{}
		}
		if ac.Target != "" {
			it.Target = m.DeviceName(ac.Target)
		}
		out = append(out, it)
	}
	return out, nil
}

// History lists what this device's syncs did, the latest first. A path
// with a name for people is "path\x00name".
func (s Sync) History(ctx context.Context) ([]cloudsync.Event, error) {
	s.a.syncer.mu.Lock()
	defer s.a.syncer.mu.Unlock()
	h := s.a.syncer.state.History
	out := make([]cloudsync.Event, 0, len(h))
	for i := len(h) - 1; i >= 0; i-- {
		out = append(out, h[i])
	}
	return out, nil
}

// Backups lists the backups of the device and, when sync is set up, of the
// server.
func (s Sync) Backups(ctx context.Context) ([]cloudsync.BackupInfo, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return nil, err
	}
	out := s.a.localBackups()
	if out == nil {
		out = []cloudsync.BackupInfo{}
	}
	if s.a.syncer.configured() {
		if eng, err := s.a.syncer.engine(); err == nil {
			if remote, err := eng.ListBackups(ctx); err == nil {
				out = append(out, remote...)
			} else {
				return out, err
			}
		}
	}
	return out, nil
}

// CreateBackup makes a backup on the device, or on the server.
func (s Sync) CreateBackup(ctx context.Context, remote bool) (string, error) {
	if !remote {
		return s.a.createLocalBackup()
	}
	eng, err := s.a.syncer.engine()
	if err != nil {
		return "", err
	}
	snap, err := s.a.snapshotAll()
	if err != nil {
		return "", err
	}
	return eng.UploadBackup(ctx, snap, s.a.settings.Get().Backup.Keep)
}

func (s Sync) load(ctx context.Context, name string, remote bool) (cloudsync.Snapshot, error) {
	if !remote {
		return s.a.readLocalBackup(name)
	}
	eng, err := s.a.syncer.engine()
	if err != nil {
		return cloudsync.Snapshot{}, err
	}
	return eng.DownloadBackup(ctx, name)
}

// RestoreBackup replaces the app's data with a backup's. A backup of the
// current data is made first.
func (s Sync) RestoreBackup(ctx context.Context, name string, remote bool) error {
	snap, err := s.load(ctx, name, remote)
	if err != nil {
		return err
	}
	if _, err := s.a.createLocalBackup(); err != nil {
		return fmt.Errorf("back up the current data first: %w", err)
	}
	return s.a.restoreSnapshot(ctx, snap)
}

// DeleteBackup deletes a backup.
func (s Sync) DeleteBackup(ctx context.Context, name string, remote bool) error {
	if !remote {
		if strings.ContainsAny(name, `/\`) {
			return errors.New("invalid backup name")
		}
		return os.Remove(filepath.Join(s.a.dirs.Backups, name))
	}
	eng, err := s.a.syncer.engine()
	if err != nil {
		return err
	}
	return eng.DeleteBackup(ctx, name)
}

// ExportBackup saves a backup to a file the user picks, encrypted with a
// password of their choice.
func (s Sync) ExportBackup(ctx context.Context, name string, remote bool, password string) (string, error) {
	var snap cloudsync.Snapshot
	var err error
	if name == "" {
		snap, err = s.a.snapshotAll()
	} else {
		snap, err = s.load(ctx, name, remote)
	}
	if err != nil {
		return "", err
	}
	data, err := cloudsync.Export(snap, password)
	if err != nil {
		return "", err
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: mygo.CallerWindow(ctx), DefaultPath: "MyGO-Clash-" + snap.Created.Local().Format("20060102-150405") + ".mgcbak"})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o600)
}

// ImportBackup restores a file made by ExportBackup.
func (s Sync) ImportBackup(ctx context.Context, password string) error {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Filters: []mygo.FileFilter{{Name: "MyGO-Clash backup", Extensions: []string{"mgcbak"}}}})
	if err != nil || len(paths) == 0 {
		return err
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		return err
	}
	snap, err := cloudsync.Import(data, password)
	if err != nil {
		return err
	}
	if _, err := s.a.createLocalBackup(); err != nil {
		return err
	}
	return s.a.restoreSnapshot(ctx, snap)
}

func cloneState(st *cloudsync.State) *cloudsync.State {
	data, _ := json.Marshal(st)
	var out cloudsync.State
	_ = json.Unmarshal(data, &out)
	return &out
}
