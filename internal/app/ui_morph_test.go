package app

import (
	"bytes"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// Nodes changing from the rows of a list to the tiles of a grid, and back,
// morph from one to the other: the cells keep one box that grows or
// shrinks from the old size to the new, instead of showing both layouts
// at once.
func TestNodesMorphBetweenListAndGrid(t *testing.T) {
	m := &mainUI{}
	items := []*ProxyItem{{Name: "HK 01", Type: "Shadowsocks", Delay: 80}, {Name: "JP 01", Type: "Trojan", Delay: 120}, {Name: "US 01", Type: "Vmess", Delay: 200}, {Name: "SG 01", Type: "Vless", Delay: 0}}
	list := true
	view := func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(false, false, "", "")
		c.SetTheme(m.pal.Theme)
		ui.Column(c).Fill().Padding(20).Children(func() {
			mode, gap := "card", float32(10)
			if list {
				mode, gap = "list", 4
			}
			cols := func(w float32) int {
				if list {
					return 1
				}
				return int((w + 10) / (196 + 10))
			}
			m.flipGridMode(c, "grid", mode, gap, cols, &nodeMotion, len(items), func(i int) any { return items[i].Name }, func(i int, box ui.Element, mode string) {
				it := items[i]
				box.Children(func() {
					var tile ui.Element
					if mode == "list" {
						tile, _ = m.nodeRow(c, it, it.Delay, i == 1, false)
					} else {
						tile, _ = m.nodeTile(c, "tile", it, it.Delay, i == 1, false)
					}
					tile.Grow(1)
				})
			})
		})
	}
	tt := ui.NewTester(view, 680, 420)
	tt.Frame()
	tt.Advance(time.Second)
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
	differing := func(a, b []byte) (n int) {
		for k := range a {
			if a[k] != b[k] {
				n++
			}
		}
		return
	}
	pix := func() []byte { return bytes.Clone(tt.Image().Pix) }
	shot("list")
	listPix := pix()
	list = false
	tt.Frame()
	// The change is measured in the frame it is asked in: the cells have not
	// jumped to their new places.
	// A cell may sit a fraction of a pixel off where it was, where it is
	// carried back from its new place; a jump would move far more.
	if d := differing(pix(), listPix); d > 30000 {
		t.Errorf("the list changed in the frame the grid was asked in: %d bytes", d)
	}
	for i, ms := range []int{16, 50, 50, 50, 60, 60} {
		tt.Advance(time.Duration(ms) * time.Millisecond)
		shot("to-grid-" + string(rune('a'+i)))
		if i == 1 && bytes.Equal(pix(), listPix) {
			t.Error("the list has not begun to morph")
		}
	}
	tt.Advance(time.Second)
	shot("grid")
	gridPix := pix()
	list = true
	tt.Frame()
	for i, ms := range []int{16, 50, 50, 50, 60, 60} {
		tt.Advance(time.Duration(ms) * time.Millisecond)
		shot("to-list-" + string(rune('a'+i)))
	}
	tt.Advance(time.Second)
	shot("list2")
	if d := differing(pix(), listPix); d > 0 {
		t.Errorf("the list is not as it was after the round trip: %d bytes", d)
	}
	// A grid asked for from the start is the one the morph ends in.
	list = false
	tt2 := ui.NewTester(view, 680, 420)
	tt2.Frame()
	tt2.Advance(time.Second)
	if !bytes.Equal(tt2.Image().Pix, gridPix) {
		t.Error("the grid the morph ended in differs from a grid laid out at once")
	}
}
