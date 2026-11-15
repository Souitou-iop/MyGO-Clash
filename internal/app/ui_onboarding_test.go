package app

import (
	"fmt"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
)

// onbTester shows the guide alone over an empty window, played from the
// settings unless first.
func onbTester(t *testing.T, dark, first bool) (*mainUI, *ui.Tester) {
	t.Helper()
	m := &mainUI{}
	m.st.Theme = "system"
	m.st.UI.OnboardingSeen = !first
	m.st.UI.CollapseNav = true
	m.onb.open = !first
	tt := ui.NewTester(func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(dark, false, "", "")
		c.SetTheme(m.pal.Theme)
		ui.Box(c).Fill().Background(m.pal.Theme.Background)
		m.onboardingView(c)
	}, 880, 640)
	tt.SetDark(dark)
	tt.Frame()
	return m, tt
}

func TestOnboardingKeys(t *testing.T) {
	m, tt := onbTester(t, true, false)
	if !m.onb.shown {
		t.Fatal("the guide does not show when played from the settings")
	}
	step := func(key ui.Key, want int) {
		t.Helper()
		tt.Key(0, key)
		tt.Advance(50 * time.Millisecond)
		if m.onb.step != want {
			t.Fatalf("after %v: step %d, want %d", key, m.onb.step, want)
		}
	}
	step(ui.KeyRight, 1)
	step(ui.KeyRight, 2)
	step(ui.KeyLeft, 1)
	step(ui.KeyEnter, 2)
	step(ui.KeyForward, 3)
	step(ui.KeyBack, 2)
	for range 5 {
		tt.Key(0, ui.KeyRight)
		tt.Advance(50 * time.Millisecond)
	}
	if m.onb.step != onbSteps-1 {
		t.Fatalf("step %d past the end", m.onb.step)
	}
	tt.Key(0, ui.KeyEscape)
	tt.Advance(50 * time.Millisecond)
	if m.onb.closing.IsZero() {
		t.Fatal("Escape does not end the guide")
	}
	tt.Advance(time.Duration(onbClosing*float32(time.Second)) + 100*time.Millisecond)
	if m.onb.shown || m.onb.open {
		t.Error("the guide is still there after it ended")
	}
}

func TestOnboardingSwipe(t *testing.T) {
	m, tt := onbTester(t, false, false)
	tt.Advance(600 * time.Millisecond)
	// Two fingers going left: the content moves right, to the next step.
	for range 6 {
		tt.Scroll(440, 300, 14, 1)
		tt.Advance(16 * time.Millisecond)
	}
	tt.Advance(50 * time.Millisecond)
	if m.onb.step != 1 {
		t.Fatalf("swipe: step %d, want 1", m.onb.step)
	}
	// The same swipe, still going, turns no second page.
	for range 6 {
		tt.Scroll(440, 300, 14, 1)
		tt.Advance(16 * time.Millisecond)
	}
	if m.onb.step != 1 {
		t.Fatalf("one swipe turned %d pages", m.onb.step)
	}
	tt.Advance(time.Second)
	for range 6 {
		tt.Scroll(440, 300, -14, 0)
		tt.Advance(16 * time.Millisecond)
	}
	tt.Advance(50 * time.Millisecond)
	if m.onb.step != 0 {
		t.Fatalf("swipe back: step %d, want 0", m.onb.step)
	}
}

func TestOnboardingCompassArrives(t *testing.T) {
	m := &mainUI{}
	stage := ui.Rect{X: 100, Y: 60, W: 680, H: 300}
	m.onb = onboarding{step: 0, at: time.Unix(100, 0), from: onbPose{X: 30, Y: 60, Size: 34, Turn: 0}}
	at := func(d time.Duration) onbPose { return m.onbPoseAt(m.onb.at.Add(d), stage, false) }
	if p := at(0); p.X != 30 || p.Size != 34 {
		t.Errorf("start %+v, want where it came from", p)
	}
	want := m.onbScenePose(0, 3, stage, false)
	if p := at(3 * time.Second); abs32(p.X-want.X) > 0.5 || abs32(p.Size-want.Size) > 0.5 || abs32(p.Turn-want.Turn) > 0.5 {
		t.Errorf("end %+v, want %+v", p, want)
	}
	// The needle swings past its mark before it settles.
	m.onb.from.Turn = -60
	m.onb.step = 1
	target := m.onbScenePose(1, 0, stage, false).Turn
	past := false
	for d := time.Duration(0); d < 2*time.Second; d += 10 * time.Millisecond {
		if at(d).Turn > target+1 {
			past = true
		}
	}
	if !past {
		t.Error("the needle settles without swinging")
	}
}

