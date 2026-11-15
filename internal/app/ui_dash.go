package app

import (
	"cmp"
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/usage"
)

// The home page is a control surface: tiles filled a step above the
// window, parted by the gaps between them and pressed like keys. The
// app's compass is the switch that takes traffic over; beside it, the node
// the traffic leaves through, the mode, and the speed now; below, the
// other routes, who moves the most, and today's traffic. The cards the
// user chooses wait under the board.

// dashState is what the home page keeps besides homePage.
type dashState struct {
	today    int64
	month    int64
	week     []int64 // the traffic of the last seven days, today last
	totalsAt time.Time
	autoBusy bool
	method   string // how the compass takes traffic over: "sys" or "tun"
	hotApp   string // the app under the pointer, lit in the share

	// The sheet a tile opens into: the tile asked for (none while it
	// closes), and the one it grows from and shrinks back to, kept while
	// it closes.
	sheet, sheetKey string
	tiles           map[string]ui.Rect // where each tile is, as last laid out
	report          *usage.StatsReport // today's traffic, for its sheet
	reportAt        time.Time

	// open goes to 1 as the sheet opens, full as it grows on into the
	// page of its tile; toPlace and toTab are that page, while it grows.
	open, full     tween
	toPlace, toTab string
	backing        bool // back from that page: shrinking to the sheet, then into the tile
	backStart      bool
	growAfter      bool // grow on into the page once open
	leftFull       bool // the page grown into took the home page's place
}

// flagOf finds the country of a node in its name: an emoji flag, or a code
// as "HK" or "US" standing alone.
func flagOf(name string) string {
	rs := []rune(name)
	for i := 0; i+1 < len(rs); i++ {
		if rs[i] >= 0x1F1E6 && rs[i] <= 0x1F1FF && rs[i+1] >= 0x1F1E6 && rs[i+1] <= 0x1F1FF {
			return string([]rune{'A' + (rs[i] - 0x1F1E6), 'A' + (rs[i+1] - 0x1F1E6)})
		}
	}
	for _, f := range strings.FieldsFunc(strings.ToUpper(name), func(r rune) bool { return !(r >= 'A' && r <= 'Z') }) {
		if _, ok := countryCenters[f]; ok && len(f) == 2 {
			return f
		}
	}
	return ""
}

// stripFlag takes an emoji flag off the front of a name.
func stripFlag(name string) string {
	rs := []rune(strings.TrimSpace(name))
	if len(rs) >= 2 && rs[0] >= 0x1F1E6 && rs[0] <= 0x1F1FF && rs[1] >= 0x1F1E6 && rs[1] <= 0x1F1FF {
		return strings.TrimSpace(string(rs[2:]))
	}
	return string(rs)
}

var (
	// tileEnter brings a tile of the board in as the page opens.
	tileEnter = ui.Pose{Y: 10, Scale: 0.98, Fade: 1}
)

// tile builds the tile key of the board in place, which holds the tile's
// place and brings it in n tiles after the first. The tile is filled a
// step above the window, lightens under the pointer and gives under a
// press, which opens its sheet out of it. fn fills it.
func (m *mainUI) tile(c *ui.Context, key string, place ui.Element, n int, fn func(c *ui.Context, t ui.Element)) ui.Element {
	p := m.pal
	place.MinWidth(0).MinHeight(0)
	m.noteTile(key, place)
	enterPose(c, place, tileEnter, ui.Animation{Duration: 460 * time.Millisecond, Ease: easeOut, Delay: time.Duration(n) * 40 * time.Millisecond})
	var t ui.Element
	place.Children(func() {
		t = ui.ButtonBase(c.Key("tile")).Column()
		bg := p.Card
		if t.Hovered() {
			bg = p.Surface2
		}
		k := t.AnimateTo("press", map[bool]float32{true: 0.985, false: 1}[t.Pressed()], pressSpring)
		t.Scale(k, k).Background(t.AnimateColor("bg", bg, ui.Animation{Duration: 160 * time.Millisecond}))
		t.Grow(1).Basis(0).MinWidth(0).MinHeight(0).Radius(tileRadius).Padding(20, 22).AlignItems(ui.Stretch).Clip()
		fn(c, t)
		if t.Clicked() {
			m.openSheet(key)
		}
	})
	return place
}

func (m *mainUI) homeView(c *ui.Context) {
	h, d, a := &m.home, &m.dash, m.a
	m.loadDash()
	open, full := m.sheetMotions(c)
	// The stage holds the page and, over it, the sheet a tile opens into,
	// which may grow on into a page of its own.
	stage := ui.Column(c.Key("stage")).Grow(1).Basis(0).MinHeight(0).Clip()
	sb := stage.LayoutBounds()
	stage.Children(func() {
		m.header(c, locales.NavHome(), func() {
			if m.button(c, "customize", locales.HomeCustomize(), icLayoutGrid, "ghost").Clicked() {
				h.editing, h.draft = true, slices.Clone(m.st.UI.HomeCards)
			}
		})
		scroll := ui.Scroll(c.Key("home")).TrackScroll(m.keepScroll("home")).Grow(1).Padding(0, 22, 22).Gap(12)
		view := scroll.LayoutBounds()
		scroll.Children(func() {
			if m.state.ConfigError != "" {
				m.banner(c, "danger", locales.HomeConfigError()+" "+m.state.ConfigError, func() {
					if h.retrying {
						ui.Spinner(c)
					} else if m.button(c, "retry", locales.ProfilesRetry(), nil, "").Clicked() {
						h.retrying = true
						m.run(locales.HomeConfigError(), func() error { return Core{a}.Reapply(context.Background()) }, func(error) { h.retrying = false })
					}
				})
			}
			// The board: five tiles whose places the code works out for
			// the width, held absolutely, so that the tree stays one tree
			// at every width and a tile is never rebuilt as the layout
			// changes; across a change of layout every tile glides, box and
			// all, to its new place.
			board := ui.Box(c.Key("board")).Shrink(0)
			bw := board.LayoutBounds().W
			if bw <= 0 {
				bw = max(view.W-44, 320)
			}
			places, height, layout := boardLayout(bw)
			if layout != h.boardLayout {
				if h.boardLayout != "" && !c.Preferences().ReduceMotion {
					h.boardFrom, h.boardStart = maps.Clone(h.boardAt), c.Now()
				}
				h.boardLayout = layout
			}
			k := float32(1)
			if t := float32(c.Now().Sub(h.boardStart)) / float32(boardGlide); !h.boardStart.IsZero() && t < 1 {
				k = easeStd(t)
				c.AnimationFrame()
			}
			if h.boardAt == nil {
				h.boardAt = map[string]ui.Rect{}
			}
			at := func(key string) ui.Rect {
				r := places[key]
				if from, ok := h.boardFrom[key]; ok && k < 1 {
					r = ui.Rect{X: lerp(from.X, r.X, k), Y: lerp(from.Y, r.Y, k), W: lerp(from.W, r.W, k), H: lerp(from.H, r.H, k)}
				}
				h.boardAt[key] = r
				return r
			}
			bh := height
			if from := h.boardFromH; k < 1 && from > 0 {
				bh = lerp(from, height, k)
			} else {
				h.boardFromH = height
			}
			board.Height(bh)
			board.Children(func() {
				for i, tl := range []struct {
					key string
					fn  func(c *ui.Context, t ui.Element)
				}{{"compass", m.compassTile}, {"exit", m.exitTile}, {"speed", m.trafficTile}, {"today", m.todayTile}} {
					r := at(tl.key)
					place := ui.Column(c.Key(tl.key)).Absolute().Left(r.X).Top(r.Y).Width(r.W).Height(r.H)
					m.tile(c, tl.key, place, i, tl.fn)
				}
			})
			// What people look at less.
			m.homeCards(c)
		})
		// The sheet grows out of its tile over the page, and back.
		if (open > 0.001 || full > 0.001) && d.sheetKey != "" {
			m.homeSheet(c, sb, open, full)
		} else if d.sheet == "" && d.toPlace == "" {
			d.sheetKey = ""
		}
	})
	if d.sheet != "" {
		c.OnShortcut(0, ui.KeyEscape, func() { d.sheet = "" })
	}
	m.sheetDialogs(c)
	m.customizeHome(c)
}

