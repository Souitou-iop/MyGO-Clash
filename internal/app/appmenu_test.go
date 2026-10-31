package app

import (
	"strings"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

func TestAppMenu(t *testing.T) {
	// A language of its own, so the updater's label needs no locale of a
	// running app.
	mygo.Use(updater.New(updater.Options{Language: "en"}))
	items := appMenuItems()
	if len(items) != 5 {
		t.Fatalf("%d menus, want 5", len(items))
	}
	app := items[0]
	if app.Role != mygo.RoleAppMenu || len(app.Submenu) == 0 {
		t.Fatalf("the first menu is %q, want the app's own", app.Role)
	}
	if app.Submenu[0].Role != mygo.RoleAbout {
		t.Errorf("the first item is %q, want about", app.Submenu[0].Role)
	}
	it := app.Submenu[1]
	if !strings.HasPrefix(it.Label, "Check for Updates") || it.Click == nil {
		t.Errorf("the second item is %q, want the updater's Check for Updates", it.Label)
	}
	if app.Submenu[2].Type != mygo.MenuItemSeparator {
		t.Errorf("the third item is %v, want a separator", app.Submenu[2].Type)
	}
}
