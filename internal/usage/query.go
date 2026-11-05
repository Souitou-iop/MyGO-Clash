package usage

import (
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"
)

// StatsQuery asks for a range of days. Range is "today", "7d", "30d" or
// "all" (everything kept); anything else uses From and To ("2006-01-02",
// both included). Top is how many names of each dimension to list. Filter
// keeps only the names that contain it, without regard to case. Where
// narrows everything to the traffic of exact names of dimensions (app,
// site, node, rule, policy, country), for drilling down. SortBy ranks by
// "traffic" (the default) or "conns".
type StatsQuery struct {
	Range  string            `json:"range"`
	From   string            `json:"from,omitempty"`
	To     string            `json:"to,omitempty"`
	Top    int               `json:"top"`
	Filter string            `json:"filter,omitempty"`
	Where  map[string]string `json:"where,omitempty"`
	SortBy string            `json:"sortBy,omitempty"`
	// Devices are whose statistics, by ID: none for this device's alone.
	// The app answers for other devices.
	Devices []string `json:"devices,omitempty"`
}

// StatsPoint is the traffic of a day, or of an hour of a day.
type StatsPoint struct {
	Label string `json:"label"` // the date, or the hour
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
	Conns int64  `json:"conns,omitempty"`
}

// StatsItem is the traffic of a name of a dimension. Other collects the
// rest, and has no name.
type StatsItem struct {
	Name  string `json:"name"`
	Other bool   `json:"other,omitempty"`
	Up    int64  `json:"up"`
	Down  int64  `json:"down"`
	Conns int64  `json:"conns,omitempty"`
}

// StatsReport is the answer to a query.
type StatsReport struct {
	From      string       `json:"from"`
	To        string       `json:"to"`
	Up        int64        `json:"up"`
	Down      int64        `json:"down"`
	Conns     int64        `json:"conns"`
	Days      []StatsPoint `json:"days"`
	Hours     []StatsPoint `json:"hours"` // when the range is a single day
	Apps      []StatsItem  `json:"apps"`
	Sites     []StatsItem  `json:"sites"`
	Nodes     []StatsItem  `json:"nodes"`
	Rules     []StatsItem  `json:"rules"`
	Policies  []StatsItem  `json:"policies"`
	Countries []StatsItem  `json:"countries"`
	// Prev is the traffic of as many days just before the range.
	Prev *Entry `json:"prev,omitempty"`
	// Partial is set when some days of the range could not be narrowed by
	// Where: days from before the combinations were recorded, or from
	// other devices, which share only their totals.
	Partial bool `json:"partial,omitempty"`
	// Oldest is the first day with traffic, "" when there is none.
	Oldest string `json:"oldest"`
}

// Source is days to add to a query, such as another device's: their
// totals, and for some, their combinations.
type Source struct {
	Days  map[string]*Day
	Flows map[string]map[flowKey]Entry
}

// source is a source as a query reads it.
type source struct {
	days  map[string]*Day
	flows func(date string) map[flowKey]Entry
}

// Query totals a range of the store's days.
func (s *Store) Query(q StatsQuery) StatsReport { return s.QueryWith(q, true) }

