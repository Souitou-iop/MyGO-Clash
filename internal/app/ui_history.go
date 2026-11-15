package app

import (
	"slices"
	"sync"
	"time"
)

// What the home page's sheets show over days, kept in the UI cache as it
// happens: each subscription's usage, a mark a day; when the app took
// traffic over and let it go; and when the core started. Only a change is
// written.

// useMark is a subscription's usage on a day.
type useMark struct {
	Day  string `json:"day"` // 2006-01-02, local
	Used int64  `json:"used"`
}

// onMark is when taking traffic over turned on or off.
type onMark struct {
	At int64 `json:"at"` // Unix seconds
	On bool  `json:"on"`
}

const (
	historyDays  = 90 // the days of usage and core starts kept
	takeoverKept = 400
)

var noted = struct {
	sync.Mutex
	use     map[string]useMark
	on      *bool
	started time.Time
}{use: map[string]useMark{}}

// noteHistory writes down what changed of state and pv since last noted.
func (a *App) noteHistory(state AppState, pv ProfilesView) {
	now := time.Now()
	day := now.Format(time.DateOnly)
	noted.Lock()
	var uses map[string]useMark
	for _, pr := range pv.Items {
		if pr.Usage == nil || pr.Usage.Total <= 0 {
			continue
		}
		mk := useMark{Day: day, Used: pr.Usage.Upload + pr.Usage.Download}
		if noted.use[pr.UID] != mk {
			noted.use[pr.UID] = mk
			if uses == nil {
				uses = map[string]useMark{}
			}
			uses[pr.UID] = mk
		}
	}
	on := state.SystemProxy || state.Tun
	switched := noted.on == nil || *noted.on != on
	noted.on = &on
	started := state.Core.StartedAt
	newStart := !started.IsZero() && !started.Equal(noted.started)
	if newStart {
		noted.started = started
	}
	noted.Unlock()
	if len(uses) == 0 && !switched && !newStart {
		return
	}
	a.saveUICache(func(c *uiCache) {
		for uid, mk := range uses {
			marks := c.ProfileUse[uid]
			if n := len(marks); n > 0 && marks[n-1].Day == mk.Day {
				marks[n-1] = mk
			} else {
				marks = append(marks, mk)
			}
			if c.ProfileUse == nil {
				c.ProfileUse = map[string][]useMark{}
			}
			c.ProfileUse[uid] = keepLast(marks, historyDays)
		}
		if switched {
			// The first note of a launch says the state, which a crash may
			// have left unsaid.
			if n := len(c.Takeover); n == 0 || c.Takeover[n-1].On != on {
				c.Takeover = keepLast(append(c.Takeover, onMark{At: now.Unix(), On: on}), takeoverKept)
			}
		}
		if newStart && !slices.Contains(c.CoreStarts, started.Unix()) {
			c.CoreStarts = append(c.CoreStarts, started.Unix())
			cut := now.AddDate(0, 0, -historyDays).Unix()
			c.CoreStarts = slices.DeleteFunc(c.CoreStarts, func(t int64) bool { return t < cut })
		}
	})
}

// takeoverOn reports, for each third of an hour of the day of now, whether
// the app took traffic over in any of it.
func takeoverOn(marks []onMark, now time.Time) [72]bool {
	var out [72]bool
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	on := false
	for _, mk := range marks {
		if mk.At < day.Unix() {
			on = mk.On
		}
	}
	for i := range 72 {
		from := day.Add(time.Duration(i) * 20 * time.Minute)
		to := from.Add(20 * time.Minute)
		if from.After(now) {
			break
		}
		lit := on
		for _, mk := range marks {
			t := time.Unix(mk.At, 0)
			if !t.Before(from) && t.Before(to) {
				lit = lit || mk.On
				on = mk.On
			}
		}
		out[i] = lit
	}
	return out
}
