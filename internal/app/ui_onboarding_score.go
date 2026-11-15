package app

import (
	"math"
	"math/rand"
	"sort"
	"strings"
)

// The guide's scores, one a scene, made from the scene's own timing: the
// times below are those its drawing uses (ui_onboarding.go), the radar's
// answers its very functions. All of it is in A major, its pentatonic
// notes for what pops and lands; under each scene a chord, going round
// the key as the pages turn (A, F#m, D, E, C#m, A), so that the guide is a
// short piece of music. Made from sines, a plucked string and filtered
// noise: soft and short, not to be noticed more than the motion.

// Notes, in Hz.
const (
	nA2, nE3, nFs3, nGs3, nA3, nB3, nCs3, nD3 = 110.0, 164.81, 185.0, 207.65, 220.0, 246.94, 138.59, 146.83
	nCs4, nE4, nA4, nCs5, nD5, nE5, nFs5      = 277.18, 329.63, 440.0, 554.37, 587.33, 659.26, 739.99
	nGs5, nA5, nB5, nCs6, nD6, nE6, nFs6      = 830.61, 880.0, 987.77, 1108.73, 1174.66, 1318.51, 1479.98
	nA6, nB6, nCs7, nE7                       = 1760.0, 1975.53, 2217.46, 2637.02
)

// sndLead is how far a score runs ahead of the drawing: how far ahead the
// audio output mixes before it is heard (audio.LatencyInteractive).
const sndLead = 0.04

// onbScoreNames are the scores the guide plays.
var onbScoreNames = []string{"build", "fly", "home", "traffic", "nodes", "tiles", "radar", "more"}

// onbSceneScore is the score of a step, the welcome's when come back to.
func onbSceneScore(step int) string {
	return [...]string{"home", "traffic", "nodes", "tiles", "radar", "more"}[step]
}

// score is a sound being made.
type score struct {
	buf  []float64
	lead float64
	rnd  *rand.Rand
}

func newScore(seconds float64) *score {
	return &score{buf: make([]float64, int(seconds*sndRate)), rnd: rand.New(rand.NewSource(7))}
}

// from is the first sample of something at seconds into the scene.
func (s *score) from(at float64) int { return max(0, int((at-s.lead)*sndRate)) }

// add adds fn(t) from at on, for dur seconds.
func (s *score) add(at, dur float64, fn func(t float64) float64) {
	start := s.from(at)
	end := min(len(s.buf), start+int(dur*sndRate))
	for i := start; i < end; i++ {
		s.buf[i] += fn(float64(i-start) / sndRate)
	}
}

func attack(t, d float64) float64 {
	if t >= d {
		return 1
	}
	return 0.5 - 0.5*math.Cos(math.Pi*t/d)
}

// bell is a struck note, its higher partials fainter and quicker to die.
func (s *score) bell(at, f, amp, decay float64) {
	partials := [][3]float64{{1, 1, 1}, {2.0, 0.28, 0.6}, {2.76, 0.12, 0.45}, {5.4, 0.04, 0.3}}
	s.add(at, decay*7, func(t float64) float64 {
		var v float64
		for _, p := range partials {
			v += p[1] * math.Exp(-t/(decay*p[2])) * math.Sin(2*math.Pi*f*p[0]*t)
		}
		return amp * attack(t, 0.004) * v
	})
}

// tap is a small pop: a short round note with a breath of click.
func (s *score) tap(at, f, amp float64) {
	s.add(at, 0.3, func(t float64) float64 {
		bend := 1 + 0.04*math.Exp(-t/0.01) // it lands a hair sharp
		v := math.Sin(2*math.Pi*f*bend*t)*math.Exp(-t/0.07) + 0.25*math.Sin(4*math.Pi*f*t)*math.Exp(-t/0.025)
		return amp * attack(t, 0.002) * v
	})
	s.tick(at, amp*0.25)
}

// tick is the softest click.
func (s *score) tick(at, amp float64) {
	prev := 0.0
	s.add(at, 0.004, func(t float64) float64 {
		n := s.rnd.Float64()*2 - 1
		v := n - prev // high, no thud
		prev = n
		return amp * v * math.Exp(-t/0.0012)
	})
}

