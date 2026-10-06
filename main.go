// MyGO-Clash is a desktop proxy client built on mihomo with MyGo.
//
// The executable runs as the app, as its core (mygo-clash core) or as its
// privileged service (mygo-clash service run|install|uninstall).
package main

import (
	"fmt"
	"os"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/app"
	"github.com/mygo-clash/mygo-clash/internal/corehost"
	"github.com/mygo-clash/mygo-clash/internal/service"
)

// version is set by the build: mygo build links the configuration's.
var version = "0.1.0-beta"

func main() {
	if v := mygo.App.Version(); v != "" {
		version = v // linked by mygo build from mygo.json
	}
	app.Version, corehost.AppVersion, service.Version = version, version, version
	if len(os.Args) > 1 {
		var err error
		switch os.Args[1] {
		case "core":
			err = corehost.Run(os.Stdin)
		case "service":
			err = service.Main(os.Args[2:])
		case "uwp-loopback":
			err = app.UWPLoopback()
		default:
			app.Main()
			return
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", os.Args[1], err)
			os.Exit(1)
		}
		return
	}
	app.Main()
}
