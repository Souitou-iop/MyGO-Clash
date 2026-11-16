package app

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/mygo-clash/mygo-clash/internal/app/locales"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/tools"
)

// The connectivity page, after MyIP (github.com/jason5ng32/MyIP): the
// addresses that sites at home and abroad see, the reachability of popular
// sites over several rounds, which streaming and AI services work, and the
// speed through the proxy.

var exitViews = []string{"domestic", "global", "cloudflare"}

const siteRounds = 5

// coreSettle is how long a core runs before the checks trust it.
const coreSettle = 3 * time.Second

type exitIP struct {
	info    *tools.IPInfo
	err     string
	loading bool
}

// unlockResult is a check's last result, as the page keeps it: the proxy
// it went through is the result's own Via.
type unlockResult struct {
	tools.Unlock
}

type speedResult struct {
	tools.SpeedSample
	At  time.Time `json:"at"`
	Via string    `json:"via,omitempty"`
}

// connPage is what the connectivity page keeps between frames.
type connectivityPage struct {
	started bool
	ips     map[string]*exitIP

	sites      []tools.Site
	rounds     map[string][]int
	sitesBusy  bool
	sitesRun   uint64      // site tests started, which replay the bars growing
	roundsVer  uint64      // how many times the rounds changed
	derived    *ctyDerived // what the page worked out of them (derived)
	derivedKey derivedKey
	// landed is when each round of each site landed, for the mosaic's
	// cells to pop in from.
	landed map[string]*[siteRounds]time.Time

	unlocks  []tools.Unlock
	results  map[string]unlockResult
	checking map[string]bool

	speed     *speedResult
	speedBusy bool
	speedStop context.CancelFunc

	exit   string
	exitAt homeKey

	ipsAsked      bool      // the home page asked for the addresses
	wake          time.Time // the core start a frame is due after
	view          int       // the tile whose details show
	viewDir       float32   // 1: the details came from a tile further along
	cat           string    // the category of sites shown
	region        string    // in the region category, the country group shown
	regionOrder   []string
	regionOpen    bool
	regionPress   string    // the country row pressed
	regionPressAt time.Time // when
	regionLift    bool      // it is dragged
	regionFrom    int       // its slot as it was pressed
	regionDragY   float32   // how far the pointer moved it
	favoriteOrder []string
	hidden        []string // the presets taken off the page
	queue         *siteQueue
	favPress      string    // the favorite card pressed
	favPressAt    time.Time // when
	favLift       bool      // it is dragged
	favFrom       int       // its slot as it was pressed
	favDrag       ui.Point  // how far the pointer moved it
	lazyKey       any       // the large grid whose cards in view lazyLo..lazyHi are
	lazyLo        int
	lazyHi        int

	manage     bool
	tab        int
	hiddenOpen bool // the hidden presets show in the list
	unlockMgr  bool // the checks to run are being chosen
	// The settings the lists were last made from (ctyFollow).
	unlockSig, siteSig, favSig string
	custName                   string
	custURL                    string
	custGrp                    int
	custErr                    string
}

// uiCache keeps what the pages remember across launches, as the web page
// kept it in its local storage: the last checks and speed test.
type uiCache struct {
	Unlock map[string]unlockResult `json:"unlock,omitempty"`
	Speed  *speedResult            `json:"speed,omitempty"`
	// What the home page's sheets show over days (ui_history.go).
	ProfileUse map[string][]useMark `json:"profileUse,omitempty"`
	Takeover   []onMark             `json:"takeover,omitempty"`
	CoreStarts []int64              `json:"coreStarts,omitempty"`
}

var uiCacheMu sync.Mutex

func (a *App) uiCachePath() string { return filepath.Join(a.dirs.Data, "ui-cache.json") }

func (a *App) loadUICache() uiCache {
	uiCacheMu.Lock()
	defer uiCacheMu.Unlock()
	var c uiCache
	if data, err := os.ReadFile(a.uiCachePath()); err == nil {
		_ = json.Unmarshal(data, &c)
	}
	return c
}

func (a *App) saveUICache(fn func(*uiCache)) {
	c := a.loadUICache()
	fn(&c)
	uiCacheMu.Lock()
	defer uiCacheMu.Unlock()
	if data, err := json.Marshal(c); err == nil {
		_ = os.WriteFile(a.uiCachePath(), data, 0o600)
	}
}

