package app

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

const logsMax = 5000

var logLevels = []string{"debug", "info", "warning", "error"}

// logsPage is what the logs page keeps between frames.
type logsPage struct {
	logs    []coreapi.LogEvent
	started bool
	stop    context.CancelFunc
	level   int // 0 all, then logLevels+1
	search  string
	paused  bool
	list    ui.ListState
}

// follow streams the core's logs into the page while it lives.
func (m *mainUI) followLogs() {
	s := &m.logs
	if s.started {
		return
	}
	s.started = true
	s.list.FollowEnd = true
	s.list.FollowAnimation = &ui.Smooth // new lines come up from below
	win, a := m.window(), m.a
	ctx, cancel := context.WithCancel(a.ctx)
	s.stop = cancel
	go func() {
		recent := a.logs.recent()
		win.Update(func() { s.logs = append(slices.Clone(recent), s.logs...) })
		if a.logs.hub == nil {
			return
		}
		// Logs come in bursts: add them a few times a second.
		var buf []coreapi.LogEvent
		ch, unsub := a.logs.hub.Subscribe()
		defer unsub()
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case e, ok := <-ch:
				if !ok {
					return
				}
				buf = append(buf, e)
			case <-tick.C:
				if len(buf) == 0 {
					continue
				}
				add := buf
				buf = nil
				win.Update(func() {
					if s.paused {
						return
					}
					s.logs = append(s.logs, add...)
					if n := len(s.logs); n > logsMax {
						s.logs = slices.Clone(s.logs[n-logsMax:])
					}
				})
			}
		}
	}()
}

func (m *mainUI) logsView(c *ui.Context) {
	s, a := &m.logs, m.a
	m.followLogs()
	q := strings.ToLower(strings.TrimSpace(s.search))
	var shown []int
	for i, l := range s.logs {
		if s.level > 0 && slices.Index(logLevels, l.Type) < s.level-1 && slices.Contains(logLevels, l.Type) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(l.Payload), q) {
			continue
		}
		shown = append(shown, i)
	}
	text := func() string {
		var b strings.Builder
		for _, i := range shown {
			l := s.logs[i]
			b.WriteString(l.Time.UTC().Format(time.RFC3339Nano) + " [" + l.Type + "] " + l.Payload + "\n")
		}
		return b.String()
	}
	label := func(t string) string {
		if slices.Contains(logLevels, t) {
			return m.T("logs." + t)
		}
		return t
	}
	m.header(c, locales.NavLogs(), func() {
		icon, txt := icPause, locales.CommonPause()
		if s.paused {
			icon, txt = icPlay, locales.CommonResume()
		}
		if m.button(c, "pause", txt, icon, "ghost").Clicked() {
			s.paused = !s.paused
		}
		if m.button(c, "copy", locales.CommonCopy(), icCopy, "ghost").Disabled(len(shown) == 0).Clicked() {
			mygo.Clipboard.WriteText(text())
			c.Toast(locales.CommonCopied())
		}
		if m.button(c, "export", locales.LogsExport(), icDownload, "ghost").Disabled(len(shown) == 0).Clicked() {
			body := text()
			m.run(locales.CommonFailed(), func() error {
				path, err := (Logs{a}).Export(context.Background(), body)
				if err == nil && path != "" {
					m.notice(Notice{Level: "success", Message: locales.LogsExported(), Detail: path})
				}
				return err
			}, nil)
		}
		if m.button(c, "clear", locales.CommonClear(), icTrash2, "ghost").Clicked() {
			s.logs = nil
			(Logs{a}).Clear()
		}
	})
	ui.Row(c).Padding(0, 22, 10).Gap(8).AlignItems(ui.Center).Shrink(0).Children(func() {
		labels := []string{locales.CommonAll()}
		for _, l := range logLevels {
			labels = append(labels, m.T("logs."+l))
		}
		if i := m.segmented(c, "level", s.level, labels...); i >= 0 {
			s.level = i
		}
		ui.SearchField(c.Key("search"), &s.search).Placeholder(locales.LogsSearch()).Width(280)
		ui.Spacer(c)
		variant := "ghost"
		if s.list.FollowEnd {
			variant = "primary"
		}
		b := m.button(c, "follow", "", icArrowDownToLine, variant).Tooltip(locales.LogsFollow()).Width(32).Padding(0)
		if b.Clicked() {
			s.list.FollowEnd = !s.list.FollowEnd
		}
	})
	// Empty and the lines crossfade as the filter or the logs change.
	m.swapFill(c, "logs-full", map[bool]string{true: "empty", false: "full"}[len(shown) == 0], func() { m.logLines(c, shown, label) })
}

// logLines is the logs shown, or what stands in for them.
func (m *mainUI) logLines(c *ui.Context, shown []int, label func(string) string) {
	s, p := &m.logs, m.pal
	if len(shown) == 0 {
		m.empty(c, locales.LogsEmpty(), func() {
			if len(s.logs) > 0 {
				return
			}
			level := m.st.Clash.LogLevel
			hint := locales.LogsLevelHint(m.T("logs." + level))
			if level == "silent" {
				hint = locales.LogsSilentHint()
			}
			ui.Text(c, hint).FontSize(12.5).TextColor(p.TextMuted).TextAlign(ui.Center).MaxWidth(420)
			if m.button(c, "level-setting", locales.LogsLevelSetting(), icSettings2, "").Clicked() {
				m.goTo("settings", "clash")
			}
		})
		return
	}
	// The lines sit on a tile of their own.
	ui.Column(c.Key("log-tile")).Grow(1).MinHeight(0).Margin(0, 22, 22).PaddingY(8).Background(p.Card).Radius(tileRadius).Clip().Children(func() {
		ui.List(c.Key("logs"), &s.list, len(shown), func(row int) {
			l := s.logs[shown[row]]
			ui.Row(c).Padding(5, 22).Gap(10).AlignItems(ui.Start).Children(func() {
				ui.Text(c, l.Time.Local().Format("15:04:05")).FontSize(11.5).TextColor(p.Faint).Font("ui-monospace").Shrink(0).PaddingY(1)
				tone := ""
				switch l.Type {
				case "info":
					tone = "info"
				case "warning":
					tone = "warning"
				case "error":
					tone = "danger"
				}
				m.badge(c, label(l.Type), tone)
				ui.Text(c, l.Payload).FontSize(12).Font("ui-monospace").Wrap().Selectable().Grow(1).Basis(0)
			})
		}).Grow(1)
	})
}
