package app

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// palette is the main window's colors: mygo/ui's theme for its widgets,
// and the steps of gray and the soft fills the app's own elements take.
// Neutral grays and one solid color, MyGO!!!!!'s blue (#0B88BB as the
// band prints it), lighter on the dark theme to stay readable; the band's
// flash yellow, deepened on the light theme, draws what goes up. No
// gradients and no glows: surfaces part by a step of gray and a hairline,
// and color is kept for what is active.
type palette struct {
	*ui.Theme
	Sidebar      ui.Color // behind the navigation
	Card         ui.Color // cards and panels, a step above the window
	Surface2     ui.Color // tiles and fields on a card
	Surface3     ui.Color
	BorderStrong ui.Color
	Text2        ui.Color
	Faint        ui.Color
	Accent2      ui.Color // what goes up
	AccentFg     ui.Color // the accent as text
	AccentSoft   ui.Color
	AccentSofter ui.Color
	SuccessSoft  ui.Color
	WarningSoft  ui.Color
	DangerSoft   ui.Color
	Spark        ui.Color // the glints of the icon
	Dark         bool     // the dark theme
}

type paletteKey struct {
	dark, oled   bool
	accent, font string
}

var palettes = map[paletteKey]*palette{}

// mainPalette is the palette of the theme, with the user's accent if set.
// On the main thread.
func mainPalette(dark, oled bool, accent, font string) *palette {
	k := paletteKey{dark, oled && dark, accent, font}
	if p, ok := palettes[k]; ok {
		return p
	}
	p := &palette{Dark: dark}
	var t *ui.Theme
	if dark {
		t = ui.DarkTheme()
		t.Background, p.Sidebar, p.Card = ui.Hex("#1a1a1d"), ui.Hex("#161618"), ui.Hex("#222226")
		p.Surface2, p.Surface3 = ui.Hex("#28282d"), ui.Hex("#323238")
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#2b2b30"), ui.Hex("#34343a"), ui.Hex("#3c3c43")
		t.Border, p.BorderStrong = ui.Hex("#313137"), ui.Hex("#45454d")
		t.Text, p.Text2, t.TextMuted, p.Faint = ui.Hex("#ececef"), ui.Hex("#c6c6cc"), ui.Hex("#8e8e97"), ui.Hex("#5f5f68")
		t.Accent, t.AccentText = ui.Hex("#46acdb"), ui.Hex("#04131b")
		p.Accent2, p.Spark = ui.Hex("#f2e46e"), ui.Hex("#fbfa70")
		t.Success, t.Warning, t.Danger = ui.Hex("#5cc985"), ui.Hex("#f0c766"), ui.Hex("#ff6b61")
		if oled {
			t.Background, p.Sidebar, p.Card = ui.Hex("#000000"), ui.Hex("#000000"), ui.Hex("#0a0a0b")
			p.Surface2, p.Surface3 = ui.Hex("#101012"), ui.Hex("#19191c")
			t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#131315"), ui.Hex("#1b1b1e"), ui.Hex("#232327")
			t.Border, p.BorderStrong = ui.Hex("#1c1c1f"), ui.Hex("#2c2c31")
		}
	} else {
		t = ui.LightTheme()
		t.Background, p.Sidebar, p.Card = ui.Hex("#f4f4f5"), ui.Hex("#ededef"), ui.Hex("#ffffff")
		p.Surface2, p.Surface3 = ui.Hex("#f6f6f7"), ui.Hex("#e9e9ec")
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#f1f1f3"), ui.Hex("#e8e8eb"), ui.Hex("#dddde1")
		t.Border, p.BorderStrong = ui.Hex("#e2e2e6"), ui.Hex("#cfcfd5")
		t.Text, p.Text2, t.TextMuted, p.Faint = ui.Hex("#18181b"), ui.Hex("#3c3c43"), ui.Hex("#6c6c75"), ui.Hex("#a0a0a8")
		t.Accent, t.AccentText = ui.Hex("#0b88bb"), ui.Hex("#ffffff")
		p.Accent2, p.Spark = ui.Hex("#b8860b"), ui.Hex("#e3c21a")
		t.Success, t.Warning, t.Danger = ui.Hex("#1f9254"), ui.Hex("#b26b00"), ui.Hex("#d1333f")
	}
	if c, ok := parseHex(accent); ok {
		t.Accent, t.AccentText = c, textOn(c)
	}
	shade := ui.Hex("#000000")
	if dark {
		shade = ui.Hex("#ffffff")
	}
	t.AccentHover, t.AccentPressed = t.Accent.Mix(shade, 0.1), t.Accent.Mix(shade, 0.2)
	t.InactiveAccent = t.Accent
	t.Selection, t.Focus = t.Accent.Alpha(0.16), t.Accent.Alpha(0.5)
	t.Radius, t.FontSize = 7, 13
	if font != "" {
		t.Font = font
	}
	if dark {
		p.AccentFg = t.Accent.Mix(ui.Hex("#ffffff"), 0.12)
		p.AccentSoft, p.AccentSofter = t.Accent.Alpha(0.18), t.Accent.Alpha(0.10)
	} else {
		p.AccentFg = t.Accent.Mix(ui.Hex("#000000"), 0.16)
		p.AccentSoft, p.AccentSofter = t.Accent.Alpha(0.12), t.Accent.Alpha(0.07)
	}
	p.SuccessSoft, p.WarningSoft, p.DangerSoft = t.Success.Alpha(0.13), t.Warning.Alpha(0.15), t.Danger.Alpha(0.12)
	// Tips wait for the pointer to rest, as the pages had them.
	t.TooltipDelay = 350 * time.Millisecond
	p.Theme = t
	palettes[k] = p
	return p
}

// parseHex reads #rgb or #rrggbb.
func parseHex(s string) (ui.Color, bool) {
	h := strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(h) == 3 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 {
		return ui.Color{}, false
	}
	if _, err := strconv.ParseUint(h, 16, 32); err != nil {
		return ui.Color{}, false
	}
	return ui.Hex("#" + h), true
}

// tileRadius rounds the tiles the window is built of.
const tileRadius = 18

// textOn picks the text color that reads best on a background: white, or
// the night's navy.
func textOn(c ui.Color) ui.Color {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	l := 0.2126*lin(c.R) + 0.7152*lin(c.G) + 0.0722*lin(c.B)
	if 1.05/(l+0.05) >= (l+0.05)/0.0567 {
		return ui.Hex("#ffffff")
	}
	return ui.Hex("#08131f")
}