// ipQualities keeps the quality of addresses for a while, so that a
// refresh or a second card asking about the same address does not ask the
// services again; a failure is kept too: the services are free, and
// hammering them helps nobody.
var ipQualities = struct {
	sync.Mutex
	m map[string]*ipQualityEntry
}{m: map[string]*ipQualityEntry{}}

type ipQualityEntry struct {
	at   time.Time
	done bool
	q    *tools.IPQuality
}

// ipQualityRows are the lines of a kv that say what kind of line an
// address is and how risky it looks. They appear after the lookup and
// stay away when it fails.
func (m *mainUI) ipQualityRows(c *ui.Context, ip string, masked bool) {
	q := m.ipQualityOf(ip)
	if q == nil || (q.Kind == "" && q.Risk < 0 && !q.VPN && !q.Tor && !q.Proxy) {
		return
	}
	m.kvRow(c, locales.IpqQuality(), func() { m.ipQualityView(c, q, masked) })
}

// ipQualityOf is the quality of an address, looked up once in a while;
// nil until the lookup is back, or when it failed.
func (m *mainUI) ipQualityOf(ip string) *tools.IPQuality {
	if ip == "" {
		return nil
	}
	ipQualities.Lock()
	defer ipQualities.Unlock()
	e := ipQualities.m[ip]
	if e == nil || time.Since(e.at) > 10*time.Minute {
		e = &ipQualityEntry{at: time.Now()}
		ipQualities.m[ip] = e
		win, a := m.window(), m.a
		go func() {
			q, err := (Tools{a}).IPQuality(context.Background(), ip)
			ipQualities.Lock()
			e.done = true
			if err == nil {
				e.q = &q
			}
			ipQualities.Unlock()
			if win != nil {
				win.Invalidate()
			}
		}()
	}
	return e.q
}

// exitNode follows the selections from the main group down to a node:
// where checks go out now.
func exitNode(v ProxiesView, mode string) string {
	groups := map[string]*ProxyGroup{}
	for i := range v.Groups {
		groups[v.Groups[i].Name] = &v.Groups[i]
	}
	var g *ProxyGroup
	if mode == "global" && v.Global != nil {
		g = v.Global
		groups[g.Name] = g
	} else {
		for i := range v.Groups {
			x := &v.Groups[i]
			if !x.Hidden && (x.Type == "Selector" || x.Type == "URLTest" || x.Type == "Fallback") {
				g = x
				break
			}
		}
	}
	name := ""
	for i := 0; g != nil && i < 10; i++ {
		name = g.Now
		g = groups[name]
	}
	return name
}

// ctyInit readies the page's state, from the cache of the last checks.
func (m *mainUI) ctyInit() {
	s, a := &m.cty, m.a
	if s.ips != nil {
		return
	}
	s.ips, s.rounds, s.checking, s.results = map[string]*exitIP{}, map[string][]int{}, map[string]bool{}, map[string]unlockResult{}
	cache := a.loadUICache()
	for k, v := range cache.Unlock {
		if k == "disney" { // the check's name before it took its site's
			k, v.ID = "disneyplus", "disneyplus"
		}
		s.results[k] = v
	}
	s.speed = cache.Speed
	s.sites = (Tools{a}).ConnectivitySites()
	s.regionOrder = regionOrder(a.settings.Get().UI.ConnectivityRegionOrder, tools.SiteRegions(s.sites))
	s.favoriteOrder = favoriteOrder(a.settings.Get().ConnectivityFavorites, s.sites)
	s.hidden = slices.Clone(a.settings.Get().ConnectivityHidden)
	if len(s.regionOrder) > 0 && s.region == "" {
		s.region = s.regionOrder[0]
	}
	s.view = ctySites
	s.unlocks = (Tools{a}).UnlockList()
}

