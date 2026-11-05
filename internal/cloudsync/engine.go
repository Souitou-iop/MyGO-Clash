// Package cloudsync synchronizes the app's data between devices through a
// WebDAV server, end-to-end encrypted (see vault.go).
//
// # Layout on the server
//
//	<dir>/vault.json             the vault's key derivation and wrapped key, in the clear
//	<dir>/manifest.bin           the manifest, sealed: every path with its object and version
//	<dir>/objects/<hash>.bin     the contents, sealed, named by keyed hash
//	<dir>/backups/<time>-<device>.bin   full snapshots, sealed
//	<dir>/lock.json              a lease, for servers without ETags
//
// # Algorithm
//
// Each path (settings, the DNS override, each profile) has a version
// vector: a counter per device, incremented by the device that writes it.
// A device remembers the manifest of its last sync, the base. A sync
//
//  1. snapshots the local data and compares it with the base: local changes;
//  2. fetches the manifest and compares each path's vector with the base's:
//     remote changes;
//  3. pushes paths changed only here, pulls paths changed only there,
//     converges paths changed to the same content on both, and resolves
//     the rest: mergeable paths (settings) merge field by field against the
//     base, others follow the conflict policy (ask, newest, local, remote);
//  4. uploads new objects, then writes the manifest with If-Match on the
//     ETag it read (or under the lease), and verifies the write by reading
//     it back. A write that lost a race starts over from step 2;
//  5. applies the pulled data and records the new base.
package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/webdav"
)

// VClock is a version vector.
type VClock map[string]uint64

// Equal reports whether two vectors are equal.
func (v VClock) Equal(o VClock) bool {
	for k, n := range v {
		if o[k] != n {
			return false
		}
	}
	for k, n := range o {
		if v[k] != n {
			return false
		}
	}
	return true
}

// Merge returns the element-wise maximum.
func (v VClock) Merge(o VClock) VClock {
	out := maps.Clone(v)
	if out == nil {
		out = VClock{}
	}
	for k, n := range o {
		if n > out[k] {
			out[k] = n
		}
	}
	return out
}

// Inc returns the vector with device's counter incremented.
func (v VClock) Inc(device string) VClock {
	out := maps.Clone(v)
	if out == nil {
		out = VClock{}
	}
	out[device]++
	return out
}

// Entry is a path of the manifest.
type Entry struct {
	Hash     string    `json:"hash,omitempty"`
	Size     int       `json:"size"`
	Modified time.Time `json:"modified"`
	Device   string    `json:"device"`
	Clock    VClock    `json:"clock"`
	Deleted  bool      `json:"deleted,omitempty"`
	// Label names the path for people, such as a profile's name.
	Label string `json:"label,omitempty"`
}

// Manifest lists every path on the server.
type Manifest struct {
	Format  int               `json:"format"`
	Seq     uint64            `json:"seq"`
	Updated time.Time         `json:"updated"`
	Writer  string            `json:"writer"`
	KeyID   string            `json:"keyId"`
	Entries map[string]Entry  `json:"entries"`
	Devices map[string]Device `json:"devices"`
	// Removed lists the devices taken out of the vault: a device that finds
	// itself here stops syncing, unless it joined again since.
	Removed map[string]Removal `json:"removed,omitempty"`
	// Activity is what the devices did, newest last, the latest maxActivity.
	Activity []Activity `json:"activity,omitempty"`
	// PrevKeyID is the data key this one replaced.
	PrevKeyID string `json:"prevKeyId,omitempty"`
}

// Device is a device that syncs.
type Device struct {
	Name     string    `json:"name"`
	OS       string    `json:"os"`
	LastSync time.Time `json:"lastSync"`
	// Label is the name the user gave the device, which wins over Name.
	Label   string `json:"label,omitempty"`
	Version string `json:"version,omitempty"`
	Arch    string `json:"arch,omitempty"`
	// Machine identifies the computer, hashed: an app installed again on it
	// takes its old place instead of becoming another device.
	Machine string `json:"machine,omitempty"`
	// PubKey is the public key of the device's Identity.
	PubKey []byte `json:"pubKey,omitempty"`
	// Tailnet is where the device takes word of changes on its tailnet,
	// host:port, when it does.
	Tailnet string    `json:"tailnet,omitempty"`
	Joined  time.Time `json:"joined,omitzero"`
}

// DisplayName is the device's label, else its name.
func (d Device) DisplayName() string {
	if d.Label != "" {
		return d.Label
	}
	return d.Name
}

// sameInfo reports whether two entries of a device say the same, but for
// the time of the last sync.
func (d Device) sameInfo(o Device) bool {
	return d.Name == o.Name && d.OS == o.OS && d.Label == o.Label && d.Version == o.Version && d.Arch == o.Arch &&
		d.Machine == o.Machine && string(d.PubKey) == string(o.PubKey) && d.Tailnet == o.Tailnet && d.Joined.Equal(o.Joined)
}

// Removal is a device taken out of the vault.
type Removal struct {
	At   time.Time `json:"at"`
	By   string    `json:"by"`
	Name string    `json:"name"`
}

// Activity is something a device did to the vault.
type Activity struct {
	At     time.Time `json:"at"`
	Device string    `json:"device"`
	// Kind is sync, join, rename, remove or rotate.
	Kind    string   `json:"kind"`
	Changes []Change `json:"changes,omitempty"`
	// Target is the device a rename or removal was about.
	Target string `json:"target,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Change is a path a sync sent.
type Change struct {
	Path    string `json:"path"`
	Label   string `json:"label,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
}

