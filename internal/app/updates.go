package app

// Updates of the app itself. mygo's updater plugin draws the window that
// offers a new version (it is not Sparkle, but works like it on every
// platform) and installs updates once their signature checks out. The app
// schedules the background checks itself, as often as the user chose; the
// plugin's own, once a day, stay off.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
)

// ReleasesURL is where every version can be downloaded, for installs that
// cannot update themselves.
const ReleasesURL = "https://github.com/Souitou-iop/MyGO-Clash/releases"

// updateIntervals are the times between background checks, by the names
// of config.Updates.Interval.
var updateIntervals = map[string]time.Duration{
	"hourly":  time.Hour,
	"daily":   24 * time.Hour,
	"weekly":  7 * 24 * time.Hour,
	"monthly": 30 * 24 * time.Hour,
}

// firstCheckWait is the least time before a background check, so that
// launching the app is not slowed by one.
const firstCheckWait = 20 * time.Second

// UpdateState tells the page about updates of the app.
type UpdateState struct {
	// Supported is whether this install updates itself. When it does not,
	// Reason says why: "dev" for development builds, "package" for apps
	// that a package manager or an AppImage holds.
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
	// AutomaticDownloads installs updates found in the background without
	// asking.
	AutomaticDownloads bool `json:"automaticDownloads"`
	// LastCheck is when the app last checked successfully, in Unix
	// milliseconds; 0 for never.
	LastCheck int64 `json:"lastCheck"`
	// Checking is set during a background check.
	Checking bool `json:"checking"`
	// Ready is the version installed in the background, which runs when
	// the app restarts.
	Ready string `json:"ready,omitempty"`
}

// UpdateEvent tells pages that UpdateState changed.
var UpdateEvent = mygo.NewEvent[UpdateState]("update")

// useUpdater installs the updater plugin, before the app runs: its window
// speaks the language of the interface at launch.
func useUpdater() {
	mygo.Use(updater.New(updater.Options{
		Language:               updaterLanguage(matchLanguage(launchLanguage())),
		Strings:                updaterStrings,
		DisableAutomaticChecks: true, // the app schedules them
	}))
}

// updateChecker checks for updates in the background, as the settings say.
type updateChecker struct {
	a    *App
	file string // keeps the time of the last check

	mu       sync.Mutex
	timer    *time.Timer
	last     time.Time // the last check that worked
	failed   time.Time // the last that failed, since
	checking bool
	ready    string
}

func newUpdateChecker(a *App) *updateChecker {
	c := &updateChecker{a: a, file: filepath.Join(a.dirs.Data, "update-check.json")}
	var saved struct {
		LastCheck time.Time `json:"lastCheck"`
	}
	if b, err := os.ReadFile(c.file); err == nil && json.Unmarshal(b, &saved) == nil {
		c.last = saved.LastCheck
	}
	if updater.AutomaticChecks() {
		updater.SetAutomaticChecks(false) // this schedule instead
	}
	updater.OnChange(c.changed)
	// Timers stop while the computer sleeps: see what is due on waking.
	mygo.Power.OnResume(func() { go c.schedule() })
	c.schedule()
	return c
}

func (c *updateChecker) state() UpdateState {
	st := UpdateState{Supported: mygo.Updater.Enabled(), AutomaticDownloads: updater.AutomaticDownloads()}
	switch {
	case st.Supported:
	case mygo.IsDev():
		st.Reason = "dev"
	default:
		st.Reason = "package"
	}
	c.mu.Lock()
	last := c.last
	st.Checking, st.Ready = c.checking, c.ready
	c.mu.Unlock()
	if t := updater.LastCheck(); t.After(last) {
		last = t // a check the user asked for
	}
	if !last.IsZero() {
		st.LastCheck = last.UnixMilli()
	}
	return st
}

// changed tells the pages about the state.
func (c *updateChecker) changed() { _ = UpdateEvent.Broadcast(c.state()) }

