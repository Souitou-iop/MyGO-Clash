package app

// The application menu of macOS, the bar the app's name heads: MyGo
// installs a default one, which this replaces so that the updater's
// Check for Updates… item follows About, as Sparkle's apps have it.
// The other platforms keep their default, which is no bar at all.

import (
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

// useAppMenu installs the application menu once the app is ready, when
// the language it labels its roles in is known.
func useAppMenu() {
	if runtime.GOOS != "darwin" {
		return
	}
	mygo.App.WhenReady(func() {
		mygo.App.SetMenu(mygo.NewMenu(appMenuItems()))
	})
}

// appMenuItems is the template of the application menu: MyGo's default
// macOS menus, with the updater's item added to the app's own.
func appMenuItems() []*mygo.MenuItem {
	return []*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout},
			updater.MenuItem(),
			mygo.Separator(),
			{Role: mygo.RoleServices},
			mygo.Separator(),
			{Role: mygo.RoleHide},
			{Role: mygo.RoleHideOthers},
			{Role: mygo.RoleUnhide},
			mygo.Separator(),
			{Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleFileMenu},
		{Role: mygo.RoleEditMenu},
		{Role: mygo.RoleViewMenu},
		{Role: mygo.RoleWindowMenu},
	}
}