// ctyFollow keeps the page's lists to the settings as they change, here or
// on another device through sync: the checks, the sites and 常用.
func (m *mainUI) ctyFollow() {
	s, st := &m.cty, m.st
	if sig := strings.Join(st.UnlockHidden, ",") + "|" + strings.Join(st.UnlockEnabled, ","); sig != s.unlockSig {
		s.unlockSig, s.unlocks = sig, tools.UnlockList(st.UnlockHidden, st.UnlockEnabled)
	}
	sig := strings.Join(st.ConnectivityHidden, ",")
	for _, c := range st.ConnectivityCustom {
		sig += "|" + c.ID + " " + c.URL
	}
	if sig != s.siteSig {
		s.siteSig = sig
		s.sites = tools.EffectiveSites(st.ConnectivityHidden, st.ConnectivityCustom)
		s.hidden = slices.Clone(st.ConnectivityHidden)
		s.regionOrder = regionOrder(st.UI.ConnectivityRegionOrder, tools.SiteRegions(s.sites))
		s.favSig = "" // the favorites of sites no longer there go
	}
	if sig := strings.Join(st.ConnectivityFavorites, ","); sig != s.favSig {
		s.favSig, s.favoriteOrder = sig, favoriteOrder(st.ConnectivityFavorites, s.sites)
	}
}