const maxActivity = 200

func (m *Manifest) addActivity(a Activity) {
	m.Activity = append(slices.Clone(m.Activity), a)
	if n := len(m.Activity) - maxActivity; n > 0 {
		m.Activity = m.Activity[n:]
	}
}

// errSealed is returned when the manifest does not open with the keys.
var errSealed = errors.New("the manifest does not open with this device's key")

// follow adopts the key in vault.json when it is not the device's: the copy
// sealed to the device, when another device replaced the key. It reports
// whether it adopted one.
func (e *Engine) follow(ctx context.Context, local map[string]Item) (bool, error) {
	meta, err := ReadVault(ctx, e.Client, e.Dir)
	if err != nil {
		return false, err
	}
	if meta.KeyID == e.Keys.ID {
		return false, nil
	}
	keys, err := meta.OpenMember(e.State.DeviceID, e.Identity)
	if errors.Is(err, ErrNoMember) {
		if e.State.KeyShared || slices.Contains(meta.Removed, e.State.DeviceID) {
			// The device's key was published, and the device that replaced
			// the vault's key left it out on purpose.
			return false, &RemovedError{}
		}
		return false, ErrKeyReplaced
	}
	if err != nil {
		return false, err
	}
	return true, e.adopt(keys, local)
}

// RemovedError is returned to a device that was removed from the vault.
type RemovedError struct {
	By string
	At time.Time
}

func (e *RemovedError) Error() string {
	if e.By == "" {
		return "this device was removed from sync"
	}
	return "this device was removed from sync by " + e.By
}

// ErrKeyReplaced is returned when another device replaced the vault's key
// and left none for this device: it needs the new passphrase.
var ErrKeyReplaced = errors.New("another device replaced the vault's key: enter the new passphrase")

// Item is a path's local data.
type Item struct {
	Data     []byte
	Modified time.Time
}

// Source is the app's data.
type Source interface {
	// Snapshot returns every path to sync.
	Snapshot(ctx context.Context) (map[string]Item, error)
	// Apply writes pulled data: new data for paths, nil for deleted ones.
	Apply(ctx context.Context, changes map[string][]byte) error
	// Merge merges the versions of a path changed on both sides; it
	// returns false when the path does not merge.
	Merge(path string, base, local, remote []byte, preferRemote bool) ([]byte, bool)
	// KeepBoth keeps the remote version of a conflicting path as a copy,
	// beside the local one, and returns the path of the copy.
	KeepBoth(ctx context.Context, path string, remote []byte, device string) (string, error)
}

// Conflict is a path both sides changed, which waits for the user.
type Conflict struct {
	Path           string    `json:"path"`
	LocalModified  time.Time `json:"localModified"`
	RemoteModified time.Time `json:"remoteModified"`
	RemoteDevice   string    `json:"remoteDevice"`
	// RemoteName is the name of the remote device.
	RemoteName    string    `json:"remoteName,omitempty"`
	LocalDeleted  bool      `json:"localDeleted"`
	RemoteDeleted bool      `json:"remoteDeleted"`
	Detected      time.Time `json:"detected"`
}

// Resolution of a conflict.
const (
	KeepLocal  = "local"
	KeepRemote = "remote"
	KeepBoth   = "both"
)

// State is what a device remembers between syncs.
type State struct {
	DeviceID string           `json:"deviceId"`
	KeyID    string           `json:"keyId"`
	Base     map[string]Entry `json:"base"`
	// BaseData keeps the base content of mergeable paths.
	BaseData    map[string][]byte `json:"baseData"`
	LastSync    time.Time         `json:"lastSync"`
	LastError   string            `json:"lastError,omitempty"`
	Conflicts   []Conflict        `json:"conflicts"`
	Resolutions map[string]string `json:"resolutions"`
	LastGC      time.Time         `json:"lastGc"`
	// Joined is when the device last joined the vault: removals before it
	// no longer apply.
	Joined time.Time `json:"joined,omitzero"`
	// History is what this device's syncs did, newest last.
	History []Event `json:"history,omitempty"`
	// KeyShared is set once the manifest lists the device's public key.
	KeyShared bool `json:"keyShared,omitempty"`
}

// Event is a sync of this device, for its history.
type Event struct {
	At        time.Time `json:"at"`
	Pushed    []string  `json:"pushed,omitempty"`
	Pulled    []string  `json:"pulled,omitempty"`
	Merged    []string  `json:"merged,omitempty"`
	Conflicts int       `json:"conflicts,omitempty"`
	Error     string    `json:"error,omitempty"`
	// Duration is in milliseconds.
	Duration int64 `json:"duration"`
}

const maxHistory = 100

// Record adds an event to the history.
func (s *State) Record(ev Event) {
	s.History = append(s.History, ev)
	if n := len(s.History) - maxHistory; n > 0 {
		s.History = slices.Clone(s.History[n:])
	}
}

