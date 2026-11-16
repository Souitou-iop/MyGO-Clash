package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/tools"
)

// The connectivity page's site cards, and 常用's grid, whose cards the
// pointer drags to another place.

// siteCardH is a site card's height: its name's line and its bars.
const siteCardH = 82

// siteCard fills box with a site: its logo and name, the dot of its
// service check where it has one, its star while the pointer is over it,
// its delay, and a bar each round.
func (m *mainUI) siteCard(c *ui.Context, st tools.Site, slowest int, box ui.Element) {
	s, p := &m.cty, m.pal
	r := s.rounds[st.ID]
	box.Padding(12).Gap(10).Radius(tileRadius).Background(p.Card).Tooltip(st.URL)
	hot := box.Hovered()
	box.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			m.logo(c, st.ID, st.Name, 22)
			ui.Text(c, st.Name).FontSize(13).FontWeight(550).SingleLine().Grow(1).Basis(0)
			m.siteUnlockDot(c, st.ID)
			star := ui.Box(c.Key("star-slot")).Shrink(0)
			star.Opacity(star.AnimateTo("show", b2f(hot), ui.Animation{Duration: 120 * time.Millisecond}))
			star.Children(func() { m.starButton(c, st.ID).Size(22, 22) })
			m.siteDelay(c, r)
		})
		m.siteBars(c, r, slowest, 26)
	})
}

// siteUnlockDot is the result of the service check of a site, a dot of
// its color, its words in its tip; nothing for a site with no check.
func (m *mainUI) siteUnlockDot(c *ui.Context, site string) {
	s, p := &m.cty, m.pal
	for _, u := range s.unlocks {
		if u.Site != site {
			continue
		}
		r, ok := s.results[u.ID]
		if !ok || s.checking[u.ID] {
			return
		}
		col := p.BorderStrong
		switch r.Status {
		case "yes":
			col = p.Success
		case "partial":
			col = p.Warning
		case "no":
			col = p.Danger
		}
		tip := m.unlockName(u) + " · " + m.T("unlock."+r.Status)
		if d := unlockDetailText(r.Detail); d != "" {
			tip += " · " + d
		}
		if r.Region != "" && m.reveal() {
			tip += " · " + r.Region
		}
		ui.Box(c.Key("unlock-"+u.ID)).Size(7, 7).Radius(4).Background(col).Shrink(0).Tooltip(tip).Transition(checkIn)
		return
	}
}

