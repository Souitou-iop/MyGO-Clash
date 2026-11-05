package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/netip"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/usage"
)

// Stats is the traffic statistics page.
type Stats struct{ a *App }

// Query totals the traffic of a range of days, and ranks what carried it.
// The device of the query is this one (""), every device of the sync vault
// ("all"), or one of them by its ID.
func (s Stats) Query(ctx context.Context, q usage.StatsQuery) (usage.StatsReport, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return usage.StatsReport{}, err
	}
	me := s.a.syncer.state.DeviceID
	switch q.Device {
	case "", me:
		return s.a.usage.Query(q), nil
	case "all":
		var srcs []usage.Source
		for _, r := range s.a.remoteUsage() {
			srcs = append(srcs, r.days)
		}
		return s.a.usage.QueryWith(q, true, srcs...), nil
	}
	for _, r := range s.a.remoteUsage() {
		if r.id == q.Device {
			return s.a.usage.QueryWith(q, false, r.days), nil
		}
	}
	return s.a.usage.QueryWith(q, false), nil
}

// StatsDevice is a device whose statistics can be shown.
type StatsDevice struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	OS      string    `json:"os"`
	This    bool      `json:"this"`
	Updated time.Time `json:"updated,omitzero"`
}

// Devices lists the devices with statistics: this one, and those whose
// statistics came through sync.
func (s Stats) Devices(ctx context.Context) ([]StatsDevice, error) {
	out := []StatsDevice{{ID: s.a.syncer.state.DeviceID, Name: s.a.syncer.deviceName(), OS: currentOS(), This: true}}
	s.a.syncer.mu.Lock()
	names := s.a.syncer.names
	s.a.syncer.mu.Unlock()
	for _, r := range s.a.remoteUsage() {
		name := r.Name
		if n := names[r.id]; n != "" {
			name = n
		}
		out = append(out, StatsDevice{ID: r.id, Name: name, OS: r.OS, Updated: r.Updated})
	}
	return out, nil
}

func currentOS() string { return runtime.GOOS }

// Clear forgets the statistics.
func (s Stats) Clear(ctx context.Context) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	return s.a.usage.Clear()
}

// Export saves the statistics of a range to a file: "csv" for a
// spreadsheet, "json" to import elsewhere. From and To are dates, "" for
// no bound. It returns the path, "" when the user cancelled.
func (s Stats) Export(ctx context.Context, format, from, to string) (string, error) {
	var data []byte
	ext := "json"
	if format == "csv" {
		data, ext = s.a.usage.ExportCSV(from, to), "csv"
	} else {
		data = s.a.usage.Export(from, to, s.a.syncer.deviceName())
	}
	name := "MyGO-Clash-traffic-" + time.Now().Format("20060102") + "." + ext
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{Parent: mygo.CallerWindow(ctx), DefaultPath: name})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o600)
}

// Import adds the statistics of a file Export wrote, and returns how many
// days it read; 0 when the user cancelled.
func (s Stats) Import(ctx context.Context) (int, error) {
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: mygo.CallerWindow(ctx), Filters: []mygo.FileFilter{{Name: "MyGO-Clash traffic", Extensions: []string{"json"}}}})
	if err != nil || len(paths) == 0 {
		return 0, err
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		return 0, err
	}
	n, err := s.a.usage.Import(data)
	if err != nil {
		return 0, errors.New(tr(s.a, "statsImportBad"))
	}
	return n, s.a.usage.Flush()
}

// Closed returns a page of the connections that closed, the latest first.
func (s Stats) Closed(ctx context.Context, q usage.ClosedQuery) (usage.ClosedPage, error) {
	return s.a.usage.Closed(q), nil
}

// ClearClosed forgets the connections that closed.
func (s Stats) ClearClosed(ctx context.Context) error { return s.a.usage.ClearClosed() }

// ---- Options ----

// applyStatsOptions gives the store the settings' retention and
// exclusions.
func (a *App) applyStatsOptions(st config.Settings) {
	a.usage.SetOptions(usage.Options{Keep: st.Stats.KeepDays, Exclude: statsExclude(st.Stats), Country: a.geo.lookup, HostCountry: a.geo.lookupHost})
}