// Report says what a sync did.
type Report struct {
	Pushed    []string   `json:"pushed"`
	Pulled    []string   `json:"pulled"`
	Merged    []string   `json:"merged"`
	Copies    []string   `json:"copies"`
	Conflicts []Conflict `json:"conflicts"`
	Retries   int        `json:"retries"`
	Devices   int        `json:"devices"`
	Duration  time.Duration
	// Labels names the paths sent and received, for people.
	Labels map[string]string `json:"labels,omitempty"`
}

// Changed reports whether the sync changed local data.
func (r Report) Changed() bool { return len(r.Pulled) > 0 || len(r.Merged) > 0 || len(r.Copies) > 0 }

// Engine syncs a source with a vault on a server.
type Engine struct {
	Client *webdav.Client
	Dir    string
	Keys   *Keys
	Source Source
	State  *State
	// Policy resolves conflicts: ask, newest, local or remote.
	Policy     string
	DeviceName string
	// Mergeable reports the paths whose versions merge.
	Mergeable func(path string) bool
	// Wanted reports the paths this device syncs, nil for all: the others
	// are left as they are here and on the server, not pulled, and not
	// pushed as deleted.
	Wanted func(path string) bool
	// Now is the clock, for tests.
	Now func() time.Time
	// Self is what the manifest says of this device, beside its name, OS
	// and the time of its sync, which the engine fills.
	Self Device
	// Identity is the device's key pair, to follow a replaced data key.
	Identity *Identity
	// Label names a path for people, from its content.
	Label func(path string, data []byte) string
	// OnKeys keeps the keys the engine adopted when another device
	// replaced the data key.
	OnKeys func(*Keys) error

	cur Manifest // the manifest of the sync in progress, for names
}

const manifestFile = "manifest.bin"

func (e *Engine) p(parts ...string) string { return path.Join(append([]string{e.Dir}, parts...)...) }

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now()
}

type remoteManifest struct {
	m      Manifest
	etag   string
	exists bool
}

func (e *Engine) fetchManifest(ctx context.Context) (remoteManifest, error) {
	data, etag, err := e.Client.Get(ctx, e.p(manifestFile))
	if errors.Is(err, webdav.ErrNotFound) {
		return remoteManifest{m: Manifest{Format: 1, Entries: map[string]Entry{}, Devices: map[string]Device{}, Removed: map[string]Removal{}}}, nil
	}
	if err != nil {
		return remoteManifest{}, err
	}
	var m Manifest
	if err := e.Keys.OpenJSON(data, "manifest", &m); err != nil {
		return remoteManifest{}, fmt.Errorf("%w: %v", errSealed, err)
	}
	if m.Entries == nil {
		m.Entries = map[string]Entry{}
	}
	if m.Devices == nil {
		m.Devices = map[string]Device{}
	}
	if m.Removed == nil {
		m.Removed = map[string]Removal{}
	}
	return remoteManifest{m: m, etag: etag, exists: true}, nil
}

type action int

const (
	actNone action = iota
	actPush
	actPull
	actConverge
	actMerge
	actCopy // keep both: copy remote aside, then push local
	actConflict
)

type step struct {
	path   string
	act    action
	local  *Item
	remote *Entry
	data   []byte // what to push (push, merge) or what was pulled
}

