// Package usage counts the traffic that passed the core, per day, app, site
// and node, from the cumulative totals of its connections.
package usage

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

const (
	// KeepDays is how many days of history are kept.
	KeepDays = 90
	// maxKeys caps the distinct apps, sites or nodes of one day; the rest
	// is counted under otherKey.
	maxKeys = 400

	// Unknown names what has no name, such as a connection without a process.
	Unknown = "—"
	// otherKey holds the traffic of the keys beyond the cap.
	otherKey = "\x00other"

	dateFormat = "2006-01-02"
)

// Entry is bytes up and down.
type Entry struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

func (e *Entry) add(up, down int64) { e.Up += up; e.Down += down }

// Day is the traffic of one local date.
type Day struct {
	Entry
	Hours [24]Entry        `json:"hours"`
	Apps  map[string]Entry `json:"apps"`
	Sites map[string]Entry `json:"sites"`
	Nodes map[string]Entry `json:"nodes"`
}

func newDay() *Day {
	return &Day{Apps: map[string]Entry{}, Sites: map[string]Entry{}, Nodes: map[string]Entry{}}
}

// fixed makes the maps of a day read from a file usable.
func (d *Day) fixed() *Day {
	if d.Apps == nil {
		d.Apps = map[string]Entry{}
	}
	if d.Sites == nil {
		d.Sites = map[string]Entry{}
	}
	if d.Nodes == nil {
		d.Nodes = map[string]Entry{}
	}
	return d
}

// Store collects the traffic and keeps it in a file.
type Store struct {
	path string
	now  func() time.Time

	mu     sync.Mutex
	days   map[string]*Day // by local date
	last   map[string]Entry
	seeded bool
	dirty  bool
}

type file struct {
	Days map[string]*Day `json:"days"`
}

// Open loads the history in path, which may not exist yet.
func Open(path string) *Store {
	s := &Store{path: path, now: time.Now, days: map[string]*Day{}, last: map[string]Entry{}}
	if data, err := os.ReadFile(path); err == nil {
		var f file
		if json.Unmarshal(data, &f) == nil {
			for k, d := range f.Days {
				if d != nil {
					s.days[k] = d.fixed()
				}
			}
		}
	}
	s.pruneAt(s.now())
	return s
}

// Observe counts what the connections moved since the last call. The first
// call only takes their totals: a core that outlived the app has been
// counting for longer than the app was there.
func (s *Store) Observe(conns []coreapi.Connection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	var day *Day
	seen := make(map[string]Entry, len(conns))
	for i := range conns {
		c := &conns[i]
		cur := Entry{Up: c.Upload, Down: c.Download}
		seen[c.ID] = cur
		if !s.seeded {
			continue
		}
		prev := s.last[c.ID] // zero for a connection not seen before
		up, down := cur.Up-prev.Up, cur.Down-prev.Down
		if up < 0 || down < 0 { // the counters started over
			up, down = cur.Up, cur.Down
		}
		if up == 0 && down == 0 {
			continue
		}
		if day == nil {
			day = s.day(now)
		}
		day.add(up, down)
		h := day.Hours[now.Hour()]
		h.add(up, down)
		day.Hours[now.Hour()] = h
		bump(day.Apps, process(c), up, down)
		bump(day.Sites, Site(c.Metadata), up, down)
		bump(day.Nodes, node(c), up, down)
		s.dirty = true
	}
	s.last, s.seeded = seen, true
}

// day returns the day of t, adding it (and pruning the old ones).
func (s *Store) day(t time.Time) *Day {
	k := t.Format(dateFormat)
	d, ok := s.days[k]
	if !ok {
		d = newDay()
		s.days[k] = d
		s.pruneAt(t)
	}
	return d
}

func bump(m map[string]Entry, key string, up, down int64) {
	e, ok := m[key]
	if !ok && len(m) >= maxKeys {
		key = otherKey
		e = m[key]
	}
	e.add(up, down)
	m[key] = e
}

func process(c *coreapi.Connection) string {
	if p := strings.TrimSpace(c.Metadata.Process); p != "" {
		return p
	}
	return Unknown
}

// node is the proxy the connection left through: the chain lists it first,
// and then the groups that chose it.
func node(c *coreapi.Connection) string {
	if len(c.Chains) == 0 || c.Chains[0] == "" {
		return Unknown
	}
	return c.Chains[0]
}

// Site names the site of a connection: its registrable domain, or the
// address when it has no name.
func Site(m coreapi.ConnectionMetadata) string {
	h := m.Host
	if h == "" {
		h = m.SniffHost
	}
	if h == "" {
		h = m.DestinationIP
	}
	return Domain(h)
}

// Domain reduces a host to its registrable domain (www.example.co.uk to
// example.co.uk). Addresses and names without a public suffix stay as they
// are.
func Domain(host string) string {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if host == "" {
		return Unknown
	}
	if ip := strings.Trim(host, "[]"); net.ParseIP(ip) != nil {
		return ip
	}
	if d, err := publicsuffix.EffectiveTLDPlusOne(host); err == nil {
		return d
	}
	return host
}