// homeCards are the cards the user chooses, under the board, in rows that
// pack themselves; as the rows rearrange, the cards glide to their places.
func (m *mainUI) homeCards(c *ui.Context) {
	h := &m.home
	cards := map[string]func(){
		"profile": func() { m.profileCard(c) }, "ip": func() { m.ipCard(c) }, "tailscale": func() { m.tailscaleCard(c) }, "test": func() { m.testCard(c) },
		"core": func() { m.coreCard(c) }, "system": func() { m.systemCard(c) },
	}
	var ids []string
	for _, card := range m.st.UI.HomeCards {
		if card.Visible && cards[card.ID] != nil {
			ids = append(ids, card.ID)
		}
	}
	if len(ids) == 0 {
		return
	}
	wrap := ui.Column(c.Key("cards")).Shrink(0)
	gb := wrap.LayoutBounds()
	width := gb.W
	if width == 0 {
		width = 1000
	}
	rows := packHome(ids, width)
	sig := fmt.Sprint(rows)
	reflowed := h.cardRows != "" && h.cardRows != sig && gb.W > 0
	h.cardRows = sig
	places := m.cardPlaces(rows, gb.W, 12)
	m.glideHeight(c, wrap, reflowed, h.cardsH)
	wrap.Children(func() {
		grid := ui.Column(c.Key("grid")).Gap(12).Shrink(0)
		h.cardsH = grid.LayoutBounds().H
		grid.Children(func() {
			for ri, r := range rows {
				ui.Row(c.Key(ri)).Gap(12).AlignItems(ui.Stretch).Children(func() {
					for _, cell := range r {
						// A press on a card, not on its buttons, opens its
						// sheet out of it.
						key := "card:" + cell.id
						box := ui.ButtonBase(c.Key(cell.id)).Column().Grow(float32(cell.span)).Basis(0).MinWidth(0).Radius(tileRadius)
						// A button centres what it holds; the card fills its
						// cell instead, in width and in its row's height.
						box.AlignItems(ui.Stretch).Justify(ui.Start)
						k := box.AnimateTo("press", map[bool]float32{true: 0.985, false: 1}[box.Pressed()], pressSpring)
						box.Scale(k, k)
						m.glide(c, "card-"+cell.id, box, ui.Point{X: gb.X, Y: gb.Y}, places[cell.id], reflowed)
						m.noteTile(key, box)
						box.Children(func() {
							m.fillCard = true
							defer func() { m.fillCard = false }()
							cards[cell.id]()
						})
						if box.Clicked() {
							m.openSheet(key)
						}
					}
				})
			}
		})
	})
}

// loadDash loads what the board shows besides the app's state: the
// proxies, where the traffic leaves, and the totals.
func (m *mainUI) loadDash() {
	h, d, a := &m.home, &m.dash, m.a
	ready := m.state.Core.Status == coremgr.StatusRunning
	mode := m.st.Clash.Mode
	win := m.window()
	if key := (homeKey{m.runtime, m.selection, ready, mode}); key != h.loaded && !h.viewLoading {
		h.loaded = key
		if ready {
			h.viewLoading = true
			go func() {
				ctx, cancel := context.WithTimeout(a.ctx, 5*time.Second)
				defer cancel()
				v, err := (Proxies{a}).View(ctx)
				win.Update(func() {
					h.viewLoading = false
					if err != nil {
						h.loaded = homeKey{}
						return
					}
					h.view, h.pending = v, ""
				})
			}()
		} else {
			h.view = ProxiesView{}
		}
	}
	if h.ipFor != m.runtime+1 {
		h.ipFor = m.runtime + 1
		m.loadIP()
	}
	if time.Since(d.totalsAt) > 30*time.Second {
		d.totalsAt = time.Now()
		go func() {
			t, err1 := (Stats{a}).Query(context.Background(), usage.StatsQuery{Range: "today", Top: 1})
			mo, err2 := (Stats{a}).Query(context.Background(), usage.StatsQuery{Range: "30d", Top: 1})
			win.Update(func() {
				if err1 == nil {
					d.today = t.Up + t.Down
				}
				if err2 == nil {
					d.month = mo.Up + mo.Down
					d.week = d.week[:0]
					for _, day := range mo.Days[max(0, len(mo.Days)-7):] {
						d.week = append(d.week, day.Up+day.Down)
					}
				}
			})
		}()
	}
}

// loadIP looks up where the traffic really leaves.
func (m *mainUI) loadIP() {
	h, a := &m.home, m.a
	h.ipLoading, h.ipErr = true, ""
	win := m.window()
	go func() {
		ctx, cancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer cancel()
		info, err := (Tools{a}).IPInfo(ctx, false)
		win.Update(func() {
			h.ipLoading = false
			if err != nil {
				h.ipErr = err.Error()
				return
			}
			h.ip = &info
		})
	}()
}

