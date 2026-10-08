package usage

import (
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

func conn(id, host, proc string, up, down int64, chain ...string) coreapi.Connection {
	c := coreapi.Connection{ID: id, Upload: up, Download: down, Chains: chain}
	c.Metadata.Host, c.Metadata.Process = host, proc
	return c
}

// snap is a snapshot of the core: its totals and open connections. Totals
// of zero leave the connections to count alone.
func snap(up, down int64, cs ...coreapi.Connection) coreapi.Connections {
	return coreapi.Connections{UploadTotal: up, DownloadTotal: down, Connections: cs}
}

var noon = time.Date(2026, 10, 8, 12, 30, 0, 0, time.Local)

func newStore(t *testing.T) *Store {
	s := Open(filepath.Join(t.TempDir(), "usage.json"))
	s.now = func() time.Time { return noon }
	return s
}

func TestDeltas(t *testing.T) {
	s := newStore(t)
	s.Observe(snap(0, 0, conn("a", "x.example.com", "curl", 100, 1000, "HK", "Proxy"))) // only seeds
	s.Observe(snap(0, 0, conn("a", "x.example.com", "curl", 150, 1500, "HK", "Proxy")))
	s.Observe(snap(0, 0, conn("a", "x.example.com", "curl", 150, 1700, "HK", "Proxy"), conn("b", "y.example.com", "", 10, 20, "DIRECT")))
	s.Observe(snap(0, 0)) // both closed
	r := s.Query(StatsQuery{Range: "today"})
	if r.Up != 60 || r.Down != 720 {
		t.Fatalf("totals up %d down %d", r.Up, r.Down)
	}
	if len(r.Apps) != 2 || r.Apps[0].Name != "curl" || r.Apps[1].Name != Unknown {
		t.Fatalf("apps %+v", r.Apps)
	}
	if r.Sites[0].Name != "example.com" || r.Sites[0].Down != 720 {
		t.Fatalf("sites %+v", r.Sites)
	}
	if r.Nodes[0].Name != "HK" || r.Nodes[1].Name != "DIRECT" {
		t.Fatalf("nodes %+v", r.Nodes)
	}
	if h := r.Hours[12]; h.Up != r.Up || h.Down != r.Down || len(r.Hours) != 24 {
		t.Fatalf("hour %+v", h)
	}
}

func TestCounterRestart(t *testing.T) {
	s := newStore(t)
	s.Observe(snap(0, 0))
	s.Observe(snap(0, 0, conn("a", "a.com", "p", 100, 100, "N")))
	s.Observe(snap(0, 0, conn("a", "a.com", "p", 30, 40, "N")))
	if r := s.Query(StatsQuery{Range: "today"}); r.Up != 130 || r.Down != 140 {
		t.Fatalf("%+v", r)
	}
}

func TestDomain(t *testing.T) {
	for in, want := range map[string]string{
		"www.example.com":    "example.com",
		"A.B.example.co.uk.": "example.co.uk",
		"user.github.io":     "user.github.io",
		"1.2.3.4":            "1.2.3.4",
		"[2001:db8::1]":      "2001:db8::1",
		"localhost":          "localhost",
		"":                   Unknown,
	} {
		if got := Domain(in); got != want {
			t.Errorf("Domain(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCap(t *testing.T) {
	s := newStore(t)
	s.Observe(snap(0, 0))
	var cs []coreapi.Connection
	for i := 0; i < maxKeys+50; i++ {
		cs = append(cs, conn(strconv.Itoa(i), "h"+strconv.Itoa(i)+".com", "app"+strconv.Itoa(i), 1, 1, "N"))
	}
	s.Observe(snap(0, 0, cs...))
	d := s.days[noon.Format(dateFormat)]
	if len(d.Apps) != maxKeys+1 || d.Apps[otherKey].Up != 50 {
		t.Fatalf("apps %d other %+v", len(d.Apps), d.Apps[otherKey])
	}
	r := s.Query(StatsQuery{Range: "today", Top: 5})
	if len(r.Apps) != 6 || !r.Apps[5].Other || r.Apps[5].Up != int64(maxKeys+50-5) {
		t.Fatalf("%+v", r.Apps)
	}
}

func TestPersistAndPrune(t *testing.T) {
	s := newStore(t)
	s.days[noon.AddDate(0, 0, -KeepDays).Format(dateFormat)] = newDay()
	s.days[noon.AddDate(0, 0, -KeepDays+1).Format(dateFormat)] = newDay()
	s.Observe(snap(0, 0))
	s.Observe(snap(0, 0, conn("a", "a.com", "p", 5, 6, "N"))) // a new day prunes
	if len(s.days) != 2 {
		t.Fatalf("days %d", len(s.days))
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	s2 := Open(s.path)
	s2.now = s.now
	if r := s2.Query(StatsQuery{Range: "30d"}); r.Up != 5 || r.Down != 6 || len(r.Days) != 30 {
		t.Fatalf("%+v", r)
	}
	if err := s2.Clear(); err != nil {
		t.Fatal(err)
	}
	s3 := Open(s.path)
	s3.now = s.now
	if r := s3.Query(StatsQuery{Range: "7d"}); r.Up != 0 || len(r.Apps) != 0 {
		t.Fatalf("%+v", r)
	}
}

func TestShortConnectionsCountAsOther(t *testing.T) {
	s := newStore(t)
	s.Observe(snap(1000, 1000)) // only seeds
	// 300 down went through a connection seen open; 500 more through ones
	// that came and went between the snapshots.
	s.Observe(snap(1000, 1800, conn("a", "a.com", "p", 0, 300, "N")))
	r := s.Query(StatsQuery{Range: "today"})
	if r.Up != 0 || r.Down != 800 {
		t.Fatalf("totals %+v", r)
	}
	if len(r.Sites) != 2 || r.Sites[0].Down != 300 || !r.Sites[1].Other || r.Sites[1].Down != 500 {
		t.Fatalf("sites %+v", r.Sites)
	}
	// The core restarted: its counters start over.
	s.Observe(snap(10, 20))
	if r := s.Query(StatsQuery{Range: "today"}); r.Up != 10 || r.Down != 820 {
		t.Fatalf("after restart %+v", r)
	}
}
