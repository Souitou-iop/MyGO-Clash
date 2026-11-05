// Package usage counts the traffic that passed the core, per day, app,
// site, node, rule, policy group and country, from the cumulative totals
// of its connections, and keeps the connections that closed.
//
// The totals of each day live in usage.json; the combinations of a day
// (which app reached which site through which node, by the hour) in a file
// of their own under usage/flows, read when a query narrows by one of
// them.
package usage

import (
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

const (
	// KeepDays is how many days of history are kept by default.
	KeepDays = 90
	// maxKeys caps the distinct names of one dimension in a day; the rest
	// is counted under otherKey.
	maxKeys = 400
	// maxFlows caps the combinations of a day.
	maxFlows = 8000

	// Unknown names what has no name, such as a connection without a process.
	Unknown = "—"
	// otherKey holds the traffic of the keys beyond the cap.
	otherKey = "\x00other"

	dateFormat = "2006-01-02"
)

// Entry is bytes up and down, and the connections that opened.
type Entry struct {
	Up    int64 `json:"up"`
	Down  int64 `json:"down"`
	Conns int64 `json:"conns,omitempty"`
}

func (e *Entry) add(up, down int64) { e.Up += up; e.Down += down }

func (e *Entry) addEntry(o Entry) { e.Up += o.Up; e.Down += o.Down; e.Conns += o.Conns }

// Dimensions a day is counted by.
const (
	DimApp     = "app"
	DimSite    = "site"
	DimNode    = "node"
	DimRule    = "rule"
	DimPolicy  = "policy"
	DimCountry = "country"
)

// Dims lists the dimensions.
var Dims = []string{DimApp, DimSite, DimNode, DimRule, DimPolicy, DimCountry}

// Day is the traffic of one local date.
type Day struct {
	Entry
	Hours     [24]Entry        `json:"hours"`
	Apps      map[string]Entry `json:"apps"`
	Sites     map[string]Entry `json:"sites"`
	Nodes     map[string]Entry `json:"nodes"`
	Rules     map[string]Entry `json:"rules,omitempty"`
	Policies  map[string]Entry `json:"policies,omitempty"`
	Countries map[string]Entry `json:"countries,omitempty"`
}

func newDay() *Day { return (&Day{}).fixed() }

// fixed makes the maps of a day read from a file usable.
func (d *Day) fixed() *Day {
	for _, m := range []*map[string]Entry{&d.Apps, &d.Sites, &d.Nodes, &d.Rules, &d.Policies, &d.Countries} {
		if *m == nil {
			*m = map[string]Entry{}
		}
	}
	return d
}

// dim returns the map of a dimension.
func (d *Day) dim(name string) map[string]Entry {
	switch name {
	case DimApp:
		return d.Apps
	case DimSite:
		return d.Sites
	case DimNode:
		return d.Nodes
	case DimRule:
		return d.Rules
	case DimPolicy:
		return d.Policies
	case DimCountry:
		return d.Countries
	}
	return nil
}

// Options are what the app sets on the store.
type Options struct {
	// Keep is how many days to keep; 0 keeps everything.
	Keep int
	// Exclude reports the connections not to count.
	Exclude func(c *coreapi.Connection) bool
	// Country returns the country code of an address, "" if not known.
	Country func(ip string) string
	// HostCountry returns the country code of a host name, for the
	// connections that have no address of their own (fake-ip DNS).
	HostCountry func(host string) string
}

// Store collects the traffic and keeps it in files.
type Store struct {
	path string
	dir  string
	now  func() time.Time

	mu     sync.Mutex
	opts   Options
	days   map[string]*Day // by local date
	flows  map[string]*dayFlows
	last   map[string]Entry
	open   map[string]*coreapi.Connection
	closed []Closed // newest last
	total  Entry    // the core's counters at the last snapshot
	seeded bool
	dirty  bool
	// closedDirty is set when the closed connections changed.
	closedDirty bool
}

type file struct {
	Days map[string]*Day `json:"days"`
}

// Open loads the history in path, which may not exist yet.
func Open(path string) *Store {
	s := &Store{
		path: path, dir: filepath.Join(filepath.Dir(path), "usage"), now: time.Now,
		opts: Options{Keep: KeepDays},
		days: map[string]*Day{}, flows: map[string]*dayFlows{}, last: map[string]Entry{}, open: map[string]*coreapi.Connection{},
	}
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
	s.loadClosed()
	s.pruneAt(s.now())
	return s
}

// SetOptions changes the options; a shorter Keep prunes at once.
func (s *Store) SetOptions(o Options) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if o.Keep < 0 {
		o.Keep = KeepDays
	}
	s.opts = o
	s.pruneAt(s.now())
}

