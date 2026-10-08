package app

import (
	"context"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
)

// speedWindow is a small window of native UI that floats over the others
// with the traffic, for Windows, where the tray has no room for it, and
// for whoever wants it elsewhere. It is dragged around and stays where it
// is left; the pointer over it shows the tray's status line; a double
// click opens the main window and a right click the tray's menu.
type speedWindow struct {
	a *App

	mu   sync.Mutex
	win  *mygo.Window
	stop context.CancelFunc
	cur  config.SpeedWindow // as last applied
	full bool               // hidden for a full-screen app

	// What the view shows, changed on the main thread (Window.Update).
	traffic coreapi.Traffic
	hist    []int64 // the last downloads, oldest first
	line    string  // statusLine
	dot     dotKind
	// The view's own state, on the main thread.
	style    string
	expanded bool
	base     mygo.Point // the window's place, not expanded
	drag     *dragStart
	pinned   bool // expanded whatever the pointer does, for the debug server
}

type dotKind int

const (
	dotOff dotKind = iota
	dotOn
	dotBusy
	dotError
)

type dragStart struct {
	cursor, win mygo.Point
	moved       bool
}

// The window's sizes: collapsed and with the status line under the pointer.
var speedSizes = map[string][2]mygo.Rectangle{
	"standard": {{Width: 168, Height: 44}, {Width: 232, Height: 62}},
	"mini":     {{Width: 176, Height: 28}, {Width: 232, Height: 46}},
}

const histLen = 30

func newSpeedWindow(a *App) *speedWindow { return &speedWindow{a: a} }

// apply shows, changes or closes the window as the settings say.
func (w *speedWindow) apply(st config.SpeedWindow) {
	if w == nil {
		return
	}
	w.mu.Lock()
	win, prev := w.win, w.cur
	w.cur = st
	w.mu.Unlock()
	if !st.Enabled {
		if win != nil {
			w.close()
		}
		return
	}
	if win == nil || win.IsDestroyed() {
		w.open(st)
		return
	}
	if prev.Opacity != st.Opacity {
		win.SetOpacity(float64(st.Opacity) / 100)
	}
	if prev.Locked != st.Locked {
		win.SetIgnoreMouseEvents(st.Locked)
	}
	if prev.Style != st.Style {
		win.Update(func() {
			w.style, w.expanded = st.Style, false
			w.resize(win)
		})
	}
}

func (w *speedWindow) open(st config.SpeedWindow) {
	size := speedSizes[st.Style][0]
	pos := speedPlace(st, size, mygo.Screen.Displays(), mygo.Screen.PrimaryDisplay())
	win := mygo.NewWindow(mygo.WindowOptions{
		Title: w.a.name, Width: size.Width, Height: size.Height, Hidden: true,
		Frameless: true, AlwaysOnTop: true, SkipTaskbar: true,
		DisableResize: true, DisableMinimize: true, DisableMaximize: true, DisableFullScreen: true,
		Content: ui.View(w.view),
	})
	ctx, cancel := context.WithCancel(w.a.ctx)
	w.mu.Lock()
	w.win, w.stop, w.full = win, cancel, false
	w.mu.Unlock()
	win.Update(func() {
		w.style, w.expanded, w.base = st.Style, false, pos
		w.hist = w.hist[:0]
	})
	win.SetPosition(pos.X, pos.Y)
	win.SetOpacity(float64(st.Opacity) / 100)
	win.SetIgnoreMouseEvents(st.Locked)
	win.SetVisibleOnAllWorkspaces(true)
	win.ShowInactive()
	go w.live(ctx, win)
}

func (w *speedWindow) close() {
	w.mu.Lock()
	win, stop := w.win, w.stop
	w.win, w.stop = nil, nil
	w.mu.Unlock()
	if stop != nil {
		stop()
	}
	if win != nil && !win.IsDestroyed() {
		win.Destroy()
	}
}

// live feeds the window the traffic and the state, and hides it while an
// app runs full screen (Windows).
func (w *speedWindow) live(ctx context.Context, win *mygo.Window) {
	ch, unsub := w.a.traffic.Subscribe()
	defer unsub()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	w.status(win)
	for {
		select {
		case <-ctx.Done():
			return
		case t, ok := <-ch:
			if !ok {
				return
			}
			win.Update(func() {
				w.traffic = t
				w.hist = append(w.hist, t.Down)
				if len(w.hist) > histLen {
					w.hist = w.hist[len(w.hist)-histLen:]
				}
			})
		case <-tick.C:
			w.status(win)
			full := fullScreenApp()
			w.mu.Lock()
			changed := full != w.full
			w.full = full
			w.mu.Unlock()
			if changed {
				if full {
					win.Hide()
				} else {
					win.ShowInactive()
				}
			}
		}
	}
}