// Sync runs one sync.
func (e *Engine) Sync(ctx context.Context) (Report, error) {
	start := time.Now()
	var rep Report
	if e.State.DeviceID == "" {
		return rep, errors.New("the device has no ID")
	}
	if e.State.Base == nil || e.State.KeyID != e.Keys.ID {
		// A new vault: everything here is new to it.
		e.State.Base, e.State.BaseData, e.State.KeyID = map[string]Entry{}, map[string][]byte{}, e.Keys.ID
	}
	if e.State.BaseData == nil {
		e.State.BaseData = map[string][]byte{}
	}
	if e.Wanted != nil {
		// Bases written before they kept to the wanted paths.
		for p := range e.State.Base {
			if !e.Wanted(p) {
				delete(e.State.Base, p)
			}
		}
	}
	if err := e.Client.MkdirAll(ctx, e.p("objects")); err != nil {
		return rep, err
	}
	local, err := e.Source.Snapshot(ctx)
	if err != nil {
		return rep, err
	}
	hashes := map[string]string{}
	for p, it := range local {
		hashes[p] = e.Keys.Hash(it.Data)
	}

	for attempt := 0; attempt < 4; attempt++ {
		rep = Report{Retries: attempt}
		rm, err := e.fetchManifest(ctx)
		if errors.Is(err, errSealed) || (err == nil && rm.exists && rm.m.KeyID != "" && rm.m.KeyID != e.Keys.ID) {
			// Another device may have replaced the key.
			followed, ferr := e.follow(ctx, local)
			if ferr != nil {
				return rep, ferr
			}
			if !followed {
				if err == nil {
					err = errors.New("the manifest on the server belongs to another vault")
				}
				return rep, err
			}
			for p, it := range local {
				hashes[p] = e.Keys.Hash(it.Data)
			}
			continue
		}
		if err != nil {
			return rep, err
		}
		if r, ok := rm.m.Removed[e.State.DeviceID]; ok && !r.At.Before(e.State.Joined) {
			return rep, e.removedError(rm.m, r)
		}
		e.cur = rm.m
		steps, err := e.plan(ctx, local, hashes, rm.m, &rep)
		if err != nil {
			return rep, err
		}
		next, err := e.buildManifest(ctx, rm.m, steps, hashes)
		if err != nil {
			return rep, err
		}
		changedRemote := false
		for _, s := range steps {
			if s.act == actPush || s.act == actMerge || s.act == actCopy {
				changedRemote = true
			}
		}
		me, known := rm.m.Devices[e.State.DeviceID]
		_, removed := rm.m.Removed[e.State.DeviceID]
		stale := !known || removed || !me.sameInfo(next.Devices[e.State.DeviceID]) || e.now().Sub(me.LastSync) > time.Hour
		if changedRemote || !rm.exists || stale {
			ok, err := e.writeManifest(ctx, rm, next)
			if err != nil {
				return rep, err
			}
			if !ok {
				continue // another device synced in between: start over
			}
		} else {
			next = rm.m
		}
		// The manifest is written: apply what came from the server.
		if err := e.applyLocal(ctx, steps, &rep); err != nil {
			return rep, err
		}
		oldBase := e.State.Base
		e.State.Base = maps.Clone(next.Entries)
		// The base is what this device has seen: a path it does not sync
		// stays out, or turning it on later would read as a deletion here.
		if e.Wanted != nil {
			for p := range e.State.Base {
				if !e.Wanted(p) {
					delete(e.State.Base, p)
				}
			}
		}
		// A conflict keeps its base until it is resolved, or the next sync
		// would take the local side for the only one that changed.
		for _, c := range rep.Conflicts {
			if en, ok := oldBase[c.Path]; ok {
				e.State.Base[c.Path] = en
			} else {
				delete(e.State.Base, c.Path)
			}
		}
		for _, s := range steps {
			if !e.mergeable(s.path) {
				continue
			}
			switch {
			case s.act == actConflict:
				// A conflict keeps its base until it is resolved.
			case s.act == actPull && s.data == nil:
				delete(e.State.BaseData, s.path) // the server deleted it
			case s.data != nil:
				e.State.BaseData[s.path] = s.data // merged, pulled or pushed data
			case s.local != nil:
				e.State.BaseData[s.path] = s.local.Data // converged, or local won
			default:
				delete(e.State.BaseData, s.path)
			}
		}
		for p := range e.State.BaseData {
			if en, ok := next.Entries[p]; !ok || en.Deleted {
				delete(e.State.BaseData, p)
			}
		}
		e.cur = next
		e.State.LastSync, e.State.LastError = e.now(), ""
		e.State.KeyShared = len(next.Devices[e.State.DeviceID].PubKey) > 0
		rep.Devices = len(next.Devices)
		rep.Duration = time.Since(start)
		rep.Labels = map[string]string{}
		for _, list := range [][]string{rep.Pushed, rep.Pulled, rep.Merged} {
			for _, p := range list {
				if l := next.Entries[p].Label; l != "" {
					rep.Labels[p] = l
				}
			}
		}
		e.maybeGC(ctx, next)
		if len(rep.Copies) > 0 {
			// The copies kept aside are local data the server lacks: one more
			// round sends them.
			more, err := e.Sync(ctx)
			rep.Pushed = append(rep.Pushed, more.Pushed...)
			rep.Duration = time.Since(start)
			return rep, err
		}
		return rep, nil
	}
	return rep, errors.New("other devices kept changing the server; try again")
}

func (e *Engine) mergeable(p string) bool { return e.Mergeable != nil && e.Mergeable(p) }

// plan decides what to do with each path.
func (e *Engine) plan(ctx context.Context, local map[string]Item, hashes map[string]string, remote Manifest, rep *Report) ([]step, error) {
	paths := map[string]bool{}
	for p := range local {
		paths[p] = true
	}
	for p := range e.State.Base {
		paths[p] = true
	}
	for p := range remote.Entries {
		paths[p] = true
	}
	sorted := slices.Sorted(maps.Keys(paths))
	var steps []step
	var conflicts []Conflict
	for _, p := range sorted {
		if e.Wanted != nil && !e.Wanted(p) {
			continue
		}
		it, lExists := local[p]
		var lp *Item
		if lExists {
			lp = &it
		}
		b, hasBase := e.State.Base[p]
		r, hasRemote := remote.Entries[p]
		var rp *Entry
		if hasRemote {
			rp = &r
		}
		bLive := hasBase && !b.Deleted
		rLive := hasRemote && !r.Deleted
		localChanged := lExists != bLive || (lExists && bLive && hashes[p] != b.Hash)
		remoteChanged := hasRemote && (!hasBase || !r.Clock.Equal(b.Clock))
		s := step{path: p, local: lp, remote: rp}

		switch {
		case !localChanged && !remoteChanged:
			if lExists && !hasRemote {
				s.act = actPush // the server lost it (a reset vault): restore it
			}
		case localChanged && !remoteChanged:
			if lExists || hasRemote {
				s.act = actPush
			}
		case !localChanged && remoteChanged:
			if rLive || lExists {
				s.act = actPull
			} else {
				s.act = actConverge
			}
		default: // both changed
			switch {
			case (lExists && rLive && hashes[p] == r.Hash) || (!lExists && !rLive):
				s.act = actConverge
			default:
				s.act = e.resolve(p, lp, r, &conflicts)
			}
		}
		if s.act == actMerge {
			// Merge needs both versions and the base.
			data, err := e.download(ctx, r)
			if err != nil {
				return nil, err
			}
			preferRemote := r.Modified.After(lp.Modified)
			if e.Policy == KeepRemote {
				preferRemote = true
			} else if e.Policy == KeepLocal {
				preferRemote = false
			}
			merged, ok := e.Source.Merge(p, e.State.BaseData[p], lp.Data, data, preferRemote)
			if !ok {
				// The versions do not merge after all (profile contents that
				// differ): a conflict, rather than merging again in a loop.
				s.act = e.conflict(p, lp, r, &conflicts)
			} else {
				s.data = merged
				hashes[p] = e.Keys.Hash(merged)
			}
		}
		// A path that merges is kept even when it is converged, so that its
		// base data is recorded for the next merge: a path that was in sync
		// before it became mergeable has none otherwise.
		if s.act != actNone || e.mergeable(p) {
			steps = append(steps, s)
		}
	}
	e.State.Conflicts = conflicts
	rep.Conflicts = conflicts
	return steps, nil
}