// ---- The compass ----

// compassTile is the switch that takes traffic over: the app's star on a
// disc, grey and turned aside while traffic goes its own way, turning
// upright in the band's blue as the app takes it over. Under it, what is
// true now, and the two ways of taking over.
func (m *mainUI) compassTile(c *ui.Context, t ui.Element) {
	h, d, p, a, s, st := &m.home, &m.dash, m.pal, m.a, m.state, m.st
	running := s.Core.Status == coremgr.StatusRunning
	on := running && (s.SystemProxy || s.Tun)
	if d.method == "" {
		d.method = "sys"
		if st.Tun.Enabled {
			d.method = "tun"
		}
	}
	setTakeover := func(sys, tun bool) {
		h.busy = "takeover"
		m.run(locales.CommonFailed(), func() error {
			_, err := (SettingsService{a}).Patch(context.Background(), map[string]any{
				"systemProxy": map[string]any{"enabled": sys}, "tun": map[string]any{"enabled": tun},
			})
			return err
		}, func(error) { h.busy = "" })
	}
	useTun := func() bool {
		if s.TunAvailable {
			return true
		}
		if s.Service.Supported && h.busy == "" {
			h.busy = "svc"
			m.run("", func() error { a.installServiceForTun(); return nil }, func(error) { h.busy = "" })
		}
		return false
	}
	// Side by side on a wide tile, the dial before what it says and does;
	// on a narrow one the dial, small, beside what is true, and the two
	// choices under them.
	box := t.LayoutBounds()
	side := box.W == 0 || box.W >= 330
	drawDial := func(size float32) {
		dial := ui.ButtonBase(c.Key("dial")).Shrink(0).Cursor(ui.CursorPointer).Label(locales.HomeTakeover())
		sz := dial.AnimateTo("size", size, ui.Animation{Duration: 280 * time.Millisecond, Ease: easeStd})
		dial.Width(sz).Height(sz).Radius(sz / 2)
		// The star springs upright, past it a little, and settles.
		lit := dial.AnimateTo("on", b2f(on), ui.Animation{Spring: ui.SpringOptions{Stiffness: 110, Damping: 11}})
		hover := dial.Animate("hover", b2f(dial.Hovered()), 140*time.Millisecond)
		k := dial.AnimateTo("press", map[bool]float32{true: 0.96, false: 1}[dial.Pressed()], pressSpring)
		dial.Scale(k, k)
		tone := ""
		switch s.Core.Status {
		case coremgr.StatusError:
			tone = "error"
		case coremgr.StatusRunning:
		default:
			tone = "warn"
		}
		dial.Draw(func(pt *ui.Painter, r ui.Rect) {
			cx, cy := r.X+r.W/2, r.Y+r.H/2
			l := float32(math.Max(0, math.Min(1, float64(lit))))
			bg := p.Surface2.Mix(p.Surface3, hover).Mix(p.Card.Mix(p.Accent, 0.16+0.04*hover), l)
			pt.FillPath((&ui.Path{}).Circle(cx, cy, r.W/2), bg)
			switch tone {
			case "error":
				pt.StrokePath((&ui.Path{}).Circle(cx, cy, r.W/2-1), 1.5, p.Danger.Alpha(0.8))
			case "warn":
				pt.StrokePath((&ui.Path{}).Circle(cx, cy, r.W/2-1).Dash(0, 3, 5), 1.5, p.Warning.Alpha(0.9))
			}
			turn := (1 - lit) * -38
			star := r.W * 0.62
			pt.Transform(ui.Translation(-cx, -cy).Then(ui.Scaling(0.92+0.08*l, 0.92+0.08*l)).Then(ui.Translation(cx, cy)), 0, 0, func() {
				if l < 0.999 {
					drawCompass(pt, cx, cy, star, monoLook(p.Faint, turn))
				}
				if l > 0.01 {
					drawCompass(pt, cx, cy, star, compassLook{Turn: turn, Alpha: l})
				}
			})
		})
		if dial.Clicked() {
			switch {
			case h.busy != "":
			case !running:
				h.busy = "core"
				m.run(locales.CommonFailed(), func() error { return Core{a}.Restart(context.Background()) }, func(error) { h.busy = "" })
			case on:
				setTakeover(false, false)
			case d.method == "tun":
				if useTun() {
					setTakeover(false, true)
				}
			default:
				setTakeover(true, false)
			}
		}
	}
	stateText := func() {
		// What is true now.
		state, col := locales.HomeStateOff(), p.Text
		switch {
		case s.Core.Status == coremgr.StatusStarting:
			state, col = locales.HomeStateStarting(), p.Warning
		case s.Core.Status == coremgr.StatusError:
			state, col = locales.HomeStateError(), p.Danger
		case !running:
			state, col = locales.HomeStateStopped(), p.Warning
		case on:
			state = locales.HomeStateOn()
		}
		ui.Box(c).Height(30).Shrink(0).Children(func() {
			ui.Text(c.Key("state-"+state), state).FontSize(22).FontWeight(650).TextColor(col).SingleLine().
				Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 10}, Exit: &ui.Motion{Y: -8}, Duration: 320 * time.Millisecond, Ease: easeOut})
		})
	}
	ways := func() {
		// The two ways of taking over.
		sel := 0
		if (on && s.Tun) || (!on && d.method == "tun") {
			sel = 1
		}
		seg := ui.Box(c.Key("ways"))
		seg.Opacity(seg.AnimateTo("dim", map[bool]float32{true: 1, false: 0.55}[on], ui.Animation{Duration: 200 * time.Millisecond}))
		seg.Children(func() {
			if i := m.segmented(c, "way", sel, locales.SettingsSystemProxy(), "TUN"); i >= 0 && h.busy == "" && running {
				if i == 1 {
					d.method = "tun"
					if useTun() {
						setTakeover(false, true)
					}
				} else {
					d.method = "sys"
					setTakeover(true, false)
				}
			}
		})
	}
	modes := func() {
		// How the traffic is routed, which was a tile of its own.
		modes := []string{"rule", "global", "direct"}
		cur := slices.Index(modes, st.Clash.Mode)
		if i := m.segmented(c, "mode", cur, locales.ModeRule(), locales.ModeGlobal(), locales.ModeDirect()); i >= 0 && i != cur {
			md := modes[i]
			m.st.Clash.Mode = md // at once; the settings confirm
			m.run(locales.CommonFailed(), func() error { return Core{a}.SetMode(context.Background(), md) }, nil)
		}
	}
	t.Children(func() {
		if side {
			size := float32(160)
			if box.H > 0 {
				// The dial takes what the controls beside it leave.
				size = max(96, min(176, box.H-40, (box.W-44-26)*0.42))
			}
			ui.Row(c.Key("lay")).Grow(1).Basis(0).MinHeight(0).Gap(26).AlignItems(ui.Center).Children(func() {
				drawDial(size)
				ui.Column(c.Key("controls")).Gap(12).Grow(1).Basis(0).MinWidth(0).AlignItems(ui.Start).Children(func() {
					stateText()
					ways()
					modes()
				})
			})
		} else {
			ui.Column(c.Key("lay")).Grow(1).Basis(0).MinHeight(0).Gap(12).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				ui.Row(c.Key("head")).Gap(14).AlignItems(ui.Center).Children(func() {
					drawDial(64)
					stateText()
				})
				ways()
				modes()
			})
		}
		if h.busy == "svc" || h.busy == "takeover" || h.busy == "core" {
			ui.Spinner(c).Absolute().Top(16).End(16)
		}
	})
}

