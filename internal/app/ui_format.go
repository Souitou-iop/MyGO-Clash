package app

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/i18n"
)

var byteUnits = []string{"B", "KB", "MB", "GB", "TB", "PB"}

// fmtBytes is a size in units of 1024, as "1.5 GB".
func fmtBytes(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	v, i := float64(n), 0
	for v >= 1024 && i < len(byteUnits)-1 {
		v /= 1024
		i++
	}
	digits := 0
	if i > 0 && v < 100 {
		digits = 1
	}
	return fmt.Sprintf("%.*f %s", digits, v, byteUnits[i])
}

// fmtRate is a size a second.
func fmtRate(n int64) string { return fmtBytes(n) + "/s" }

// fmtDuration is a time passed, as "3h 12m".
func fmtDuration(d time.Duration) string {
	s := int(max(0, d).Seconds())
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm %ds", s/60, s%60)
	case s < 86400:
		return fmt.Sprintf("%dh %dm", s/3600, s/60%60)
	}
	return fmt.Sprintf("%dd %dh", s/86400, s/3600%24)
}

// fmtPercent is a's share of b, from 0 to 100.
func fmtPercent(a, b int64) float64 {
	if b == 0 {
		return 0
	}
	return math.Max(0, math.Min(100, float64(a)/float64(b)*100))
}

// relTime is a time from now in the interface's language, as "3 minutes
// ago" or "in 2 hours", to the nearest unit.
func (m *mainUI) relTime(t time.Time) string {
	if t.IsZero() || t.Unix() <= 0 {
		return "—"
	}
	diff := time.Until(t).Seconds()
	abs := math.Abs(diff)
	var unit i18n.RelativeTimeUnit
	var span float64
	switch {
	case abs < 45:
		unit, span = i18n.RelativeSecond, 0
	case abs < 2700:
		unit, span = i18n.RelativeMinute, abs/60
	case abs < 79200:
		unit, span = i18n.RelativeHour, abs/3600
	case abs < 2592000:
		unit, span = i18n.RelativeDay, abs/86400
	case abs < 31536000:
		unit, span = i18n.RelativeMonth, abs/2592000
	default:
		unit, span = i18n.RelativeYear, abs/31536000
	}
	n := math.Round(span)
	if diff < 0 {
		n = -n
	}
	return i18n.FormatRelativeTime(i18n.Locale(), n, unit, i18n.RelativeTimeOptions{Auto: true})
}

func fmtDate(unix int64) string {
	if unix == 0 {
		return "—"
	}
	return time.Unix(unix, 0).Format("2006-01-02")
}

func fmtDateTime(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04")
}