// resolve decides a path changed differently on both sides.
func (e *Engine) resolve(p string, lp *Item, r Entry, conflicts *[]Conflict) action {
	if choice, ok := e.State.Resolutions[p]; ok {
		delete(e.State.Resolutions, p)
		switch choice {
		case KeepLocal:
			return actPush
		case KeepRemote:
			return actPull
		case KeepBoth:
			if lp != nil && !r.Deleted {
				return actCopy
			}
			if lp != nil {
				return actPush
			}
			return actPull
		}
	}
	if lp != nil && !r.Deleted && e.mergeable(p) {
		if _, ok := e.State.BaseData[p]; ok {
			return actMerge
		}
	}
	// A deletion against a change keeps the change: data is never lost to
	// a conflict.
	if lp == nil && !r.Deleted {
		return actPull
	}
	if lp != nil && r.Deleted {
		return actPush
	}
	switch e.Policy {
	case KeepLocal:
		return actPush
	case KeepRemote:
		return actPull
	case "newest":
		if r.Modified.After(lp.Modified) {
			return actPull
		}
		return actPush
	}
	return e.conflict(p, lp, r, conflicts)
}

// conflict records a path waiting for the user to choose a side.
func (e *Engine) conflict(p string, lp *Item, r Entry, conflicts *[]Conflict) action {
	*conflicts = append(*conflicts, Conflict{
		Path: p, LocalModified: lp.Modified, RemoteModified: r.Modified, RemoteDevice: r.Device, RemoteName: e.deviceName(r.Device),
		LocalDeleted: lp == nil, RemoteDeleted: r.Deleted, Detected: e.now(),
	})
	return actConflict
}

func (e *Engine) download(ctx context.Context, en Entry) ([]byte, error) {
	data, _, err := e.Client.Get(ctx, e.p("objects", en.Hash+".bin"))
	if err != nil {
		return nil, fmt.Errorf("download %s: %w", en.Hash[:8], err)
	}
	plain, err := e.Keys.Open(data, en.Hash)
	if err != nil {
		return nil, err
	}
	if e.Keys.Hash(plain) != en.Hash {
		return nil, fmt.Errorf("the object %s does not match its name", en.Hash[:8])
	}
	return plain, nil
}

// upload stores an object, unless the server has it.
func (e *Engine) upload(ctx context.Context, hash string, plain []byte) error {
	sealed, err := e.Keys.Seal(plain, hash)
	if err != nil {
		return err
	}
	_, err = e.Client.Put(ctx, e.p("objects", hash+".bin"), sealed, webdav.Condition{IfNoneMatch: true})
	if errors.Is(err, webdav.ErrPrecondition) {
		return nil // content-addressed: it is the same
	}
	return err
}

// buildManifest uploads the objects of pushed paths and returns the new
// manifest.
func (e *Engine) buildManifest(ctx context.Context, remote Manifest, steps []step, hashes map[string]string) (Manifest, error) {
	next := remote.clone()
	now := e.now().UTC()
	var changes []Change
	for i := range steps {
		s := &steps[i]
		switch s.act {
		case actPush, actMerge, actCopy:
			clock := e.State.Base[s.path].Clock
			if s.remote != nil {
				clock = clock.Merge(s.remote.Clock)
			}
			en := Entry{Device: e.State.DeviceID, Clock: clock.Inc(e.State.DeviceID), Modified: now}
			data := s.data
			if data == nil && s.local != nil {
				data = s.local.Data
			}
			if s.local == nil && s.act == actPush {
				en.Deleted = true
				if s.remote != nil {
					en.Label = s.remote.Label
				} else {
					en.Label = e.State.Base[s.path].Label
				}
			} else {
				en.Label = e.label(s.path, data)
				en.Hash, en.Size = hashes[s.path], len(data)
				if s.local != nil && !s.local.Modified.IsZero() && s.act == actPush {
					en.Modified = s.local.Modified.UTC()
				}
				if err := e.upload(ctx, en.Hash, data); err != nil {
					return Manifest{}, err
				}
			}
			next.Entries[s.path] = en
			changes = append(changes, Change{Path: s.path, Label: en.Label, Deleted: en.Deleted})
		}
	}
	if len(changes) > 0 {
		next.addActivity(Activity{At: now, Device: e.State.DeviceID, Kind: "sync", Changes: changes})
	}
	// Forget deletions everyone has seen for a month.
	for p, en := range next.Entries {
		if en.Deleted && now.Sub(en.Modified) > 30*24*time.Hour {
			delete(next.Entries, p)
		}
	}
	next.Format, next.KeyID = 1, e.Keys.ID
	next.Seq++
	next.Updated, next.Writer = now, e.State.DeviceID
	old, known := next.Devices[e.State.DeviceID]
	if r, ok := next.Removed[e.State.DeviceID]; ok && r.At.Before(e.State.Joined) {
		// Removed once, joined again since.
		delete(next.Removed, e.State.DeviceID)
		known = false
	}
	next.Devices[e.State.DeviceID] = e.self(old, known, now)
	if !known {
		next.addActivity(Activity{At: now, Device: e.State.DeviceID, Kind: "join"})
	}
	next.pruneRemoved(now)
	return next, nil
}