func (s *Store) pruneAt(t time.Time) {
	cut := t.AddDate(0, 0, -(KeepDays - 1)).Format(dateFormat)
	for k := range s.days {
		if k < cut {
			delete(s.days, k)
			s.dirty = true
		}
	}
}

// Flush writes the history if it changed.
func (s *Store) Flush() error {
	s.mu.Lock()
	if !s.dirty {
		s.mu.Unlock()
		return nil
	}
	data, err := json.Marshal(file{s.days})
	s.dirty = false
	s.mu.Unlock()
	if err == nil {
		err = writeAtomic(s.path, data)
	}
	if err != nil {
		s.mu.Lock()
		s.dirty = true
		s.mu.Unlock()
	}
	return err
}

func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".usage-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path) // replaces path on Windows too
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// Clear forgets the history.
func (s *Store) Clear() error {
	s.mu.Lock()
	s.days = map[string]*Day{}
	s.dirty = true
	s.mu.Unlock()
	return s.Flush()
}

// StatsQuery asks for a range of days. Range is "today", "7d" or "30d";
// anything else uses From and To ("2006-01-02", both included). Top is
// how many apps, sites and nodes to list.
type StatsQuery struct {
	Range string `json:"range"`
	From  string `json:"from,omitempty"`
	To    string `json:"to,omitempty"`
	Top   int    `json:"top"`
}

// StatsPoint is the traffic of a day, or of an hour of a day.
type StatsPoint struct {
	Label string `json:"label"` // the date, or the hour
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
}

// StatsItem is the traffic of an app, site or node. Other collects the
// rest, and has no name.
type StatsItem struct {
	Name  string `json:"name"`
	Other bool   `json:"other,omitempty"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
}

// StatsReport is the answer to a query.
type StatsReport struct {
	From  string       `json:"from"`
	To    string       `json:"to"`
	Up    int64        `json:"up"`
	Down  int64        `json:"down"`
	Days  []StatsPoint `json:"days"`
	Hours []StatsPoint `json:"hours"` // when the range is a single day
	Apps  []StatsItem  `json:"apps"`
	Sites []StatsItem  `json:"sites"`
	Nodes []StatsItem  `json:"nodes"`
}

// Query totals a range of days.
func (s *Store) Query(q StatsQuery) StatsReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	from := to
	switch q.Range {
	case "today":
	case "7d":
		from = to.AddDate(0, 0, -6)
	case "30d":
		from = to.AddDate(0, 0, -29)
	default:
		if t, err := time.ParseInLocation(dateFormat, q.From, now.Location()); err == nil {
			from = t
		}
		if t, err := time.ParseInLocation(dateFormat, q.To, now.Location()); err == nil {
			to = t
		}
	}
	if from.After(to) {
		from, to = to, from
	}
	if min := to.AddDate(0, 0, -(KeepDays - 1)); from.Before(min) {
		from = min
	}
	top := q.Top
	if top <= 0 {
		top = 10
	}
	r := StatsReport{From: from.Format(dateFormat), To: to.Format(dateFormat), Days: []StatsPoint{}, Hours: []StatsPoint{}}
	apps, sites, nodes := map[string]Entry{}, map[string]Entry{}, map[string]Entry{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		k := d.Format(dateFormat)
		day := s.days[k]
		if day == nil {
			r.Days = append(r.Days, StatsPoint{Label: k})
			continue
		}
		r.Up += day.Up
		r.Down += day.Down
		r.Days = append(r.Days, StatsPoint{Label: k, Up: day.Up, Down: day.Down})
		merge(apps, day.Apps)
		merge(sites, day.Sites)
		merge(nodes, day.Nodes)
	}
	if from.Equal(to) {
		day := s.days[r.From]
		for h := 0; h < 24; h++ {
			p := StatsPoint{Label: string([]byte{byte('0' + h/10), byte('0' + h%10)})}
			if day != nil {
				p.Up, p.Down = day.Hours[h].Up, day.Hours[h].Down
			}
			r.Hours = append(r.Hours, p)
		}
	}
	r.Apps, r.Sites, r.Nodes = rank(apps, top), rank(sites, top), rank(nodes, top)
	return r
}

func merge(dst, src map[string]Entry) {
	for k, v := range src {
		e := dst[k]
		e.add(v.Up, v.Down)
		dst[k] = e
	}
}

// rank lists the top keys by traffic, with the rest as one Other item.
func rank(m map[string]Entry, top int) []StatsItem {
	items := make([]StatsItem, 0, len(m))
	var other Entry
	for k, v := range m {
		if k == otherKey {
			other.add(v.Up, v.Down)
			continue
		}
		items = append(items, StatsItem{Name: k, Up: v.Up, Down: v.Down})
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i].Up+items[i].Down, items[j].Up+items[j].Down
		if a != b {
			return a > b
		}
		return items[i].Name < items[j].Name
	})
	if len(items) > top {
		for _, it := range items[top:] {
			other.add(it.Up, it.Down)
		}
		items = items[:top]
	}
	if other.Up+other.Down > 0 {
		items = append(items, StatsItem{Other: true, Up: other.Up, Down: other.Down})
	}
	return items
}