func regionOrder(saved, available []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(available))
	for _, id := range saved {
		if slices.Contains(available, id) && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range available {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func (m *mainUI) saveRegionOrder() {
	m.set("ui.connectivityRegionOrder", slices.Clone(m.cty.regionOrder))
}

// favoriteOrder is the favorites saved, of the sites the page has.
func favoriteOrder(saved []string, sites []tools.Site) []string {
	known := map[string]bool{}
	for _, st := range sites {
		known[st.ID] = true
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(saved))
	for _, id := range saved {
		if known[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

func favoriteSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func (m *mainUI) saveFavoriteOrder() {
	m.set("connectivityFavorites", slices.Clone(m.cty.favoriteOrder))
}

func (m *mainUI) toggleFavorite(id string) {
	if m.cty.favoriteOrder == nil {
		m.cty.favoriteOrder = []string{}
	}
	order := m.cty.favoriteOrder
	if i := slices.Index(order, id); i >= 0 {
		m.cty.favoriteOrder = slices.Delete(order, i, i+1)
	} else {
		m.cty.favoriteOrder = append(order, id)
	}
	m.saveFavoriteOrder()
}

// moveFavorite moves the favorite at from to to, as a drag drops it.
func (m *mainUI) moveFavorite(from, to int) {
	order := m.cty.favoriteOrder
	if from < 0 || from >= len(order) || to < 0 || to >= len(order) || from == to {
		return
	}
	id := order[from]
	order = slices.Insert(slices.Delete(order, from, from+1), to, id)
	m.cty.favoriteOrder = order
	m.saveFavoriteOrder()
}

// siteOp changes the page's sites through op, a Tools method, and takes
// the list it answers with; the favorites and hidden presets follow the
// settings it left.
func (m *mainUI) siteOp(op func(Tools) ([]tools.Site, error), retest bool) {
	s, a, win := &m.cty, m.a, m.window()
	running := m.state.Core.Status == coremgr.StatusRunning
	go func() {
		sites, err := op(Tools{a})
		cfg := a.settings.Get()
		win.Update(func() {
			if err != nil {
				m.notices = append(m.notices, Notice{Level: "error", Message: locales.CommonFailed(), Detail: err.Error()})
				return
			}
			s.sites = sites
			s.hidden = slices.Clone(cfg.ConnectivityHidden)
			s.favoriteOrder = favoriteOrder(cfg.ConnectivityFavorites, sites)
			s.regionOrder = regionOrder(cfg.UI.ConnectivityRegionOrder, tools.SiteRegions(sites))
			if retest && running {
				m.testCtySites()
			}
		})
	}()
}

// setUnlockChecks turns a check on or off, and runs it as it comes on.
func (m *mainUI) setUnlockChecks(id string, on bool) {
	s, cfg := &m.cty, m.a.settings.Get()
	hidden, enabled := slices.Clone(cfg.UnlockHidden), slices.Clone(cfg.UnlockEnabled)
	optional := slices.ContainsFunc(tools.UnlockCatalog(), func(u tools.Unlock) bool { return u.ID == id && u.Optional })
	drop := func(l []string) []string { return slices.DeleteFunc(l, func(v string) bool { return v == id }) }
	switch {
	case optional && on:
		enabled = append(drop(enabled), id)
	case optional:
		enabled = drop(enabled)
	case on:
		hidden = drop(hidden)
	default:
		hidden = append(drop(hidden), id)
	}
	m.patch(map[string]any{"unlockHidden": hidden, "unlockEnabled": enabled})
	s.unlocks = tools.UnlockList(hidden, enabled)
	if on && m.state.Core.Status == coremgr.StatusRunning {
		m.checkUnlock([]string{id})
	}
}

func (m *mainUI) reorderRegion(from, to int) bool {
	order := m.cty.regionOrder
	if from < 0 || from >= len(order) || to < 0 || to >= len(order) || from == to {
		return false
	}
	id := order[from]
	order = slices.Delete(order, from, from+1)
	order = slices.Insert(order, to, id)
	m.cty.regionOrder = order
	return true
}

func (m *mainUI) moveRegion(from, to int) {
	if m.reorderRegion(from, to) {
		m.saveRegionOrder()
	}
}

// ctyStart runs the quick checks, the addresses and the sites, once, as
// they are first shown.
// coreSettled reports whether the core has run long enough to take
// connections: one just started may not yet, and every check through it
// would fail. Until then it asks for a frame once it will have.
func (m *mainUI) coreSettled() bool {
	at := m.state.Core.StartedAt
	if since := time.Since(at); !at.IsZero() && since < coreSettle {
		if win := m.window(); win != nil && !m.cty.wake.Equal(at) {
			m.cty.wake = at // one wake for this start, not one a frame
			time.AfterFunc(coreSettle-since, win.Invalidate)
		}
		return false
	}
	return true
}

func (m *mainUI) ctyStart(running bool) {
	if s := &m.cty; running && !s.started {
		if !m.coreSettled() {
			return
		}
		s.started = true
		m.loadExitIPs()
		m.testCtySites()
	}
}

func (m *mainUI) loadExitIPs() {
	s, a := &m.cty, m.a
	win := m.window()
	for _, v := range exitViews {
		e := s.ips[v]
		if e == nil {
			e = &exitIP{}
			s.ips[v] = e
		}
		if e.loading {
			continue
		}
		e.loading = true
		go func() {
			ctx, cancel := context.WithTimeout(a.ctx, 20*time.Second)
			defer cancel()
			info, err := (Tools{a}).ExitIP(ctx, v)
			win.Update(func() {
				e.loading = false
				if err != nil {
					e.err, e.info = err.Error(), nil
					return
				}
				e.info, e.err = &info, ""
			})
		}()
	}
}

func (m *mainUI) testCtySites() {
	s, a := &m.cty, m.a
	if s.sitesBusy {
		return
	}
	s.roundsVer++
	s.sitesBusy, s.rounds, s.landed = true, map[string][]int{}, map[string]*[siteRounds]time.Time{}
	s.sitesRun++
	// A few dozen sites at a time, each through its rounds, the sites of
	// the category in view first: with hundreds of sites, those looked at
	// answer in seconds and the rest follow.
	q := &siteQueue{sites: slices.Clone(s.sites), cat: s.cat, region: s.region, favorite: favoriteSet(s.favoriteOrder)}
	s.queue = q
	proxy, win := a.proxyURL(), m.window()
	// A round lands in the page a few times a second, in batches, rather
	// than a frame each: with a few dozen sites in flight a round lands every
	// ten milliseconds or so, and each frame builds every card in view.
	batch := &roundBatch{}
	flush := func() {
		if done := batch.take(); len(done) > 0 {
			at := time.Now()
			win.Update(func() {
				s.roundsVer++
				for _, r := range done {
					k := len(s.rounds[r.id])
					s.rounds[r.id] = append(s.rounds[r.id], r.delay)
					if k < siteRounds {
						l := s.landed[r.id]
						if l == nil {
							l = new([siteRounds]time.Time)
							s.landed[r.id] = l
						}
						l[k] = at
					}
				}
			})
		}
	}
	go func() {
		var wg sync.WaitGroup
		for range siteTestSlots {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for {
					st, ok := q.next()
					if !ok {
						return
					}
					for range siteRounds {
						if a.ctx.Err() != nil {
							return
						}
						r := tools.TestSite(a.ctx, proxy, st)
						d := r.DelayMs
						if r.Error != "" {
							d = 0
						}
						batch.add(siteRound{st.ID, d}, flush)
					}
				}
			}()
		}
		wg.Wait()
		flush()
		win.Update(func() { s.sitesBusy, s.queue = false, nil })
	}()
}

// siteFlush is how often the rounds that landed are handed to the page.
const siteFlush = 100 * time.Millisecond

type siteRound struct {
	id    string
	delay int
}

// roundBatch gathers the rounds that landed since the page last had any.
type roundBatch struct {
	mu      sync.Mutex
	pending []siteRound
	armed   bool
}

// add keeps a round, and has flush called once siteFlush has passed if it
// is not already due.
func (b *roundBatch) add(r siteRound, flush func()) {
	b.mu.Lock()
	b.pending = append(b.pending, r)
	arm := !b.armed
	b.armed = true
	b.mu.Unlock()
	if arm {
		time.AfterFunc(siteFlush, flush)
	}
}

// take hands over the rounds kept, and disarms.
func (b *roundBatch) take() []siteRound {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := b.pending
	b.pending, b.armed = nil, false
	return out
}

// siteQueue is the sites still to test, handed out the category in view
// first; the view may change on the way.
type siteQueue struct {
	mu          sync.Mutex
	sites       []tools.Site
	cat, region string
	favorite    map[string]bool
}

func (q *siteQueue) prefer(cat, region string) {
	q.mu.Lock()
	q.cat, q.region = cat, region
	q.mu.Unlock()
}

func (q *siteQueue) next() (tools.Site, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.sites) == 0 {
		return tools.Site{}, false
	}
	i := slices.IndexFunc(q.sites, func(s tools.Site) bool {
		if q.cat == tools.CategoryGeneral {
			return q.favorite[s.ID]
		}
		return tools.SiteCategory(s) == q.cat && (q.cat != tools.CategoryRegion || tools.SiteRegion(s) == q.region)
	})
	i = max(i, 0)
	s := q.sites[i]
	q.sites = slices.Delete(q.sites, i, i+1)
	return s, true
}

func bestRound(r []int) int {
	b := 0
	for _, d := range r {
		if d > 0 && (b == 0 || d < b) {
			b = d
		}
	}
	if len(r) == 0 {
		return -1
	}
	return b
}

func (m *mainUI) checkUnlock(ids []string) {
	s, a := &m.cty, m.a
	if len(ids) == 0 {
		for _, u := range s.unlocks {
			ids = append(ids, u.ID)
		}
	}
	for _, id := range ids {
		s.checking[id] = true
	}
	exit, win := s.exit, m.window()
	go func() {
		tools.CheckUnlock(a.ctx, a.proxyURL(), ids, a.routeOf, func(u tools.Unlock) {
			// The core names the proxy each check went through; if it
			// could not, the page's exit is the best guess.
			u.Via = cmp.Or(u.Via, exit)
			r := unlockResult{u}
			a.saveUICache(func(c *uiCache) {
				if c.Unlock == nil {
					c.Unlock = map[string]unlockResult{}
				}
				c.Unlock[u.ID] = r
			})
			win.Update(func() {
				s.results[u.ID] = r
				delete(s.checking, u.ID)
			})
		})
		win.Update(func() { clear(s.checking) })
	}()
}

func mbps(bps float64) string {
	v := bps / 1e6
	switch {
	case v >= 100:
		return fmt.Sprintf("%.0f", v)
	case v >= 10:
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

// sitesDialog manages the page's sites: one of the user's own added, and
// the whole list, each site once under its category, with its star (常用)
// and its ✕, which hides a preset (to be restored below) and deletes one of
// the user's own.
func (m *mainUI) sitesDialog(c *ui.Context, running bool) {
	s, p := &m.cty, m.pal
	if !s.manage {
		return
	}
	hostOf := func(u string) string {
		if v, err := url.Parse(u); err == nil && v.Host != "" {
			return strings.ToLower(v.Host)
		}
		return u
	}
	hosts := map[string]bool{}
	for _, st := range s.sites {
		hosts[hostOf(st.URL)] = true
	}
	m.modal(c, "sites", &s.manage, func() {
		ui.Column(c).Width(620).MaxHeight(600).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icCirclePlus).FontSize(16).TextColor(p.TextMuted)
				ui.Text(c, locales.ConnectivityManage()).FontSize(15).FontWeight(650)
			})
			m.segmentedOf(c, "tab", &s.tab, true, locales.ConnectivityTabCustom(), locales.ConnectivityTabList())
			switch s.tab {
			case 0:
				// The fields share a row: a field grows along its parent, which
				// in this column would squeeze it to nothing.
				ui.Row(c.Key("fields")).Gap(12).AlignItems(ui.Start).Shrink(0).Children(func() {
					ui.Row(c).Width(180).Shrink(0).Children(func() {
						m.field(c, locales.ConnectivityCustomName(), "", "", func() {
							ui.TextInput(c.Key("name"), &s.custName).Placeholder(locales.ConnectivityCustomNamePh()).MaxLength(20)
						})
					})
					m.field(c, locales.ConnectivityCustomUrl(), "", s.custErr, func() {
						ui.TextInput(c.Key("url"), &s.custURL).Placeholder(locales.ConnectivityCustomUrlPh())
					})
				})
				ui.Text(c, locales.ConnectivityCustomHint()).FontSize(12).TextColor(p.TextMuted).Wrap()
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if i := m.segmented(c, "grp", s.custGrp, locales.ConnectivityGroupGlobal(), locales.ConnectivityGroupDomestic()); i >= 0 {
						s.custGrp = i
					}
					ui.Spacer(c)
					if m.button(c, "add", locales.CommonAdd(), nil, "primary").Disabled(strings.TrimSpace(s.custName) == "" || strings.TrimSpace(s.custURL) == "").Clicked() {
						u, err := url.Parse(strings.TrimSpace(s.custURL))
						switch {
						case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "":
							s.custErr = locales.ConnectivityCustomBadUrl()
						case hosts[strings.ToLower(u.Host)]:
							s.custErr = locales.ConnectivityCustomKnown()
						default:
							host := strings.ToLower(u.Host)
							site := tools.Site{ID: host, Name: cmp.Or(strings.TrimSpace(s.custName), host), URL: strings.TrimSpace(s.custURL), Group: []string{"global", "domestic"}[s.custGrp]}
							m.siteOp(func(t Tools) ([]tools.Site, error) { return t.AddSite(context.Background(), site) }, true)
							s.custName, s.custURL, s.custErr = "", "", ""
						}
					}
				})
			default:
				gutter(ui.Scroll(c).Grow(1).Gap(10), 14).Children(func() {
					for _, g := range tools.SiteCategories(s.sites) {
						var of []tools.Site
						for _, st := range s.sites {
							if tools.SiteCategory(st) == g {
								of = append(of, st)
							}
						}
						ui.Text(c.Key("g-"+g), m.catLabel(g)).FontSize(11.5).TextColor(p.Faint)
						m.rows(c, "list-"+g, func() {
							for _, st := range of {
								ui.Row(c.Key(st.ID)).Padding(6, 10).Gap(8).AlignItems(ui.Center).Transition(listRow).Children(func() {
									m.logo(c, st.ID, st.Name, 20)
									ui.Text(c, st.Name).SingleLine().Grow(1).Basis(0)
									ui.Text(c, hostOf(st.URL)).FontSize(12).TextColor(p.Faint).SingleLine()
									m.starButton(c, st.ID)
									id, tip := st.ID, locales.CommonDelete()
									if tools.IsKnownSite(id) {
										tip = locales.ConnectivityHide()
									}
									if m.iconButton(c, "del", icX, tip).Clicked() {
										m.siteOp(func(t Tools) ([]tools.Site, error) { return t.RemoveSite(context.Background(), id) }, false)
									}
								})
							}
						})
					}
					// The presets taken off, folded away, each to be restored.
					if len(s.hidden) > 0 {
						fold := ui.ButtonBase(c.Key("hidden-head")).Row().Gap(6).PaddingY(4).AlignItems(ui.Center).Justify(ui.Start)
						fold.Children(func() {
							icon := icChevronRight
							if s.hiddenOpen {
								icon = icChevronDown
							}
							ui.Icon(c, icon).FontSize(12).TextColor(p.Faint)
							ui.Text(c, locales.ConnectivityHidden(fmt.Sprint(len(s.hidden)))).FontSize(11.5).TextColor(p.Faint).ContentTransition(numRoll)
						})
						if fold.Clicked() {
							s.hiddenOpen = !s.hiddenOpen
						}
						if s.hiddenOpen {
							byID := map[string]tools.Site{}
							for _, st := range tools.ConnectivitySites {
								byID[st.ID] = st
							}
							ui.Column(c.Key("hidden")).Transition(ui.ElementTransition{Enter: &ui.Motion{Collapse: true}, Exit: &ui.Motion{Collapse: true}, Duration: 240 * time.Millisecond, Ease: easeOut}).Children(func() {
								m.rows(c, "hidden-rows", func() {
									for _, id := range s.hidden {
										st, ok := byID[id]
										if !ok {
											continue
										}
										ui.Row(c.Key(id)).Padding(6, 10).Gap(8).AlignItems(ui.Center).Transition(listRow).Children(func() {
											ui.Box(c).Opacity(0.5).Children(func() { m.logo(c, st.ID, st.Name, 20) })
											ui.Text(c, st.Name).TextColor(p.TextMuted).SingleLine().Grow(1).Basis(0)
											ui.Text(c, m.catLabel(tools.SiteCategory(st))).FontSize(12).TextColor(p.Faint).SingleLine()
											if m.button(c, "restore", locales.ConnectivityRestore(), icRotateCcw, "ghost").Clicked() {
												m.siteOp(func(t Tools) ([]tools.Site, error) { return t.RestoreSites(context.Background(), []string{id}) }, true)
											}
										})
									}
								})
							})
						}
					}
					ui.Row(c).Justify(ui.End).Children(func() {
						if m.button(c, "reset", locales.ConnectivityListReset(), nil, "ghost").Clicked() {
							m.confirm(locales.ConnectivityListReset(), locales.ConnectivityListResetMsg(), locales.ConnectivityListReset(), func() {
								m.siteOp(func(t Tools) ([]tools.Site, error) { return t.ResetSites(context.Background()) }, true)
							})
						}
					})
				})
			}
		})
	})
}

