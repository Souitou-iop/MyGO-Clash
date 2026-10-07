// Package uwp lists the Windows Store apps (app containers) and which of
// them may reach the loopback, where the proxy listens: Windows keeps them
// from it unless they are exempted, as CheckNetIsolation does.
package uwp

import (
	"bufio"
	"os"
	"slices"
	"strings"
)

// StoreApp is an app container: a Store app.
type StoreApp struct {
	// SID identifies it; exemptions are by SID.
	SID string `json:"sid"`
	// Name is its package family name, such as
	// Microsoft.WindowsStore_8wekyb3d8bbwe.
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Exempt      bool   `json:"exempt"`
}

// Exemptions returns the SIDs to exempt when the user chose selected among
// listed: the exemptions of containers the list does not show, such as
// another user's, stay as they are.
func Exemptions(current []string, listed []StoreApp, selected []string) []string {
	shown := map[string]bool{}
	for _, a := range listed {
		shown[strings.ToUpper(a.SID)] = true
	}
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !slices.ContainsFunc(out, func(x string) bool { return strings.EqualFold(x, s) }) {
			out = append(out, s)
		}
	}
	for _, s := range current {
		if !shown[strings.ToUpper(s)] {
			add(s)
		}
	}
	for _, s := range selected {
		if shown[strings.ToUpper(s)] {
			add(s)
		}
	}
	return out
}

// WriteList writes SIDs one per line, for the elevated helper.
func WriteList(path string, sids []string) error {
	return os.WriteFile(path, []byte(strings.Join(sids, "\n")+"\n"), 0o600)
}

// ReadList reads what WriteList wrote.
func ReadList(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if s := strings.TrimSpace(sc.Text()); s != "" {
			out = append(out, s)
		}
	}
	return out, sc.Err()
}
