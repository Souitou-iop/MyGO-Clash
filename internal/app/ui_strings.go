package app

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/i18n"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
)

// The main window says its messages with the functions of package locales,
// which mygo generate writes from the catalogs in locales/ (en.json has
// every message; the others fall back to it).

// T says a message whose key the code makes as it runs, as "nav."+page, in
// the language the app shows: the key of a catalog with its dots and dashes
// taken out, "navHome". args are pairs of a placeholder's name and its
// value.
func (m *mainUI) T(key string, args ...any) string {
	var vals map[string]any
	if len(args) > 1 {
		vals = make(map[string]any, len(args)/2)
		for i := 0; i+1 < len(args); i += 2 {
			vals[fmt.Sprint(args[i])] = fmt.Sprint(args[i+1])
		}
	}
	return locales.Catalog.Format(i18n.Locale(), catalogKey(key), vals)
}

// catalogKey turns a key of the old catalogs, "action.install-tun", into
// the catalogs' own, "actionInstallTun".
func catalogKey(key string) string {
	var b strings.Builder
	up := false
	for _, r := range key {
		switch {
		case r == '.' || r == '-':
			up = true
		case up:
			b.WriteString(strings.ToUpper(string(r)))
			up = false
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
