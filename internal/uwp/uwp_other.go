//go:build !windows

package uwp

import "errors"

var errWindows = errors.New("only Windows has Store apps")

// List returns the app containers of the user, with their exemptions.
func List() ([]StoreApp, error) { return nil, errWindows }

// Exempted returns the SIDs exempted from the loopback restriction.
func Exempted() ([]string, error) { return nil, errWindows }

// SetExempted replaces the exemptions; it takes an administrator.
func SetExempted([]string) error { return errWindows }