// starButton adds a site to 常用 and takes it out: an outline star, or a
// filled one, which pops as it turns.
func (m *mainUI) starButton(c *ui.Context, id string) ui.Element {
	p := m.pal
	on := slices.Contains(m.cty.favoriteOrder, id)
	tip := locales.ConnectivityFavAdd()
	if on {
		tip = locales.ConnectivityFavRemove()
	}
	b := ui.ButtonBase(c.Key("star")).Size(28, 28).Radius(8).AlignItems(ui.Center).Justify(ui.Center).Label(tip).Tooltip(tip).Shrink(0)
	if b.Hovered() {
		b.Background(p.SurfaceHover)
	}
	pressable(b)
	b.Children(func() {
		ui.Box(c).Size(15, 15).Children(func() {
			icon, col := icStar, p.TextMuted
			if on {
				icon, col = icStarFill, p.Accent2
			}
			ui.Box(c.Key(on)).Absolute().Left(0).Top(0).Right(0).Bottom(0).AlignItems(ui.Center).Justify(ui.Center).
				Transition(starPop).Children(func() { ui.Icon(c, icon).FontSize(15).TextColor(col) })
		})
	})
	if b.Clicked() {
		m.toggleFavorite(id)
	}
	return b
}

// starPop is how a star fills and empties: the filled one springs in.
var starPop = ui.ElementTransition{
	Enter: &ui.Motion{Scale: 0.4}, Exit: &ui.Motion{Scale: 0.7},
	Colors: true, Animation: &ui.Animation{Spring: ui.SpringOptions{Duration: 360 * time.Millisecond, Bounce: 0.45}}, ExitDuration: 120 * time.Millisecond,
}

