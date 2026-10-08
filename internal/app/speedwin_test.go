package app

import (
	"testing"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/config"
)

var (
	waMain = mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040} // taskbar at the bottom
	waLeft = mygo.Rectangle{X: -1280, Y: 0, Width: 1280, Height: 1024}
	small  = mygo.Rectangle{Width: 168, Height: 44}
	big    = mygo.Rectangle{Width: 232, Height: 62}
)

func TestSpeedPlace(t *testing.T) {
	displays := []mygo.Display{{WorkArea: waMain}, {WorkArea: waLeft}}
	primary := displays[0]
	for _, c := range []struct {
		name string
		st   config.SpeedWindow
		want mygo.Point
	}{
		{"never placed", config.SpeedWindow{}, mygo.Point{X: 1920 - 168 - 16, Y: 1040 - 44 - 16}},
		{"kept", config.SpeedWindow{Placed: true, X: 100, Y: 200}, mygo.Point{X: 100, Y: 200}},
		{"on the other screen", config.SpeedWindow{Placed: true, X: -600, Y: 10}, mygo.Point{X: -600, Y: 10}},
		{"screen gone", config.SpeedWindow{Placed: true, X: 2500, Y: 10}, mygo.Point{X: 1920 - 168 - 16, Y: 1040 - 44 - 16}},
		{"half off", config.SpeedWindow{Placed: true, X: 1900, Y: 10}, mygo.Point{X: 1920 - 168 - 16, Y: 1040 - 44 - 16}},
	} {
		if got := speedPlace(c.st, small, displays, primary); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSnapEdges(t *testing.T) {
	for _, c := range []struct{ in, want mygo.Point }{
		{mygo.Point{X: 5, Y: 500}, mygo.Point{X: 0, Y: 500}},
		{mygo.Point{X: 1745, Y: 990}, mygo.Point{X: 1752, Y: 996}},
		{mygo.Point{X: 800, Y: 400}, mygo.Point{X: 800, Y: 400}},
		{mygo.Point{X: -30, Y: -8}, mygo.Point{X: 0, Y: 0}},
	} {
		if got := snapEdges(c.in, small, waMain); got != c.want {
			t.Errorf("%v: %v, want %v", c.in, got, c.want)
		}
	}
}

func TestGrowInside(t *testing.T) {
	if got := growInside(mygo.Point{X: 100, Y: 100}, small, big, waMain); got != (mygo.Point{X: 100, Y: 100}) {
		t.Errorf("top left grew to %v", got)
	}
	if got := growInside(mygo.Point{X: 1752, Y: 996}, small, big, waMain); got != (mygo.Point{X: 1752 - 64, Y: 996 - 18}) {
		t.Errorf("bottom right grew to %v", got)
	}
}
