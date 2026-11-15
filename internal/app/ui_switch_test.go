package app

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// paneView is a segmented control and the panes it switches: the first
// tall, the second short.
func paneView(m *mainUI, sel *int) func(c *ui.Context) {
	return func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(false, false, "", "")
		ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
			m.segmentedOf(c, "seg", sel, false, "One", "Two")
			m.switchPane(c, "pane", *sel, 8, func() {
				if *sel == 0 {
					for _, s := range []string{"Alpha", "a2", "a3", "a4", "a5", "a6"} {
						ui.Text(c, s).Height(30)
					}
					return
				}
				ui.Text(c, "Beta").Height(30)
			})
			ui.Text(c, "After")
		})
	}
}

func TestSwitchPaneSlidesFromItsSide(t *testing.T) {
	m, sel := &mainUI{}, 0
	tt := ui.NewTester(paneView(m, &sel), 400, 600)
	tt.Frame()
	rest, _ := tt.Find("Alpha")
	after0, _ := tt.Find("After")
	if err := tt.Click("Two"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(160 * time.Millisecond)
	mid, ok := tt.Find("Beta")
	if !ok {
		t.Fatal("no Beta while switching")
	}
	if mid.X <= rest.X {
		t.Errorf("forward: Beta at x=%v, want it coming from the right of %v", mid.X, rest.X)
	}
	// The room glides: what follows is on its way up, not there at once.
	afterMid, _ := tt.Find("After")
	tt.Advance(time.Second)
	end, _ := tt.Find("Beta")
	afterEnd, _ := tt.Find("After")
	if end.X != rest.X {
		t.Errorf("at rest Beta at x=%v, want %v", end.X, rest.X)
	}
	if !(afterMid.Y < after0.Y && afterMid.Y > afterEnd.Y) {
		t.Errorf("After went %v → %v → %v, want it gliding up", after0.Y, afterMid.Y, afterEnd.Y)
	}
	if tt.HasText("a6") {
		t.Error("the old pane is still there at rest")
	}
	// Back, a moment later: the pane shown before comes in anew (not as
	// the element it was), from the left, where the host clips it.
	if err := tt.Click("One"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(160 * time.Millisecond)
	if back, _ := tt.Find("Alpha"); back == rest {
		t.Errorf("back: Alpha already at rest at %v", back)
	}
	tt.Advance(time.Second)
	if back, _ := tt.Find("Alpha"); back != rest {
		t.Errorf("back at rest: Alpha at %v, want %v", back, rest)
	}
}

func TestSwitchPaneLessMotion(t *testing.T) {
	m, sel := &mainUI{}, 0
	tt := ui.NewTester(paneView(m, &sel), 400, 600)
	tt.SetPreferences(ui.Preferences{ReduceMotion: true})
	tt.Frame()
	rest, _ := tt.Find("Alpha")
	if err := tt.Click("Two"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(160 * time.Millisecond)
	if mid, _ := tt.Find("Beta"); mid.X != rest.X {
		t.Errorf("with less motion Beta at x=%v, want it in place at %v", mid.X, rest.X)
	}
}

func TestSwitchFillSides(t *testing.T) {
	m, sel := &mainUI{}, 0
	tt := ui.NewTester(func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(true, false, "", "")
		ui.Column(c).Fill().Padding(20).Gap(12).Children(func() {
			m.tabs(c, "tabs", &sel, "One", "Two")
			m.switchFill(c, "pane", sel, func() {
				ui.Text(c, []string{"Alpha", "Beta"}[sel])
			})
		})
	}, 400, 300)
	tt.Frame()
	rest, _ := tt.Find("Alpha")
	tt.Click("Two")
	tt.Advance(160 * time.Millisecond)
	if b, _ := tt.Find("Beta"); b.X <= rest.X {
		t.Errorf("forward: Beta at x=%v, want right of %v", b.X, rest.X)
	}
	tt.Advance(time.Second)
	tt.Click("One")
	tt.Advance(160 * time.Millisecond)
	if a, _ := tt.Find("Alpha"); a.X >= rest.X {
		t.Errorf("back: Alpha at x=%v, want left of %v", a.X, rest.X)
	}
	// Right to left, forward comes from the left.
	tt.Advance(time.Second)
	tt.SetDirection(ui.RightToLeft)
	tt.Frame()
	restRTL, _ := tt.Find("Alpha")
	tt.Click("Two")
	tt.Advance(160 * time.Millisecond)
	if b, _ := tt.Find("Beta"); b.X >= restRTL.X {
		t.Errorf("RTL forward: Beta at x=%v, want left of %v", b.X, restRTL.X)
	}
}