// self is this device's entry in the manifest, keeping what the old one
// had of the user's.
func (e *Engine) self(old Device, known bool, now time.Time) Device {
	d := e.Self
	d.Name, d.OS, d.LastSync = e.DeviceName, runtime.GOOS, now
	if known {
		d.Label, d.Joined = old.Label, old.Joined
	}
	if d.Joined.IsZero() {
		d.Joined = now
		if !e.State.Joined.IsZero() {
			d.Joined = e.State.Joined.UTC()
		}
	}
	return d
}

func (e *Engine) removedError(m Manifest, r Removal) error {
	by := r.By
	if d, ok := m.Devices[r.By]; ok {
		by = d.DisplayName()
	}
	return &RemovedError{By: by, At: r.At}
}

// clone copies the manifest's maps and lists, so that changes to the copy
// leave the original as it is.
func (m Manifest) clone() Manifest {
	out := m
	out.Entries = maps.Clone(m.Entries)
	out.Devices = maps.Clone(m.Devices)
	out.Removed = maps.Clone(m.Removed)
	out.Activity = slices.Clone(m.Activity)
	if out.Entries == nil {
		out.Entries = map[string]Entry{}
	}
	if out.Devices == nil {
		out.Devices = map[string]Device{}
	}
	if out.Removed == nil {
		out.Removed = map[string]Removal{}
	}
	return out
}

// pruneRemoved forgets removals older than half a year.
func (m *Manifest) pruneRemoved(now time.Time) {
	for id, r := range m.Removed {
		if now.Sub(r.At) > 180*24*time.Hour {
			delete(m.Removed, id)
		}
	}
}

func (e *Engine) label(path string, data []byte) string {
	if e.Label == nil {
		return ""
	}
	return e.Label(path, data)
}

// adopt switches to keys another device put in place of the current ones,
// and carries the base over: a path's base hash names its content under the
// old naming key; the device that replaced the key kept every content as it
// was, so a path unchanged here gets its hash under the new key, and one
// changed here stays changed.
func (e *Engine) adopt(keys *Keys, local map[string]Item) error {
	old := e.Keys
	base := maps.Clone(e.State.Base)
	for p, b := range base {
		if b.Deleted {
			continue
		}
		if it, ok := local[p]; ok && old.Hash(it.Data) == b.Hash {
			b.Hash = keys.Hash(it.Data)
		} else {
			b.Hash = "changed"
		}
		base[p] = b
	}
	if e.OnKeys != nil {
		if err := e.OnKeys(keys); err != nil {
			return err
		}
	}
	e.Keys, e.State.Base, e.State.KeyID = keys, base, keys.ID
	return nil
}

// writeManifest writes next if the server still has what was read. It
// returns false when it lost a race.
func (e *Engine) writeManifest(ctx context.Context, rm remoteManifest, next Manifest) (bool, error) {
	sealed, err := e.Keys.SealJSON(next, "manifest")
	if err != nil {
		return false, err
	}
	cond := webdav.Condition{IfMatch: rm.etag, IfNoneMatch: !rm.exists}
	var unlock func()
	if rm.exists && rm.etag == "" {
		// The server gives no ETags to compare: take the lease instead.
		if unlock, err = e.lock(ctx); err != nil {
			return false, err
		}
		defer unlock()
		cur, err := e.fetchManifest(ctx)
		if err != nil {
			return false, err
		}
		if cur.m.Seq != rm.m.Seq || cur.m.Writer != rm.m.Writer {
			return false, nil
		}
	}
	if _, err := e.Client.Put(ctx, e.p(manifestFile), sealed, cond); errors.Is(err, webdav.ErrPrecondition) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	// Some servers ignore conditions; read the manifest back to be sure
	// that this write is the one that stayed.
	check, err := e.fetchManifest(ctx)
	if err != nil {
		return false, err
	}
	if check.m.Seq != next.Seq || check.m.Writer != next.Writer || !check.m.Updated.Equal(next.Updated) {
		return false, nil
	}
	return true, nil
}

type lease struct {
	Device  string    `json:"device"`
	Expires time.Time `json:"expires"`
}

