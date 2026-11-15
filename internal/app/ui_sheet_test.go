package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestSheetEnds(t *testing.T) {
	stage := ui.Rect{X: 100, Y: 50, W: 1000, H: 700}
	tile := ui.Rect{X: 600, Y: 80, W: 300, H: 200}
	from, to := sheetEnds(stage, tile, true, false, 0)
	if want := (ui.Rect{X: 500, Y: 30, W: 300, H: 200}); from != want {
		t.Errorf("from = %v, want the tile inside the stage %v", from, want)
	}
	if want := (ui.Rect{X: 22, Y: 0, W: 956, H: 678}); to != want {
		t.Errorf("to = %v, want %v", to, want)
	}
	// Less motion: the sheet does not move, it fades where it ends.
	if from, to := sheetEnds(stage, tile, true, true, 0); from != to {
		t.Errorf("with less motion from = %v, want to %v", from, to)
	}
	// A tile not laid out yet: the sheet starts in place too.
	if from, to := sheetEnds(stage, tile, false, false, 0); from != to {
		t.Errorf("unknown tile: from = %v, want to %v", from, to)
	}
}

func TestSheetFrame(t *testing.T) {
	from := ui.Rect{X: 500, Y: 30, W: 300, H: 200}
	to := ui.Rect{X: 22, Y: 0, W: 956, H: 678}
	if r := sheetFrame(from, to, 0); r != from {
		t.Errorf("k=0: %v, want the tile %v", r, from)
	}
	if r := sheetFrame(from, to, 1); r != to {
		t.Errorf("k=1: %v, want %v", r, to)
	}
	// Turned back at any point, the frame is the same function of k: it
	// goes back from where it is, with no jump.
	a, b := sheetFrame(from, to, 0.4), sheetFrame(from, to, 0.41)
	if d := b.W - a.W; d <= 0 || d > 10 {
		t.Errorf("a step of k moves the width by %v", d)
	}
}

func TestOpenSheetGrowsFromOneTile(t *testing.T) {
	m := &mainUI{}
	m.openSheet("speed")
	if m.dash.sheet != "speed" || m.dash.sheetKey != "speed" {
		t.Fatalf("open: sheet %q key %q", m.dash.sheet, m.dash.sheetKey)
	}
	// Closing keeps the tile it shrinks back to.
	m.dash.sheet = ""
	m.openSheet("exit")
	if m.dash.sheet != "" || m.dash.sheetKey != "speed" {
		t.Errorf("another tile while one shrinks: sheet %q key %q", m.dash.sheet, m.dash.sheetKey)
	}
	// The same tile again turns it back.
	m.openSheet("speed")
	if m.dash.sheet != "speed" {
		t.Errorf("the same tile again: sheet %q", m.dash.sheet)
	}
}