// QueryWith totals a range of days: the store's own when local is set, and
// those of the sources (other devices).
func (s *Store) QueryWith(q StatsQuery, local bool, sources ...Source) StatsReport {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	loc := now.Location()
	all := []source{}
	if local {
		all = append(all, source{s.days, s.flowsOf})
	}
	for _, src := range sources {
		all = append(all, source{src.Days, func(d string) map[flowKey]Entry { return src.Flows[d] }})
	}
	oldest := ""
	for _, src := range all {
		for k, d := range src.days {
			if d != nil && d.Up+d.Down > 0 && (oldest == "" || k < oldest) {
				oldest = k
			}
		}
	}
	to := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	from := to
	switch q.Range {
	case "today":
	case "7d":
		from = to.AddDate(0, 0, -6)
	case "30d":
		from = to.AddDate(0, 0, -29)
	case "all":
		if t, err := time.ParseInLocation(dateFormat, oldest, loc); err == nil && t.Before(from) {
			from = t
		}
	default:
		if t, err := time.ParseInLocation(dateFormat, q.From, loc); err == nil {
			from = t
		}
		if t, err := time.ParseInLocation(dateFormat, q.To, loc); err == nil {
			to = t
		}
	}
	if from.After(to) {
		from, to = to, from
	}
	if cut := s.cut(now); cut != "" {
		if min, err := time.ParseInLocation(dateFormat, cut, loc); err == nil && from.Before(min) {
			from = min
		}
		if from.After(to) {
			to = from
		}
	}
	top := q.Top
	if top <= 0 {
		top = 10
	}
	where := map[string]string{}
	for k, v := range q.Where {
		if v != "" {
			where[k] = v
		}
	}
	r := StatsReport{From: from.Format(dateFormat), To: to.Format(dateFormat), Days: []StatsPoint{}, Hours: []StatsPoint{}, Oldest: oldest}
	dims := map[string]map[string]Entry{}
	for _, d := range Dims {
		dims[d] = map[string]Entry{}
	}
	single := from.Equal(to)
	var hours [24]Entry
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		k := d.Format(dateFormat)
		var day Entry
		for _, src := range all {
			sum := src.days[k]
			if sum == nil {
				continue
			}
			if len(where) == 0 {
				day.addEntry(sum.Entry)
				for _, dim := range Dims {
					merge(dims[dim], sum.dim(dim))
				}
				if single {
					for h := range hours {
						hours[h].addEntry(sum.Hours[h])
					}
				}
				continue
			}
			flows := src.flows(k)
			if flows == nil {
				if sum.Up+sum.Down > 0 {
					r.Partial = true
				}
				continue
			}
			for fk, e := range flows {
				if !matches(fk, where) {
					continue
				}
				day.addEntry(e)
				for _, dim := range Dims {
					bumpQuery(dims[dim], fk.get(dim), e)
				}
				if single && fk.Hour >= 0 && fk.Hour < 24 {
					hours[fk.Hour].addEntry(e)
				}
			}
		}
		r.Up += day.Up
		r.Down += day.Down
		r.Conns += day.Conns
		r.Days = append(r.Days, StatsPoint{Label: k, Up: day.Up, Down: day.Down, Conns: day.Conns})
	}
	if single {
		for h := 0; h < 24; h++ {
			r.Hours = append(r.Hours, StatsPoint{Label: fmt.Sprintf("%02d", h), Up: hours[h].Up, Down: hours[h].Down, Conns: hours[h].Conns})
		}
	}
	if f := strings.ToLower(strings.TrimSpace(q.Filter)); f != "" {
		for _, d := range Dims {
			dims[d] = filtered(dims[d], f)
		}
	}
	byConns := q.SortBy == "conns"
	r.Apps, r.Sites, r.Nodes = rank(dims[DimApp], top, byConns), rank(dims[DimSite], top, byConns), rank(dims[DimNode], top, byConns)
	r.Rules, r.Policies, r.Countries = rank(dims[DimRule], top, byConns), rank(dims[DimPolicy], top, byConns), rank(dims[DimCountry], top, byConns)

	// The period before, as long.
	n := int(to.Sub(from).Hours()/24+0.5) + 1
	pFrom, pTo := from.AddDate(0, 0, -n), from.AddDate(0, 0, -1)
	if cut := s.cut(now); cut == "" || pTo.Format(dateFormat) >= cut {
		var prev Entry
		for d := pFrom; !d.After(pTo); d = d.AddDate(0, 0, 1) {
			k := d.Format(dateFormat)
			for _, src := range all {
				sum := src.days[k]
				if sum == nil {
					continue
				}
				if len(where) == 0 {
					prev.addEntry(sum.Entry)
					continue
				}
				for fk, e := range src.flows(k) {
					if matches(fk, where) {
						prev.addEntry(e)
					}
				}
			}
		}
		r.Prev = &prev
	}
	return r
}

func matches(k flowKey, where map[string]string) bool {
	for dim, want := range where {
		got := k.get(dim)
		if want == "\x00other" || want == otherKey {
			if got != otherKey {
				return false
			}
			continue
		}
		if got != want {
			return false
		}
	}
	return true
}

// bumpQuery counts into a query's map, which has no cap.
func bumpQuery(m map[string]Entry, key string, e Entry) {
	cur := m[key]
	cur.addEntry(e)
	m[key] = cur
}

// filtered keeps the entries whose key contains f, without regard to
// case. The unnamed Other is dropped: what it collects cannot be
// attributed to a name.
func filtered(m map[string]Entry, f string) map[string]Entry {
	out := make(map[string]Entry, len(m))
	for k, v := range m {
		if k != otherKey && strings.Contains(strings.ToLower(k), f) {
			out[k] = v
		}
	}
	return out
}

func merge(dst, src map[string]Entry) {
	for k, v := range src {
		e := dst[k]
		e.addEntry(v)
		dst[k] = e
	}
}

// rank lists the top keys by traffic (or connections), with the rest as
// one Other item.
func rank(m map[string]Entry, top int, byConns bool) []StatsItem {
	items := make([]StatsItem, 0, len(m))
	var other Entry
	for k, v := range m {
		if k == otherKey {
			other.addEntry(v)
			continue
		}
		items = append(items, StatsItem{Name: k, Up: v.Up, Down: v.Down, Conns: v.Conns})
	}
	weight := func(it StatsItem) int64 {
		if byConns {
			return it.Conns
		}
		return it.Up + it.Down
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := weight(items[i]), weight(items[j])
		if a != b {
			return a > b
		}
		return items[i].Name < items[j].Name
	})
	if len(items) > top {
		for _, it := range items[top:] {
			other.addEntry(Entry{Up: it.Up, Down: it.Down, Conns: it.Conns})
		}
		items = items[:top]
	}
	if other.Up+other.Down > 0 || other.Conns > 0 {
		items = append(items, StatsItem{Other: true, Up: other.Up, Down: other.Down, Conns: other.Conns})
	}
	return items
}

// Days returns a copy of the store's days.
func (s *Store) Days() map[string]*Day {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]*Day, len(s.days))
	for k, d := range s.days {
		c := *d
		c.Apps, c.Sites, c.Nodes = maps.Clone(d.Apps), maps.Clone(d.Sites), maps.Clone(d.Nodes)
		c.Rules, c.Policies, c.Countries = maps.Clone(d.Rules), maps.Clone(d.Policies), maps.Clone(d.Countries)
		out[k] = &c
	}
	return out
}
