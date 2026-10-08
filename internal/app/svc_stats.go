package app

import (
	"context"

	"github.com/mygo-clash/mygo-clash/internal/usage"
)

// Stats is the traffic statistics page.
type Stats struct{ a *App }

// Query totals the traffic of a range of days, and ranks its apps, sites
// and nodes.
func (s Stats) Query(ctx context.Context, q usage.StatsQuery) (usage.StatsReport, error) {
	if err := s.a.waitReady(ctx); err != nil {
		return usage.StatsReport{}, err
	}
	return s.a.usage.Query(q), nil
}

// Clear forgets the statistics.
func (s Stats) Clear(ctx context.Context) error {
	if err := s.a.waitReady(ctx); err != nil {
		return err
	}
	return s.a.usage.Clear()
}