// bigChip is a large filled button of the board, with an icon before its
// label: the accent when primary, a step of grey otherwise.
func (m *mainUI) bigChip(c *ui.Context, key, label string, icon *ui.SVG, primary, busy bool) ui.Element {
	p := m.pal
	b := ui.ButtonBase(c.Key(key)).Row().Height(36).Padding(0, 14).Gap(7).Radius(11).AlignItems(ui.Center).Shrink(0).Label(label)
	bg, fg := p.Surface2, p.Text2
	if primary {
		bg, fg = p.Accent, p.AccentText
		if b.Hovered() {
			bg = p.AccentHover
		}
	} else if b.Hovered() {
		bg = p.Surface3
	}
	k := b.AnimateTo("press", map[bool]float32{true: 0.96, false: 1}[b.Pressed()], pressSpring)
	b.Scale(k, k).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 150 * time.Millisecond}))
	b.Children(func() {
		if busy {
			ui.Spinner(c)
		} else if icon != nil {
			m.icon(c, icon, 15, fg, b.Hovered())
		}
		ui.Text(c, label).FontSize(13.5).FontWeight(620).TextColor(fg).SingleLine()
	})
	return b
}

// exitTile is the node the traffic leaves through, as large as it
// matters, how far it is, and what to do about it. A press opens the node
// sheet out of it.
func (m *mainUI) exitTile(c *ui.Context, t ui.Element) {
	h, d, p, s := &m.home, &m.dash, m.pal, m.state
	g, byName := m.heroGroup()
	mode := m.st.Clash.Mode
	big := func(title, sub string, col ui.Color) {
		t.Justify(ui.Center).Gap(6).Children(func() {
			ui.Text(c, title).FontSize(26).FontWeight(680).TextColor(col).SingleLine()
			if sub != "" {
				ui.Text(c, sub).FontSize(13.5).TextColor(p.TextMuted).Wrap()
			}
		})
	}
	switch {
	case s.Core.Status != coremgr.StatusRunning:
		big(locales.CommonCoreNotRunning(), locales.CoreStoppedHint(), p.TextMuted)
		return
	case mode == "direct":
		big(locales.ModeDirect(), locales.ModeDirectDesc(), p.Text)
		return
	case g == nil:
		big(locales.HomeNoGroups(), "", p.TextMuted)
		return
	}
	now := g.Now
	if h.pending != "" {
		now = h.pending
	}
	path := proxyChain(g, byName, now)
	exit := path[len(path)-1]
	var item *ProxyItem
	for _, x := range []*ProxyGroup{g, byName[path[max(0, len(path)-2)]]} {
		if x == nil {
			continue
		}
		for i := range x.All {
			if x.All[i].Name == exit {
				item = &x.All[i]
			}
		}
	}
	delay := -1
	if item != nil {
		delay = item.Delay
	}
	if v, ok := h.delays[exit]; ok {
		delay = v
	}
	cc := flagOf(exit)
	if cc == "" && h.ip != nil && exit != "DIRECT" {
		cc = h.ip.CountryCode
	}
	var facts []string
	if h.ip != nil && exit != "DIRECT" {
		facts = append(facts, cmp.Or(h.ip.City, h.ip.Country))
	}
	if item != nil && item.Type != "" {
		facts = append(facts, item.Type)
	}
	if len(path) > 2 {
		facts = append(facts, strings.Join(path[1:len(path)-1], " → "))
	}
	if s.ProfileName != "" {
		facts = append(facts, s.ProfileName)
	}
	t.Gap(14)
	t.Children(func() {
		ui.Row(c).Gap(16).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Size(58, 58).Radius(16).Background(p.Surface2).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Children(func() {
				if svg := svgImage("assets/flags/" + strings.ToLower(cc) + ".svg"); cc != "" && svg != nil {
					ui.Box(c).Size(36, 27).Radius(5).Clip().Children(func() { ui.Image(c, svg).Fill() })
				} else {
					ui.Icon(c, icGlobe2).FontSize(24).TextColor(p.TextMuted)
				}
			})
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Gap(4).Children(func() {
				ui.Text(c.Key("name-"+exit), stripFlag(exit)).FontSize(26).FontWeight(680).LetterSpacing(-0.3).SingleLine().
					Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 10}, Duration: 340 * time.Millisecond, Ease: easeOut})
				if len(facts) > 0 {
					ui.Text(c, strings.Join(facts, " · ")).FontSize(13.5).TextColor(p.TextMuted).SingleLine()
				}
			})
			ui.Column(c).AlignItems(ui.End).Gap(6).Shrink(0).Children(func() {
				lit := 0
				switch {
				case delay <= 0:
				case delay < 120:
					lit = 4
				case delay < 250:
					lit = 3
				case delay < 500:
					lit = 2
				default:
					lit = 1
				}
				ui.Box(c).Size(23, 16).Draw(func(pt *ui.Painter, r ui.Rect) {
					for i := 0; i < 4; i++ {
						bh := float32(5 + i*11/3)
						col := p.Surface3
						if i < lit {
							col = p.Text2
						}
						pt.Fill(ui.Rect{X: r.X + float32(i)*6, Y: r.Y + r.H - bh, W: 4, H: bh}, col, 2)
					}
				})
				ui.Row(c).Gap(3).AlignItems(ui.Baseline).Children(func() {
					switch {
					case h.testing:
						ui.Text(c, "…").FontSize(40).FontWeight(680).TextColor(p.TextMuted)
					case delay > 0:
						ui.Text(c, fmt.Sprint(delay)).FontSize(44).FontWeight(680).LetterSpacing(-0.8).FontFeatures("tnum").ContentTransition(numRoll)
						ui.Text(c, "ms").FontSize(15).TextColor(p.TextMuted)
					case delay == 0:
						ui.Text(c, locales.CommonTimeout()).FontSize(24).FontWeight(650).TextColor(p.Danger)
					default:
						ui.Text(c, "—").FontSize(30).FontWeight(500).TextColor(p.Faint)
					}
				})
			})
		})
		ui.Spacer(c)
		m.routePills(c, g, byName)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if m.bigChip(c, "change", locales.HomeChangeNode(), icLayoutGrid, true, false).Clicked() {
				m.openSheet("exit")
			}
			auto := m.bigChip(c, "auto", locales.HomeAutoFastest(), icZap, false, d.autoBusy)
			if auto.Clicked() && !d.autoBusy && g.Type == "Selector" {
				m.autoFastest(g)
			}
		})
	})
}

