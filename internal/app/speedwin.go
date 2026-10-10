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
// for whoever wants it elsewhere. It is dragged anywhere on a screen, the
// taskbar too, and stays where it is left; a double click opens the main
// window and a right click the tray's menu.
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
	style string
	drag  *dragStart
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

// The window's sizes: the standard one has the status line and the graph,
// the mini one only the traffic, low enough for a taskbar.
var speedSizes = map[string]mygo.Rectangle{
	"standard": {Width: 212, Height: 46},
	"mini":     {Width: 176, Height: 28},
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
			w.style = st.Style
			x, y := win.Position()
			size := speedSizes[w.style]
			win.SetBounds(mygo.Rectangle{X: x, Y: y, Width: size.Width, Height: size.Height})
		})
	}
}

func (w *speedWindow) open(st config.SpeedWindow) {
	size := speedSizes[st.Style]
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
		w.style = st.Style
		w.hist = w.hist[:0]
	})
	win.SetPosition(pos.X, pos.Y)
	win.SetOpacity(float64(st.Opacity) / 100)
	win.SetIgnoreMouseEvents(st.Locked)
	win.SetVisibleOnAllWorkspaces(true)
	toolWindow(win)
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

// live feeds the window the traffic and the state, keeps it over the
// taskbar, and hides it while an app runs full screen (Windows).
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
			keepOnTop(win)
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
	up, down := "↑ "+shortRate(w.traffic.Up)+"/s", "↓ "+shortRate(w.traffic.Down)+"/s"

	root := ui.Row(c).Fill().Padding(0, 10).Gap(8).AlignItems(ui.Center).Background(t.Background).Border(1, t.Border)
	root.Children(func() {
		if w.style == "mini" {
			ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
			ui.Text(c, up).FontSize(12).FontFeatures("tnum").TextColor(upC).Width(66)
			ui.Text(c, down).FontSize(12).FontFeatures("tnum").TextColor(downC).Width(66)
			return
		}
		ui.Column(c).Grow(1).Gap(3).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Box(c).Size(7, 7).Radius(3.5).Background(dot)
				ui.Text(c, w.line).FontSize(11).TextColor(t.TextMuted).SingleLine()
			})
			ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
				ui.Text(c, up).FontSize(12).FontFeatures("tnum").TextColor(upC).Width(70)
				ui.Text(c, down).FontSize(12).FontFeatures("tnum").TextColor(downC).Width(70)
				ui.Spacer(c)
				hist := w.hist
				ui.Box(c).Size(44, 16).Draw(func(p *ui.Painter, r ui.Rect) { spark(p, r, hist, downC) })
			})
		})
	})

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
func (w *speedWindow) dragging(root ui.Element, win *mygo.Window) {
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
		size := speedSizes[w.style]
		d := mygo.Screen.DisplayNearestPoint(mygo.Point{X: x + size.Width/2, Y: y + size.Height/2})
		p := snapEdges(mygo.Point{X: x, Y: y}, size, d.Bounds, d.WorkArea)
		win.SetPosition(p.X, p.Y)
		go w.save(p)
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
			b := d.Bounds
			if st.X >= b.X && st.Y >= b.Y && st.X+size.Width <= b.X+b.Width && st.Y+size.Height <= b.Y+b.Height {
				return mygo.Point{X: st.X, Y: st.Y}
			}
		}
	}
	wa := primary.WorkArea
	return mygo.Point{X: wa.X + wa.Width - size.Width - 16, Y: wa.Y + wa.Height - size.Height - 16}
}

// snapEdges keeps the window on the screen whose bounds are b, against an
// edge of its work area wa it was left near. Over the taskbar, outside the
// work area, it stays where it is put.
func snapEdges(p mygo.Point, size, b, wa mygo.Rectangle) mygo.Point {
	p.X = snapAxis(p.X, size.Width, b.X, b.X+b.Width, wa.X, wa.X+wa.Width)
	p.Y = snapAxis(p.Y, size.Height, b.Y, b.Y+b.Height, wa.Y, wa.Y+wa.Height)
	return p
}

// snapAxis places a span of n at v on one axis, between lo and hi, moved to
// an edge of the work area between waLo and waHi when it is near it and
// mostly inside.
func snapAxis(v, n, lo, hi, waLo, waHi int) int {
	const near = 12
	switch {
	case abs(v-waLo) < near:
		v = waLo
	case abs(v+n-waHi) < near:
		v = waHi - n
	}
	return min(max(v, lo), hi-n)
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