// unlockDialog chooses the checks that run: each default one may be turned
// off, and the optional ones (a few regions' services) on.
func (m *mainUI) unlockDialog(c *ui.Context) {
	s, p := &m.cty, m.pal
	if !s.unlockMgr {
		return
	}
	on := map[string]bool{}
	for _, u := range s.unlocks {
		on[u.ID] = true
	}
	catalog := tools.UnlockCatalog()
	m.modal(c, "unlock-checks", &s.unlockMgr, func() {
		ui.Column(c).Width(520).MaxHeight(600).Gap(12).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icSlidersHorizontal).FontSize(16).TextColor(p.TextMuted)
				ui.Text(c, locales.UnlockManage()).FontSize(15).FontWeight(650)
			})
			gutter(ui.Scroll(c).Grow(1).Gap(10), 14).Children(func() {
				for _, g := range tools.UnlockGroups {
					var of []tools.Unlock
					for _, u := range catalog {
						if u.Group == g {
							of = append(of, u)
						}
					}
					if len(of) == 0 {
						continue
					}
					ui.Text(c.Key("g-"+g), m.unlockGroupLabel(g)).FontSize(11.5).TextColor(p.Faint)
					m.rows(c, "checks-"+g, func() {
						for _, u := range of {
							ui.Row(c.Key(u.ID)).Padding(6, 10).Gap(10).AlignItems(ui.Center).Children(func() {
								m.logo(c, cmp.Or(u.Site, u.ID), u.Name, 20)
								ui.Text(c, m.unlockName(u)).SingleLine().Grow(1).Basis(0)
								v := on[u.ID]
								if ui.Switch(c.Key("sw"), &v).Changed() {
									m.setUnlockChecks(u.ID, v)
								}
							})
						}
					})
				}
			})
		})
	})
}

