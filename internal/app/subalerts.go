package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/profiles"
)

// ---- Rollback ----

// rollbackWindow is how long after an update a refused configuration is
// blamed on the update.
const rollbackWindow = 10 * time.Minute

// rollbackAfter puts the current profile's previous version back when the
// core refused a configuration right after the profile was updated, and
// reports whether it did.
func (a *App) rollbackAfter(ctx context.Context, applyErr error) bool {
	if !a.settings.Get().Subscriptions.Rollback {
		return false
	}
	cur, ok := a.profiles.Current()
	if !ok || cur.Type != profiles.TypeRemote || cur.Prev == nil || time.Since(time.Unix(cur.Updated, 0)) > rollbackWindow {
		return false
	}
	if _, err := a.profiles.Rollback(cur.UID); err != nil {
		log.Printf("roll %s back: %v", cur.Name, err)
		return false
	}
	// The rollback changed the content, which applies the configuration
	// again. Should the old one fail too, the next failure is reported:
	// the profile is no longer freshly updated.
	a.notify(Notice{Level: "warning", Message: fmt.Sprintf(tr(a, "profileRolledBack"), cur.Name), Detail: applyErr.Error(), Page: "profiles", Important: true})
	return true
}

// Rollback puts back the content a remote profile had before its last
// update; another rollback undoes it.
func (s Profiles) Rollback(ctx context.Context, uid string) (profiles.Profile, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return profiles.Profile{}, err
	}
	return s.a.profiles.Rollback(uid)
}

// ---- Traffic and expiry of subscriptions ----

// quotaAlerts remembers which warnings were given for each subscription,
// so that each is given once per billing period.
type quotaAlerts struct {
	mu   sync.Mutex
	path string
	Told map[string]string `json:"told"` // by profile: the warnings given, as a key
}

func (a *App) quotaState() *quotaAlerts {
	a.alertsOnce.Do(func() {
		q := &quotaAlerts{path: filepath.Join(a.dirs.Data, "alerts.json"), Told: map[string]string{}}
		if data, err := os.ReadFile(q.path); err == nil {
			_ = json.Unmarshal(data, q)
			if q.Told == nil {
				q.Told = map[string]string{}
			}
		}
		a.alerts = q
	})
	return a.alerts
}

// checkQuotas warns of subscriptions that run low on traffic or time.
func (a *App) checkQuotas() {
	st := a.settings.Get().Subscriptions
	if !st.QuotaAlert {
		return
	}
	q := a.quotaState()
	q.mu.Lock()
	defer q.mu.Unlock()
	changed := false
	now := time.Now()
	for _, p := range a.profiles.List().Items {
		if p.Type != profiles.TypeRemote || p.Usage == nil {
			continue
		}
		u := p.Usage
		type alert struct {
			part string // its part of the key
			n    Notice
		}
		var alerts []alert
		if u.Total > 0 {
			left := u.Total - u.Upload - u.Download
			pct := int(max(left, 0) * 100 / u.Total)
			if pct <= st.QuotaPercent {
				// One warning per period: a new period changes the total or
				// the expiry.
				alerts = append(alerts, alert{fmt.Sprintf("low:%d:%d;", u.Total, u.Expire), Notice{Level: "warning", Message: fmt.Sprintf(tr(a, "quotaLow"), p.Name, pct), Page: "profiles", Important: true}})
			}
		}
		if u.Expire > 0 {
			exp := time.Unix(u.Expire, 0)
			switch days := int(exp.Sub(now).Hours() / 24); {
			case !exp.After(now):
				alerts = append(alerts, alert{fmt.Sprintf("expired:%d;", u.Expire), Notice{Level: "warning", Message: fmt.Sprintf(tr(a, "quotaExpired"), p.Name), Page: "profiles", Important: true}})
			case days < st.ExpireDays:
				alerts = append(alerts, alert{fmt.Sprintf("expire:%d;", u.Expire), Notice{Level: "warning", Message: fmt.Sprintf(tr(a, "quotaExpire"), p.Name, max(days, 1)), Page: "profiles", Important: true}})
			}
		}
		key := ""
		for _, al := range alerts {
			key += al.part
		}
		old := q.Told[p.UID]
		if key == old {
			continue
		}
		q.Told[p.UID], changed = key, true
		for _, al := range alerts {
			if !strings.Contains(old, al.part) {
				a.notify(al.n)
			}
		}
	}
	for uid := range q.Told {
		if _, err := a.profiles.Get(uid); err != nil {
			delete(q.Told, uid)
			changed = true
		}
	}
	if changed {
		data, _ := json.Marshal(q)
		_ = os.WriteFile(q.path, data, 0o600)
	}
}
