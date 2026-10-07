package cloudsync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/webdav"
)

// ErrNoVault is returned when the server has no vault and none was to be
// created.
var ErrNoVault = errors.New("the server has no vault yet")

// OpenVault opens the vault in dir on the server with the passphrase. When
// there is none and create is set, it creates one, plain if so asked, and
// reports so. Opening follows the vault: a plain one needs no passphrase.
func OpenVault(ctx context.Context, c *webdav.Client, dir, passphrase string, create, plain bool) (*Keys, bool, error) {
	if err := c.MkdirAll(ctx, dir); err != nil {
		return nil, false, err
	}
	data, _, err := c.Get(ctx, path.Join(dir, "vault.json"))
	if errors.Is(err, webdav.ErrNotFound) {
		if !create {
			return nil, false, ErrNoVault
		}
		var meta VaultMeta
		var keys *Keys
		if plain {
			meta, keys = NewPlainVault()
		} else if meta, keys, err = NewVault(passphrase); err != nil {
			return nil, false, err
		}
		out, _ := json.MarshalIndent(meta, "", "  ")
		if _, err := c.Put(ctx, path.Join(dir, "vault.json"), out, webdav.Condition{IfNoneMatch: true}); errors.Is(err, webdav.ErrPrecondition) {
			// Another device created it just now: open theirs.
			return OpenVault(ctx, c, dir, passphrase, false, plain)
		} else if err != nil {
			return nil, false, err
		}
		return keys, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	var meta VaultMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, false, fmt.Errorf("vault.json is damaged: %w", err)
	}
	keys, err := meta.Open(passphrase)
	return keys, false, err
}

// ReadVault returns the vault in dir on the server, ErrNoVault if none.
func ReadVault(ctx context.Context, c *webdav.Client, dir string) (VaultMeta, error) {
	data, _, err := c.Get(ctx, path.Join(dir, "vault.json"))
	if errors.Is(err, webdav.ErrNotFound) {
		return VaultMeta{}, ErrNoVault
	}
	if err != nil {
		return VaultMeta{}, err
	}
	var meta VaultMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return VaultMeta{}, fmt.Errorf("vault.json is damaged: %w", err)
	}
	return meta, nil
}

// ChangePassphrase wraps the vault's key with a new passphrase. Other
// devices keep working with the key they hold; new devices need the new
// passphrase.
func ChangePassphrase(ctx context.Context, c *webdav.Client, dir string, keys *Keys, passphrase string) error {
	data, etag, err := c.Get(ctx, path.Join(dir, "vault.json"))
	if err != nil {
		return err
	}
	var meta VaultMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	if meta.KeyID != keys.ID {
		return errors.New("the vault on the server has another key")
	}
	meta, err = meta.Rewrap(keys, passphrase)
	if err != nil {
		return err
	}
	out, _ := json.MarshalIndent(meta, "", "  ")
	_, err = c.Put(ctx, path.Join(dir, "vault.json"), out, webdav.Condition{IfMatch: etag})
	return err
}

// exported is the format of backups exported to a file: a vault of its
// own, opened with the password chosen at export.
type exported struct {
	Format  string    `json:"format"`
	Vault   VaultMeta `json:"vault"`
	Payload []byte    `json:"payload"`
}

// Export encrypts a snapshot with a password, for a file the user keeps.
func Export(snap Snapshot, password string) ([]byte, error) {
	meta, keys, err := NewVault(password)
	if err != nil {
		return nil, err
	}
	payload, err := keys.SealJSON(snap, "export")
	if err != nil {
		return nil, err
	}
	return json.Marshal(exported{Format: "mygo-clash-backup/v1", Vault: meta, Payload: payload})
}

// Import decrypts a file made by Export.
func Import(data []byte, password string) (Snapshot, error) {
	var ex exported
	if err := json.Unmarshal(data, &ex); err != nil || ex.Format != "mygo-clash-backup/v1" {
		return Snapshot{}, errors.New("the file is not a MyGO-Clash backup")
	}
	keys, err := ex.Vault.Open(password)
	if err != nil {
		if errors.Is(err, ErrPassphrase) {
			return Snapshot{}, errors.New("the password does not open this backup")
		}
		return Snapshot{}, err
	}
	var snap Snapshot
	if err := keys.OpenJSON(ex.Payload, "export", &snap); err != nil {
		return Snapshot{}, err
	}
	if snap.Created.IsZero() {
		snap.Created = time.Now()
	}
	return snap, nil
}
