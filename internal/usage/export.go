package usage

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"
)

const exportFormat = "mygo-clash-usage/v1"

// exported is the file Export writes and Import reads.
type exported struct {
	Format   string                     `json:"format"`
	Exported time.Time                  `json:"exported"`
	Device   string                     `json:"device,omitempty"`
	Days     map[string]*Day            `json:"days"`
	Flows    map[string]json.RawMessage `json:"flows,omitempty"`
}

func (s *Store) daysIn(from, to string) []string {
	var out []string
	for k := range s.days {
		if (from == "" || k >= from) && (to == "" || k <= to) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Export returns the days from from to to (both included, "" for no
// bound), with their combinations, as JSON that Import reads back.
func (s *Store) Export(from, to, device string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	ex := exported{Format: exportFormat, Exported: s.now().UTC(), Device: device, Days: map[string]*Day{}, Flows: map[string]json.RawMessage{}}
	for _, k := range s.daysIn(from, to) {
		ex.Days[k] = s.days[k]
		if m := s.flowsOf(k); m != nil {
			ex.Flows[k] = encodeFlows(m)
		}
	}
	data, _ := json.MarshalIndent(ex, "", " ")
	return data
}

// ExportCSV returns the combinations of the days from from to to, one row
// per day, hour, app, site, node, rule, policy and country; a day from
// before combinations were recorded gives its total alone.
func (s *Store) ExportCSV(from, to string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b bytes.Buffer
	b.WriteString("\xef\xbb\xbf") // spreadsheets read UTF-8 by it
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"date", "hour", "app", "site", "node", "rule", "policy", "country", "upload", "download", "connections"})
	name := func(v string) string {
		if v == otherKey {
			return "(other)"
		}
		return v
	}
	for _, k := range s.daysIn(from, to) {
		m := s.flowsOf(k)
		if m == nil {
			d := s.days[k]
			_ = w.Write([]string{k, "", "", "", "", "", "", "", strconv.FormatInt(d.Up, 10), strconv.FormatInt(d.Down, 10), strconv.FormatInt(d.Conns, 10)})
			continue
		}
		keys := make([]flowKey, 0, len(m))
		for fk := range m {
			keys = append(keys, fk)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].Hour != keys[j].Hour {
				return keys[i].Hour < keys[j].Hour
			}
			return m[keys[i]].Up+m[keys[i]].Down > m[keys[j]].Up+m[keys[j]].Down
		})
		for _, fk := range keys {
			e := m[fk]
			_ = w.Write([]string{k, fmt.Sprintf("%02d", fk.Hour), name(fk.App), name(fk.Site), name(fk.Node), name(fk.Rule), name(fk.Policy), name(fk.Country),
				strconv.FormatInt(e.Up, 10), strconv.FormatInt(e.Down, 10), strconv.FormatInt(e.Conns, 10)})
		}
	}
	w.Flush()
	return b.Bytes()
}

// Import adds the days of a file Export wrote. A day both have takes the
// larger count of each figure, so that importing a file twice, or a file of
// this device's own, counts nothing twice. It returns how many days it
// read.
func (s *Store) Import(data []byte) (int, error) {
	var ex exported
	if err := json.Unmarshal(data, &ex); err != nil || ex.Format != exportFormat {
		return 0, errors.New("the file is not a MyGO-Clash statistics export")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for k, d := range ex.Days {
		if _, err := time.Parse(dateFormat, k); err != nil || d == nil {
			continue
		}
		d.fixed()
		cur, ok := s.days[k]
		if !ok {
			s.days[k] = d
		} else {
			maxDay(cur, d)
		}
		n++
		if raw, ok := ex.Flows[k]; ok {
			m, err := decodeFlows(raw)
			if err != nil {
				continue
			}
			f := s.flowsFor(k)
			if f.m == nil {
				f.m = map[flowKey]Entry{}
			}
			for fk, e := range m {
				f.m[fk] = maxEntry(f.m[fk], e)
			}
			f.dirty = true
		}
	}
	s.dirty = true
	s.pruneAt(s.now())
	return n, nil
}

func maxEntry(a, b Entry) Entry {
	return Entry{Up: max(a.Up, b.Up), Down: max(a.Down, b.Down), Conns: max(a.Conns, b.Conns)}
}

func maxDay(dst, src *Day) {
	dst.Entry = maxEntry(dst.Entry, src.Entry)
	for h := range dst.Hours {
		dst.Hours[h] = maxEntry(dst.Hours[h], src.Hours[h])
	}
	for _, dim := range Dims {
		d := dst.dim(dim)
		for k, v := range src.dim(dim) {
			d[k] = maxEntry(d[k], v)
		}
	}
}

// Compact returns the days' totals, with the top names of each dimension
// and the rest as other, for another device to show.
func (s *Store) Compact(top int) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]*Day{}
	for k, d := range s.days {
		c := &Day{Entry: d.Entry, Hours: d.Hours}
		c.fixed()
		for _, dim := range Dims {
			trim(c.dim(dim), d.dim(dim), top)
		}
		out[k] = c
	}
	data, _ := json.Marshal(file{out})
	return data
}

func trim(dst, src map[string]Entry, top int) {
	type kv struct {
		k string
		v Entry
	}
	list := make([]kv, 0, len(src))
	for k, v := range src {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if a, b := list[i].v.Up+list[i].v.Down, list[j].v.Up+list[j].v.Down; a != b {
			return a > b
		}
		return list[i].k < list[j].k
	})
	for i, e := range list {
		if i < top && e.k != otherKey {
			dst[e.k] = e.v
			continue
		}
		o := dst[otherKey]
		o.addEntry(e.v)
		dst[otherKey] = o
	}
}

// ParseCompact reads what Compact wrote.
func ParseCompact(data []byte) (Source, error) {
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	out := Source{}
	for k, d := range f.Days {
		if d != nil {
			out[k] = d.fixed()
		}
	}
	return out, nil
}
