// Package geodata holds the geo databases that release builds carry, so a
// first start applies a profile with GEOIP or GEOSITE rules without a
// download, as Clash Verge does. tools/geodata fetches them before a build;
// development builds may carry none.
package geodata

import (
	"embed"
	"io/fs"
)

//go:embed all:files
var files embed.FS

// Read returns a carried database by its name in meta-rules-dat, such as
// geoip.metadb, or false when this build carries none.
func Read(name string) ([]byte, bool) {
	b, err := fs.ReadFile(files, "files/"+name)
	return b, err == nil && len(b) > 0
}
