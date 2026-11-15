package app

import "time"

// The machine's network throughput, from the byte counters of its physical
// interfaces: what the menu bar shows while the app is not taking traffic
// over, so that the rate keeps moving with the machine's use of the network
// as it does in Shadowrocket. Only physical interfaces count, which also
// keeps the TUN interface, and whatever else tunnels over the physical one,
// from being counted twice.

// ifBytes are the received and sent byte counters of one interface.
type ifBytes struct{ in, out uint64 }

// netRate turns successive readings of the counters into bytes per second.
type netRate struct {
	prev map[string]ifBytes
	at   time.Time
}

// counterDelta is how much a counter grew. A counter that went down was
// reset (the interface was recreated), and what it counts now is all there
// is to count since.
func counterDelta(prev, cur uint64) uint64 {
	if cur < prev {
		return cur
	}
	return cur - prev
}

// Update takes a reading made at now and returns the rates since the last
// one. The first reading has no rate, and neither has one made at the same
// time or earlier. An interface that is new since the last reading counts
// from zero, with no delta, because its counters hold its whole past; one
// that has gone drops out of the sum.
func (n *netRate) Update(now time.Time, cur map[string]ifBytes) (up, down int64) {
	prev, at := n.prev, n.at
	n.prev, n.at = cur, now
	if prev == nil || !now.After(at) {
		return 0, 0
	}
	var in, out uint64
	for name, c := range cur {
		p, ok := prev[name]
		if !ok {
			continue
		}
		in += counterDelta(p.in, c.in)
		out += counterDelta(p.out, c.out)
	}
	secs := now.Sub(at).Seconds()
	return int64(float64(out) / secs), int64(float64(in) / secs)
}

// Reset forgets the last reading, as after the machine slept.
func (n *netRate) Reset() { n.prev = nil }