// knock is low, short, felt more than heard: a thing set down.
func (s *score) knock(at, f, amp float64) {
	s.add(at, 0.15, func(t float64) float64 {
		g := f - 60*t/0.12
		return amp * math.Exp(-t/0.035) * math.Sin(2*math.Pi*g*t) * math.Min(1, t/0.002)
	})
}

// pluck is a plucked string (Karplus–Strong), for a line drawn.
func (s *score) pluck(at, f, amp float64) {
	n := int(sndRate / f)
	ring := make([]float64, n)
	for i := range ring {
		ring[i] = s.rnd.Float64()*2 - 1
	}
	i := 0
	s.add(at, 1.4, func(t float64) float64 {
		v := ring[i%n]
		next := ring[(i+1)%n]
		ring[i%n] = 0.996 * 0.5 * (v + next)
		i++
		return amp * v * attack(t, 0.003)
	})
}

// swell is a breath of air, its brightness going from f0 to f1 Hz: a line
// drawing itself, a thing flying.
func (s *score) swell(at, dur, amp, f0, f1 float64) {
	lp := 0.0
	s.add(at, dur, func(t float64) float64 {
		x := t / dur
		fc := f0 * math.Pow(f1/f0, x)
		k := 1 - math.Exp(-2*math.Pi*fc/sndRate)
		lp += k * ((s.rnd.Float64()*2 - 1) - lp)
		env := math.Pow(math.Sin(math.Pi*x), 2)
		return amp * env * lp * 3
	})
}

// glide is a note sliding from f0 to f1 over dur: a thing that latches.
func (s *score) glide(at, dur, f0, f1, amp float64) {
	phase := 0.0
	s.add(at, dur+0.25, func(t float64) float64 {
		x := math.Min(1, t/dur)
		f := f0 * math.Pow(f1/f0, x*x*(3-2*x))
		phase += 2 * math.Pi * f / sndRate
		env := attack(t, 0.02) * math.Exp(-math.Max(0, t-dur)/0.08)
		return amp * env * (math.Sin(phase) + 0.2*math.Sin(2*phase))
	})
}

// pad is the chord under a scene: slow to come, breathing, slowly gone.
func (s *score) pad(at, amp float64, notes ...float64) {
	dur := float64(len(s.buf))/sndRate - at + s.lead
	s.add(at, dur, func(t float64) float64 {
		env := attack(t, 0.8) * math.Exp(-math.Max(0, t-1.0)/2.6) * (1 + 0.08*math.Sin(2*math.Pi*0.25*t))
		var v float64
		for i, f := range notes {
			d := 1 + 0.0018*float64(i%2*2-1) // a pair a little apart, for width
			v += math.Sin(2*math.Pi*f*t) + 0.6*math.Sin(2*math.Pi*f*d*t) + 0.12*math.Sin(4*math.Pi*f*t)
		}
		return amp * env * v / float64(len(notes))
	})
}

// byRank is notes given to delays: the fastest the highest.
func byRank(delays []int, notes []float64) []float64 {
	idx := make([]int, len(delays))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return delays[idx[a]] < delays[idx[b]] })
	out := make([]float64, len(delays))
	for rank, i := range idx {
		out[i] = notes[min(rank, len(notes)-1)]
	}
	return out
}

// chimeWAV is the landing's sound, on the home chord: the stars rise round
// the compass, a note each, and as they go into it the two bells sound.
func chimeWAV() []byte {
	s := newScore(3.4)
	s.knock(0, 180, 0.22)
	for i, f := range []float64{nA6, nB6, nCs7, nE7, nFs6 * 2} {
		s.tap(0.08+0.17*float64(i), f, 0.07)
		s.bell(0.08+0.17*float64(i), f, 0.03, 0.25)
	}
	s.swell(0.7, 0.4, 0.06, 1200, 5000)
	s.bell(onbStarsIn, nA5, 0.30, 0.5)
	s.bell(onbStarsIn+0.06, nE6, 0.24, 0.55)
	s.bell(onbStarsIn+0.12, nA6, 0.08, 0.3)
	return encodeWAV(s.buf, 0.36, 1.2)
}