// favoriteGrid lays out 常用's cards in slots, each gliding to its own on
// a spring. A card dragged (as soon as it moves, or once held) lifts and
// follows the pointer, the cards it passes making way, a slot along; let
// go, it springs into the slot it is over and the order is kept.
func (m *mainUI) favoriteGrid(c *ui.Context, of []tools.Site, card func(tools.Site, ui.Element)) {
	s := &m.cty
	const gap, least = 12, 180
	n := len(of)
	wrap := ui.Box(c.Key("fav-grid"))
	w := wrap.LayoutBounds().W
	cols := max(1, int((w+gap)/(least+gap)))
	rows := (n + cols - 1) / cols
	wrap.Height(float32(rows)*(siteCardH+gap) - gap)
	if w <= 0 {
		return // the slots wait for the width, so that no card glides in from none
	}
	cellW := (w - float32(cols-1)*gap) / float32(cols)
	slot := func(i int) (float32, float32) {
		return float32(i%cols) * (cellW + gap), float32(i/cols) * (siteCardH + gap)
	}
	lifted := s.favPress != "" && s.favLift
	target := func() int {
		x, y := slot(s.favFrom)
		col := int(math.Floor(float64((x + s.favDrag.X + cellW/2) / (cellW + gap))))
		row := int(math.Floor(float64((y + s.favDrag.Y + siteCardH/2) / (siteCardH + gap))))
		col, row = max(0, min(cols-1, col)), max(0, min(rows-1, row))
		return min(n-1, row*cols+col)
	}
	commit := false
	wrap.Children(func() {
		for i, st := range of {
			box := ui.Column(c.Key(st.ID)).Absolute().Width(cellW).Height(siteCardH)
			// Held, wherever the pointer is: Pressed is false while it is
			// outside the card, which a quick drag outruns.
			dx, dy, pressed := box.Dragged()
			now := c.Now()
			switch {
			case pressed && s.favPress == "":
				s.favPress, s.favPressAt, s.favFrom, s.favDrag, s.favLift = st.ID, now, i, ui.Point{X: dx, Y: dy}, false
			case pressed && s.favPress == st.ID:
				s.favDrag.X += dx
				s.favDrag.Y += dy
			}
			mine := s.favPress == st.ID
			if mine && pressed && !s.favLift {
				if math.Hypot(float64(s.favDrag.X), float64(s.favDrag.Y)) >= 4 || now.Sub(s.favPressAt) >= regionHold {
					s.favLift, lifted = true, true
				} else {
					c.After(16 * time.Millisecond)
				}
			}
			dragged := mine && s.favLift
			released := mine && !pressed
			var x, y float32
			switch {
			case dragged && !released:
				sx, sy := slot(i)
				maxX, maxY := float32(cols-1)*(cellW+gap), float32(rows-1)*(siteCardH+gap)
				x = box.Jump("x", rubberBand(sx+s.favDrag.X, 0, maxX))
				y = box.Jump("y", rubberBand(sy+s.favDrag.Y, 0, maxY))
			case dragged:
				tx, ty := slot(target())
				x, y = box.Spring("x", tx, regionSettle), box.Spring("y", ty, regionSettle)
			default:
				at := i
				if lifted && !mine {
					from, to := s.favFrom, target()
					switch {
					case from < i && i <= to:
						at--
					case to <= i && i < from:
						at++
					}
				}
				tx, ty := slot(at)
				x, y = box.Spring("x", tx, regionMakeWay), box.Spring("y", ty, regionMakeWay)
			}
			box.Left(x).Top(y)
			if released {
				commit = commit || dragged
				if !dragged {
					s.favPress = ""
				}
			}
			// Lifted, it grows a little and casts a shadow.
			up := box.Spring("lift", b2f(dragged && !released), regionLiftSpring)
			if up > 0.001 {
				box.Scale(1+0.03*up, 1+0.03*up).ZIndex(10).Shadow(0, 6*up, 18*up, 0, ui.RGBA(0, 0, 0, 0.22*up))
			}
			cursor := ui.CursorGrab
			if dragged && !released {
				cursor = ui.CursorGrabbing
			}
			box.Cursor(cursor)
			card(st, box)
		}
	})
	if commit {
		m.moveFavorite(s.favFrom, target())
		s.favPress, s.favLift, s.favDrag = "", false, ui.Point{}
	}
}

// lazyCards are the cards of a grid at most this many are built whole; of a
// larger one, a screenful and a margin are, the rest being plates. A
// category of 248 sites builds ten times the elements of one of 30, every
// frame its sites are tested or it is scrolled, which most are out of view
// for.
const lazyCards = 48

// lazyMargin is how many cards beyond those in view a large grid builds
// whole, which a scroll shows before the frame after it builds them.
const lazyMargin = 18

// siteGrid is the sites of a category as cards in a grid. A large one
// builds the cards in view, as the frame before had them, and a margin of
// them; any other is a plate, a card's box and color, which the frame after
// it comes into view builds whole.
func (m *mainUI) siteGrid(c *ui.Context, key any, n int, id func(i int) any, card func(i int, box ui.Element)) {
	s := &m.cty
	if n <= lazyCards {
		m.cardGridOf(c, key, 180, n, id, func(i int) { card(i, ui.Column(c).Grow(1)) })
		return
	}
	lo, hi := s.lazyLo, s.lazyHi
	if s.lazyKey != key {
		lo, hi = 0, 0 // the first screenful, until the grid has been seen
		s.lazyKey = key
	}
	seen := [2]int{n, -1}
	m.flipGrid(c, key, 12, func(w float32) int { return int((w + 12) / (180 + 12)) }, &reflowOut, n, id, func(i int, box ui.Element) {
		if box.InView() {
			seen[0], seen[1] = min(seen[0], i), max(seen[1], i)
		}
		if i >= lo-lazyMargin && i <= hi+lazyMargin+lazyCards/2 {
			box.Children(func() { card(i, ui.Column(c).Grow(1)) })
			return
		}
		box.Children(func() {
			ui.Box(c).Grow(1).Height(siteCardH).Radius(tileRadius).Background(m.pal.Card)
		})
	})
	if seen[1] >= 0 {
		s.lazyLo, s.lazyHi = seen[0], seen[1]
	}
}