// modeTile is the three modes, one under another, the soft fill behind the
// one in use sliding to the one chosen.
func (m *mainUI) modeTile(c *ui.Context, t ui.Element) {
	p, a := m.pal, m.a
	mode := m.st.Clash.Mode
	t.Padding(12).Gap(4)
	thumb := segThumb{t.ID()}
	t.Children(func() {
		for _, md := range []string{"rule", "global", "direct"} {
			on := md == mode
			b := ui.ButtonBase(c.Key(md)).Row().Grow(1).Basis(0).MinHeight(36).Padding(0, 16).Gap(10).Radius(13).AlignItems(ui.Center).Label(m.T("mode." + md))
			if !on && b.Hovered() {
				b.Background(p.Surface2)
			}
			b.Transition(hoverFade)
			b.Children(func() {
				if on {
					ui.Box(c.Key("thumb")).Absolute().Left(0).Top(0).Right(0).Bottom(0).Radius(13).Background(p.AccentSoft).
						Transition(ui.ElementTransition{Match: thumb, Animation: &segSlide})
				}
				col := p.TextMuted
				if on {
					col = p.AccentFg
				}
				txt := ui.Text(c, m.T("mode."+md)).FontSize(17).FontWeight(620).SingleLine().Grow(1).ZIndex(1)
				txt.TextColor(txt.AnimateColor("fg", col, ui.Animation{Duration: 200 * time.Millisecond}))
				if on {
					ui.Icon(c.Key("check"), icCheck).FontSize(17).TextColor(p.AccentFg).ZIndex(1).Transition(checkIn)
				}
			})
			// The mode in use, pressed again, opens the sheet of the modes.
			if b.Clicked() && on {
				m.openSheet("mode")
			}
			if b.Clicked() && !on {
				m.st.Clash.Mode = md // at once; the settings confirm
				m.run(locales.CommonFailed(), func() error { return Core{a}.SetMode(context.Background(), md) }, nil)
			}
		}
	})
}

// speedTile is the speed now, large, over the last half minute in bars
// that slide in from the end.
func (m *mainUI) speedTile(c *ui.Context, t ui.Element) {
	p := m.pal
	t.Gap(2)
	t.Children(func() {
		line := func(icon *ui.SVG, col ui.Color, v int64, size float32) {
			num, unit, _ := strings.Cut(fmtRate(v), " ")
			ui.Row(c).Gap(6).AlignItems(ui.Baseline).Children(func() {
				ui.Icon(c, icon).FontSize(size * 0.55).TextColor(col)
				ui.Text(c, num).FontSize(size).FontWeight(map[bool]int{true: 680, false: 600}[size > 24]).LetterSpacing(-0.4).FontFeatures("tnum").ContentTransition(numRoll)
				ui.Text(c, unit).FontSize(13.5).TextColor(p.TextMuted)
			})
		}
		line(icArrowDown, p.Accent, m.now.Down, 32)
		line(icArrowUp, p.Accent2, m.now.Up, 20)
		// Right to left, the newest comes in from the left.
		rtl := c.Direction() == ui.RightToLeft
		ui.Box(c).Grow(1).MinHeight(36).Margin(10, 0, 0).Draw(func(pt *ui.Painter, r ui.Rect) {
			const n = 26
			down := m.down[len(m.down)-n-1:]
			var peak int64 = 64 * 1024
			for _, v := range down {
				peak = max(peak, v)
			}
			frac := float32(pt.Now().Sub(m.sampleAt).Seconds())
			if frac < 1 && frac >= 0 {
				pt.AnimationFrame()
			} else {
				frac = 1
			}
			bw := r.W / n
			pt.Clip(r, 0, func() {
				for k := 0; k < len(down); k++ {
					v := down[len(down)-1-k]
					bh := max(3, r.H*float32(math.Sqrt(float64(v)/float64(peak))))
					x := r.X + r.W - (float32(k)+frac)*bw
					if rtl {
						x = r.X + (float32(k)+frac-1)*bw
					}
					col := p.Surface3
					if k == 0 {
						col = p.Accent
					}
					pt.Fill(ui.Rect{X: x + 1.5, Y: r.Y + r.H - bh, W: bw - 3, H: bh}, col, 2)
				}
			})
		})
	})
}

