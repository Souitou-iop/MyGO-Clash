package app

import (
	"fmt"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// Lucide icons that move as what holds them is hovered or becomes active,
// as the pages had them (the motions of lucide-animated and
// movingicons.dev): each plays once and ends where it began. An icon is
// drawn part by part, its parts in the units of its 24-unit box.

// iconKind is how a motion goes.
type iconKind int

const (
	iconNudge  iconKind = iota // out to a pose by 45% of the time, and back
	iconTurn                   // around by turn degrees
	iconWiggle                 // a few swings either way
	iconDraw                   // the stroke drawing itself
	iconFlash                  // blinking twice
)

// iconMotion is one motion of an icon, of some of its parts or all of it.
type iconMotion struct {
	kind iconKind
	dur  time.Duration
	// parts are the icon's elements it moves, counted from 1; none moves
	// the whole icon, whose distances are DIPs rather than icon units.
	parts []int
	// delays hold some parts back, by part.
	delays map[int]time.Duration
	// The pose a nudge goes to: a move, a turn, and a scale (0 is 1).
	dx, dy, rot, sx, sy float32
	// turn is how far a turn goes.
	turn float32
	ease ui.Easing
}

var (
	iconEase = ui.CubicBezier(0.4, 0, 0.2, 1)
	ms       = func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	nudge    = func(parts ...int) iconMotion { return iconMotion{kind: iconNudge, dur: ms(550), parts: parts} }
	stagger  = func(m iconMotion, d map[int]time.Duration) iconMotion { m.delays = d; return m }
)

func (m iconMotion) move(dx, dy float32) iconMotion  { m.dx, m.dy = dx, dy; return m }
func (m iconMotion) scale(sx, sy float32) iconMotion { m.sx, m.sy = sx, sy; return m }
func (m iconMotion) rotate(deg float32) iconMotion   { m.rot = deg; return m }
func turn(deg float32, d time.Duration) iconMotion {
	return iconMotion{kind: iconTurn, dur: d, turn: deg}
}
func wiggle() iconMotion { return iconMotion{kind: iconWiggle, dur: ms(600)} }
func drawIn(d time.Duration, parts ...int) iconMotion {
	return iconMotion{kind: iconDraw, dur: d, parts: parts}
}
func drawEase(m iconMotion, e ui.Easing) iconMotion { m.ease = e; return m }
func flash() iconMotion                             { return iconMotion{kind: iconFlash, dur: ms(600)} }

// iconMotions are the motions of the icons that have them.
var iconMotions = map[*ui.SVG][]iconMotion{
	// The sidebar.
	icLayoutDashboard: {stagger(nudge().scale(0.78, 0.78), map[int]time.Duration{2: ms(60), 3: ms(120), 4: ms(180)})},
	icGlobe2:          {stagger(drawIn(ms(700), 1, 2, 3), map[int]time.Duration{2: ms(80), 3: ms(160)})},
	icFileStack:       {nudge(3).move(2.5, -2.5), nudge(1).move(-1.5, 1.5)},
	icCable:           {nudge(1, 2, 4).move(0, 2), nudge(5, 6, 7).move(0, -2)},
	icListFilter: {
		nudge(1).scale(0.55, 1),
		stagger(nudge(2).scale(1.3, 1), map[int]time.Duration{2: ms(50)}),
		stagger(nudge(3).scale(2, 1), map[int]time.Duration{3: ms(100)}),
	},
	icScrollText: {stagger(drawEase(drawIn(ms(450), 1, 2), ui.EaseOut), map[int]time.Duration{1: ms(150)})},
	icWaypoints:  {stagger(nudge(4, 5, 6, 7).scale(1.4, 1.4), map[int]time.Duration{6: ms(70), 4: ms(140), 7: ms(210)})},
	icActivity:   {drawIn(ms(800), 1)},
	icSettings:   {turn(180, ms(600))},
	// Settings.
	icSettings2:         {wiggle()},
	icWrench:            {wiggle()},
	icNetwork:           {nudge(3).move(0, -3)},
	icSlidersHorizontal: {nudge(3).move(-4, 0), nudge(4).move(-3, 0), nudge(8).move(4, 0)},
	icCloud:             {nudge().move(2, 0)},
	// Buttons.
	icRefreshCw:       {turn(180, ms(600))},
	icHistory:         {stagger(drawEase(drawIn(ms(450), 1, 3), ui.EaseOut), map[int]time.Duration{3: ms(160)})},
	icPlus:            {turn(90, ms(350))},
	icX:               {turn(90, ms(350))},
	icCopy:            {nudge(1).move(2.5, 2.5)},
	icTrash2:          {nudge(4, 5).move(0, -3)},
	icExternalLink:    {nudge(1, 2).move(2.5, -2.5)},
	icDownload:        {nudge(1, 3).move(0, 3)},
	icArrowDownToLine: {nudge(1, 2).move(0, 3)},
	icLogOut:          {nudge(1, 2).move(2.5, 0)},
	icPanelLeftClose:  {nudge(3).move(-2, 0)},
	icPanelLeftOpen:   {nudge(3).move(2, 0)},
	icPlay:            {nudge().move(1.5, 0)},
	icPause:           {stagger(nudge(1, 2).scale(1, 0.7), map[int]time.Duration{1: ms(80)})},
	icZap:             {flash()},
	icStethoscope:     {nudge(5).scale(1.6, 1.6)},
	icDices:           {wiggle()},
	icSearch:          {nudge().move(-1.5, -1.5)},
	icFolderOpen:      {nudge().move(0, -1.5)},
	icPin:             {nudge().rotate(-20)},
	icPinOff:          {nudge().rotate(-20)},
}

// iconSources are the elements of the icons, by icon (panelIcon).
var (
	iconSourcesMu sync.Mutex
	iconSources   = map[*ui.SVG]string{}
	iconPartsMu   sync.Mutex
	iconParts     = map[*ui.SVG][]iconPart{}
)

// iconPart is an element of an icon, and its center, around which it
// turns and scales.
type iconPart struct {
	path   *ui.Path
	cx, cy float32
	// dot is a part of no length, as the pips of a die, which a round cap
	// draws as a dot.
	dot bool
	// drawn is the part's path for a motion that draws it, trimmed.
	drawn *ui.Path
}

var (
	elemRe = regexp.MustCompile(`<(\w+)([^>]*)/>`)
	attrRe = regexp.MustCompile(`([\w-]+)="([^"]*)"`)
)

// partsOf parses an icon's elements, once.
func partsOf(icon *ui.SVG) []iconPart {
	iconPartsMu.Lock()
	defer iconPartsMu.Unlock()
	if ps, ok := iconParts[icon]; ok {
		return ps
	}
	iconSourcesMu.Lock()
	src := iconSources[icon]
	iconSourcesMu.Unlock()
	var ps []iconPart
	for _, m := range elemRe.FindAllStringSubmatch(src, -1) {
		a := map[string]float32{}
		d := ""
		for _, kv := range attrRe.FindAllStringSubmatch(m[2], -1) {
			if kv[1] == "d" || kv[1] == "points" {
				d = kv[2]
				continue
			}
			f, _ := strconv.ParseFloat(kv[2], 32)
			a[kv[1]] = float32(f)
		}
		var p *ui.Path
		switch m[1] {
		case "path":
			p = ui.MustParsePath(d)
		case "circle":
			p = (&ui.Path{}).Circle(a["cx"], a["cy"], a["r"])
		case "line":
			p = (&ui.Path{}).MoveTo(a["x1"], a["y1"]).LineTo(a["x2"], a["y2"])
		case "polyline", "polygon":
			p = ui.MustParsePath("M" + d)
			if m[1] == "polygon" {
				p.Close()
			}
		case "rect":
			p = rectPath(a["x"], a["y"], a["width"], a["height"], a["rx"])
		default:
			continue
		}
		p.Cap(ui.RoundCap).Join(ui.RoundJoin)
		b := p.Bounds()
		part := iconPart{path: p, cx: b.X + b.W/2, cy: b.Y + b.H/2, dot: b.W < 0.05 && b.H < 0.05}
		if m[1] == "path" {
			part.drawn = ui.MustParsePath(d).Cap(ui.RoundCap).Join(ui.RoundJoin)
		}
		ps = append(ps, part)
	}
	iconParts[icon] = ps
	return ps
}

// rectPath is a rectangle with round corners of radius r.
func rectPath(x, y, w, h, r float32) *ui.Path {
	if r <= 0 {
		return ui.MustParsePath(fmt.Sprintf("M%g %gh%gv%gh%gZ", x, y, w, h, -w))
	}
	r = min(r, w/2, h/2)
	return ui.MustParsePath(fmt.Sprintf("M%g %gH%gA%g %g 0 0 1 %g %gV%gA%g %g 0 0 1 %g %gH%gA%g %g 0 0 1 %g %gV%gA%g %g 0 0 1 %g %gZ",
		x+r, y, x+w-r, r, r, x+w, y+r, y+h-r, r, r, x+w-r, y+h, x+r, r, r, x, y+h-r, y+r, r, r, x+r, y))
}

// iconState is when an icon's motion last started.
type iconState struct {
	hot   bool
	start time.Time
}

// icon draws an icon of size DIPs in col, moving as hot turns on: the
// pointer over what holds it, or that becoming active. Icons without a
// motion are ui.Icon.
func (m *mainUI) icon(c *ui.Context, icon *ui.SVG, size float32, col ui.Color, hot bool) ui.Element {
	motions, ok := iconMotions[icon]
	if !ok {
		return ui.Icon(c, icon).FontSize(size).TextColor(col)
	}
	box := ui.Box(c).Size(size, size).Shrink(0)
	st := ui.Local(box, "icon", func() *iconState { return &iconState{hot: hot} })
	if hot && !(*st).hot {
		(*st).start = c.Now()
	}
	(*st).hot = hot
	start := (*st).start
	parts := partsOf(icon)
	return box.Draw(func(pt *ui.Painter, r ui.Rect) {
		t := pt.Now().Sub(start)
		moving := !start.IsZero() && !pt.Preferences().ReduceMotion
		k := r.W / 24
		whole := ui.Transform{}
		alpha := float32(1)
		wholeSet := false
		trims := map[int]float32{}
		poses := map[int]ui.Transform{}
		if moving {
			done := true
			for _, mo := range motions {
				if len(mo.parts) == 0 {
					p, a, live := mo.at(t, 0, k)
					done = done && !live
					whole, wholeSet = p, true
					alpha = a
					continue
				}
				for _, i := range mo.parts {
					if mo.kind == iconDraw {
						f, live := mo.drawn(t, mo.delays[i])
						done = done && !live
						trims[i] = f
						continue
					}
					p, _, live := mo.at(t, mo.delays[i], 1)
					done = done && !live
					poses[i] = p
				}
			}
			if !done {
				pt.AnimationFrame()
			}
		}
		ink := col
		if alpha < 1 {
			ink = col.Alpha(float32(col.A) / 255 * alpha)
		}
		paint := func() {
			for i, part := range parts {
				n := i + 1
				p := part.path
				if f, ok := trims[n]; ok && part.drawn != nil {
					p = part.drawn.Trim(0, f)
				}
				stroke := func() {
					if part.dot {
						pt.FillPath((&ui.Path{}).Circle(part.cx, part.cy, 1), ink)
						return
					}
					pt.StrokePath(p, 2, ink)
				}
				if pose, ok := poses[n]; ok {
					pt.Transform(pose, part.cx, part.cy, stroke)
				} else {
					stroke()
				}
			}
		}
		pt.Transform(ui.Scaling(k, k).Then(ui.Translation(r.X, r.Y)), 0, 0, func() {
			if wholeSet {
				pt.Transform(whole, 12, 12, paint)
				return
			}
			paint()
		})
	})
}

// at is the transform and opacity of a motion t into it, held back by
// delay; px is how many DIPs an icon unit is, for the moves of a whole
// icon, which are in DIPs. live reports it is still moving.
func (mo iconMotion) at(t, delay time.Duration, px float32) (ui.Transform, float32, bool) {
	t -= delay
	if t <= 0 {
		return ui.Transform{}, 1, true
	}
	f := float32(t) / float32(mo.dur)
	if f >= 1 {
		return ui.Transform{}, 1, false
	}
	switch mo.kind {
	case iconTurn:
		return ui.Rotation(mo.turn * iconEase(f)), 1, true
	case iconWiggle:
		return ui.Rotation(keyframes(f, []float32{0, 0.25, 0.55, 0.8, 1}, []float32{0, -12, 9, -4, 0}, ui.EaseInOut)), 1, true
	case iconFlash:
		return ui.Transform{}, keyframes(f, []float32{0, 0.2, 0.4, 0.6, 0.8, 1}, []float32{1, 0.3, 1, 0.3, 1, 1}, nil), true
	}
	// A nudge: out by 45%, back by the end, each half eased.
	var e float32
	if f < 0.45 {
		e = iconEase(f / 0.45)
	} else {
		e = 1 - iconEase((f-0.45)/0.55)
	}
	sx, sy := mo.sx, mo.sy
	if sx == 0 {
		sx = 1
	}
	if sy == 0 {
		sy = 1
	}
	tr := ui.Scaling(1+(sx-1)*e, 1+(sy-1)*e).Then(ui.Rotation(mo.rot * e)).Then(ui.Translation(mo.dx*e/px, mo.dy*e/px))
	return tr, 1, true
}

// drawn is how much of a part a drawing motion shows.
func (mo iconMotion) drawn(t, delay time.Duration) (float32, bool) {
	t -= delay
	if t <= 0 {
		return 0, true
	}
	f := float32(t) / float32(mo.dur)
	if f >= 1 {
		return 1, false
	}
	e := mo.ease
	if e == nil {
		e = iconEase
	}
	return e(f), true
}

// keyframes interpolates values at the fractions at, easing each step.
func keyframes(f float32, at, values []float32, ease ui.Easing) float32 {
	for i := 1; i < len(at); i++ {
		if f <= at[i] {
			g := (f - at[i-1]) / (at[i] - at[i-1])
			if ease != nil {
				g = ease(g)
			}
			return values[i-1] + (values[i]-values[i-1])*g
		}
	}
	return values[len(values)-1]
}