// TestOnboardingShots writes frames of the guide to $MYGO_ONB_SHOTS, to
// look at: each step settled and in motion, both themes, and Arabic.
func TestOnboardingShots(t *testing.T) {
	dir := os.Getenv("MYGO_ONB_SHOTS")
	if dir == "" {
		t.Skip("MYGO_ONB_SHOTS not set")
	}
	save := func(tt *ui.Tester, name string) {
		f, err := os.Create(filepath.Join(dir, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	for _, lang := range []string{"zh-Hans", "ar"} {
		i18n.SetLocaleFunc(func() string { return lang })
		for _, dark := range []bool{false, true} {
			theme := "light"
			if dark {
				theme = "dark"
			}
			if lang == "ar" && !dark {
				continue
			}
			_, tt := onbTester(t, dark, true)
			if lang == "ar" {
				tt.SetDirection(ui.RightToLeft)
			}
			tt.SetScale(2)
			pre := lang + "-" + theme
			// The compass assembling.
			last := time.Duration(0)
			for _, ms := range []int{200, 450, 700} {
				tt.Advance(time.Duration(ms)*time.Millisecond - last)
				last = time.Duration(ms) * time.Millisecond
				if lang == "zh-Hans" {
					save(tt, fmt.Sprintf("%s-0-build-%03d", pre, ms))
				}
			}
			tt.Advance(3 * time.Second)
			save(tt, pre+"-0")
			fwd := ui.KeyRight
			if lang == "ar" {
				fwd = ui.KeyLeft // right to left, the next page is on the left
			}
			for step := 1; step < onbSteps; step++ {
				tt.Key(0, fwd)
				if lang == "zh-Hans" {
					tt.Advance(180 * time.Millisecond)
					save(tt, fmt.Sprintf("%s-%d-mid", pre, step))
				}
				if step == 4 && lang == "zh-Hans" {
					// The first ripple going out, and the services answering.
					at := 180 * time.Millisecond
					for j, ms := range []int{600, 900, 1150, 1400, 1650, 1900, 2300, 2900} {
						d := time.Duration(ms) * time.Millisecond
						tt.Advance(d - at)
						at = d
						save(tt, fmt.Sprintf("%s-4-sweep-%d", pre, j))
					}
					tt.Advance(3200*time.Millisecond - at)
				} else {
					tt.Advance(3200 * time.Millisecond)
				}
				save(tt, fmt.Sprintf("%s-%d", pre, step))
			}
			if lang == "zh-Hans" {
				// Played again: out of the sidebar's mark, and home to it.
				_, tt := onbTester(t, dark, false)
				tt.SetScale(2)
				tt.Advance(200 * time.Millisecond)
				save(tt, pre+"-replay-in")
				tt.Advance(2 * time.Second)
				for range onbSteps - 1 {
					tt.Key(0, ui.KeyRight)
					tt.Advance(100 * time.Millisecond)
				}
				tt.Advance(time.Second)
				tt.Key(0, ui.KeyEnter)
				tt.Advance(330 * time.Millisecond)
				save(tt, pre+"-home-mid")
			}
		}
	}
	i18n.SetLocaleFunc(nil)
}

func TestScores(t *testing.T) {
	names := append([]string{"chime"}, onbScoreNames...)
	for _, n := range onbScoreNames {
		names = append(names, n+"-still")
	}
	dir := os.Getenv("MYGO_ONB_SOUNDS") // to listen: written there, and played
	for _, name := range names {
		wav := renderScore(name)
		if len(wav) < 44 || string(wav[:4]) != "RIFF" || string(wav[8:16]) != "WAVEfmt " || string(wav[36:40]) != "data" {
			t.Fatalf("%s: not a WAV", name)
		}
		var peak, last int
		for i := 44; i+1 < len(wav); i += 2 {
			v := int(int16(uint16(wav[i]) | uint16(wav[i+1])<<8))
			peak = max(peak, v, -v)
			last = v
		}
		// Gentle at its loudest, about a third of full scale (the burst a
		// little more), and it ends in silence, with no click.
		if peak < 7000 || peak > 14000 {
			t.Errorf("%s: peak %d", name, peak)
		}
		if last != 0 {
			t.Errorf("%s: ends at %d", name, last)
		}
		// It dies away: over its last seconds, no tenth of a second is far
		// quieter than the one before, as a sound cut short would be.
		const win = 4410
		samples := (len(wav) - 44) / 2
		prev := 0.0
		for start := max(0, samples-30*win); start+win <= samples; start += win {
			var sum float64
			for i := start; i < start+win; i++ {
				v := float64(int16(uint16(wav[44+2*i]) | uint16(wav[45+2*i])<<8))
				sum += v * v
			}
			rms := math.Sqrt(sum / win)
			if start+3*win > samples && start+2*win <= samples && rms > float64(peak)/100 {
				t.Errorf("%s: still at %.0f of %d just before its end", name, rms, peak)
			}
			if prev > float64(peak)/100 && rms < prev*0.35 {
				t.Errorf("%s: drops from %.0f to %.0f at %.1fs", name, prev, rms, float64(start)/sndRate)
			}
			prev = rms
		}
		if dir != "" {
			path := filepath.Join(dir, name+".wav")
			if err := os.WriteFile(path, wav, 0o644); err != nil {
				t.Fatal(err)
			}
			if os.Getenv("MYGO_ONB_PLAY") != "" {
				if err := playWAV(wav); err != nil {
					t.Errorf("%s: play: %v", name, err)
				}
			}
		}
	}
	if renderScore("nothing") != nil {
		t.Error("a score for nothing")
	}
}

// TestLandingFilm writes the frames of the landing, at 60 a second, to
// $MYGO_ONB_FILM/, over a sketch of the window behind the guide: the rail,
// with the sidebar's mark where the real one is, and some tiles; and its
// sound, with lead.txt, the seconds into the frames it starts.
func TestLandingFilm(t *testing.T) {
	out := os.Getenv("MYGO_ONB_FILM")
	if out == "" {
		t.Skip("MYGO_ONB_FILM not set")
	}
	m := &mainUI{}
	m.st.Theme = "system"
	m.st.UI.OnboardingSeen = true
	m.st.UI.CollapseNav = true
	m.onb.open = true
	tt := ui.NewTester(func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(true, false, "", "")
		p := m.pal
		c.SetTheme(p.Theme)
		brand := m.onbBrand(c, 880)
		ui.Box(c).Fill().Background(p.Theme.Background).Draw(func(pt *ui.Painter, r ui.Rect) {
			drawCompass(pt, brand.X, brand.Y, brand.Size, compassLook{Ring: true})
			for i := range 6 {
				pt.Fill(ui.Rect{X: brand.X - 10, Y: 118 + float32(i)*46, W: 20, H: 20}, p.Faint.Alpha(0.6), 6)
			}
			pt.Fill(ui.Rect{X: 92, Y: 20, W: 120, H: 18}, p.Text.Alpha(0.85), 6)
			for i, rr := range []ui.Rect{{X: 92, Y: 64, W: 370, H: 230}, {X: 474, Y: 64, W: 386, H: 230}, {X: 92, Y: 306, W: 470, H: 170}, {X: 574, Y: 306, W: 286, H: 170}, {X: 92, Y: 488, W: 370, H: 140}, {X: 474, Y: 488, W: 386, H: 140}} {
				pt.Fill(rr, p.Card, tileRadius)
				pt.Fill(ui.Rect{X: rr.X + 20, Y: rr.Y + 22, W: 90 + float32(i%3)*30, H: 12}, p.Surface3, 6)
			}
		})
		m.onboardingView(c)
	}, 880, 640)
	tt.SetDark(true)
	tt.SetScale(2)
	tt.Frame()
	tt.Advance(1500 * time.Millisecond)
	for range onbSteps - 1 {
		tt.Key(0, ui.KeyRight)
		tt.Advance(120 * time.Millisecond)
	}
	tt.Advance(2500 * time.Millisecond)
	frame := 0
	save := func() {
		f, err := os.Create(filepath.Join(out, fmt.Sprintf("%04d.png", frame)))
		if err != nil {
			t.Fatal(err)
		}
		png.Encode(f, tt.Image())
		f.Close()
		frame++
	}
	for range 18 { // a moment before
		tt.Advance(time.Second / 60)
		save()
	}
	tt.Key(0, ui.KeyEnter)
	for float32(frame-18)/60 < onbClosing+0.45 {
		tt.Advance(time.Second / 60)
		save()
	}
	if err := os.WriteFile(filepath.Join(out, "sound.wav"), renderScore("chime"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "lead.txt"), []byte(fmt.Sprintf("%.3f", 0.3+onbFly)), 0o644); err != nil {
		t.Fatal(err)
	}
}
