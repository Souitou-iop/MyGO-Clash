package app

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// A drop-down's list grows in as it opens and stays a moment as it closes,
// and choosing in it still changes the value.
func TestDropdownOpensAndClosesWithMotion(t *testing.T) {
	m := &mainUI{}
	val := "Inline"
	opts := []string{"Submenu", "Inline", "Hidden"}
	view := func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(false, false, "", "")
		c.SetTheme(m.pal.Theme)
		ui.Column(c).Fill().Padding(40).Children(func() { dropdown(c.Key("d"), &val, opts).Width(200) })
	}
	tt := ui.NewTester(view, 400, 300)
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Hidden") {
		t.Fatal("the list shows before it opens")
	}
	shot := func(name string) {
		dir := os.Getenv("MYGO_SHOT_DIR")
		if dir == "" {
			return
		}
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		_ = png.Encode(f, tt.Image())
	}
	if err := tt.Click("Inline"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(40 * time.Millisecond)
	shot("opening")
	tt.Advance(time.Second)
	shot("open")
	if !tt.HasText("Hidden") {
		t.Fatal("the list does not show once opened")
	}
	if err := tt.Click("Hidden"); err != nil {
		t.Fatal(err)
	}
	if val != "Hidden" {
		t.Errorf("chose %q, want Hidden", val)
	}
	tt.Advance(40 * time.Millisecond)
	shot("closing")
	// Where the list was, under the trigger, the page shows again only
	// once it has shrunk back.
	page := tt.Image().At(350, 250)
	if tt.Image().At(100, 110) == page {
		t.Error("the list is gone at once instead of shrinking back")
	}
	tt.Advance(time.Second)
	if tt.Image().At(100, 110) != page {
		t.Error("the list stays after it closed")
	}
}

// The menu of a "more" button grows out of its button, as a drop-down's
// list does, and stays a moment as it closes.
func TestButtonMenuOpensAndClosesWithMotion(t *testing.T) {
	m := &mainUI{}
	chosen := ""
	view := func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(false, false, "", "")
		c.SetTheme(m.pal.Theme)
		ui.Row(c).Fill().Padding(40).Justify(ui.End).Children(func() {
			ui.Button(c.Key("more"), "More").PopoverMenuButton(func(menu *ui.Menu) {
				if menu.Item("Export").Chosen() {
					chosen = "Export"
				}
				if menu.Item("Clear").Chosen() {
					chosen = "Clear"
				}
			}, menuOptions(c))
		})
	}
	tt := ui.NewTester(view, 400, 300)
	tt.Frame()
	tt.Advance(time.Second)
	if tt.HasText("Export") {
		t.Fatal("the menu shows before it opens")
	}
	if err := tt.Click("More"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if !tt.HasText("Export") {
		t.Fatal("the menu does not show once opened")
	}
	r, _ := tt.Find("Clear")
	page := tt.Image().At(20, 280)
	if err := tt.Click("Clear"); err != nil {
		t.Fatal(err)
	}
	if chosen != "Clear" {
		t.Errorf("chose %q, want Clear", chosen)
	}
	tt.Advance(40 * time.Millisecond)
	if tt.Image().At(int(r.X+r.W/2), int(r.Y+r.H/2)) == page {
		t.Error("the menu is gone at once instead of shrinking back")
	}
	tt.Advance(time.Second)
	if tt.Image().At(int(r.X+r.W/2), int(r.Y+r.H/2)) != page {
		t.Error("the menu stays after it closed")
	}
}
