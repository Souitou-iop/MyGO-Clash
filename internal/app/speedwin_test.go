package app

import (
	"testing"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/config"
)

var (
	waMain = mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040} // taskbar at the bottom
	bMain  = mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1080}
	waLeft = mygo.Rectangle{X: -1280, Y: 0, Width: 1280, Height: 1024}
	small  = mygo.Rectangle{Width: 168, Height: 44}
)

func TestSpeedPlace(t *testing.T) {
	displays := []mygo.Display{{Bounds: bMain, WorkArea: waMain}, {Bounds: waLeft, WorkArea: waLeft}}
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
		{"on the taskbar", config.SpeedWindow{Placed: true, X: 1500, Y: 1036}, mygo.Point{X: 1500, Y: 1036}},
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
		{mygo.Point{X: 1745, Y: 990}, mygo.Point{X: 1752, Y: 996}}, // the taskbar's edge
		{mygo.Point{X: 800, Y: 400}, mygo.Point{X: 800, Y: 400}},
		{mygo.Point{X: 1500, Y: 1036}, mygo.Point{X: 1500, Y: 1036}}, // on the taskbar
		{mygo.Point{X: -30, Y: 1070}, mygo.Point{X: 0, Y: 1036}},     // off the screen
	} {
		if got := snapEdges(c.in, small, bMain, waMain); got != c.want {
			t.Errorf("%v: %v, want %v", c.in, got, c.want)
		}
	}
}