// routesTile is the other groups and where each leaves, as keys that
// open a menu of their nodes.
func (m *mainUI) routesTile(c *ui.Context, t ui.Element) {
	h, p := &m.home, m.pal
	hero, byName := m.heroGroup()
	var gs []*ProxyGroup
	for i := range h.view.Groups {
		g := &h.view.Groups[i]
		if g != hero && !g.Hidden && (g.Type == "Selector" || g.Type == "URLTest" || g.Type == "Fallback") && len(gs) < 4 {
			gs = append(gs, g)
		}
	}
	t.Padding(10).Gap(6)
	if len(gs) == 0 {
		t.Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Text(c, locales.HomeNoOtherRoutes()).FontSize(13.5).TextColor(p.TextMuted)
		})
		return
	}
	quiet := m.st.Clash.Mode != "rule"
	t.Opacity(t.AnimateTo("quiet", map[bool]float32{true: 0.45, false: 1}[quiet], ui.Animation{Duration: 250 * time.Millisecond}))
	t.Children(func() {
		for r := 0; r < (len(gs)+1)/2; r++ {
			ui.Row(c.Key(r)).Grow(1).Basis(0).Gap(6).AlignItems(ui.Stretch).Children(func() {
				for k := 0; k < 2; k++ {
					i := r*2 + k
					if i >= len(gs) {
						ui.Box(c.Key(k)).Grow(1).Basis(0)
						continue
					}
					g := gs[i]
					path := proxyChain(g, byName, g.Now)
					exit := path[len(path)-1]
					delay := -1
					for _, it := range g.All {
						if it.Name == g.Now {
							delay = it.Delay
						}
					}
					if v, ok := h.delays[exit]; ok {
						delay = v
					}
					b := ui.ButtonBase(c.Key(g.Name)).Column().Grow(1).Basis(0).MinWidth(0).Justify(ui.Center).AlignItems(ui.Stretch).Gap(5).Padding(0, 16).Radius(14).Label(g.Name)
					bg := p.Surface2
					if b.Hovered() {
						bg = p.Surface3
					}
					kp := b.AnimateTo("press", map[bool]float32{true: 0.975, false: 1}[b.Pressed()], pressSpring)
					b.Scale(kp, kp).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 150 * time.Millisecond}))
					b.Children(func() {
						ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(c, g.Name).FontSize(13.5).TextColor(p.TextMuted).SingleLine().Grow(1).Basis(0)
							switch {
							case delay > 0:
								ui.Text(c, fmt.Sprintf("%d ms", delay)).FontSize(13).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll)
							case delay == 0:
								ui.Text(c, locales.CommonTimeout()).FontSize(13).TextColor(p.Danger)
							}
						})
						ui.Row(c).Gap(7).AlignItems(ui.Center).Children(func() {
							if cc := flagOf(exit); cc != "" {
								m.flagSmall(c, cc)
							}
							ui.Text(c.Key("n-"+exit), stripFlag(exit)).FontSize(17).FontWeight(650).SingleLine().Shrink(1).
								Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 8}, Duration: 300 * time.Millisecond, Ease: easeOut})
						})
					})
					m.noteTile("group:"+g.Name, b)
					if b.Clicked() {
						m.openSheet("group:" + g.Name)
					}
				}
			})
		}
	})
}

// selectIn chooses a node of a group other than the one the page leads
// with.
func (m *mainUI) selectIn(group, name string) {
	h, a := &m.home, m.a
	m.run(locales.ProxiesSelectFailed(), func() error { return Proxies{a}.Select(context.Background(), group, name) }, func(error) {
		h.loaded = homeKey{}
	})
}

// appsTile is who moves the most now: their shares in one bar, the one
// under the pointer lit, and the first four by name.
func (m *mainUI) appsTile(c *ui.Context, t ui.Element) {
	d, p := &m.dash, m.pal
	type app struct {
		name string
		rate int64
		n    int
	}
	byName := map[string]*app{}
	var order []*app
	for _, cn := range m.connList {
		name := cmp.Or(cn.Metadata.Process, cn.Metadata.Host, cn.Metadata.DestinationIP)
		a := byName[name]
		if a == nil {
			a = &app{name: name}
			byName[name] = a
			order = append(order, a)
		}
		a.rate += cn.DownloadSpeed + cn.UploadSpeed
		a.n++
	}
	list := slices.Clone(order)
	sort.SliceStable(list, func(i, j int) bool {
		return cmp.Or(cmp.Compare(list[j].rate, list[i].rate), cmp.Compare(list[j].n, list[i].n), strings.Compare(list[i].name, list[j].name)) < 0
	})
	shares := list[:min(8, len(list))]
	slices.SortStableFunc(shares, func(x, y *app) int { return strings.Compare(x.name, y.name) })
	var total int64
	for _, a := range shares {
		total += a.rate
	}
	t.Gap(12)
	t.Children(func() {
		if len(list) == 0 {
			ui.Column(c).Grow(1).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
				ui.Text(c, locales.ConnEmpty()).FontSize(13.5).TextColor(p.TextMuted)
			})
			return
		}
		ui.Row(c.Key("share")).Height(12).Gap(2).Radius(6).Clip().Shrink(0).Children(func() {
			for i, a := range shares {
				share := float32(a.rate+1) / float32(total+int64(len(shares)))
				seg := ui.Box(c.Key(a.name)).Basis(0).MinWidth(3)
				seg.Grow(seg.AnimateTo("g", share, ui.Animation{Duration: 900 * time.Millisecond, Ease: easeStd}))
				col := p.Surface3
				if i%2 == 1 {
					col = p.BorderStrong
				}
				if d.hotApp == a.name {
					col = p.Accent
				}
				seg.Background(seg.AnimateColor("bg", col, ui.Animation{Duration: 160 * time.Millisecond}))
			}
		})
		hot := ""
		ui.Column(c).Gap(2).MarginX(-8).Children(func() {
			for _, a := range list[:min(4, len(list))] {
				row := ui.Row(c.Key(a.name)).Gap(8).Padding(5, 8).Radius(9).AlignItems(ui.Baseline)
				if row.Hovered() {
					row.Background(p.Surface2)
					hot = a.name
				}
				row.Transition(hoverFade)
				row.Children(func() {
					ui.Text(c, a.name).FontSize(14.5).FontWeight(600).SingleLine().Grow(1).Basis(0)
					ui.Text(c, fmtRate(a.rate)).FontSize(13).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll).SingleLine()
				})
			}
		})
		d.hotApp = hot
	})
}

// todayTile is today's traffic, large, and the last seven days in bars,
// today's in the accent.
func (m *mainUI) todayTile(c *ui.Context, t ui.Element) {
	d, p := &m.dash, m.pal
	t.Gap(2)
	t.Children(func() {
		num, unit, _ := strings.Cut(fmtBytes(d.today), " ")
		ui.Row(c).Gap(4).AlignItems(ui.Baseline).Children(func() {
			ui.Text(c, num).FontSize(32).FontWeight(680).LetterSpacing(-0.4).FontFeatures("tnum").ContentTransition(numRoll)
			ui.Text(c, unit).FontSize(14).TextColor(p.TextMuted)
		})
		ui.Text(c, locales.HomeMonth(fmtBytes(d.month))).FontSize(13.5).TextColor(p.TextMuted).SingleLine()
		week := slices.Clone(d.week)
		if len(week) == 0 {
			week = []int64{d.today}
		}
		week[len(week)-1] = max(week[len(week)-1], d.today)
		rtl := c.Direction() == ui.RightToLeft
		ui.Box(c).Grow(1).MinHeight(36).Margin(12, 0, 0).Draw(func(pt *ui.Painter, r ui.Rect) {
			var peak int64 = 1
			for _, v := range week {
				peak = max(peak, v)
			}
			bw := r.W / 7
			off := 7 - len(week)
			for i, v := range week {
				bh := max(4, r.H*float32(v)/float32(peak))
				col := p.Surface3
				if i == len(week)-1 {
					col = p.Accent
				}
				x := r.X + float32(i+off)*bw + 4
				if rtl {
					x = r.X + r.W - float32(i+off+1)*bw + 4
				}
				pt.Fill(ui.Rect{X: x, Y: r.Y + r.H - bh, W: bw - 8, H: bh}, col, 4)
			}
		})
	})
}