// status reads the state the window shows besides the traffic.
func (w *speedWindow) status(win *mygo.Window) {
	s := w.a.snapshot()
	node := ""
	if t := w.a.tray; t != nil {
		t.mu.Lock()
		node = t.node
		t.mu.Unlock()
	}
	line := statusLine(w.a, s, node)
	dot := dotOff
	switch s.Core.Status {
	case coremgr.StatusRunning:
		if s.Tun || s.SystemProxy {
			dot = dotOn
		}
	case coremgr.StatusStarting:
		dot = dotBusy
	case coremgr.StatusError:
		dot = dotError
	}
	win.Update(func() { w.line, w.dot = line, dot })
}

// resize sizes the window for its style and whether it is expanded,
// growing toward the middle of the screen, so that it never leaves it.
// On the main thread.
func (w *speedWindow) resize(win *mygo.Window) {
	sizes := speedSizes[w.style]
	size := sizes[0]
	pos := w.base
	if w.expanded {
		size = sizes[1]
		wa := mygo.Screen.DisplayNearestPoint(w.base).WorkArea
		pos = growInside(w.base, sizes[0], size, wa)
	}
	win.SetBounds(mygo.Rectangle{X: pos.X, Y: pos.Y, Width: size.Width, Height: size.Height})
}

// view draws the window.
func (w *speedWindow) view(c *ui.Context) {
	a := w.a
	c.SetTheme(panelTheme(c.Theme().Dark, a.settings.Get().OLED))
	t := c.Theme()
	w.mu.Lock()
	win := w.win
	w.mu.Unlock()
	if win == nil {
		return
	}
	// As the main window's sidebar: upload in the second color, download
	// and its graph in the accent.
	upC, downC := ui.Hex("#9a6a00"), ui.Hex("#0b88bb")
	if t.Dark {
		upC, downC = ui.Hex("#f2e46e"), ui.Hex("#46acdb")
	}
	dot := t.TextMuted
	switch w.dot {
	case dotOn:
		dot = t.Accent
	case dotBusy:
		dot = t.Warning
	case dotError:
		dot = t.Danger
	}
	mini := w.style == "mini"

	root := ui.Column(c).Fill().Padding(0, 10).Gap(2).Justify(ui.Center).Background(t.Background).Border(1, t.Border)
	root.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
			up, down := "↑ "+shortRate(w.traffic.Up)+"/s", "↓ "+shortRate(w.traffic.Down)+"/s"
			if mini {
				ui.Text(c, up).FontSize(12).FontFeatures("tnum").TextColor(upC).Width(66)
				ui.Text(c, down).FontSize(12).FontFeatures("tnum").TextColor(downC).Width(66)
				return
			}
			ui.Column(c).Width(70).Children(func() {
				ui.Text(c, up).FontSize(12).FontFeatures("tnum").TextColor(upC)
				ui.Text(c, down).FontSize(12).FontFeatures("tnum").TextColor(downC)
			})
			ui.Spacer(c)
			hist := w.hist
			ui.Box(c).Size(52, 24).Draw(func(p *ui.Painter, r ui.Rect) { spark(p, r, hist, downC) })
		})
		if w.expanded {
			ui.Text(c, w.line).FontSize(11).TextColor(t.TextMuted).SingleLine()
		}
	})

	// Hovering shows the status line.
	if hover := root.Hovered() || w.pinned; hover != w.expanded && w.drag == nil {
		w.expanded = hover
		w.resize(win)
	}
	if root.DoubleClicked() {
		go a.showMain()
	}
	if root.RightClicked() {
		go a.tray.popup()
	}
	w.dragging(root, win)
}

