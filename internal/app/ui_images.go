package app

import (
	"embed"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// Every country's flag (flag-icons, MIT) and the logos of the sites the
// app tests, by name. The system's emoji flags are not used: Windows has
// no flag glyphs, and Linux only has them when its emoji font does.
//
//go:embed assets/flags/*.svg assets/logos/*.svg assets/logos/*.png
var imageFS embed.FS

var (
	imageMu sync.Mutex
	images  = map[string]*ui.SVG{}
	bitmaps = map[string]*ui.Bitmap{}
)

// bitmapImage decodes an embedded bitmap once; nil when there is none.
func bitmapImage(path string) *ui.Bitmap {
	imageMu.Lock()
	defer imageMu.Unlock()
	if b, ok := bitmaps[path]; ok {
		return b
	}
	var b *ui.Bitmap
	if data, err := imageFS.ReadFile(path); err == nil {
		b, _ = ui.DecodeBitmap(data)
	}
	bitmaps[path] = b
	return b
}

// logoPlates are the logos drawn in one dark or light ink that the
// theme of that shade would hide: they sit on a plate of the other
// shade, as an app icon does.
var logoPlates = map[string]bool{ // true: dark ink, plated in the dark theme
	"artmotion": true, "dailymotion": true, "digitalocean": true, "guardian": true, "heroku": true,
	"kleinanzeigen": true, "manus": true, "midjourney": true, "musixmatch": true, "phonepe": true,
	"physicswallah": true, "tidal": true,
	"skillshare": false, "western-union": false,
}

// svgImage parses an embedded SVG once; nil when there is none, or MyGo
// cannot read it.
func svgImage(path string) *ui.SVG {
	imageMu.Lock()
	defer imageMu.Unlock()
	if s, ok := images[path]; ok {
		return s
	}
	var s *ui.SVG
	if data, err := imageFS.ReadFile(path); err == nil {
		s, _ = ui.ParseSVG(data)
	}
	images[path] = s
	return s
}

// flag shows a country's flag from the bundled set. While masked it is a
// neutral block: a blurred flag would still tell its colors.
func (m *mainUI) flag(c *ui.Context, code string, masked bool) {
	box := ui.Box(c).Size(26, 19.5).Radius(3).Clip().Shrink(0).Background(m.pal.Surface3).AlignItems(ui.Center).Justify(ui.Center)
	if len(code) != 2 {
		return
	}
	s := svgImage("assets/flags/" + strings.ToLower(code) + ".svg")
	if s == nil {
		return
	}
	// Hidden, a globe stands in for the country, the two crossfading.
	box.Children(func() {
		if masked {
			ui.Icon(c.Key("globe"), icGlobe).FontSize(12).TextColor(m.pal.Faint).Transition(flagFade)
			return
		}
		ui.Image(c.Key("flag-"+code), s).Fill().Transition(flagFade)
	})
}

// logo shows a site's logo, or its initial when the app has none. Most
// are vectors; the few brands with none are their sites' own icons.
func (m *mainUI) logo(c *ui.Context, id, name string, size float32) {
	var src ui.ImageSource
	if s := svgImage("assets/logos/" + id + ".svg"); s != nil {
		src = s
	} else if b := bitmapImage("assets/logos/" + id + ".png"); b != nil {
		src = b
	}
	if src != nil {
		if ink, ok := logoPlates[id]; ok && ink == m.pal.Dark {
			plate := ui.Hex("#f4f4f5")
			if !ink {
				plate = ui.Hex("#2b2b30")
			}
			ui.Box(c).Size(size, size).Radius(size / 4.5).Background(plate).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
				ui.Image(c, src).Size(size*0.72, size*0.72)
			})
			return
		}
		ui.Image(c, src).Size(size, size).Shrink(0)
		return
	}
	initial := "?"
	if r := []rune(name); len(r) > 0 {
		initial = strings.ToUpper(string(r[0]))
	}
	ui.Box(c).Size(size, size).Radius(size / 4).Background(m.pal.Surface3).AlignItems(ui.Center).Justify(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, initial).FontSize(size * 0.6).FontWeight(700).TextColor(m.pal.TextMuted)
	})
}
