package app

import (
	"context"
	"runtime"
	"strconv"
	"sync"
	"time"
)

// The delay classes, with the thresholds of delayClass in src/lib/format.ts.
type delayKind int

const (
	delayNone delayKind = iota // untested
	delayGood
	delayOK
	delayBad // slow, or failed
)

func delayKindOf(d int) delayKind {
	switch {
	case d < 0:
		return delayNone
	case d == 0:
		return delayBad
	case d < 200:
		return delayGood
	case d < 600:
		return delayOK
	}
	return delayBad
}

// delayMarker is the colored dot of a delay in a native menu, whose text
// has no color and whose items have no icon. The Windows menu draws emoji
// in one color, so it gets none.
func delayMarker(d int) string {
	if runtime.GOOS == "windows" {
		return ""
	}
	switch delayKindOf(d) {
	case delayGood:
		return "🟢"
	case delayOK:
		return "🟡"
	case delayBad:
		return "🔴"
	}
	return ""
}

// delayLabel is a delay as the tray menu shows it, after a name; empty
// when untested.
func delayLabel(a *App, d int) string {
	var text string
	switch {
	case d < 0:
		return ""
	case d == 0:
		text = tr(a, "timeout")
	default:
		text = strconv.Itoa(d) + " ms"
	}
	if m := delayMarker(d); m != "" {
		return m + " " + text
	}
	return text
}

var groupTests sync.Map // group name -> struct{}, while its test runs

// testingGroup reports whether a test of the group is running.
func testingGroup(group string) bool {
	_, ok := groupTests.Load(group)
	return ok
}

// testGroup tests every member of a group, one test at a time per group,
// then has the tray show the new delays. A failure is notified. It reports
// whether the test ran.
func (a *App) testGroup(group, testURL string) bool {
	if _, busy := groupTests.LoadOrStore(group, struct{}{}); busy {
		return false
	}
	if a.tray != nil {
		a.tray.rebuild() // "testing…"
	}
	ctx, cancel := context.WithTimeout(a.ctx, 60*time.Second)
	_, err := Proxies{a}.GroupDelay(ctx, group, testURL)
	cancel()
	groupTests.Delete(group)
	a.notifyErr("proxies", tr(a, "testDelay"), err)
	if a.tray != nil {
		a.tray.rebuild()
	}
	return true
}
