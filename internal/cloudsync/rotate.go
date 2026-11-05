package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"slices"

	"github.com/mygo-clash/mygo-clash/internal/webdav"
)

// Rotate replaces the vault's data key and passphrase, and removes the
// devices given (with the paths given, as RemoveDevice does). Everything on
// the server is encrypted again under the new key; the devices that stay
// find the new key sealed to their identity in vault.json and follow at
// their next sync, while a removed device, and anyone with only the old
// passphrase, can no longer read what the vault holds from now on.
//
// A device that never published its identity (an older version) needs
// the new passphrase to follow.
//
// It writes vault.json first, then the manifest; when the manifest lost a
// race with another device, vault.json is put back and nothing changed.
// Backups are encrypted again last: a failure there is returned with the
// new keys, which are in place by then.
func (e *Engine) Rotate(ctx context.Context, passphrase string, remove []string, paths []string) (*Keys, error) {
	if e.Keys.Plain() {
		return nil, ErrPlain
	}
	rm, err := e.fetchManifest(ctx)
	if err != nil {
		return nil, err
	}
	if !rm.exists {
		return nil, errors.New("the vault has no manifest yet: sync first")
	}
	if rm.m.KeyID != e.Keys.ID {
		return nil, errors.New("sync first: the vault's key changed")
	}
	vaultPath := path.Join(e.Dir, "vault.json")
	oldVault, vEtag, err := e.Client.Get(ctx, vaultPath)
	if err != nil {
		return nil, err
	}
	var meta VaultMeta
	if err := json.Unmarshal(oldVault, &meta); err != nil {
		return nil, fmt.Errorf("vault.json is damaged: %w", err)
	}
	if meta.KeyID != e.Keys.ID {
		return nil, errors.New("the vault on the server has another key")
	}
	meta2, keys2, err := NewVault(passphrase)
	if err != nil {
		return nil, err
	}
	meta2.Created = meta.Created

	next := rm.m.clone()
	e.removeFrom(&next, remove, paths)
	// Every object again, under the new key.
	renamed := map[string]string{}
	for p, en := range next.Entries {
		if en.Deleted || en.Hash == "" {
			continue
		}
		h2, ok := renamed[en.Hash]
		if !ok {
			data, err := e.download(ctx, en)
			if err != nil {
				return nil, err
			}
			h2 = keys2.Hash(data)
			sealed, err := keys2.Seal(data, h2)
			if err != nil {
				return nil, err
			}
			if _, err := e.Client.Put(ctx, e.p("objects", h2+".bin"), sealed, webdav.Condition{IfNoneMatch: true}); err != nil && !errors.Is(err, webdav.ErrPrecondition) {
				return nil, err
			}
			renamed[en.Hash] = h2
		}
		en.Hash = h2
		next.Entries[p] = en
	}
	now := e.now().UTC()
	next.KeyID, next.PrevKeyID = keys2.ID, e.Keys.ID
	next.Seq++
	next.Updated, next.Writer = now, e.State.DeviceID
	next.addActivity(Activity{At: now, Device: e.State.DeviceID, Kind: "rotate", Detail: fmt.Sprint(len(remove))})

	meta2.Removed = append(slices.Clone(meta.Removed), remove...)
	if n := len(meta2.Removed) - 200; n > 0 {
		meta2.Removed = meta2.Removed[n:]
	}
	meta2.Members = map[string][]byte{}
	for id, d := range next.Devices {
		if len(d.PubKey) == 0 {
			continue
		}
		sealed, err := SealTo(d.PubKey, keys2.data)
		if err != nil {
			continue // a damaged key: that device needs the passphrase
		}
		meta2.Members[id] = sealed
	}
	if e.Identity != nil {
		// This device, whether or not its key was published yet: should
		// keeping the new key here fail, the next sync finds it there.
		if sealed, err := SealTo(e.Identity.Public(), keys2.data); err == nil {
			meta2.Members[e.State.DeviceID] = sealed
		}
	}
	newVault, _ := json.MarshalIndent(meta2, "", "  ")
	if _, err := e.Client.Put(ctx, vaultPath, newVault, webdav.Condition{IfMatch: vEtag}); err != nil {
		if errors.Is(err, webdav.ErrPrecondition) {
			return nil, errors.New("another device changed the vault meanwhile; try again")
		}
		return nil, err
	}
	restore := func() { _, _ = e.Client.Put(context.WithoutCancel(ctx), vaultPath, oldVault, webdav.Condition{}) }

	ok, err := e.writeRotated(ctx, rm, next, keys2)
	if err != nil || !ok {
		restore()
		if err == nil {
			err = errors.New("another device synced meanwhile; try again")
		}
		return nil, err
	}

	// The new key is in place: carry the base over (see adopt).
	base := map[string]Entry{}
	for p, b := range e.State.Base {
		if !b.Deleted {
			if h2, ok := renamed[b.Hash]; ok {
				b.Hash = h2
			} else {
				b.Hash = "changed"
			}
		}
		base[p] = b
	}
	old := e.Keys
	if e.OnKeys != nil {
		if err := e.OnKeys(keys2); err != nil {
			return keys2, err
		}
	}
	e.Keys, e.State.Base, e.State.KeyID = keys2, base, keys2.ID
	e.cur = next

	// The backups, last.
	list, err := (&Engine{Client: e.Client, Dir: e.Dir, Keys: old}).ListBackups(ctx)
	if err != nil {
		return keys2, fmt.Errorf("the key was replaced, but the backups could not be listed: %w", err)
	}
	var errs []error
	for _, b := range list {
		data, _, err := e.Client.Get(ctx, e.p("backups", b.Name))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		plain, err := old.Open(data, "backup/"+b.Name)
		if err != nil {
			continue // not of the old key: an older one, already unreadable
		}
		sealed, err := keys2.Seal(plain, "backup/"+b.Name)
		if err == nil {
			_, err = e.Client.Put(ctx, e.p("backups", b.Name), sealed, webdav.Condition{})
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return keys2, fmt.Errorf("the key was replaced, but %d backups could not be encrypted again: %w", len(errs), errors.Join(errs...))
	}
	return keys2, nil
}

// writeRotated writes the manifest sealed under the new keys if the server
// still has the one that was read.
func (e *Engine) writeRotated(ctx context.Context, rm remoteManifest, next Manifest, keys *Keys) (bool, error) {
	sealed, err := keys.SealJSON(next, "manifest")
	if err != nil {
		return false, err
	}
	if rm.etag == "" {
		unlock, err := e.lock(ctx)
		if err != nil {
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
	if _, err := e.Client.Put(ctx, e.p(manifestFile), sealed, webdav.Condition{IfMatch: rm.etag}); errors.Is(err, webdav.ErrPrecondition) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	data, _, err := e.Client.Get(ctx, e.p(manifestFile))
	if err != nil {
		return false, err
	}
	var check Manifest
	if err := keys.OpenJSON(data, "manifest", &check); err != nil {
		return false, nil
	}
	return check.Seq == next.Seq && check.Writer == next.Writer && check.Updated.Equal(next.Updated), nil
}

// Rejoin opens the vault with its passphrase after another device replaced
// its key without leaving a copy for this one, and carries the state over.
func (e *Engine) Rejoin(ctx context.Context, passphrase string) (*Keys, error) {
	meta, err := ReadVault(ctx, e.Client, e.Dir)
	if err != nil {
		return nil, err
	}
	keys, err := meta.Open(passphrase)
	if err != nil {
		return nil, err
	}
	if keys.ID == e.Keys.ID {
		return keys, nil
	}
	local, err := e.Source.Snapshot(ctx)
	if err != nil {
		return nil, err
	}
	return keys, e.adopt(keys, local)
}