// schedule arms the timer of the next background check: an interval after
// the last one that worked, at most an hour after one that failed.
func (c *updateChecker) schedule() {
	st := c.a.settings.Get().Updates
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	if !st.AutoCheck || c.ready != "" || !mygo.Updater.Enabled() {
		return
	}
	now := time.Now()
	if c.last.After(now) { // the clock went back
		c.last = now
	}
	interval := updateIntervals[st.Interval]
	due := c.last.Add(interval)
	if retry := c.failed.Add(min(interval, time.Hour)); !c.failed.IsZero() && retry.After(due) {
		due = retry
	}
	c.timer = time.AfterFunc(max(due.Sub(now), firstCheckWait), c.check)
}

// check looks for a new version. Unless the user skipped it, the update
// window offers it; with automatic downloads, it is installed without
// asking and runs at the next launch.
func (c *updateChecker) check() {
	c.mu.Lock()
	if c.checking {
		c.mu.Unlock()
		return
	}
	c.checking = true
	c.mu.Unlock()
	c.changed()

	ctx, cancel := context.WithTimeout(c.a.ctx, 15*time.Minute)
	defer cancel()
	up, err := mygo.Updater.Check(ctx)
	ready := ""
	switch {
	case err != nil:
		log.Printf("checking for updates: %v", err)
	case up == nil, up.Version == skippedVersion():
	case updater.AutomaticDownloads():
		if err := up.Install(ctx, nil); err != nil {
			log.Printf("installing version %s: %v", up.Version, err)
			updater.CheckForUpdates() // the window offers it again, and says what fails
		} else {
			ready = up.Version
		}
	default:
		updater.CheckForUpdates()
	}

	c.mu.Lock()
	c.checking = false
	if err != nil {
		c.failed = time.Now()
	} else {
		c.failed, c.last = time.Time{}, time.Now()
		c.save()
	}
	if ready != "" {
		c.ready = ready
	}
	c.mu.Unlock()
	if ready != "" {
		c.a.notify(Notice{Level: "success", Message: fmt.Sprintf(tr(c.a, "updateReady"), ready), Action: "relaunch", Page: "settings/advanced", Important: true})
	}
	c.changed()
	c.schedule()
}

// save keeps the time of the last check. c.mu is held.
func (c *updateChecker) save() {
	b, _ := json.Marshal(map[string]time.Time{"lastCheck": c.last})
	if err := os.WriteFile(c.file, b, 0o600); err != nil {
		log.Printf("update check: %v", err)
	}
}

// skippedVersion is the version the user skipped in the update window,
// which the plugin keeps in updater.json among the user's data.
func skippedVersion() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return ""
	}
	var st struct {
		SkippedVersion string `json:"skippedVersion"`
	}
	b, err := os.ReadFile(filepath.Join(dir, "updater.json"))
	if err != nil || json.Unmarshal(b, &st) != nil {
		return ""
	}
	return st.SkippedVersion
}

// launchLanguage is the language setting, read before the settings are
// open, or the system's.
func launchLanguage() string {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err == nil {
		var st struct {
			Language string `json:"language"`
		}
		b, err := os.ReadFile(filepath.Join(dir, "settings.json"))
		if err == nil && json.Unmarshal(b, &st) == nil && st.Language != "" {
			return st.Language
		}
	}
	return mygo.App.Locale()
}

// updaterLanguage turns a language of the interface into the plugin's tag.
func updaterLanguage(l string) string {
	switch l {
	case "zh-CN":
		return "zh-Hans"
	case "zh-TW":
		return "zh-Hant"
	}
	return l
}

// UpdateService checks for updates of the app.
type UpdateService struct{ a *App }

// State returns the state of updates.
func (s UpdateService) State(ctx context.Context) (UpdateState, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return UpdateState{}, err
	}
	return s.a.updates.state(), nil
}

// Check checks for an update now, in the update window, which also says
// when the app is up to date or why it cannot update itself.
func (s UpdateService) Check() { updater.CheckForUpdates() }

// SetAutomaticDownloads installs updates found in the background without
// asking, or not.
func (s UpdateService) SetAutomaticDownloads(on bool) error {
	if !mygo.Updater.Enabled() {
		return errors.New("this install does not update itself")
	}
	updater.SetAutomaticDownloads(on)
	return nil
}

// ReleasesURL returns the page to download versions from.
func (s UpdateService) ReleasesURL() string { return ReleasesURL }