// nodeTile is a node as a key: its flag and how far it is along the top,
// what it is and its name below; the accent's soft fill when chosen. The
// delay is a button of its own, which tests the node; it is returned too.
func (m *mainUI) nodeTile(c *ui.Context, key string, it *ProxyItem, delay int, active, testing bool) (ui.Element, ui.Element) {
	p := m.pal
	b := ui.ButtonBase(c.Key(key)).Column().Height(96).Padding(12, 14).Radius(16).MinWidth(0).AlignItems(ui.Stretch).Tooltip(it.Name)
	bg := p.Surface2
	switch {
	case active:
		bg = p.AccentSoft
	case b.Hovered():
		bg = p.Surface3
	}
	kp := b.AnimateTo("press", map[bool]float32{true: 0.965, false: 1}[b.Pressed()], pressSpring)
	b.Scale(kp, kp).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 160 * time.Millisecond}))
	var test ui.Element
	b.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			if cc := flagOf(it.Name); cc != "" {
				m.flagSmall(c, cc)
			} else if it.IsGroup {
				ui.Icon(c, icLayers).FontSize(14).TextColor(p.TextMuted)
			}
			ui.Spacer(c)
			test = ui.ButtonBase(c.Key("delay")).Row().Height(24).Padding(0, 6).MarginEnd(-6).Radius(7).AlignItems(ui.Baseline).Justify(ui.Center).Shrink(0)
			if test.Hovered() {
				test.Background(p.Surface3.Mix(p.SurfacePressed, 0.5))
			}
			test.Transition(hoverFade)
			freshDelay(test, delay, testing)
			test.Children(func() {
				switch {
				case testing:
					ui.Spinner(c)
				case delay > 0:
					ui.Text(c, fmt.Sprint(delay)).FontSize(15).FontWeight(650).FontFeatures("tnum").ContentTransition(numRoll)
					ui.Text(c, "ms").FontSize(11.5).TextColor(p.TextMuted).MarginStart(1)
				case delay == 0:
					ui.Text(c, locales.CommonTimeout()).FontSize(12.5).FontWeight(600).TextColor(p.Danger)
				default:
					ui.Text(c, "—").FontSize(14).TextColor(p.Faint)
				}
			})
		})
		ui.Spacer(c)
		sub := it.Type
		if it.IsGroup && it.Now != "" {
			sub = "→ " + it.Now
		}
		ui.Text(c, sub).FontSize(12).TextColor(p.TextMuted).SingleLine()
		name := ui.Text(c, stripFlag(it.Name)).FontSize(15).FontWeight(620).SingleLine().Margin(2, 0, 0)
		if active {
			name.TextColor(p.AccentFg)
		}
	})
	return b, test
}

// heroGroup is the group whose exit leads the page: the global one in
// global mode, else the first that chooses.
func (m *mainUI) heroGroup() (*ProxyGroup, map[string]*ProxyGroup) {
	v := &m.home.view
	byName := map[string]*ProxyGroup{}
	var first *ProxyGroup
	for i := range v.Groups {
		g := &v.Groups[i]
		byName[g.Name] = g
		if first == nil && !g.Hidden && (g.Type == "Selector" || g.Type == "URLTest" || g.Type == "Fallback") {
			first = g
		}
	}
	if v.Global != nil {
		byName[v.Global.Name] = v.Global
		if m.st.Clash.Mode == "global" {
			return v.Global, byName
		}
	}
	return first, byName
}

func (m *mainUI) homeSelect(group, name string) {
	h, a := &m.home, m.a
	h.pending = name
	m.run(locales.ProxiesSelectFailed(), func() error { return Proxies{a}.Select(context.Background(), group, name) }, func(err error) {
		if err != nil {
			h.pending = ""
		}
		h.loaded = homeKey{}
	})
}

// homeTest tests a group's delays, then does what follows, if anything.
func (m *mainUI) homeTest(g *ProxyGroup, then func(res map[string]int)) {
	h, a := &m.home, m.a
	h.testing = true
	group, url, members := g.Name, g.TestURL, g.All
	win := m.window()
	go func() {
		res, err := (Proxies{a}).GroupDelay(context.Background(), group, url)
		if err != nil {
			m.notice(Notice{Level: "error", Message: locales.ProxiesTestFailed(), Detail: err.Error()})
		}
		win.Update(func() {
			h.testing = false
			if err == nil {
				h.delays = map[string]int{}
				for _, it := range members {
					h.delays[it.Name] = res[it.Name]
				}
				if then != nil {
					then(res)
				}
			}
			h.loaded = homeKey{}
		})
	}()
}

// autoFastest tests the group and chooses its fastest node.
func (m *mainUI) autoFastest(g *ProxyGroup) {
	d := &m.dash
	d.autoBusy = true
	group := g.Name
	m.homeTest(g, func(res map[string]int) {
		d.autoBusy = false
		best, bd := "", math.MaxInt
		for name, v := range res {
			if v > 0 && v < bd {
				best, bd = name, v
			}
		}
		if best != "" {
			m.homeSelect(group, best)
		}
	})
	if !m.home.testing {
		d.autoBusy = false
	}
}

// glideH is the height a glided element had as last laid out.
func (m *mainUI) glideH(id string) float32 {
	if st := m.glides[id]; st != nil {
		return st.last.H
	}
	return 0
}

