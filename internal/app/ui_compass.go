package app

import (
	_ "embed"
	"math"
	"regexp"
	"time"

	"github.com/egoist/mygo/ui"
)

// The app's mark, drawn as paths so that its parts can move: the wheel,
// the faceted star on it, the M, and the glints. assets/compass.svg is
// the icon's own drawing, in a box of 125.4.
//
//go:embed assets/compass.svg
var compassSVG string

const (
	compassBox = 125.4
	compassCX  = 63.17 // the star's hub
	compassCY  = 62.72
)

// compassTilt is how far the star's north point leans east in the icon,
// in degrees.
var compassTilt = float32(math.Atan2(69.28-compassCX, compassCY-11.98) * 180 / math.Pi)

type compassFacet struct {
	path  *ui.Path
	color ui.Color
}

type compassKite struct {
	clip   *ui.Path
	facets []compassFacet
}

type compassArt struct {
	ring, inlay *ui.Path
	star        *ui.Path
	kites       []compassKite
	m           *ui.Path
	glints      *ui.Path
}

var compass = parseCompass(compassSVG)

func parseCompass(svg string) *compassArt {
	a := &compassArt{}
	clips := map[string]*ui.Path{}
	for _, m := range regexp.MustCompile(`<clipPath id="(\w+)"><path d="([^"]+)"/></clipPath>`).FindAllStringSubmatch(svg, -1) {
		clips[m[1]] = ui.MustParsePath(m[2])
	}
	a.star = clips["star"]
	facet := regexp.MustCompile(`<path fill="(#\w+)" d="([^"]+)"/>`)
	for _, g := range regexp.MustCompile(`<g clip-path="url\(#(kite\d+)\)">(.*?)</g>`).FindAllStringSubmatch(svg, -1) {
		k := compassKite{clip: clips[g[1]]}
		for _, f := range facet.FindAllStringSubmatch(g[2], -1) {
			k.facets = append(k.facets, compassFacet{ui.MustParsePath(f[2]), ui.Hex(f[1])})
		}
		a.kites = append(a.kites, k)
	}
	rings := regexp.MustCompile(`<path fill="#\w+" fill-rule="evenodd" d="([^"]+)"/>`).FindAllStringSubmatch(svg, -1)
	a.ring = ui.MustParsePath(rings[0][1]).FillRule(ui.EvenOdd)
	a.inlay = ui.MustParsePath(rings[1][1]).FillRule(ui.EvenOdd)
	a.m = ui.MustParsePath(regexp.MustCompile(`<path d="([^"]+)" fill="#026b9c"`).FindStringSubmatch(svg)[1]).Join(ui.MiterJoin).MiterLimit(8)
	a.glints = ui.MustParsePath(regexp.MustCompile(`<path fill="#fbfa70" d="([^"]+)"/>`).FindStringSubmatch(svg)[1])
	return a
}

// compassLook is how the mark is drawn.
type compassLook struct {
	Ring   bool    // the wheel around the star
	Turn   float32 // degrees the star's north point is turned from up
	Alpha  float32 // 0 draws it fully: the zero look is the icon
	Glints float32 // the scale of the glints, 0 for none
	Glint  ui.Color
	// Mono draws the star alone in one color, its lit facets lighter, with
	// no M: the mark at rest.
	Mono ui.Color
	mono bool
}

// monoLook is the star alone in col.
func monoLook(col ui.Color, turn float32) compassLook {
	return compassLook{Mono: col, mono: true, Turn: turn}
}

// drawCompass draws the mark with its hub at (cx, cy), in a box of size.
func drawCompass(p *ui.Painter, cx, cy, size float32, l compassLook) {
	alpha := l.Alpha
	if alpha == 0 {
		alpha = 1
	}
	k := size / compassBox
	at := func(turn float32) ui.Transform {
		return ui.Translation(-compassCX, -compassCY).Then(ui.Rotation(turn)).Then(ui.Scaling(k, k)).Then(ui.Translation(cx, cy))
	}
	white := ui.Hex("#fafcfc").Alpha(alpha)
	if l.Ring {
		p.Transform(at(0), 0, 0, func() {
			p.FillPath(compass.ring, ui.Hex("#0283c0").Alpha(alpha))
			p.FillPath(compass.inlay, white)
		})
	}
	p.Transform(at(l.Turn-compassTilt), 0, 0, func() {
		p.ClipPath(compass.star, func() {
			for _, kite := range compass.kites {
				p.ClipPath(kite.clip, func() {
					for _, f := range kite.facets {
						col := f.color.Alpha(alpha)
						if l.mono {
							// The icon's white facets catch the light.
							k := float32(1)
							if f.color.R > 200 && f.color.G > 200 {
								k = 0.42
							} else if f.color.B > 200 {
								k = 0.78
							}
							col = l.Mono.Alpha(alpha * k)
						}
						p.FillPath(f.path, col)
					}
				})
			}
		})
	})
	if l.mono {
		return
	}
	// The M stays upright, as a letter does.
	p.Transform(at(0), 0, 0, func() {
		p.StrokePath(compass.m, 5.6, white)
		p.FillPath(compass.m, ui.Hex("#026b9c").Alpha(alpha))
	})
	if l.Glints > 0 {
		// Each glint grows from its own middle.
		for _, g := range [][3]float32{{106.1, 29.55, 1}, {17.6, 97.2, 1}, {38.35, 41.65, 1}} {
			gx, gy := cx+(g[0]-compassCX)*k, cy+(g[1]-compassCY)*k
			t := ui.Translation(-g[0], -g[1]).Then(ui.Scaling(k*l.Glints, k*l.Glints)).Then(ui.Translation(gx, gy))
			p.Transform(t, 0, 0, func() {
				p.ClipPath((&ui.Path{}).Circle(g[0], g[1], 14), func() { p.FillPath(compass.glints, l.Glint) })
			})
		}
	}
}

// drawSparkle draws the icon's four-pointed glint.
func drawSparkle(p *ui.Painter, x, y, r float32, c ui.Color) {
	if r <= 0.2 {
		return
	}
	path := (&ui.Path{}).MoveTo(x, y-r).QuadTo(x, y, x+r, y).QuadTo(x, y, x, y+r).QuadTo(x, y, x-r, y).QuadTo(x, y, x, y-r).Close()
	p.FillPath(path, c)
}

// compassSpin gives a compass mark a spin as b, its button, is pressed: a
// whole turn more each press, springing past north and back as a needle
// settles. It returns the degrees to turn the mark by.
func compassSpin(b ui.Element) float32 {
	turns := ui.Local(b, "spin", func() float32 { return 0 })
	if b.Clicked() {
		*turns += 360
	}
	return b.Spring("spin", *turns, ui.SpringOptions{Duration: 1100 * time.Millisecond, Bounce: 0.32})
}