// unlockName is a check's name as the page says it: the service's, or
// what is checked of it.
func (m *mainUI) unlockName(u tools.Unlock) string {
	switch u.ID {
	case "google-search":
		return locales.UnlockNameGoogleSearch()
	case "apple-region":
		return locales.UnlockNameAppleRegion()
	case "bing-region":
		return locales.UnlockNameBingRegion()
	case "steam":
		return locales.UnlockNameSteam()
	case "wikipedia-edit":
		return locales.UnlockNameWikipediaEdit()
	case "bilibili-hkmotw":
		return locales.UnlockNameBilibiliHkmotw()
	case "bilibili-tw":
		return locales.UnlockNameBilibiliTw()
	case "google-play":
		return locales.UnlockNameGooglePlay()
	case "youtube-cdn":
		return locales.UnlockNameYoutubeCdn()
	}
	return u.Name
}

// unlockDetailText says a result's detail in the page's language.
func unlockDetailText(d string) string {
	switch d {
	case "originals only":
		return locales.UnlockDetailOriginals()
	case "web only":
		return locales.UnlockDetailWebOnly()
	case "app only":
		return locales.UnlockDetailAppOnly()
	case "captcha":
		return locales.UnlockDetailCaptcha()
	case "blocked":
		return locales.UnlockDetailBlocked()
	case "challenge":
		return locales.UnlockDetailChallenge()
	case "overseas only":
		return locales.UnlockDetailOverseas()
	case "network error":
		return locales.UnlockDetailNetwork()
	case "unrecognized page":
		return locales.UnlockDetailUnrecognized()
	}
	return d
}

