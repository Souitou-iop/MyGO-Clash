package usage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

func ruled(c coreapi.Connection, rule, payload, ip string) coreapi.Connection {
	c.Rule, c.RulePayload, c.Metadata.DestinationIP = rule, payload, ip
	return c
}

func TestDrillDown(t *testing.T) {
	s := newStore(t)
	s.SetOptions(Options{Keep: 90, Country: func(ip string) string {
		if ip == "1.1.1.1" {
			return "us"
		}
		return "jp"
	}})
	s.Observe(snap(0, 0))
	s.Observe(snap(0, 0,
		ruled(conn("a", "www.google.com", "chrome", 10, 100, "HK1", "Proxy"), "DomainSuffix", "google.com", "1.1.1.1"),
		ruled(conn("b", "github.com", "chrome", 5, 50, "JP1", "Proxy"), "Match", "", "2.2.2.2"),
		ruled(conn("c", "github.com", "git", 1, 1000, "JP1", "Proxy"), "Match", "", "2.2.2.2"),
	))
	r := s.Query(StatsQuery{Range: "today"})
	if r.Conns != 3 || len(r.Rules) != 2 || len(r.Policies) != 1 || r.Policies[0].Name != "Proxy" {
		t.Fatalf("report: %+v", r)
	}
	if r.Countries[0].Name != "JP" || r.Countries[0].Down != 1050 {
		t.Fatalf("countries: %+v", r.Countries)
	}
	// Narrowed to chrome: its sites and nodes only.
	r = s.Query(StatsQuery{Range: "today", Where: map[string]string{DimApp: "chrome"}})
	if r.Up != 15 || r.Down != 150 || len(r.Sites) != 2 || len(r.Apps) != 1 || r.Partial {
		t.Fatalf("narrowed: %+v", r)
	}
	if r.Hours[12].Down != 150 {
		t.Fatalf("narrowed hours: %+v", r.Hours[12])
	}
	r = s.Query(StatsQuery{Range: "today", Where: map[string]string{DimSite: "github.com", DimNode: "JP1"}, SortBy: "conns"})
	if r.Down != 1050 || len(r.Apps) != 2 {
		t.Fatalf("two dimensions: %+v", r)
	}
	// The detail survives a restart.
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s2 := Open(s.path)
	s2.now = s.now
	if r := s2.Query(StatsQuery{Range: "today", Where: map[string]string{DimApp: "git"}}); r.Down != 1000 {
		t.Fatalf("after restart: %+v", r)
	}
}

func TestExcludeAndClosed(t *testing.T) {
	s := newStore(t)
	s.SetOptions(Options{Keep: 90, Exclude: func(c *coreapi.Connection) bool { return c.Metadata.Host == "lan.local" }})
	s.Observe(snap(0, 0))
	s.Observe(snap(110, 1100, conn("a", "a.com", "p", 100, 1000, "N"), conn("b", "lan.local", "p", 10, 100, "DIRECT")))
	r := s.Query(StatsQuery{Range: "today"})
	if r.Up != 100 || r.Down != 1000 {
		t.Fatalf("excluded counted: %+v", r)
	}
	s.Observe(snap(110, 1100, conn("b", "lan.local", "p", 10, 100, "DIRECT")))
	page := s.Closed(ClosedQuery{Limit: 10})
	if page.Total != 1 || page.Items[0].Host != "a.com" || page.Items[0].Down != 1000 {
		t.Fatalf("closed: %+v", page)
	}
	if p := s.Closed(ClosedQuery{Filter: "zzz"}); p.Total != 0 {
		t.Fatalf("filter: %+v", p)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	if p := Open(s.path).Closed(ClosedQuery{}); p.Total != 1 {
		t.Fatalf("closed after restart: %+v", p)
	}
}

func TestExportImportAndCompare(t *testing.T) {
	s := newStore(t)
	s.Observe(snap(0, 0))
	s.Observe(snap(0, 0, conn("a", "a.com", "p", 5, 50, "N")))
	yesterday := noon.AddDate(0, 0, -1)
	s.now = func() time.Time { return yesterday }
	s.Observe(snap(0, 0, conn("a", "a.com", "p", 7, 70, "N")))
	s.now = func() time.Time { return noon }
	r := s.Query(StatsQuery{Range: "today"})
	if r.Prev == nil || r.Prev.Down != 20 {
		t.Fatalf("prev: %+v", r.Prev)
	}
	data := s.Export("", "", "laptop")
	csv := string(s.ExportCSV("", "", "laptop", nil))
	if !strings.Contains(csv, "laptop,") || !strings.Contains(csv, "a.com") || !strings.HasPrefix(csv, "\xef\xbb\xbfdevice,date,hour") {
		t.Fatalf("csv: %q", csv)
	}
	other := Open(filepath.Join(t.TempDir(), "usage.json"))
	other.now = s.now
	for i := 0; i < 2; i++ { // twice counts once
		if n, err := other.Import(data); err != nil || n != 2 {
			t.Fatalf("import: %d %v", n, err)
		}
	}
	if r := other.Query(StatsQuery{Range: "7d"}); r.Down != 70 || r.Up != 7 {
		t.Fatalf("imported: %+v", r)
	}
	if r := other.Query(StatsQuery{Range: "today", Where: map[string]string{DimApp: "p"}}); r.Down != 50 {
		t.Fatalf("imported flows: %+v", r)
	}
	if _, err := other.Import([]byte(`{}`)); err == nil {
		t.Fatal("imported junk")
	}
	src, err := ParseCompact(s.Compact(5))
	if err != nil {
		t.Fatal(err)
	}
	r = other.QueryWith(StatsQuery{Range: "7d"}, true, src)
	if r.Down != 140 {
		t.Fatalf("with another device: %+v", r)
	}
	// Another device's latest days narrow, from the combinations it shares.
	if r := other.QueryWith(StatsQuery{Range: "7d", Where: map[string]string{DimApp: "p"}}, true, src); r.Partial || r.Down != 140 {
		t.Fatalf("another device's days narrowed: %+v", r)
	}
	if r := other.QueryWith(StatsQuery{Range: "7d"}, false, src); r.Down != 70 {
		t.Fatalf("another device alone: %+v", r)
	}
	if csv := string(other.ExportCSV("", "", "", map[string]Source{"desktop": src})); !strings.Contains(csv, "desktop,") || strings.Contains(csv, "laptop,") {
		t.Fatalf("csv of another device: %q", csv)
	}
}

func TestKeepForever(t *testing.T) {
	s := newStore(t)
	s.SetOptions(Options{Keep: 0})
	old := noon.AddDate(-1, 0, 0)
	s.now = func() time.Time { return old }
	s.Observe(snap(0, 0))
	s.Observe(snap(0, 0, conn("a", "a.com", "p", 1, 2, "N")))
	s.now = func() time.Time { return noon }
	s.Observe(snap(0, 0, conn("a", "a.com", "p", 2, 4, "N")))
	if r := s.Query(StatsQuery{Range: "all"}); r.Down != 4 || r.Oldest != old.Format(dateFormat) {
		t.Fatalf("all: %+v", r)
	}
	s.SetOptions(Options{Keep: 30})
	if r := s.Query(StatsQuery{Range: "all"}); r.Down != 2 {
		t.Fatalf("after keeping 30 days: %+v", r)
	}
}
