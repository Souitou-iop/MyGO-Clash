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