// Keep is how many days the store keeps, 0 for all.
func (s *Store) Keep() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opts.Keep
}

// labels is how a connection is counted in each dimension.
type labels struct {
	app, site, node, rule, policy, country string
}

func (s *Store) labelsOf(c *coreapi.Connection) labels {
	return labels{process(c), Site(c.Metadata), node(c), rule(c), policy(c), s.country(c)}
}

func (l labels) key(hour int) flowKey {
	return flowKey{App: l.app, Site: l.site, Node: l.node, Rule: l.rule, Policy: l.policy, Country: l.country, Hour: int8(hour)}
}

// Observe counts what moved since the last snapshot. The core's own totals
// count everything; the connections tell where it went, but not for those
// that opened and closed between two snapshots (most web requests), whose
// share is counted as other. The first call only takes the counters: a
// core that outlived the app has been counting for longer than the app was
// there.
func (s *Store) Observe(snap coreapi.Connections) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	total := Entry{Up: snap.UploadTotal, Down: snap.DownloadTotal}
	seen := make(map[string]Entry, len(snap.Connections))
	open := make(map[string]*coreapi.Connection, len(snap.Connections))
	type part struct {
		c        *coreapi.Connection
		up, down int64
		opened   bool
	}
	var parts []part
	var named, excluded Entry
	for i := range snap.Connections {
		c := &snap.Connections[i]
		cur := Entry{Up: c.Upload, Down: c.Download}
		seen[c.ID] = cur
		open[c.ID] = c
		if !s.seeded {
			continue
		}
		prev, known := s.last[c.ID] // zero for a connection not seen before
		up, down := cur.Up-prev.Up, cur.Down-prev.Down
		if up < 0 || down < 0 { // the counters started over
			up, down = cur.Up, cur.Down
		}
		if s.opts.Exclude != nil && s.opts.Exclude(c) {
			excluded.add(up, down)
			continue
		}
		if up == 0 && down == 0 && known {
			continue
		}
		parts = append(parts, part{c, up, down, !known})
		named.add(up, down)
	}
	if s.seeded {
		for id, c := range s.open {
			if _, ok := seen[id]; !ok {
				s.recordClosed(c, s.last[id], now)
			}
		}
	}
	moved := Entry{Up: total.Up - s.total.Up, Down: total.Down - s.total.Down}
	if moved.Up < 0 || moved.Down < 0 { // the core started over
		moved = total
	}
	seeded := s.seeded
	s.last, s.open, s.total, s.seeded = seen, open, total, true
	if !seeded {
		return
	}
	moved.Up, moved.Down = max(moved.Up-excluded.Up, 0), max(moved.Down-excluded.Down, 0)
	// The totals and the connections come from one snapshot, but stay
	// safe should they disagree.
	moved.Up, moved.Down = max(moved.Up, named.Up), max(moved.Down, named.Down)
	var opened int64
	for _, p := range parts {
		if p.opened {
			opened++
		}
	}
	if moved.Up == 0 && moved.Down == 0 && opened == 0 {
		return
	}
	k := now.Format(dateFormat)
	day := s.day(now)
	fl := s.flowsFor(k)
	hour := now.Hour()
	day.add(moved.Up, moved.Down)
	day.Conns += opened
	h := day.Hours[hour]
	h.add(moved.Up, moved.Down)
	h.Conns += opened
	day.Hours[hour] = h
	for _, p := range parts {
		e := Entry{Up: p.up, Down: p.down}
		if p.opened {
			e.Conns = 1
		}
		l := s.labelsOf(p.c)
		bump(day.Apps, l.app, e)
		bump(day.Sites, l.site, e)
		bump(day.Nodes, l.node, e)
		bump(day.Rules, l.rule, e)
		bump(day.Policies, l.policy, e)
		bump(day.Countries, l.country, e)
		fl.add(l.key(hour), e)
	}
	if up, down := moved.Up-named.Up, moved.Down-named.Down; up > 0 || down > 0 {
		e := Entry{Up: up, Down: down}
		for _, d := range Dims {
			m := day.dim(d)
			o := m[otherKey]
			o.addEntry(e)
			m[otherKey] = o
		}
		fl.add(otherFlow(hour), e)
	}
	s.dirty = true
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

