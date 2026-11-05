package usage

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

// maxClosed is how many closed connections are kept.
const maxClosed = 3000

// Closed is a connection that closed.
type Closed struct {
	ID              string    `json:"id"`
	Start           time.Time `json:"start"`
	End             time.Time `json:"end"`
	Network         string    `json:"network"`
	Type            string    `json:"type"`
	Host            string    `json:"host"`
	DestinationIP   string    `json:"destinationIP,omitempty"`
	DestinationPort string    `json:"destinationPort,omitempty"`
	SourceIP        string    `json:"sourceIP,omitempty"`
	Process         string    `json:"process,omitempty"`
	ProcessPath     string    `json:"processPath,omitempty"`
	Rule            string    `json:"rule"`
	RulePayload     string    `json:"rulePayload,omitempty"`
	Chains          []string  `json:"chains"`
	Country         string    `json:"country,omitempty"`
	Up              int64     `json:"up"`
	Down            int64     `json:"down"`
}

func (s *Store) recordClosed(c *coreapi.Connection, final Entry, end time.Time) {
	host := c.Metadata.Host
	if host == "" {
		host = c.Metadata.SniffHost
	}
	cc := s.country(c)
	if cc == Unknown {
		cc = ""
	}
	s.closed = append(s.closed, Closed{
		ID: c.ID, Start: c.Start, End: end, Network: c.Metadata.Network, Type: c.Metadata.Type,
		Host: host, DestinationIP: c.Metadata.DestinationIP, DestinationPort: c.Metadata.DestinationPort, SourceIP: c.Metadata.SourceIP,
		Process: c.Metadata.Process, ProcessPath: c.Metadata.ProcessPath, Rule: c.Rule, RulePayload: c.RulePayload,
		Chains: c.Chains, Country: cc, Up: max(final.Up, c.Upload), Down: max(final.Down, c.Download),
	})
	if n := len(s.closed) - maxClosed; n > 0 {
		s.closed = append([]Closed(nil), s.closed[n:]...)
	}
	s.closedDirty = true
}

// ClosedQuery asks for a page of closed connections, the latest first.
// Filter keeps those whose host, address, process, rule or chain contains
// it, without regard to case.
type ClosedQuery struct {
	Filter string `json:"filter,omitempty"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

// ClosedPage is a page of closed connections.
type ClosedPage struct {
	Items []Closed `json:"items"`
	// Total counts those that match the filter.
	Total int `json:"total"`
	// Kept is how many the store keeps at most.
	Kept int `json:"kept"`
}

// Closed returns a page of the closed connections.
func (s *Store) Closed(q ClosedQuery) ClosedPage {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := strings.ToLower(strings.TrimSpace(q.Filter))
	limit := q.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	page := ClosedPage{Items: []Closed{}, Kept: maxClosed}
	for i := len(s.closed) - 1; i >= 0; i-- {
		c := s.closed[i]
		if f != "" && !closedMatches(c, f) {
			continue
		}
		if page.Total >= q.Offset && len(page.Items) < limit {
			page.Items = append(page.Items, c)
		}
		page.Total++
	}
	return page
}

func closedMatches(c Closed, f string) bool {
	for _, v := range []string{c.Host, c.DestinationIP, c.Process, c.Rule, c.RulePayload, c.Network, c.Country, strings.Join(c.Chains, " ")} {
		if strings.Contains(strings.ToLower(v), f) {
			return true
		}
	}
	return false
}

// ClearClosed forgets the closed connections.
func (s *Store) ClearClosed() error {
	s.mu.Lock()
	s.closed = nil
	s.closedDirty = true
	s.mu.Unlock()
	return s.Flush()
}

func (s *Store) closedPath() string { return filepath.Join(s.dir, "closed.json") }

func (s *Store) loadClosed() {
	data, err := os.ReadFile(s.closedPath())
	if err != nil {
		return
	}
	var list []Closed
	if json.Unmarshal(data, &list) == nil {
		if n := len(list) - maxClosed; n > 0 {
			list = list[n:]
		}
		s.closed = list
	}
}

// closedData encodes the closed connections when they changed.
func (s *Store) closedData() []byte {
	if !s.closedDirty {
		return nil
	}
	s.closedDirty = false
	data, _ := json.Marshal(s.closed)
	if s.closed == nil {
		data = []byte("[]")
	}
	return data
}

func (s *Store) writeClosed(data []byte) error {
	err := writeAtomic(s.closedPath(), data)
	if err != nil {
		s.mu.Lock()
		s.closedDirty = true
		s.mu.Unlock()
	}
	return err
}
