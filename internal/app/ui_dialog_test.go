package app

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/mygo-clash/mygo-clash/internal/app/locales"
)

func dialogTester(m *mainUI, build func(c *ui.Context)) *ui.Tester {
	view := func(c *ui.Context) {
		m.frame++
		m.pal = mainPalette(false, false, "", "")
		ui.Column(c).Fill().Children(func() { build(c) })
	}
	tt := ui.NewTester(view, 1000, 800)
	tt.Frame()
	tt.Advance(time.Second)
	return tt
}

// A dialog of settings shows its body, as tall as it is; a body taller
// than the window scrolls and leaves the buttons in sight.
func TestSettingsDialogShowsItsBody(t *testing.T) {
	for _, lines := range []int{2, 40} {
		m := &mainUI{}
		m.settings.dialog = "x"
		tt := dialogTester(m, func(c *ui.Context) {
			m.dialogFrame(c, "x", "TitleText", 480, func() {
				for i := 0; i < lines; i++ {
					ui.Text(c, fmt.Sprintf("Body%02d", i)).Height(30)
				}
			}, func() bool { return true })
		})
		body, ok := tt.Find("Body00")
		if !ok || body.W == 0 || body.H == 0 {
			t.Fatalf("%d lines: the body is not shown (%v, found %v)", lines, body, ok)
		}
		save, ok := tt.Find(locales.CommonSave())
		if !ok || save.Y+save.H > 800 {
			t.Errorf("%d lines: the Save button is off the window (%v)", lines, save)
		}
	}
}

// A field in a column is as tall as its label, its control and its hint:
// the next one starts below them, not on top of them.
func TestFieldInColumnKeepsItsHeight(t *testing.T) {
	m := &mainUI{}
	m.settings.dialog = "x"
	tt := dialogTester(m, func(c *ui.Context) {
		m.dialogFrame(c, "x", "TitleText", 480, func() {
			m.field(c, "FieldA", "hintA", "", func() { ui.TextInput(c.Key("a"), new(string)) })
			m.field(c, "FieldB", "hintB", "", func() { ui.TextArea(c.Key("b"), new(string)).Height(64) })
			m.field(c, "FieldC", "", "", func() { ui.TextInput(c.Key("c"), new(string)) })
		}, func() bool { return true })
	})
	a, _ := tt.Find("FieldA")
	hintA, _ := tt.Find("hintA")
	b, _ := tt.Find("FieldB")
	hintB, _ := tt.Find("hintB")
	cc, _ := tt.Find("FieldC")
	if hintA.Y-a.Y < 30 || b.Y-hintA.Y < 14 || hintB.Y-b.Y < 64+5 || cc.Y-hintB.Y < 14 {
		t.Errorf("the fields lie on each other: A %.0f, hint %.0f, B %.0f (area 64 tall), hint %.0f, C %.0f", a.Y, hintA.Y, b.Y, hintB.Y, cc.Y)
	}
}

// The profile's edit dialog, with a long subscription link: the address
// has its area, and User-Agent starts below it, not under its text.
func TestProfileDialogAreaHoldsTheLink(t *testing.T) {
	m := &mainUI{}
	m.profs.dialog, m.profs.dlgUID, m.profs.dlgType = true, "x", 0
	m.profs.dlgURL = "https://example.com/api/v1/client/subscribe?token=7c85e4a316de33ed6fe8575836e9f1b2c3d4e5f6&flag=clash.meta&udp=true&tfo=false&expand=true&scv=true"
	m.profs.dlgIntvl, m.profs.dlgTO = "0", "60"
	tt := dialogTester(m, m.profileDialog)
	url, _ := tt.Find(locales.ProfilesUrl())
	ua, _ := tt.Find(locales.ProfilesUserAgent())
	if ua.Y-url.Y < 64 {
		t.Errorf("User-Agent at y=%.0f is within the link's 64-tall area from y=%.0f", ua.Y, url.Y)
	}
	name, _ := tt.Find(locales.ProfilesName())
	desc, _ := tt.Find(locales.ProfilesDesc())
	if name.W < 200 || desc.W < 200 || name.W != desc.W || desc.X <= name.X {
		t.Errorf("name and description do not share a row: name %v, desc %v", name, desc)
	}
}
