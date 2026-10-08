package webrtc

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

type browser struct {
	installed string // a key of the user or the machine that it is installed by
	policy    string
	name      string
}

var browsers = []browser{
	{`Software\Microsoft\Windows\CurrentVersion\App Paths\chrome.exe`, `Software\Policies\Google\Chrome`, "WebRtcIPHandling"},
	{`Software\Microsoft\Windows\CurrentVersion\App Paths\msedge.exe`, `Software\Policies\Microsoft\Edge`, "WebRtcLocalhostIpHandling"},
	{`Software\Microsoft\Windows\CurrentVersion\App Paths\brave.exe`, `Software\Policies\BraveSoftware\Brave`, "WebRtcIPHandling"},
	{`Software\Chromium`, `Software\Policies\Chromium`, "WebRtcIPHandling"},
}

func installed(key string) bool {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		if k, err := registry.OpenKey(root, key, registry.QUERY_VALUE); err == nil {
			k.Close()
			return true
		}
	}
	return false
}

// Apply sets the policy of the installed browsers to h, or removes it for
// Unset. A value it did not set, by the user or an administrator, stays:
// state, a file, remembers what it set.
func Apply(h Handling, state string) error {
	set := map[string]string{}
	if b, err := os.ReadFile(state); err == nil {
		_ = json.Unmarshal(b, &set)
	}
	var errs []error
	for _, b := range browsers {
		id := b.policy + `\` + b.name
		mine, ours := set[id]
		if h != Unset && !installed(b.installed) && !ours {
			continue // no keys for browsers that are not there
		}
		var k registry.Key
		var err error
		if h == Unset {
			k, err = registry.OpenKey(registry.CURRENT_USER, b.policy, registry.QUERY_VALUE|registry.SET_VALUE)
		} else {
			k, _, err = registry.CreateKey(registry.CURRENT_USER, b.policy, registry.QUERY_VALUE|registry.SET_VALUE)
		}
		if err != nil {
			if h != Unset {
				errs = append(errs, fmt.Errorf("%s: %w", b.policy, err))
			} else {
				delete(set, id)
			}
			continue
		}
		cur, _, err := k.GetStringValue(b.name)
		present := err == nil
		switch {
		case present && (!ours || cur != mine):
			// Someone else's.
			delete(set, id)
		case h == Unset:
			if present {
				if err := k.DeleteValue(b.name); err != nil {
					errs = append(errs, err)
				}
			}
			delete(set, id)
		default:
			if err := k.SetStringValue(b.name, string(h)); err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", b.policy, err))
				break
			}
			set[id] = string(h)
		}
		k.Close()
	}
	data, _ := json.Marshal(set)
	if err := os.WriteFile(state, data, 0o600); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}
