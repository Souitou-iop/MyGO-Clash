package cloudsync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func withIdentity(d *device) *device {
	d.eng.Identity = NewIdentity()
	d.eng.Self = Device{PubKey: d.eng.Identity.Public(), Version: "test"}
	d.eng.OnKeys = func(k *Keys) error { return nil }
	return d
}

func TestRenameAndRemoveDevice(t *testing.T) {
	s := newServer(t, true)
	a := newDevice(t, s, "laptop", "correct horse battery", "ask")
	b := newDevice(t, s, "desktop", "correct horse battery", "ask")
	a.src.set("profile/x", "x1")
	a.sync(t)
	b.sync(t)

	ctx := context.Background()
	if err := a.eng.RenameDevice(ctx, "desktop", "Office PC"); err != nil {
		t.Fatal(err)
	}
	// The device's own syncs keep the user's name for it.
	b.src.set("profile/x", "x2")
	b.sync(t)
	m, err := a.eng.Manifest(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Devices["desktop"].DisplayName(); got != "Office PC" {
		t.Fatalf("label after sync = %q", got)
	}
	kinds := map[string]int{}
	for _, ac := range m.Activity {
		kinds[ac.Kind]++
	}
	if kinds["join"] != 2 || kinds["rename"] != 1 || kinds["sync"] < 2 {
		t.Fatalf("activity = %v", kinds)
	}

	b.src.set("usage/desktop", "{}")
	b.sync(t)
	if err := a.eng.RemoveDevice(ctx, []string{"desktop"}, []string{"usage/desktop"}); err != nil {
		t.Fatal(err)
	}
	var rerr *RemovedError
	if _, err := b.eng.Sync(ctx); !errors.As(err, &rerr) || rerr.By != "laptop" {
		t.Fatalf("removed device synced: %v", err)
	}
	a.sync(t)
	if _, ok := a.src.files["usage/desktop"]; ok {
		t.Fatal("the removed device's data stayed")
	}
	m, _ = a.eng.Manifest(ctx)
	if _, ok := m.Devices["desktop"]; ok {
		t.Fatal("the removed device is still listed")
	}

	// Joining again lifts the removal.
	b.eng.State = &State{DeviceID: "desktop", Joined: time.Now().Add(time.Second)}
	time.Sleep(1100 * time.Millisecond)
	b.sync(t)
	m, _ = a.eng.Manifest(ctx)
	if _, ok := m.Devices["desktop"]; !ok {
		t.Fatal("the device that joined again is not listed")
	}
	if _, ok := m.Removed["desktop"]; ok {
		t.Fatal("the removal stayed after joining again")
	}
}

func TestRotate(t *testing.T)       { testRotate(t, true) }
func TestRotateNoETag(t *testing.T) { testRotate(t, false) }

func testRotate(t *testing.T, etags bool) {
	s := newServer(t, etags)
	a := withIdentity(newDevice(t, s, "laptop", "correct horse battery", "ask"))
	b := withIdentity(newDevice(t, s, "desktop", "correct horse battery", "ask"))
	c := withIdentity(newDevice(t, s, "stolen", "correct horse battery", "ask"))
	old := newDevice(t, s, "old-version", "correct horse battery", "ask")  // no identity
	oldGone := newDevice(t, s, "old-gone", "correct horse battery", "ask") // no identity, removed

	a.src.set("settings", `{"theme":"dark"}`)
	a.src.set("profile/x", "x1")
	a.sync(t)
	for _, d := range []*device{b, c, old, oldGone} {
		d.sync(t)
	}
	ctx := context.Background()
	if _, err := a.eng.UploadBackup(ctx, Snapshot{Created: time.Now(), Files: map[string][]byte{"profile/x": []byte("x1")}}, 5); err != nil {
		t.Fatal(err)
	}
	oldID := a.eng.Keys.ID
	keys, err := a.eng.Rotate(ctx, "a brand new passphrase", []string{"stolen", "old-gone"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if keys.ID == oldID || a.eng.Keys.ID != keys.ID {
		t.Fatal("the key did not change")
	}
	// Nothing is pushed again after the rotation.
	if r := a.sync(t); len(r.Pushed) != 0 {
		t.Fatalf("pushed after rotating: %+v", r)
	}

	// The device that stays follows with the key sealed to it, and keeps
	// syncing without pushing everything again.
	if r := b.sync(t); len(r.Pushed) != 0 || len(r.Pulled) != 0 || b.eng.Keys.ID != keys.ID {
		t.Fatalf("follower: %+v, key %s", r, b.eng.Keys.ID)
	}
	b.src.set("profile/x", "x2")
	b.sync(t)
	if r := a.sync(t); len(r.Pulled) != 1 || a.src.get("profile/x") != "x2" {
		t.Fatalf("after rotation: %+v", r)
	}

	// The removed device is out.
	var rerr *RemovedError
	if _, err := c.eng.Sync(ctx); !errors.As(err, &rerr) {
		t.Fatalf("removed device: %v", err)
	}
	if _, err := oldGone.eng.Sync(ctx); !errors.As(err, &rerr) {
		t.Fatalf("removed device without identity: %v", err)
	}
	// The old passphrase no longer opens the vault.
	meta, err := ReadVault(ctx, a.eng.Client, a.eng.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := meta.Open("correct horse battery"); !errors.Is(err, ErrPassphrase) {
		t.Fatalf("old passphrase: %v", err)
	}
	// A device without an identity needs the new passphrase, then follows.
	if _, err := old.eng.Sync(ctx); !errors.Is(err, ErrKeyReplaced) {
		t.Fatalf("device without identity: %v", err)
	}
	k, err := old.eng.Rejoin(ctx, "a brand new passphrase")
	if err != nil || k.ID != keys.ID {
		t.Fatalf("rejoin: %v", err)
	}
	if r := old.sync(t); len(r.Pushed) != 0 || old.src.get("profile/x") != "x2" {
		t.Fatalf("after rejoin: %+v %q", r, old.src.get("profile/x"))
	}
	// Backups follow the new key.
	list, err := a.eng.ListBackups(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("backups: %v %v", list, err)
	}
	if _, err := a.eng.DownloadBackup(ctx, list[0].Name); err != nil {
		t.Fatalf("backup after rotation: %v", err)
	}
}

func TestMemberSeal(t *testing.T) {
	id := NewIdentity()
	sealed, err := SealTo(id.Public(), []byte("secret key"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := id.Open(sealed)
	if err != nil || string(got) != "secret key" {
		t.Fatalf("open = %q, %v", got, err)
	}
	if _, err := NewIdentity().Open(sealed); err == nil {
		t.Fatal("another identity opened it")
	}
	back, err := IdentityFromRaw(id.Raw())
	if err != nil || string(back.Public()) != string(id.Public()) {
		t.Fatal("raw round trip")
	}
}

// A path a device starts syncing later comes from the server; it is not
// deleted there as if the device had deleted it.
func TestWantedLater(t *testing.T) {
	s := newServer(t, true)
	a := newDevice(t, s, "laptop", "correct horse battery", "ask")
	b := newDevice(t, s, "desktop", "correct horse battery", "ask")
	a.src.set("usage/laptop", "stats")
	a.src.set("profile/x", "x1")
	a.sync(t)
	want := false
	b.eng.Wanted = func(p string) bool { return want || !strings.HasPrefix(p, "usage/") }
	b.sync(t)
	if _, ok := b.src.files["usage/laptop"]; ok {
		t.Fatal("an unwanted path was pulled")
	}
	want = true
	if r := b.sync(t); len(r.Pulled) != 1 || len(r.Pushed) != 0 {
		t.Fatalf("turning the path on: %+v", r)
	}
	if a.sync(t); a.src.get("usage/laptop") != "stats" {
		t.Fatal("the path was deleted")
	}
}