// dragging moves the window with the pointer, from the screen's
// coordinates: the window's own move under the pointer as it follows
// would otherwise feed back into the drag.
func (w *speedWindow) dragging(root *ui.Element, win *mygo.Window) {
	_, _, pressed := root.Dragged()
	switch {
	case pressed && w.drag == nil:
		x, y := win.Position()
		w.drag = &dragStart{cursor: mygo.Screen.CursorScreenPoint(), win: mygo.Point{X: x, Y: y}}
	case pressed:
		cur := mygo.Screen.CursorScreenPoint()
		dx, dy := cur.X-w.drag.cursor.X, cur.Y-w.drag.cursor.Y
		if dx*dx+dy*dy >= 9 {
			w.drag.moved = true
		}
		if w.drag.moved {
			win.SetPosition(w.drag.win.X+dx, w.drag.win.Y+dy)
		}
	case w.drag != nil:
		moved := w.drag.moved
		w.drag = nil
		if !moved {
			return
		}
		x, y := win.Position()
		if w.expanded {
			// Back to the collapsed window's place.
			sizes := speedSizes[w.style]
			wa := mygo.Screen.DisplayNearestPoint(mygo.Point{X: x, Y: y}).WorkArea
			g := growInside(mygo.Point{X: x, Y: y}, sizes[0], sizes[1], wa)
			x, y = x-(g.X-x), y-(g.Y-y)
		}
		size := speedSizes[w.style][0]
		wa := mygo.Screen.DisplayNearestPoint(mygo.Point{X: x + size.Width/2, Y: y + size.Height/2}).WorkArea
		w.base = snapEdges(mygo.Point{X: x, Y: y}, size, wa)
		w.resize(win)
		go w.save(w.base)
		return
	}
	if w.drag != nil {
		root.Draw(func(p *ui.Painter, _ ui.Rect) { p.AnimationFrame() })
	}
}

func (w *speedWindow) save(p mygo.Point) {
	_, _, _ = w.a.settings.Update(func(s *config.Settings) {
		s.SpeedWindow.X, s.SpeedWindow.Y, s.SpeedWindow.Placed = p.X, p.Y, true
	})
}

// spark draws the downloads as a line, scaled to the largest.
func spark(p *ui.Painter, r ui.Rect, hist []int64, c ui.Color) {
	if len(hist) < 2 {
		return
	}
	peak := int64(1)
	for _, v := range hist {
		peak = max(peak, v)
	}
	var path ui.Path
	step := r.W / float32(histLen-1)
	x0 := r.X + r.W - step*float32(len(hist)-1)
	for i, v := range hist {
		x := x0 + step*float32(i)
		y := r.Y + r.H - 1 - (r.H-2)*float32(v)/float32(peak)
		if i == 0 {
			path.MoveTo(x, y)
		} else {
			path.LineTo(x, y)
		}
	}
	p.StrokePath(&path, 1.5, c)
}

// speedPlace is where the window opens: where it was left, if that is
// still on a screen, or else the primary screen's bottom right corner.
func speedPlace(st config.SpeedWindow, size mygo.Rectangle, displays []mygo.Display, primary mygo.Display) mygo.Point {
	if st.Placed {
		for _, d := range displays {
			wa := d.WorkArea
			if st.X >= wa.X && st.Y >= wa.Y && st.X+size.Width <= wa.X+wa.Width && st.Y+size.Height <= wa.Y+wa.Height {
				return mygo.Point{X: st.X, Y: st.Y}
			}
		}
	}
	wa := primary.WorkArea
	return mygo.Point{X: wa.X + wa.Width - size.Width - 16, Y: wa.Y + wa.Height - size.Height - 16}
}

// snapEdges keeps the window inside the work area, against an edge it was
// left near.
func snapEdges(p mygo.Point, size, wa mygo.Rectangle) mygo.Point {
	const near = 12
	right, bottom := wa.X+wa.Width-size.Width, wa.Y+wa.Height-size.Height
	if p.X-wa.X < near {
		p.X = wa.X
	} else if right-p.X < near {
		p.X = right
	}
	if p.Y-wa.Y < near {
		p.Y = wa.Y
	} else if bottom-p.Y < near {
		p.Y = bottom
	}
	return p
}

// growInside is where a window at p grows from small to big: away from
// the edges it is nearest to, so it stays in the work area.
func growInside(p mygo.Point, small, big, wa mygo.Rectangle) mygo.Point {
	if p.X+small.Width/2 > wa.X+wa.Width/2 {
		p.X -= big.Width - small.Width
	}
	if p.Y+small.Height/2 > wa.Y+wa.Height/2 {
		p.Y -= big.Height - small.Height
	}
	return p
}
