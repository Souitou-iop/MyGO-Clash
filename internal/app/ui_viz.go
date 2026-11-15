package app

import (
	"cmp"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

// The drawing that leads the connections: the flow of the connections
// (apps through the compass to their exits).

// vizState is what the drawings keep between frames.
type vizState struct {
	frameAt time.Time
	dt      float32

	parts  []flowPart
	acc    map[string]float32
	spin   float32
	pulse  float32
	sumOut map[string]float32 // eased rates of the exits
}

type flowPart struct {
	src, dst string
	f, v     float32
	up       bool
}

// frame advances the drawings' clock once a frame.
func (m *mainUI) vizFrame(c *ui.Context) {
	v := &m.viz
	now := c.Now()
	v.dt = float32(now.Sub(v.frameAt).Seconds())
	if v.frameAt.IsZero() || v.dt > 0.1 || v.dt < 0 {
		v.dt = 0
	}
	v.frameAt = now
}

// moving reports whether the drawings should move: the window shows, is in
// front or may move behind, and the user did not ask for less motion.
func (m *mainUI) moving(c *ui.Context) bool {
	return !c.WindowHidden() && !c.Preferences().ReduceMotion && (c.WindowActive() || !m.st.UI.PauseOnBlur)
}

func clamp01(x float64) float64 { return math.Max(0, math.Min(1, x)) }

func ease(x, target, dt, rate float32) float32 {
	return x + (target-x)*float32(1-math.Exp(float64(-dt*rate)))
}

// countryCenters places a country by its middle, for addresses whose
// lookup gave no coordinates.
var countryCenters = map[string][2]float64{
	"CN": {35.0, 105.0}, "HK": {22.3, 114.2}, "MO": {22.2, 113.5}, "TW": {23.7, 121.0}, "JP": {36.2, 138.3}, "KR": {36.5, 127.9},
	"SG": {1.35, 103.8}, "MY": {4.2, 101.9}, "TH": {15.9, 100.9}, "VN": {14.1, 108.3}, "PH": {12.9, 121.8}, "ID": {-0.8, 113.9},
	"IN": {20.6, 78.9}, "AU": {-25.3, 133.8}, "NZ": {-40.9, 174.9}, "US": {37.1, -95.7}, "CA": {56.1, -106.3}, "MX": {23.6, -102.6},
	"BR": {-14.2, -51.9}, "AR": {-38.4, -63.6}, "CL": {-35.7, -71.5}, "GB": {55.4, -3.4}, "IE": {53.4, -8.2}, "FR": {46.2, 2.2},
	"DE": {51.2, 10.5}, "NL": {52.1, 5.3}, "BE": {50.5, 4.5}, "CH": {46.8, 8.2}, "AT": {47.5, 14.6}, "IT": {41.9, 12.6},
	"ES": {40.5, -3.7}, "PT": {39.4, -8.2}, "SE": {60.1, 18.6}, "NO": {60.5, 8.5}, "FI": {61.9, 25.7}, "DK": {56.3, 9.5},
	"PL": {51.9, 19.1}, "CZ": {49.8, 15.5}, "UA": {48.4, 31.2}, "RU": {61.5, 105.3}, "TR": {38.9, 35.2}, "IL": {31.0, 34.9},
	"AE": {23.4, 53.8}, "SA": {23.9, 45.1}, "IR": {32.4, 53.7}, "EG": {26.8, 30.8}, "ZA": {-30.6, 22.9}, "NG": {9.1, 8.7},
	"KZ": {48.0, 66.9}, "PK": {30.4, 69.3}, "BD": {23.7, 90.4}, "LU": {49.8, 6.1}, "RO": {45.9, 24.9}, "HU": {47.2, 19.5},
}

// ---- The flow of the connections ----

type flowNode struct {
	name, sub string
	rate      int64
	exit      string
	blocked   bool
}

// flowView draws the connections as they flow: the busiest apps on the
// left, the compass in the middle as the rules, their exits on the right.
// Lanes are as thick as what they carry, dots go along them, the compass
// turns with the traffic, and what is rejected stops at it.
func (m *mainUI) flowView(c *ui.Context, conns []*coreapi.Connection) {
	v, p := &m.viz, m.pal
	if v.acc == nil {
		v.acc, v.sumOut = map[string]float32{}, map[string]float32{}
	}
	// The apps by what they carry, and where most of it goes.
	type agg struct {
		rate  int64
		n     int
		exits map[string]int64
		host  string
	}
	apps := map[string]*agg{}
	outs, outN := map[string]int64{}, map[string]int{}
	var total int64
	for _, cn := range conns {
		name := cmp.Or(cn.Metadata.Process, cn.Metadata.Host, cn.Metadata.DestinationIP)
		exit := "DIRECT"
		if len(cn.Chains) > 0 {
			exit = cn.Chains[0]
		}
		r := cn.DownloadSpeed + cn.UploadSpeed
		a := apps[name]
		if a == nil {
			a = &agg{exits: map[string]int64{}, host: cmp.Or(cn.Metadata.Host, cn.Metadata.DestinationIP)}
			apps[name] = a
		}
		a.rate += r
		a.n++
		a.exits[exit] += r + 1
		outs[exit] += r
		outN[exit]++
		total += r
	}
	srcs := make([]flowNode, 0, len(apps))
	for name, a := range apps {
		best, bv := "", int64(-1)
		for e, r := range a.exits {
			if r > bv {
				best, bv = e, r
			}
		}
		sub := a.host
		if a.n > 1 {
			sub = locales.FlowConns(fmt.Sprint(a.n))
		}
		srcs = append(srcs, flowNode{name: name, sub: sub, rate: a.rate, exit: best, blocked: best == "REJECT" || best == "REJECT-DROP"})
	}
	sort.Slice(srcs, func(i, j int) bool {
		return srcs[i].rate > srcs[j].rate || srcs[i].rate == srcs[j].rate && srcs[i].name < srcs[j].name
	})
	srcs = srcs[:min(7, len(srcs))]
	exits := make([]string, 0, len(outs))
	for e := range outs {
		exits = append(exits, e)
	}
	sort.Slice(exits, func(i, j int) bool {
		a, b := exits[i], exits[j]
		return cmp.Or(cmp.Compare(outs[b], outs[a]), cmp.Compare(outN[b], outN[a]), strings.Compare(a, b)) < 0
	})
	exits = exits[:min(6, len(exits))]
	// Exits of the apps shown that fell off the right go to the last one.
	for i := range srcs {
		if !slices.Contains(exits, srcs[i].exit) && len(exits) > 0 {
			srcs[i].exit = exits[len(exits)-1]
		}
	}
	// Dots: as many as an app carries, a few at least while it is open.
	dt := v.dt
	for _, s := range srcs {
		v.acc[s.name] += dt * (1.2 + 9*float32(math.Sqrt(clamp01(float64(s.rate)/2.2e6))))
		for v.acc[s.name] > 1 {
			v.acc[s.name]--
			v.parts = append(v.parts, flowPart{src: s.name, dst: s.exit, v: 0.55 + float32(time.Now().UnixNano()%250)/1000, up: time.Now().UnixNano()%5 == 0})
		}
	}
	kept := v.parts[:0]
	for _, pa := range v.parts {
		pa.f += dt * pa.v
		blocked := pa.dst == "REJECT" || pa.dst == "REJECT-DROP"
		if pa.f >= 1 && blocked {
			v.pulse = 1
			continue
		}
		if pa.f < 2 && slices.ContainsFunc(srcs, func(s flowNode) bool { return s.name == pa.src }) {
			kept = append(kept, pa)
		}
	}
	v.parts = kept
	for _, e := range exits {
		v.sumOut[e] = ease(v.sumOut[e], float32(outs[e]), dt, 4)
	}
	v.spin += dt * (8 + 120*float32(clamp01(float64(total)/4e6)))
	v.pulse = max(0, v.pulse-dt*6)
	if m.moving(c) && len(srcs) > 0 {
		c.AnimationFrame()
	}
	parts, spin, pulse := slices.Clone(v.parts), v.spin, v.pulse
	exitCol := func(e string) ui.Color {
		switch {
		case strings.HasPrefix(e, "REJECT"):
			return p.Danger
		case e == "DIRECT":
			return p.TextMuted
		}
		return p.Accent
	}
	box := ui.Box(c.Key("flow")).Grow(1).MinHeight(320)
	box.Draw(func(pt *ui.Painter, r ui.Rect) {
		if len(srcs) == 0 {
			return
		}
		lx := r.X + max(190, r.W*0.3)
		rx := r.X + r.W - max(170, r.W*0.26)
		hx, hy := (lx+rx)/2, r.Y+r.H/2
		hr := min(float32(42), r.H*0.11)
		top := r.Y + 16
		sy := func(i int) float32 { return top + (float32(i)+0.5)*((r.H-32)/float32(len(srcs))) }
		ey := func(j int) float32 { return top + 10 + (float32(j)+0.5)*((r.H-52)/float32(len(exits))) }
		in := func(i int) [8]float32 {
			return [8]float32{lx, sy(i), lx + (hx-lx)*0.55, sy(i), lx + (hx-lx)*0.5, hy, hx - hr - 6, hy}
		}
		out := func(j int) [8]float32 {
			return [8]float32{hx + hr + 6, hy, hx + (rx-hx)*0.5, hy, hx + (rx-hx)*0.45, ey(j), rx, ey(j)}
		}
		curve := func(q [8]float32) *ui.Path {
			return (&ui.Path{}).MoveTo(q[0], q[1]).CubeTo(q[2], q[3], q[4], q[5], q[6], q[7])
		}
		bez := func(q [8]float32, f float32) (float32, float32) {
			u := 1 - f
			return u*u*u*q[0] + 3*u*u*f*q[2] + 3*u*f*f*q[4] + f*f*f*q[6], u*u*u*q[1] + 3*u*u*f*q[3] + 3*u*f*f*q[5] + f*f*f*q[7]
		}
		for i, s := range srcs {
			w := 1 + 5*float32(math.Sqrt(clamp01(float64(s.rate)/2.2e6)))
			pt.StrokePath(curve(in(i)), w, p.TextMuted.Alpha(0.22))
		}
		for j, e := range exits {
			w := 1 + 6*float32(math.Sqrt(clamp01(float64(v.sumOut[e])/2.6e6)))
			pt.StrokePath(curve(out(j)), w, exitCol(e).Alpha(0.3))
		}
		for _, pa := range parts {
			i := slices.IndexFunc(srcs, func(s flowNode) bool { return s.name == pa.src })
			j := slices.Index(exits, pa.dst)
			if i < 0 {
				continue
			}
			e := pa.f
			if e >= 1 {
				e--
			}
			e = e * e * (3 - 2*e)
			if pa.f < 1 {
				x, y := bez(in(i), e)
				pt.FillPath((&ui.Path{}).Circle(x, y, 2.2), p.Text2.Alpha(0.8))
				continue
			}
			if j < 0 {
				continue
			}
			x, y := bez(out(j), e)
			if pa.up {
				drawSparkle(pt, x, y, 4.2, p.Spark)
			} else {
				pt.FillPath((&ui.Path{}).Circle(x, y, 2.6), exitCol(pa.dst))
			}
		}
		// The compass, as the rules.
		ring := (&ui.Path{}).Circle(hx, hy, hr+6)
		pt.FillPath(ring, p.Card)
		col := p.BorderStrong
		if pulse > 0 {
			col = p.Danger.Alpha(0.3 + pulse*0.7)
		}
		pt.StrokePath(ring, 1, col)
		for k := 0; k < 24; k++ {
			a := float64(k)*15*math.Pi/180 + float64(spin)*math.Pi/360
			r0, r1 := hr+6, hr+9
			if k%3 == 0 {
				r1 = hr + 12
			}
			cs, sn := float32(math.Cos(a)), float32(math.Sin(a))
			pt.StrokePath((&ui.Path{}).MoveTo(hx+cs*r0, hy+sn*r0).LineTo(hx+cs*r1, hy+sn*r1), 1, p.BorderStrong)
		}
		drawCompass(pt, hx, hy, hr*2.25, compassLook{Turn: spin})
		rt := ui.Span{Text: fmtRate(total), Size: 11, Weight: 500, Color: p.TextMuted, Features: "tnum"}
		w, _ := pt.MeasureText(0, rt)
		pt.RichText(hx-w/2, hy+hr+18, 0, rt)
		// The names.
		fit := func(s string, size, width float32) string {
			for len([]rune(s)) > 4 {
				if w, _ := pt.MeasureText(0, ui.Span{Text: s, Size: size}); w <= width {
					return s
				}
				rs := []rune(s)
				s = string(rs[:len(rs)-2]) + "…"
			}
			return s
		}
		for i, s := range srcs {
			y := sy(i)
			pt.RichText(r.X+4, y-16, 0, ui.Span{Text: fit(s.name, 12.5, lx-r.X-90), Size: 12.5, Weight: 600, Color: p.Text})
			pt.RichText(r.X+4, y+1, 0, ui.Span{Text: fit(s.sub, 11, lx-r.X-90), Size: 11, Color: p.TextMuted})
			rate := ui.Span{Text: fmtRate(s.rate), Size: 11, Weight: 500, Color: p.Text2, Features: "tnum"}
			if s.blocked {
				rate = ui.Span{Text: "×", Size: 12, Weight: 600, Color: p.Danger}
			}
			w, h := pt.MeasureText(0, rate)
			pt.RichText(lx-10-w, y-h/2, 0, rate)
		}
		for j, e := range exits {
			y := ey(j)
			dot := (&ui.Path{}).Circle(rx+1, y, 4)
			pt.FillPath(dot, p.Card)
			pt.StrokePath(dot, 1.6, exitCol(e))
			pt.RichText(rx+14, y-16, 0, ui.Span{Text: fit(e, 12.5, r.X+r.W-rx-20), Size: 12.5, Weight: 600, Color: p.Text})
			if !strings.HasPrefix(e, "REJECT") {
				pt.RichText(rx+14, y+1, 0, ui.Span{Text: fmtRate(int64(v.sumOut[e])), Size: 11, Color: p.TextMuted, Features: "tnum"})
			}
		}
	})
}