// lock takes the lease of the vault, for servers without ETags.
func (e *Engine) lock(ctx context.Context) (func(), error) {
	data, _ := json.Marshal(lease{Device: e.State.DeviceID, Expires: e.now().Add(time.Minute)})
	for i := 0; i < 20; i++ {
		_, err := e.Client.Put(ctx, e.p("lock.json"), data, webdav.Condition{IfNoneMatch: true})
		if err == nil {
			// Servers that ignore If-None-Match overwrite: check whose lease stayed.
			got, _, gerr := e.Client.Get(ctx, e.p("lock.json"))
			var l lease
			if gerr == nil && json.Unmarshal(got, &l) == nil && l.Device == e.State.DeviceID {
				return func() { _ = e.Client.Delete(context.Background(), e.p("lock.json"), "") }, nil
			}
		} else if !errors.Is(err, webdav.ErrPrecondition) {
			return nil, err
		}
		got, _, err := e.Client.Get(ctx, e.p("lock.json"))
		var l lease
		if err == nil && json.Unmarshal(got, &l) == nil && e.now().After(l.Expires) {
			_ = e.Client.Delete(ctx, e.p("lock.json"), "") // a stale lease
			continue
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(500+100*i) * time.Millisecond):
		}
	}
	return nil, errors.New("another device holds the vault's lock")
}

// applyLocal writes what was pulled, merged or copied.
func (e *Engine) applyLocal(ctx context.Context, steps []step, rep *Report) error {
	changes := map[string][]byte{}
	for i := range steps {
		s := &steps[i]
		switch s.act {
		case actPull:
			if s.remote == nil || s.remote.Deleted {
				changes[s.path] = nil
			} else {
				data, err := e.download(ctx, *s.remote)
				if err != nil {
					return err
				}
				changes[s.path] = data
				s.data = data
			}
			rep.Pulled = append(rep.Pulled, s.path)
		case actMerge:
			changes[s.path] = s.data
			rep.Merged = append(rep.Merged, s.path)
		case actCopy:
			data, err := e.download(ctx, *s.remote)
			if err != nil {
				return err
			}
			cp, err := e.Source.KeepBoth(ctx, s.path, data, e.deviceName(s.remote.Device))
			if err != nil {
				return err
			}
			rep.Copies = append(rep.Copies, cp)
		case actPush:
			rep.Pushed = append(rep.Pushed, s.path)
		}
	}
	if len(changes) == 0 {
		return nil
	}
	return e.Source.Apply(ctx, changes)
}

func (e *Engine) deviceName(id string) string { return e.cur.DeviceName(id) }

// DeviceName is the name of a device in the manifest, else its ID.
func (m Manifest) DeviceName(id string) string {
	if d, ok := m.Devices[id]; ok {
		return d.DisplayName()
	}
	if r, ok := m.Removed[id]; ok && r.Name != "" {
		return r.Name
	}
	return id
}

// maybeGC deletes objects nothing refers to, once a day.
func (e *Engine) maybeGC(ctx context.Context, m Manifest) {
	if e.now().Sub(e.State.LastGC) < 24*time.Hour {
		return
	}
	e.State.LastGC = e.now()
	used := map[string]bool{}
	for _, en := range m.Entries {
		if en.Hash != "" {
			used[en.Hash+".bin"] = true
		}
	}
	infos, err := e.Client.List(ctx, e.p("objects"))
	if err != nil {
		return
	}
	for _, info := range infos {
		// Young objects may belong to a sync in progress on another device.
		if info.IsDir || used[info.Name] || e.now().Sub(info.ModTime) < 24*time.Hour {
			continue
		}
		_ = e.Client.Delete(ctx, e.p("objects", info.Name), "")
	}
}

// Devices returns the devices that synced with the vault.
func (e *Engine) Devices(ctx context.Context) (map[string]Device, error) {
	m, err := e.Manifest(ctx)
	if err != nil {
		return nil, err
	}
	return m.Devices, nil
}

// Manifest reads the manifest on the server.
func (e *Engine) Manifest(ctx context.Context) (Manifest, error) {
	rm, err := e.fetchManifest(ctx)
	if err != nil {
		return Manifest{}, err
	}
	return rm.m, nil
}

// UpdateManifest changes the manifest on the server with fn, starting over
// when another device writes it in between.
func (e *Engine) UpdateManifest(ctx context.Context, fn func(m *Manifest) error) error {
	for attempt := 0; attempt < 5; attempt++ {
		rm, err := e.fetchManifest(ctx)
		if err != nil {
			return err
		}
		if !rm.exists {
			return errors.New("the vault has no manifest yet: sync first")
		}
		if rm.m.KeyID != "" && rm.m.KeyID != e.Keys.ID {
			return errors.New("sync first: the vault's key changed")
		}
		next := rm.m.clone()
		if err := fn(&next); err != nil {
			return err
		}
		next.Seq++
		next.Updated, next.Writer = e.now().UTC(), e.State.DeviceID
		ok, err := e.writeManifest(ctx, rm, next)
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
	}
	return errors.New("other devices kept changing the server; try again")
}

// RenameDevice gives a device a name of the user's, "" for its own.
func (e *Engine) RenameDevice(ctx context.Context, id, label string) error {
	label = strings.TrimSpace(label)
	return e.UpdateManifest(ctx, func(m *Manifest) error {
		d, ok := m.Devices[id]
		if !ok {
			return errors.New("no such device")
		}
		d.Label = label
		m.Devices[id] = d
		m.addActivity(Activity{At: e.now().UTC(), Device: e.State.DeviceID, Kind: "rename", Target: id, Detail: label})
		return nil
	})
}