// unlockGroupLabel names a group of checks, by the site categories' words
// where there is one.
func (m *mainUI) unlockGroupLabel(g string) string {
	if g == "platform" {
		return locales.UnlockGroupPlatform()
	}
	return m.catLabel(g)
}

// ipQualityBadges says what kind of line an address is and how risky it
// looks, once the lookup is back.
func (m *mainUI) ipQualityBadges(c *ui.Context, ip string, masked bool) {
	if q := m.ipQualityOf(ip); q != nil {
		m.ipQualityView(c, q, masked)
	} else {
		ui.Text(c, "—").FontSize(13).TextColor(m.pal.Faint)
	}
}

func (m *mainUI) ipQualityView(c *ui.Context, q *tools.IPQuality, masked bool) {
	kinds := map[string]string{"residential": "ipq.residential", "datacenter": "ipq.datacenter", "mobile": "ipq.mobile", "business": "ipq.business"}
	var flags []string
	if q.VPN {
		flags = append(flags, "VPN")
	}
	if q.Tor {
		flags = append(flags, "Tor")
	}
	if q.Proxy && !q.VPN && !q.Tor {
		flags = append(flags, locales.IpqProxy())
	}
	kind := kinds[q.Kind]
	scored := q.Risk >= 0
	var parts []string
	if kind != "" {
		parts = append(parts, m.T(kind))
	}
	if scored {
		parts = append(parts, locales.IpqRisk(fmt.Sprint(q.Risk)))
	}

	// The badges and their mask take turns, each fading and growing in.
	turn := ui.ElementTransition{Enter: &ui.Motion{Scale: 0.85}, Duration: 220 * time.Millisecond, Ease: easeOut}
	if masked {
		ui.Text(c.Key("ipq-masked"), "••••••").FontSize(12.5).TextColor(m.pal.Faint).Transition(turn)
		return
	}
	row := ui.Row(c.Key("ipq-shown")).Gap(4).AlignItems(ui.Center).Transition(turn).Tooltip(strings.Join(slices.DeleteFunc([]string{q.Org, q.Source}, func(s string) bool { return s == "" }), " · "))
	row.Children(func() {
		if len(parts) > 0 {
			tone := ""
			if scored {
				tone = "danger"
				switch {
				case q.Risk < 25:
					tone = "success"
				case q.Risk < 60:
					tone = "warning"
				}
			}
			m.badge(c, strings.Join(parts, " · "), tone)
		}
		for _, f := range flags {
			m.badge(c, f, "warning")
		}
	})
}
