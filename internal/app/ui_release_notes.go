package app

import (
	"context"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
)

// releaseNotesPage is what the release notes dialog keeps between frames.
type releaseNotesPage struct {
	open     bool
	loaded   bool
	loading  bool
	items    []ReleaseNote
	err      string
	expanded map[string]bool
	from     ui.Rect // the button that opens it, which it comes out of and closes into
	md       markdown.Options
	folds    map[string]*noteFoldState
}

// openReleaseNotes opens the dialog, out of its button, and fetches the
// notes the first time.
func (m *mainUI) openReleaseNotes() {
	s := &m.releaseNotes
	s.open = true
	if s.loaded || s.loading {
		return
	}
	s.loading, s.err = true, ""
	win := m.window()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		notes, err := (UpdateService{m.a}).ReleaseNotes(ctx)
		if win == nil {
			return
		}
		win.Update(func() {
			s.loading = false
			if err != nil {
				s.err = err.Error()
				return
			}
			s.items, s.loaded, s.err = notes, true, ""
			s.expanded = map[string]bool{}
			if len(notes) > 0 {
				s.expanded[notes[0].Version] = true
			}
		})
	}()
}

// releaseNotesTheme is the look of the notes: text set for reading, a
// little larger and more open than the settings around it, and the
// headings of their sections as quiet labels.
func (m *mainUI) releaseNotesTheme() *markdown.Theme {
	p := m.pal
	th := markdown.DefaultTheme(p.Theme)
	th.FontSize = 13.5
	th.LineHeight = 1.7
	th.Text = p.Text
	th.Strong = p.Text
	th.Muted = p.TextMuted
	th.Link = p.Accent
	th.BlockSpacing = 0.8
	th.ItemSpacing = 0.5
	th.ListIndent = 1.2
	for i := range th.Headings {
		th.Headings[i] = markdown.HeadingStyle{Size: 0.93, Weight: 600, Color: p.TextMuted, Top: 1.3, Bottom: 0.45}
	}
	return th
}

func (m *mainUI) releaseNotesDialog(c *ui.Context) {
	s, p := &m.releaseNotes, m.pal
	_, wh := c.Size()
	m.modalFrom(c, "release-notes", &s.open, s.from, func() {
		ui.Column(c).Width(640).Height(min(620, wh-80)).Gap(14).Children(func() {
			ui.Text(c, m.T("updates.notesTitle")).FontSize(17).FontWeight(650).Padding(2, 4, 0, 4)
			switch {
			case s.loading:
				ui.Column(c).Grow(1).Gap(10).Padding(0, 4).Children(func() {
					for range 5 {
						m.skeleton(c, 40)
					}
				})
			case s.err != "":
				ui.Column(c.Key("failed")).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(10).Transition(revealIn).Children(func() {
					ui.Text(c, m.T("updates.notesFailed")).TextColor(p.TextMuted)
					ui.Text(c, s.err).FontSize(12).TextColor(p.Faint).Wrap().Selectable().MaxWidth(480).TextAlign(ui.Center)
					if m.button(c, "retry", m.T("updates.notesRetry"), icRefreshCw, "").Clicked() {
						s.loaded, s.expanded = false, nil
						m.openReleaseNotes()
					}
				})
			case len(s.items) == 0:
				ui.Column(c.Key("empty")).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Transition(revealIn).Children(func() {
					ui.Text(c, m.T("updates.notesEmpty")).TextColor(p.TextMuted)
				})
			default:
				s.md.Theme = m.releaseNotesTheme()
				chinese := strings.HasPrefix(lang(m.a), "zh")
				ui.Scroll(c.Key("list")).Grow(1).Gap(2).Transition(revealIn).Children(func() {
					for _, note := range s.items {
						m.releaseNote(c, note, chinese)
					}
				})
			}
			ui.Row(c).Justify(ui.End).Children(func() {
				if m.button(c, "done", locales.CommonDone(), nil, "primary").Clicked() {
					s.open = false
				}
			})
		})
	})
}