// statsExclude matches the connections the settings leave out of the
// statistics, nil when none.
func statsExclude(st config.Stats) func(c *coreapi.Connection) bool {
	var domains, procs []string
	var prefixes []netip.Prefix
	for _, p := range st.Exclude {
		p = strings.ToLower(strings.TrimSpace(p))
		switch {
		case strings.HasPrefix(p, "process:"):
			if n := strings.TrimSpace(strings.TrimPrefix(p, "process:")); n != "" {
				procs = append(procs, n)
			}
		default:
			if pr, err := netip.ParsePrefix(p); err == nil {
				prefixes = append(prefixes, pr.Masked())
			} else if ip, err := netip.ParseAddr(p); err == nil {
				prefixes = append(prefixes, netip.PrefixFrom(ip, ip.BitLen()))
			} else if d := strings.Trim(strings.TrimPrefix(p, "*."), "."); d != "" {
				domains = append(domains, d)
			}
		}
	}
	if st.ExcludeLocal {
		for _, c := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "169.254.0.0/16", "100.64.0.0/10", "fc00::/7", "fe80::/10", "::1/128", "fd7a:115c:a1e0::/48"} {
			prefixes = append(prefixes, netip.MustParsePrefix(c))
		}
		domains = append(domains, "local", "lan", "ts.net", "localhost")
	}
	if len(domains)+len(procs)+len(prefixes) == 0 {
		return nil
	}
	return func(c *coreapi.Connection) bool {
		host := strings.ToLower(strings.TrimSuffix(c.Metadata.Host, "."))
		if host == "" {
			host = strings.ToLower(c.Metadata.SniffHost)
		}
		for _, d := range domains {
			if host == d || strings.HasSuffix(host, "."+d) {
				return true
			}
		}
		if ip, err := netip.ParseAddr(c.Metadata.DestinationIP); err == nil {
			ip = ip.Unmap()
			for _, p := range prefixes {
				if p.Contains(ip) {
					return true
				}
			}
		}
		proc := strings.ToLower(c.Metadata.Process)
		for _, p := range procs {
			if proc == p || strings.TrimSuffix(proc, ".exe") == strings.TrimSuffix(p, ".exe") {
				return true
			}
		}
		return false
	}
}

// ---- Countries ----

// geoCache remembers the countries of addresses, which the core looks up
// in its GeoIP database, and of host names without an address of their
// own (fake-ip DNS), which the core's resolver resolves first.
type geoCache struct {
	a     *App
	mu    sync.Mutex
	m     map[string]string // address: country
	hosts map[string]string // host: address
}

func newGeoCache(a *App) *geoCache {
	return &geoCache{a: a, m: map[string]string{}, hosts: map[string]string{}}
}

// lookup is the country of an address that prepare looked up.
func (g *geoCache) lookup(ip string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.m[ip]
}

// lookupHost is the country of a host that prepare resolved.
func (g *geoCache) lookupHost(host string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.m[g.hosts[strings.ToLower(host)]]
}

func usableIP(s string) bool {
	a, err := netip.ParseAddr(s)
	if err != nil {
		return false
	}
	a = a.Unmap()
	return a.IsGlobalUnicast() && !a.IsPrivate() && !netip.MustParsePrefix("198.18.0.0/15").Contains(a)
}