func bump(m map[string]Entry, key string, e Entry) {
	cur, ok := m[key]
	if !ok && len(m) >= maxKeys {
		key = otherKey
		cur = m[key]
	}
	cur.addEntry(e)
	m[key] = cur
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

// policy is the group the rule chose, last in the chain.
func policy(c *coreapi.Connection) string {
	if len(c.Chains) == 0 || c.Chains[len(c.Chains)-1] == "" {
		return Unknown
	}
	return c.Chains[len(c.Chains)-1]
}

// rule is the rule that matched, with its payload.
func rule(c *coreapi.Connection) string {
	r := strings.TrimSpace(c.Rule)
	if r == "" {
		return Unknown
	}
	if p := strings.TrimSpace(c.RulePayload); p != "" {
		return r + "," + p
	}
	return r
}

// country is where the connection went: the country of its destination
// address, or of what the core's rules found out, "" when not known.
func (s *Store) country(c *coreapi.Connection) string {
	if ip, err := netip.ParseAddr(DestinationAddr(c)); err == nil {
		ip = ip.Unmap()
		if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || tailnetRange.Contains(ip) {
			return "LAN"
		}
		if s.opts.Country != nil && !fakeIPRange.Contains(ip) {
			if cc := strings.ToUpper(s.opts.Country(ip.String())); len(cc) == 2 {
				return cc
			}
		}
	}
	for _, g := range c.Metadata.DestinationGeoIP {
		if len(g) == 2 {
			return strings.ToUpper(g)
		}
	}
	if s.opts.HostCountry != nil {
		host := c.Metadata.Host
		if host == "" {
			host = c.Metadata.SniffHost
		}
		if host != "" {
			if cc := strings.ToUpper(s.opts.HostCountry(host)); len(cc) == 2 {
				return cc
			}
		}
	}
	return Unknown
}

// DestinationAddr is the address a connection went to: its destination,
// or, for a direct connection whose destination is a name or a fake
// address (fake-ip DNS), the address the core dialed.
func DestinationAddr(c *coreapi.Connection) string {
	ip, err := netip.ParseAddr(c.Metadata.DestinationIP)
	if err == nil && !fakeIPRange.Contains(ip.Unmap()) {
		return c.Metadata.DestinationIP
	}
	if node(c) == "DIRECT" && c.Metadata.RemoteDestination != "" {
		r := c.Metadata.RemoteDestination
		if ap, err := netip.ParseAddrPort(r); err == nil {
			return ap.Addr().String()
		}
		if a, err := netip.ParseAddr(r); err == nil {
			return a.String()
		}
	}
	return c.Metadata.DestinationIP
}

var (
	fakeIPRange  = netip.MustParsePrefix("198.18.0.0/15")
	tailnetRange = netip.MustParsePrefix("100.64.0.0/10")
)

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

// cut is the first date kept at t, "" when everything is.
func (s *Store) cut(t time.Time) string {
	if s.opts.Keep <= 0 {
		return ""
	}
	return t.AddDate(0, 0, -(s.opts.Keep - 1)).Format(dateFormat)
}

func (s *Store) pruneAt(t time.Time) {
	cut := s.cut(t)
	if cut == "" {
		return
	}
	for k := range s.days {
		if k < cut {
			delete(s.days, k)
			s.dirty = true
		}
	}
	for k := range s.flows {
		if k < cut {
			delete(s.flows, k)
		}
	}
	s.pruneFlowFiles(cut)
}

// Flush writes the history if it changed.
func (s *Store) Flush() error {
	s.mu.Lock()
	var data []byte
	var err error
	if s.dirty {
		data, err = json.Marshal(file{s.days})
		s.dirty = false
	}
	flows := s.dirtyFlows()
	closed := s.closedData()
	s.mu.Unlock()
	if data != nil && err == nil {
		err = writeAtomic(s.path, data)
		if err != nil {
			s.mu.Lock()
			s.dirty = true
			s.mu.Unlock()
		}
	}
	for date, data := range flows {
		if ferr := s.writeFlows(date, data); ferr != nil && err == nil {
			err = ferr
		}
	}
	if closed != nil {
		if cerr := s.writeClosed(closed); cerr != nil && err == nil {
			err = cerr
		}
	}
	return err
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
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

// Clear forgets the history and the closed connections.
func (s *Store) Clear() error {
	s.mu.Lock()
	s.days = map[string]*Day{}
	s.flows = map[string]*dayFlows{}
	s.closed = nil
	s.dirty, s.closedDirty = true, true
	_ = os.RemoveAll(filepath.Join(s.dir, "flows"))
	s.mu.Unlock()
	return s.Flush()
}
