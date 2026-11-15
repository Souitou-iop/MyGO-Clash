package app

import (
	"testing"
	"time"
)

func TestNetRate(t *testing.T) {
	t0 := time.Unix(1000, 0)
	var n netRate
	if u, d := n.Update(t0, map[string]ifBytes{"en0": {100, 50}}); u != 0 || d != 0 {
		t.Fatalf("first reading: %d %d", u, d)
	}
	// a second later: 1000 in, 500 out; a new interface does not count yet.
	u, d := n.Update(t0.Add(time.Second), map[string]ifBytes{"en0": {1100, 550}, "en1": {9e9, 9e9}})
	if u != 500 || d != 1000 {
		t.Fatalf("delta: up %d down %d", u, d)
	}
	// en1 now counts; en0 vanished and drops out; 2s elapsed.
	u, d = n.Update(t0.Add(3*time.Second), map[string]ifBytes{"en1": {9e9 + 4000, 9e9 + 2000}})
	if u != 1000 || d != 2000 {
		t.Fatalf("new/gone: up %d down %d", u, d)
	}
	// a counter reset: what is there now is all that was counted.
	u, d = n.Update(t0.Add(4*time.Second), map[string]ifBytes{"en1": {300, 100}})
	if u != 100 || d != 300 {
		t.Fatalf("reset: up %d down %d", u, d)
	}
	// no time passed: no rate, and no division by zero.
	if u, d = n.Update(t0.Add(4*time.Second), map[string]ifBytes{"en1": {900, 900}}); u != 0 || d != 0 {
		t.Fatalf("same instant: %d %d", u, d)
	}
	n.Reset()
	if u, d = n.Update(t0.Add(9*time.Second), map[string]ifBytes{"en1": {1, 1}}); u != 0 || d != 0 {
		t.Fatalf("after reset: %d %d", u, d)
	}
}

func TestCounterDelta(t *testing.T) {
	for _, c := range []struct{ p, c, want uint64 }{{5, 9, 4}, {9, 9, 0}, {9, 5, 5}, {^uint64(0), 3, 3}} {
		if got := counterDelta(c.p, c.c); got != c.want {
			t.Errorf("counterDelta(%d,%d)=%d want %d", c.p, c.c, got, c.want)
		}
	}
}