// cardPlaces are where the cards below the board go, relative to their
// grid, in rows of spans gap apart, from the heights they had.
func (m *mainUI) cardPlaces(rows [][]struct {
	id   string
	span int
}, w, gap float32) map[string]ui.Rect {
	out := map[string]ui.Rect{}
	y := float32(0)
	for _, r := range rows {
		total, rh := 0, float32(0)
		for _, cell := range r {
			total += cell.span
			rh = max(rh, m.glideH("card-"+cell.id))
		}
		free := w - gap*float32(len(r)-1)
		x := float32(0)
		for _, cell := range r {
			cw := free * float32(cell.span) / float32(max(total, 1))
			out[cell.id] = ui.Rect{X: x, Y: y, W: cw, H: rh}
			x += cw + gap
		}
		y += rh + gap
	}
	return out
}

// boardGlide is how long the board's tiles take to their places in a new
// layout.
const boardGlide = 420 * time.Millisecond

func lerp(a, b, k float32) float32 { return a + (b-a)*k }

// boardLayout places the board's tiles for a board w wide: the switch and
// the exit side by side, then the traffic and today's total, in two rows of
// two on a narrower board too, and one under another on a narrow one. Heights follow what a tile holds, not the window, so
// that no tile stands large and empty.
func boardLayout(w float32) (places map[string]ui.Rect, height float32, layout string) {
	const gap = 12
	places = map[string]ui.Rect{}
	y := float32(0)
	row := func(h float32, keys []string, weights []float32) {
		total := float32(0)
		for _, v := range weights {
			total += v
		}
		free := w - gap*float32(len(keys)-1)
		x := float32(0)
		for i, key := range keys {
			cw := free * weights[i] / total
			places[key] = ui.Rect{X: x, Y: y, W: cw, H: h}
			x += cw + gap
		}
		y += h + gap
	}
	switch {
	case w >= 880:
		layout = "wide"
		row(232, []string{"compass", "exit"}, []float32{5.4, 6.6})
		row(184, []string{"speed", "today"}, []float32{8.6, 3.4})
	case w >= 560:
		layout = "mid"
		row(252, []string{"compass", "exit"}, []float32{1, 1})
		row(184, []string{"speed", "today"}, []float32{1.8, 1})
	default:
		layout = "narrow"
		row(262, []string{"compass"}, []float32{1})
		row(252, []string{"exit"}, []float32{1})
		row(188, []string{"speed"}, []float32{1})
		row(176, []string{"today"}, []float32{1})
	}
	return places, y - gap, layout
}

// trafficTile is the traffic now: the speed, large, over its last half
// minute, and beside it who moves the most, a region of its own that
// opens the connections out of it. On a narrow tile the speed has it all.
func (m *mainUI) trafficTile(c *ui.Context, t ui.Element) {
	p := m.pal
	w := t.LayoutBounds().W
	t.Children(func() {
		ui.Row(c).Grow(1).Basis(0).MinHeight(0).Gap(16).AlignItems(ui.Stretch).Children(func() {
			m.speedTile(c, ui.Column(c.Key("rate")).Grow(1).Basis(0).MinWidth(0))
			if w > 0 && w < 380 {
				return
			}
			apps := ui.ButtonBase(c.Key("apps")).Column().Width(max(170, min(300, w*0.42))).Shrink(0).
				Padding(12, 14).Margin(-8, -10, -8, 0).Radius(14).Label(locales.HomeWhoUses()).AlignItems(ui.Stretch)
			bg := p.Surface2.Alpha(0)
			if apps.Hovered() {
				bg = p.Surface2
			}
			k := apps.AnimateTo("press", map[bool]float32{true: 0.98, false: 1}[apps.Pressed()], pressSpring)
			apps.Scale(k, k).Background(apps.AnimateColor("bg", bg, ui.Animation{Duration: 160 * time.Millisecond}))
			m.noteTile("apps", apps)
			m.appsTile(c, apps)
			if apps.Clicked() {
				m.openSheet("apps")
			}
		})
	})
}

// routePills are the other groups and where each leaves, small, under the
// exit they share its tile with: each opens its group's sheet out of it.
// Out of the rule mode they recede, as the rules no longer pick them.
func (m *mainUI) routePills(c *ui.Context, hero *ProxyGroup, byName map[string]*ProxyGroup) {
	h, p := &m.home, m.pal
	var gs []*ProxyGroup
	for i := range h.view.Groups {
		g := &h.view.Groups[i]
		if g != hero && !g.Hidden && (g.Type == "Selector" || g.Type == "URLTest" || g.Type == "Fallback") && len(gs) < 6 {
			gs = append(gs, g)
		}
	}
	if len(gs) == 0 {
		return
	}
	row := ui.Row(c.Key("routes")).Gap(6).Wrap().Clip().MaxHeight(72)
	row.Opacity(row.AnimateTo("quiet", map[bool]float32{true: 0.45, false: 1}[m.st.Clash.Mode != "rule"], ui.Animation{Duration: 250 * time.Millisecond}))
	row.Children(func() {
		for _, g := range gs {
			path := proxyChain(g, byName, g.Now)
			exit := path[len(path)-1]
			delay := -1
			for _, it := range g.All {
				if it.Name == g.Now {
					delay = it.Delay
				}
			}
			if v, ok := h.delays[exit]; ok {
				delay = v
			}
			b := ui.ButtonBase(c.Key(g.Name)).Row().Height(30).Padding(0, 10).Gap(6).Radius(10).AlignItems(ui.Center).Shrink(0).Label(g.Name)
			bg := p.Surface2
			if b.Hovered() {
				bg = p.Surface3
			}
			k := b.AnimateTo("press", map[bool]float32{true: 0.96, false: 1}[b.Pressed()], pressSpring)
			b.Scale(k, k).Background(b.AnimateColor("bg", bg, ui.Animation{Duration: 150 * time.Millisecond}))
			b.Children(func() {
				ui.Text(c, g.Name).FontSize(12.5).TextColor(p.TextMuted).SingleLine()
				if cc := flagOf(exit); cc != "" {
					m.flagSmall(c, cc)
				}
				ui.Text(c.Key("n-"+exit), stripFlag(exit)).FontSize(13).FontWeight(620).SingleLine().
					Transition(ui.ElementTransition{Enter: &ui.Motion{Y: 6}, Duration: 280 * time.Millisecond, Ease: easeOut})
				switch {
				case delay > 0:
					ui.Text(c, fmt.Sprint(delay)).FontSize(12).TextColor(p.TextMuted).FontFeatures("tnum").ContentTransition(numRoll)
				case delay == 0:
					ui.Text(c, locales.CommonTimeout()).FontSize(12).TextColor(p.Danger)
				}
			})
			m.noteTile("group:"+g.Name, b)
			if b.Clicked() {
				m.openSheet("group:" + g.Name)
			}
		}
	})
}