// RemoveDevice takes devices out of the vault: they leave the list and
// stop syncing when they next try. The paths given are deleted with them,
// such as the data only they wrote. Their copy of the key still opens what
// they have seen; Rotate takes that away too.
func (e *Engine) RemoveDevice(ctx context.Context, ids []string, paths []string) error {
	return e.UpdateManifest(ctx, func(m *Manifest) error {
		e.removeFrom(m, ids, paths)
		return nil
	})
}

func (e *Engine) removeFrom(m *Manifest, ids []string, paths []string) {
	now := e.now().UTC()
	for _, id := range ids {
		if id == e.State.DeviceID {
			continue
		}
		d := m.Devices[id]
		delete(m.Devices, id)
		m.Removed[id] = Removal{At: now, By: e.State.DeviceID, Name: d.DisplayName()}
		m.addActivity(Activity{At: now, Device: e.State.DeviceID, Kind: "remove", Target: id, Detail: d.DisplayName()})
	}
	for _, p := range paths {
		en, ok := m.Entries[p]
		if !ok || en.Deleted {
			continue
		}
		m.Entries[p] = Entry{Device: e.State.DeviceID, Clock: en.Clock.Inc(e.State.DeviceID), Modified: now, Deleted: true, Label: en.Label}
	}
}

// Resolve records the user's choice for a conflict, applied at the next
// sync.
func (s *State) Resolve(path, choice string) error {
	switch choice {
	case KeepLocal, KeepRemote, KeepBoth:
	default:
		return fmt.Errorf("unknown resolution %q", choice)
	}
	if s.Resolutions == nil {
		s.Resolutions = map[string]string{}
	}
	s.Resolutions[path] = choice
	s.Conflicts = slices.DeleteFunc(s.Conflicts, func(c Conflict) bool { return c.Path == path })
	return nil
}

// ---- Snapshots ----

// Snapshot is a full copy of the data.
type Snapshot struct {
	Created time.Time         `json:"created"`
	Device  string            `json:"device"`
	OS      string            `json:"os"`
	Version string            `json:"version"`
	Files   map[string][]byte `json:"files"`
}

// BackupInfo describes a snapshot on the server or the device.
type BackupInfo struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	Created time.Time `json:"created"`
	Device  string    `json:"device"`
	Remote  bool      `json:"remote"`
}

// ParseBackupName reads the time and device of a backup's file name,
// <20060102-150405>-<device>.bin.
func ParseBackupName(name string) (time.Time, string) {
	base := strings.TrimSuffix(name, ".bin")
	if len(base) < 15 {
		return time.Time{}, ""
	}
	t, err := time.ParseInLocation("20060102-150405", base[:15], time.UTC)
	if err != nil {
		return time.Time{}, ""
	}
	return t, strings.TrimPrefix(base[15:], "-")
}

// BackupName names a backup of device at t.
func BackupName(t time.Time, device string) string {
	clean := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, device)
	return t.UTC().Format("20060102-150405") + "-" + clean + ".bin"
}

// UploadBackup stores a snapshot on the server and keeps the newest keep
// backups of this device.
func (e *Engine) UploadBackup(ctx context.Context, snap Snapshot, keep int) (string, error) {
	if err := e.Client.MkdirAll(ctx, e.p("backups")); err != nil {
		return "", err
	}
	name := BackupName(snap.Created, e.DeviceName)
	sealed, err := e.Keys.SealJSON(snap, "backup/"+name)
	if err != nil {
		return "", err
	}
	if _, err := e.Client.Put(ctx, e.p("backups", name), sealed, webdav.Condition{}); err != nil {
		return "", err
	}
	list, err := e.ListBackups(ctx)
	if err == nil && keep > 0 {
		_, me := ParseBackupName(BackupName(snap.Created, e.DeviceName))
		n := 0
		for _, b := range list { // newest first
			if b.Device != me {
				continue
			}
			if n++; n > keep {
				_ = e.Client.Delete(ctx, e.p("backups", b.Name), "")
			}
		}
	}
	return name, nil
}

// ListBackups lists the backups on the server, newest first.
func (e *Engine) ListBackups(ctx context.Context) ([]BackupInfo, error) {
	infos, err := e.Client.List(ctx, e.p("backups"))
	if errors.Is(err, webdav.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []BackupInfo
	for _, info := range infos {
		if info.IsDir || !strings.HasSuffix(info.Name, ".bin") {
			continue
		}
		t, dev := ParseBackupName(info.Name)
		out = append(out, BackupInfo{Name: info.Name, Size: info.Size, Created: t, Device: dev, Remote: true})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}

// DownloadBackup reads a backup from the server.
func (e *Engine) DownloadBackup(ctx context.Context, name string) (Snapshot, error) {
	if strings.ContainsAny(name, `/\`) {
		return Snapshot{}, errors.New("invalid backup name")
	}
	data, _, err := e.Client.Get(ctx, e.p("backups", name))
	if err != nil {
		return Snapshot{}, err
	}
	var snap Snapshot
	err = e.Keys.OpenJSON(data, "backup/"+name, &snap)
	return snap, err
}

// DeleteBackup deletes a backup from the server.
func (e *Engine) DeleteBackup(ctx context.Context, name string) error {
	if strings.ContainsAny(name, `/\`) {
		return errors.New("invalid backup name")
	}
	return e.Client.Delete(ctx, e.p("backups", name), "")
}

// Current is the manifest of the last sync, as it was written or read.
func (e *Engine) Current() Manifest { return e.cur }