// prepare looks the new destinations of a snapshot up, at once, before the
// statistics count it.
func (g *geoCache) prepare(snap coreapi.Connections) {
	c := g.a.core.Client()
	if c == nil {
		return
	}
	var want, names []string
	g.mu.Lock()
	if len(g.m) > 20000 || len(g.hosts) > 20000 {
		g.m, g.hosts = map[string]string{}, map[string]string{}
	}
	for i := range snap.Connections {
		conn := &snap.Connections[i]
		if ip := usage.DestinationAddr(conn); usableIP(ip) {
			if _, ok := g.m[ip]; !ok {
				g.m[ip] = "" // asked once, even if the answer does not come
				want = append(want, ip)
			}
			continue
		}
		host := strings.ToLower(conn.Metadata.Host)
		if host == "" {
			host = strings.ToLower(conn.Metadata.SniffHost)
		}
		if _, ok := g.hosts[host]; host != "" && !ok && len(names) < 16 {
			g.hosts[host] = ""
			names = append(names, host)
		}
	}
	g.mu.Unlock()
	ctx, cancel := context.WithTimeout(g.a.ctx, 2*time.Second)
	defer cancel()
	if len(names) > 0 {
		var wg sync.WaitGroup
		for _, h := range names {
			wg.Add(1)
			go func() {
				defer wg.Done()
				r, err := c.DNSQuery(ctx, h, "A")
				if err != nil {
					return
				}
				for _, an := range r.Answer {
					if an.Type == 1 && usableIP(an.Data) {
						g.mu.Lock()
						g.hosts[h] = an.Data
						if _, ok := g.m[an.Data]; !ok {
							g.m[an.Data] = ""
							want = append(want, an.Data)
						}
						g.mu.Unlock()
						return
					}
				}
			}()
		}
		wg.Wait()
	}
	if len(want) == 0 {
		return
	}
	codes, err := c.GeoIP(ctx, want)
	if err != nil {
		return
	}
	g.mu.Lock()
	for ip, cc := range codes {
		g.m[ip] = cc
	}
	g.mu.Unlock()
}

// ---- Statistics of the other devices, through sync ----

// remoteStats is the statistics of another device of the vault.
type remoteStats struct {
	id      string
	days    usage.Source
	Name    string          `json:"name"`
	OS      string          `json:"os"`
	Updated time.Time       `json:"updated"`
	Usage   json.RawMessage `json:"usage"`
}

func (a *App) remoteUsageDir() string { return filepath.Join(a.dirs.Sync, "usage") }

// remoteUsage reads the statistics of the other devices.
func (a *App) remoteUsage() []remoteStats {
	entries, _ := os.ReadDir(a.remoteUsageDir())
	me := a.syncer.state.DeviceID
	var out []remoteStats
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		if e.IsDir() || id == me || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(a.remoteUsageDir(), e.Name()))
		if err != nil {
			continue
		}
		var r remoteStats
		if json.Unmarshal(data, &r) != nil {
			continue
		}
		days, err := usage.ParseCompact(r.Usage)
		if err != nil {
			continue
		}
		r.id, r.days = id, days
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ownUsage is this device's statistics as sync sends them: the totals,
// made again once an hour at most, so that a sync sends them that often.
type ownUsage struct {
	mu   sync.Mutex
	hour string
	data []byte
}

func (a *App) ownUsageData() []byte {
	o := &a.usageOut
	o.mu.Lock()
	defer o.mu.Unlock()
	hour := time.Now().Format("2006-01-02T15")
	if o.data == nil || o.hour != hour {
		r := remoteStats{Name: a.syncer.deviceName(), OS: currentOS(), Updated: time.Now().UTC().Truncate(time.Hour), Usage: a.usage.Compact(60)}
		o.data, _ = json.Marshal(r)
		o.hour = hour
	}
	return o.data
}

// readRemoteUsage returns the files of the other devices' statistics, by
// sync path.
func (a *App) readRemoteUsage() map[string][]byte {
	out := map[string][]byte{}
	entries, _ := os.ReadDir(a.remoteUsageDir())
	me := a.syncer.state.DeviceID
	for _, e := range entries {
		id := strings.TrimSuffix(e.Name(), ".json")
		if e.IsDir() || id == me || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if data, err := os.ReadFile(filepath.Join(a.remoteUsageDir(), e.Name())); err == nil {
			out[usagePrefix+id] = data
		}
	}
	return out
}

// writeRemoteUsage keeps another device's statistics from sync; nil
// deletes them.
func (a *App) writeRemoteUsage(id string, data []byte) error {
	if id == "" || strings.ContainsAny(id, `/\.`) {
		return fmt.Errorf("invalid device %q", id)
	}
	path := filepath.Join(a.remoteUsageDir(), id+".json")
	if data == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(a.remoteUsageDir(), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		log.Printf("keep the statistics of %s: %v", id, err)
		return err
	}
	return nil
}
