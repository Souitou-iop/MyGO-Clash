package app

import "github.com/egoist/mygo"

// bind gives the page the app's services, under the names the generated
// client uses.
func (a *App) bind() {
	mygo.BindAs("App", AppService{a})
	mygo.BindAs("Settings", SettingsService{a})
	mygo.BindAs("Profiles", Profiles{a})
	mygo.BindAs("Proxies", Proxies{a})
	mygo.BindAs("Connections", Connections{a})
	mygo.BindAs("Rules", Rules{a})
	mygo.BindAs("Logs", Logs{a})
	mygo.BindAs("Core", Core{a})
	mygo.BindAs("System", System{a})
	mygo.BindAs("Tailscale", Tailscale{a})
	mygo.BindAs("Sync", Sync{a})
	mygo.BindAs("Tools", Tools{a})
	mygo.BindAs("Updates", UpdateService{a})
}
