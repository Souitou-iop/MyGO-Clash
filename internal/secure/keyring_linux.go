package secure

import (
	"errors"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
)

// DefaultKeyring returns the desktop's Secret Service (GNOME Keyring,
// KWallet), falling back to a file when the session has none.
func DefaultKeyring(service, dataDir string) Keyring {
	if forceFile() {
		return FileKeyring(dataDir)
	}
	conn, err := dbus.SessionBus()
	if err != nil {
		return FileKeyring(dataDir)
	}
	var owner string
	if err := conn.BusObject().Call("org.freedesktop.DBus.GetNameOwner", 0, ssName).Store(&owner); err != nil {
		// Activate the service when it is installed but not running.
		var ok bool
		if err := conn.BusObject().Call("org.freedesktop.DBus.StartServiceByName", 0, ssName, uint32(0)).Store(&ok); err != nil {
			return FileKeyring(dataDir)
		}
	}
	return secretService{conn: conn, service: service}
}

const (
	ssName       = "org.freedesktop.secrets"
	ssPath       = "/org/freedesktop/secrets"
	ssIface      = "org.freedesktop.Secret.Service"
	itemIface    = "org.freedesktop.Secret.Item"
	collIface    = "org.freedesktop.Secret.Collection"
	promptIface  = "org.freedesktop.Secret.Prompt"
	defaultAlias = "/org/freedesktop/secrets/aliases/default"
)

type secretService struct {
	conn    *dbus.Conn
	service string
}

// ssSecret is the Secret structure of the Secret Service API.
type ssSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

func (k secretService) attrs(account string) map[string]string {
	return map[string]string{"service": k.service, "account": account, "application": "mygo-clash"}
}

func (k secretService) session() (dbus.ObjectPath, error) {
	var out dbus.Variant
	var path dbus.ObjectPath
	err := k.conn.Object(ssName, ssPath).Call(ssIface+".OpenSession", 0, "plain", dbus.MakeVariant("")).Store(&out, &path)
	return path, err
}

func (k secretService) find(account string) (dbus.ObjectPath, error) {
	svc := k.conn.Object(ssName, ssPath)
	var unlocked, locked []dbus.ObjectPath
	if err := svc.Call(ssIface+".SearchItems", 0, k.attrs(account)).Store(&unlocked, &locked); err != nil {
		return "", err
	}
	if len(unlocked) > 0 {
		return unlocked[0], nil
	}
	if len(locked) == 0 {
		return "", ErrNotFound
	}
	if err := k.unlock(locked[:1]); err != nil {
		return "", err
	}
	return locked[0], nil
}

// unlock unlocks objects, showing the desktop's prompt if it asks for one.
func (k secretService) unlock(paths []dbus.ObjectPath) error {
	var done []dbus.ObjectPath
	var prompt dbus.ObjectPath
	if err := k.conn.Object(ssName, ssPath).Call(ssIface+".Unlock", 0, paths).Store(&done, &prompt); err != nil {
		return err
	}
	return k.prompt(prompt)
}

// prompt runs a prompt of the Secret Service and waits for its answer.
func (k secretService) prompt(path dbus.ObjectPath) error {
	if path == "/" || path == "" {
		return nil
	}
	ch := make(chan *dbus.Signal, 4)
	k.conn.Signal(ch)
	defer k.conn.RemoveSignal(ch)
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(path), dbus.WithMatchInterface(promptIface), dbus.WithMatchMember("Completed")}
	if err := k.conn.AddMatchSignal(match...); err != nil {
		return err
	}
	defer k.conn.RemoveMatchSignal(match...)
	if err := k.conn.Object(ssName, path).Call(promptIface+".Prompt", 0, "").Err; err != nil {
		return err
	}
	timeout := time.After(2 * time.Minute)
	for {
		select {
		case sig := <-ch:
			if sig.Path != path || sig.Name != promptIface+".Completed" {
				continue
			}
			if len(sig.Body) > 0 {
				if dismissed, _ := sig.Body[0].(bool); dismissed {
					return errors.New("the keyring prompt was dismissed")
				}
			}
			return nil
		case <-timeout:
			return errors.New("the keyring prompt timed out")
		}
	}
}

func (k secretService) Get(account string) ([]byte, error) {
	item, err := k.find(account)
	if err != nil {
		return nil, err
	}
	sess, err := k.session()
	if err != nil {
		return nil, err
	}
	defer k.conn.Object(ssName, sess).Call("org.freedesktop.Secret.Session.Close", 0)
	var s ssSecret
	if err := k.conn.Object(ssName, item).Call(itemIface+".GetSecret", 0, sess).Store(&s); err != nil {
		return nil, err
	}
	return s.Value, nil
}

func (k secretService) Set(account string, secret []byte) error {
	sess, err := k.session()
	if err != nil {
		return err
	}
	defer k.conn.Object(ssName, sess).Call("org.freedesktop.Secret.Session.Close", 0)
	coll := k.conn.Object(ssName, defaultAlias)
	props := map[string]dbus.Variant{
		"org.freedesktop.Secret.Item.Label":      dbus.MakeVariant(fmt.Sprintf("%s (%s)", k.service, account)),
		"org.freedesktop.Secret.Item.Attributes": dbus.MakeVariant(k.attrs(account)),
	}
	s := ssSecret{Session: sess, Value: secret, ContentType: "application/octet-stream"}
	var item, prompt dbus.ObjectPath
	call := coll.Call(collIface+".CreateItem", 0, props, s, true)
	if call.Err != nil {
		// The default collection may be locked.
		if err := k.unlock([]dbus.ObjectPath{defaultAlias}); err != nil {
			return call.Err
		}
		call = coll.Call(collIface+".CreateItem", 0, props, s, true)
		if call.Err != nil {
			return call.Err
		}
	}
	if err := call.Store(&item, &prompt); err != nil {
		return err
	}
	if err := k.prompt(prompt); err != nil {
		return err
	}
	if _, err := k.Get(account); err != nil {
		return fmt.Errorf("secret service: the secret did not save: %w", err)
	}
	return nil
}

func (k secretService) Delete(account string) error {
	item, err := k.find(account)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var prompt dbus.ObjectPath
	if err := k.conn.Object(ssName, item).Call(itemIface+".Delete", 0).Store(&prompt); err != nil {
		return err
	}
	return k.prompt(prompt)
}

func (secretService) Name() string { return "secret-service" }
func (secretService) Secure() bool { return true }