// renderScore makes the score called name: a scene's, "-still" for its
// calm form with less motion, or the chime.
func renderScore(name string) []byte {
	if name == "chime" {
		return chimeWAV()
	}
	base, still := strings.CutSuffix(name, "-still")
	chords := map[string][]float64{
		"build": {nA3, nCs4, nE4}, "fly": {nA3, nCs4, nE4}, "home": {nA3, nCs4, nE4},
		"traffic": {nFs3, nA3, nCs4}, "nodes": {nD3, nFs3, nA3}, "tiles": {nE3, nGs3, nB3},
		"radar": {nCs3, nE3, nGs3}, "more": {nA2, nE3, nB3, nCs4},
	}
	chord, ok := chords[base]
	if !ok {
		return nil
	}
	if still {
		// With less motion nothing moves to sound with: the chord, and a
		// note as the page settles.
		s := newScore(3 + sndTail)
		s.lead = sndLead
		s.pad(0, 0.10, chord...)
		s.bell(0.12, chord[len(chord)-1]*4, 0.08, 0.4)
		return encodeWAV(s.buf, 0.26, sndRelease)
	}
	pent := []float64{nA5, nB5, nCs6, nE6, nFs6, nA6, nB6, nCs7}
	// A scene's score runs a little ahead of its drawing, and rings on past
	// its end (sndTail).
	scene := func(seconds float64) *score {
		s := newScore(seconds + sndTail)
		s.lead = sndLead
		return s
	}
	var s *score
	switch base {
	case "build":
		// The facets click into the compass one by one, rising; the wheel
		// draws itself round as a breath; the M lands with the chord; the
		// glints, and the band's five marks, twinkle in.
		s = scene(3.6)
		for i := range 8 {
			s.tap(0.05*float64(i)+0.27, pent[i], 0.16)
		}
		s.swell(0.05, 0.6, 0.10, 500, 3500)
		s.knock(0.8, 170, 0.25)
		s.bell(0.8, nA4, 0.14, 0.8)
		s.pad(0.72, 0.11, chord...)
		s.bell(1.12, nE7, 0.05, 0.22)
		for i := range 5 {
			s.tap(1.3+0.08*float64(i), pent[3+i%5]*2, 0.035)
		}
	case "fly", "home":
		// The compass comes on a breath and settles on the chord; the five
		// marks twinkle in after it.
		s = scene(3.2)
		arrive, marks := 0.5, 0.65
		if base == "home" {
			arrive, marks = 0.42, 0.3
		}
		s.swell(0, arrive+0.1, 0.08, 600, 2600)
		s.knock(arrive, 175, 0.18)
		s.bell(arrive, nA4, 0.12, 0.8)
		s.pad(arrive-0.08, 0.10, chord...)
		if base == "fly" {
			s.bell(0.58, nE7, 0.045, 0.22)
		}
		for i := range 5 {
			s.tap(marks+0.08*float64(i), pent[3+i%5]*2, 0.035)
		}
	case "traffic":
		// The apps pop up, the lines draw themselves, the world comes; then
		// the traffic flows, a drop as each packet passes the compass, and
		// the terminal's line latches into it, and lets go.
		s = scene(6.0)
		s.pad(0, 0.10, chord...)
		for i, f := range []float64{nE5, nFs5, nA5} {
			s.tap(0.25+0.07*float64(i), f, 0.13)
		}
		s.swell(0.3, 0.7, 0.06, 400, 2200)
		s.tap(0.68, nCs6, 0.11)
		s.bell(0.7, nCs6, 0.05, 0.3)
		for j := range 3 {
			for n := 0; ; n++ {
				t := (0.5 - float64(j)/3 + float64(n)) / 0.36
				if t > 5.8 {
					break
				}
				if t >= 1.0 {
					f := nA6
					if (n+j)%2 == 1 {
						f = nE6
					}
					s.tap(t, f, 0.03)
				}
			}
		}
		s.glide(onbModeStart, 0.6, nE5, nA5, 0.05)
		s.tick(onbModeStart+0.6, 0.12)
		s.glide(onbModeStart+onbModePeriod, 0.6, nA5, nE5, 0.045)
		s.tick(onbModeStart+onbModePeriod+0.6, 0.1)
	case "nodes":
		// The tiles pop; they tick while measured; each delay lands as a
		// note, the faster the higher; the fastest is picked with a ding,
		// and the needle swings to it.
		s = scene(4.6)
		s.pad(0, 0.10, chord...)
		for i, f := range []float64{nD5, nE5, nFs5, nA5, nB5, nD6} {
			s.tap(0.3+0.05*float64(i), f, 0.11)
		}
		for t := onbNodeStart + 0.05; t < onbNodeStart+0.35+0.1*5; t += 1.0 / 14 {
			s.tick(t, 0.025)
		}
		notes := byRank(onbDelays[0], []float64{nA6, nFs6, nE6, nCs6, nB5, nA5})
		for i, f := range notes {
			s.tap(onbNodeStart+0.35+0.1*float64(i)+0.05, f, 0.09)
		}
		pick := onbNodeStart + onbNodePicked
		s.glide(pick, 0.45, nA5, nD6, 0.04)
		s.bell(pick+0.2, nD6, 0.17, 0.5)
		s.bell(pick+0.26, nA6, 0.09, 0.4)
	case "tiles":
		// The window grows, its tiles pop; the pointer comes and presses;
		// the tile blooms open as a rising arpeggio, its rows ticking in;
		// it folds back down, and settles.
		s = scene(5.4)
		s.pad(0, 0.10, chord...)
		s.swell(0, 0.55, 0.05, 300, 1500)
		for i, f := range []float64{nE5, nGs5, nB5, nCs6, nE6} {
			s.tap(0.48+0.06*float64(i), f, 0.1)
		}
		s.swell(1.0, 0.6, 0.025, 800, 1400)
		s.tick(1.62, 0.08)
		s.tap(1.62, 600, 0.05)
		for i, f := range []float64{nE5, nGs5, nB5, nE6} {
			s.pluck(1.75+0.1*float64(i), f, 0.09)
		}
		s.swell(1.75, 0.6, 0.04, 600, 3000)
		for j := range 4 {
			s.tick(2.1+0.05*float64(j), 0.05)
		}
		for i, f := range []float64{nE6, nB5, nGs5} {
			s.pluck(4.3+0.1*float64(i), f, 0.06)
		}
		s.knock(4.9, 150, 0.12)
	case "radar":
		// The rings draw themselves; the compass sends each ripple out with
		// a low pulse; each service answers as it is reached, a note and its
		// echo, the faster the higher; later ripples answer softly.
		s = scene(5.2)
		s.pad(0, 0.10, chord...)
		for i := range 3 {
			s.swell(0.05+0.08*float64(i), 0.8, 0.025, 500, 1800)
		}
		delays := make([]int, len(onbSites))
		for i, site := range onbSites {
			delays[i] = site.delay
		}
		notes := byRank(delays, []float64{nE6, nCs6, nB5, nA5, nFs5, nE5})
		for k := 0; k < 3; k++ {
			at, _ := onbPulse(k)
			if at > 5.0 {
				break
			}
			amp := 0.07
			if k > 0 {
				amp = 0.04
			}
			s.glide(float64(at), 0.35, 300, 190, amp)
			for i, site := range onbSites {
				t := float64(onbAnswer(k, site.delay))
				if t > 5.0 {
					continue
				}
				a := 0.15
				if k > 0 {
					a = 0.05
				}
				s.bell(t, notes[i], a, 0.42)
				s.bell(t+0.21, notes[i], a*0.3, 0.3)
			}
		}
	case "more":
		// The devices pop; the mesh plucks itself together; the cloud and
		// its devices come; the lock clicks shut; sync shimmers between.
		s = scene(3.8)
		s.pad(0, 0.10, chord...)
		for i, f := range []float64{nA5, nCs6, nE6, nFs6} {
			s.tap(0.27+0.07*float64(i), f, 0.1)
		}
		for j, f := range []float64{nA4, nCs5, nE5, nFs5, nA5, nB5} {
			s.pluck(0.4+0.07*float64(j), f, 0.1)
		}
		s.tap(0.37, nE6, 0.09)
		s.tap(0.47, nB5, 0.07)
		s.tap(0.55, nCs6, 0.07)
		s.tick(1.1, 0.12)
		s.bell(1.1, nE7*1.33, 0.04, 0.08)
		for i := 0; 1.3+0.28*float64(i) < 3.3; i++ {
			f := nE6
			if i%2 == 1 {
				f = nB6
			}
			s.tap(1.3+0.28*float64(i), f, 0.035*(1-float64(i)/8))
		}
	}
	return encodeWAV(s.buf, 0.30, sndRelease)
}
