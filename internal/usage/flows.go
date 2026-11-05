package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// flowKey is one combination of the dimensions, in one hour.
type flowKey struct {
	App, Site, Node, Rule, Policy, Country string
	Hour                                   int8
}

func otherFlow(hour int) flowKey {
	return flowKey{otherKey, otherKey, otherKey, otherKey, otherKey, otherKey, int8(hour)}
}

func (k flowKey) get(dim string) string {
	switch dim {
	case DimApp:
		return k.App
	case DimSite:
		return k.Site
	case DimNode:
		return k.Node
	case DimRule:
		return k.Rule
	case DimPolicy:
		return k.Policy
	case DimCountry:
		return k.Country
	}
	return ""
}

// dayFlows are the combinations of a day.
type dayFlows struct {
	m     map[flowKey]Entry
	dirty bool
}

func (f *dayFlows) add(k flowKey, e Entry) {
	cur, ok := f.m[k]
	if !ok && len(f.m) >= maxFlows {
		k = otherFlow(int(k.Hour))
		cur = f.m[k]
	}
	cur.addEntry(e)
	f.m[k] = cur
	f.dirty = true
}

// flowRow is a combination as files hold it:
// [app, site, node, rule, policy, country, hour, up, down, conns].
type flowRow []any

type flowFile struct {
	Rows []flowRow `json:"rows"`
}

func encodeFlows(m map[flowKey]Entry) []byte {
	rows := make([]flowRow, 0, len(m))
	for k, e := range m {
		rows = append(rows, flowRow{k.App, k.Site, k.Node, k.Rule, k.Policy, k.Country, k.Hour, e.Up, e.Down, e.Conns})
	}
	data, _ := json.Marshal(flowFile{rows})
	return data
}

func decodeFlows(data []byte) (map[flowKey]Entry, error) {
	var raw struct {
		Rows [][]json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	m := make(map[flowKey]Entry, len(raw.Rows))
	for _, r := range raw.Rows {
		if len(r) < 9 {
			continue
		}
		var k flowKey
		var e Entry
		var hour int
		strs := []*string{&k.App, &k.Site, &k.Node, &k.Rule, &k.Policy, &k.Country}
		ok := true
		for i, p := range strs {
			if json.Unmarshal(r[i], p) != nil {
				ok = false
			}
		}
		if !ok || json.Unmarshal(r[6], &hour) != nil || json.Unmarshal(r[7], &e.Up) != nil || json.Unmarshal(r[8], &e.Down) != nil {
			continue
		}
		if len(r) > 9 {
			_ = json.Unmarshal(r[9], &e.Conns)
		}
		k.Hour = int8(hour)
		cur := m[k]
		cur.addEntry(e)
		m[k] = cur
	}
	return m, nil
}

func (s *Store) flowPath(date string) string {
	return filepath.Join(s.dir, "flows", date+".json")
}

// flowsFor returns the combinations of a day to count into, read from its
// file the first time.
func (s *Store) flowsFor(date string) *dayFlows {
	if f, ok := s.flows[date]; ok {
		return f
	}
	f := &dayFlows{m: map[flowKey]Entry{}}
	if data, err := os.ReadFile(s.flowPath(date)); err == nil {
		if m, err := decodeFlows(data); err == nil {
			f.m = m
		}
	}
	s.flows[date] = f
	// Only the days being counted stay in memory; queries read the others.
	today := s.now().Format(dateFormat)
	for k, o := range s.flows {
		if k != date && k != today && !o.dirty {
			delete(s.flows, k)
		}
	}
	return f
}

// flowsOf returns the combinations of a day for a query, nil when the day
// has none recorded (days from before they were).
func (s *Store) flowsOf(date string) map[flowKey]Entry {
	if f, ok := s.flows[date]; ok {
		return f.m
	}
	data, err := os.ReadFile(s.flowPath(date))
	if err != nil {
		return nil
	}
	m, err := decodeFlows(data)
	if err != nil {
		return nil
	}
	return m
}

// dirtyFlows encodes the days whose combinations changed.
func (s *Store) dirtyFlows() map[string][]byte {
	var out map[string][]byte
	for date, f := range s.flows {
		if !f.dirty {
			continue
		}
		if out == nil {
			out = map[string][]byte{}
		}
		out[date] = encodeFlows(f.m)
		f.dirty = false
	}
	return out
}

func (s *Store) writeFlows(date string, data []byte) error {
	err := writeAtomic(s.flowPath(date), data)
	if err != nil {
		s.mu.Lock()
		if f, ok := s.flows[date]; ok {
			f.dirty = true
		}
		s.mu.Unlock()
	}
	return err
}

// pruneFlowFiles deletes the files of days before cut.
func (s *Store) pruneFlowFiles(cut string) {
	entries, err := os.ReadDir(filepath.Join(s.dir, "flows"))
	if err != nil {
		return
	}
	for _, e := range entries {
		date := strings.TrimSuffix(e.Name(), ".json")
		if len(date) == len(dateFormat) && date < cut {
			_ = os.Remove(filepath.Join(s.dir, "flows", e.Name()))
		}
	}
}