// releaseNote is one version: a row that opens and closes it, and its
// notes folding out below it and back into it.
func (m *mainUI) releaseNote(c *ui.Context, note ReleaseNote, chinese bool) {
	s, p := &m.releaseNotes, m.pal
	open := s.expanded[note.Version]
	row := ui.ButtonBase(c.Key("toggle-"+note.Version)).Row().Gap(10).Padding(9, 10).Radius(9).AlignItems(ui.Center).Justify(ui.Start)
	if row.Hovered() {
		row.Background(p.SurfaceHover)
	}
	pressable(row)
	row.Children(func() {
		// The chevron turns as the notes fold out and back.
		dir := float32(1)
		if c.Direction() == ui.RightToLeft {
			dir = -1
		}
		chev := ui.Icon(c.Key("chev-"+note.Version), icChevronDown).FontSize(13).TextColor(p.Faint)
		k := chev.AnimateTo("open", b2f(open), ui.Animation{Duration: collapseDur, Ease: easeStd})
		chev.Rotate(-90 * dir * (1 - k))
		ui.Text(c, "v"+note.Version).FontSize(15).FontWeight(650).FontFeatures("tnum")
		if note.Version == Version {
			ui.Text(c, m.T("updates.current")).FontSize(11.5).FontWeight(550).TextColor(p.Accent).Background(p.AccentSoft).Padding(2, 7).Radius(6)
		}
		ui.Spacer(c)
		if !note.PublishedAt.IsZero() {
			ui.Text(c, note.PublishedAt.Local().Format("2006-01-02")).FontSize(12).TextColor(p.Faint).FontFeatures("tnum")
		}
	})
	if row.Clicked() {
		s.expanded[note.Version] = !open
	}
	m.noteFold(c, note.Version, open, func() {
		body := releaseNotesIn(note.Body, chinese)
		if body == "" {
			body = "—"
		}
		// The notes line up with the version above them, past the
		// chevron.
		ui.Box(c).Padding(2, 12, 14, 33).Children(func() {
			markdown.View(c.Key("md-"+note.Version), body, &s.md)
		})
	})
}

// noteFoldState is a version's notes folding out or back: the height they
// glide from, since when, and the height they last showed at.
type noteFoldState struct {
	frame   uint64
	open    bool
	from    float32
	start   time.Time
	shown   float32
	natural float32 // the notes' own height
}

// noteFold shows fn's notes below their row while open. The box takes the
// notes' own height step by step, so that the versions below are pushed
// down as they fold out and close up as they fold back, the notes fading
// with it.
func (m *mainUI) noteFold(c *ui.Context, version string, open bool, fn func()) {
	s := &m.releaseNotes
	if s.folds == nil {
		s.folds = map[string]*noteFoldState{}
	}
	st := s.folds[version]
	now := c.Now()
	if st == nil || st.frame+1 < m.frame {
		if !open {
			delete(s.folds, version)
			return
		}
		st = &noteFoldState{open: false, start: now}
		s.folds[version] = st
	}
	st.frame = m.frame
	if st.open != open {
		st.open, st.from, st.start = open, st.shown, now
	}
	t := float32(1)
	if !c.Preferences().ReduceMotion {
		t = min(1, float32(now.Sub(st.start))/float32(collapseDur))
	}
	if !open && t >= 1 {
		st.shown = 0
		return
	}
	box := ui.Column(c.Key("fold-" + version)).Clip().Shrink(0)
	natural := st.natural // as the notes were laid out in the frame before
	target := float32(0)
	if open {
		target = natural
	}
	e := easeStd(t)
	if t < 1 || natural == 0 {
		st.shown = st.from + (target-st.from)*e
		box.Height(st.shown)
		c.AnimationFrame()
	} else {
		st.shown = natural
	}
	fade := e
	if !open {
		fade = 1 - e
	}
	box.Opacity(fade)
	if !open {
		box.IgnoreInput(true)
	}
	box.Children(func() {
		inner := ui.Column(c.Key("fold-in-" + version)).Shrink(0)
		if h := inner.LayoutBounds().H; h > 0 {
			st.natural = h
		}
		inner.Children(fn)
	})
}
