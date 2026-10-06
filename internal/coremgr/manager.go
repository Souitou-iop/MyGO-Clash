// Package coremgr runs the core for the app: it starts it (as a child
// process, or through the service), watches its health, restarts it when it
// dies, and applies configurations to it.
package coremgr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

// Status of the core.
const (
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusError    = "error"
)

// State describes the core.
type State struct {
	Status      string    `json:"status"`
	Mode        string    `json:"mode"` // sidecar or service
	Error       string    `json:"error,omitempty"`
	PID         int       `json:"pid,omitempty"`
	Version     string    `json:"version,omitempty"`     // the core binary's app version
	CoreVersion string    `json:"coreVersion,omitempty"` // mihomo's
	Privileged  bool      `json:"privileged"`
	StartedAt   time.Time `json:"startedAt,omitzero"`
	Restarts    int       `json:"restarts"`
}

// Process is a running core.
type Process interface {
	// Done is closed when the core exits, after which Err says why.
	Done() <-chan struct{}
	Err() error
	PID() int
	// Stop stops the core: politely, then by force.
	Stop(ctx context.Context) error
}

// Launcher starts cores.
type Launcher interface {
	Mode() string
	// Start starts a core with the bootstrap and returns once it runs or
	// failed to.
	Start(ctx context.Context, boot coreapi.Bootstrap) (Process, error)
}

// Manager keeps a core running.
type Manager struct {
	// Bootstrap returns the bootstrap of a core started by a launcher.
	Bootstrap func(mode string) coreapi.Bootstrap
	// OnState is called on the changes of the core's state.
	OnState func(State)
	// OnReady is called each time a core is up, before it gets a
	// configuration from Apply, so that the app can apply one.
	OnReady func(ctx context.Context, c *coreapi.Client)

	mu       sync.Mutex
	launcher Launcher
	proc     Process
	client   *coreapi.Client
	state    State
	wantRun  bool
	gen      int
	lastReq  *coreapi.ApplyRequest
	restarts int
	cancel   context.CancelFunc
}

// State returns the core's state.
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

// Client returns a client of the running core, or nil.
func (m *Manager) Client() *coreapi.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.Status != StatusRunning {
		return nil
	}
	return m.client
}

// ErrNotRunning is returned when the core is not running.
var ErrNotRunning = errors.New("the core is not running")

// Must returns a client of the running core, or ErrNotRunning.
func (m *Manager) Must() (*coreapi.Client, error) {
	if c := m.Client(); c != nil {
		return c, nil
	}
	return nil, ErrNotRunning
}

func (m *Manager) setState(fn func(*State)) {
	m.mu.Lock()
	fn(&m.state)
	s := m.state
	m.mu.Unlock()
	if m.OnState != nil {
		m.OnState(s)
	}
}

// Start starts a core with launcher, stopping the one running, and keeps
// it running until Stop.
func (m *Manager) Start(ctx context.Context, launcher Launcher) error {
	m.Stop(ctx)
	m.mu.Lock()
	m.launcher = launcher
	m.wantRun = true
	m.gen++
	gen := m.gen
	m.restarts = 0
	sctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.mu.Unlock()
	err := m.launch(ctx, gen)
	go m.supervise(sctx, gen)
	return err
}

// launch starts the core of generation gen.
func (m *Manager) launch(ctx context.Context, gen int) error {
	m.mu.Lock()
	launcher := m.launcher
	if gen != m.gen || launcher == nil {
		m.mu.Unlock()
		return context.Canceled
	}
	m.mu.Unlock()
	m.setState(func(s *State) {
		s.Status, s.Mode, s.Error, s.PID = StatusStarting, launcher.Mode(), "", 0
	})
	boot := m.Bootstrap(launcher.Mode())
	sctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	proc, err := launcher.Start(sctx, boot)
	if err != nil {
		m.setState(func(s *State) { s.Status, s.Error = StatusError, err.Error() })
		return err
	}
	client := coreapi.New(boot.Socket)
	health, err := waitHealthy(sctx, client, proc)
	if err != nil {
		_ = proc.Stop(context.Background())
		client.Close()
		m.setState(func(s *State) { s.Status, s.Error = StatusError, err.Error() })
		return err
	}
	m.mu.Lock()
	if gen != m.gen {
		m.mu.Unlock()
		_ = proc.Stop(context.Background())
		return context.Canceled
	}
	if m.client != nil {
		m.client.Close()
	}
	m.proc, m.client = proc, client
	req := m.lastReq
	m.mu.Unlock()

	m.setState(func(s *State) {
		s.Status, s.PID = StatusRunning, proc.PID()
		if s.PID == 0 {
			s.PID = health.PID
		}
		s.Version, s.CoreVersion = health.Version, health.CoreVersion
		s.Privileged, s.StartedAt = health.Privileged, health.StartedAt
	})
	// Every new core gets a configuration: a fresh one from the app, which
	// carries the latest state of the tailnet node, or else the last one.
	actx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if m.OnReady != nil {
		m.OnReady(actx, client)
	} else if req != nil {
		if _, err := client.Apply(actx, *req); err != nil {
			m.setState(func(s *State) { s.Error = "apply: " + err.Error() })
		}
	}
	return nil
}

func waitHealthy(ctx context.Context, c *coreapi.Client, proc Process) (coreapi.Health, error) {
	for {
		h, err := c.Health(ctx)
		if err == nil {
			return h, nil
		}
		select {
		case <-proc.Done():
			if perr := proc.Err(); perr != nil {
				return coreapi.Health{}, fmt.Errorf("the core exited: %w", perr)
			}
			return coreapi.Health{}, errors.New("the core exited")
		case <-ctx.Done():
			return coreapi.Health{}, fmt.Errorf("the core did not answer: %w", err)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// supervise restarts the core when it dies, with a growing delay.
func (m *Manager) supervise(ctx context.Context, gen int) {
	for {
		m.mu.Lock()
		proc := m.proc
		m.mu.Unlock()
		var done <-chan struct{}
		if proc != nil {
			done = proc.Done()
		}
		health := time.NewTicker(10 * time.Second)
		failures := 0
	watch:
		for {
			select {
			case <-ctx.Done():
				health.Stop()
				return
			case <-done:
				break watch
			case <-health.C:
				c := m.Client()
				if c == nil {
					if proc == nil {
						break watch // never started: retry
					}
					continue
				}
				hctx, cancel := context.WithTimeout(ctx, 5*time.Second)
				_, err := c.Health(hctx)
				cancel()
				if err == nil {
					failures = 0
					continue
				}
				if failures++; failures >= 3 {
					// Hung: restart it.
					_ = proc.Stop(context.Background())
					break watch
				}
			}
		}
		health.Stop()
		m.mu.Lock()
		if gen != m.gen || !m.wantRun {
			m.mu.Unlock()
			return
		}
		if !m.state.StartedAt.IsZero() && time.Since(m.state.StartedAt) > 2*time.Minute {
			m.restarts = 0 // it ran a while: a new incident, not a crash loop
		}
		m.restarts++
		n := m.restarts
		m.proc = nil
		m.mu.Unlock()
		reason := "the core stopped"
		if proc != nil && proc.Err() != nil {
			reason = proc.Err().Error()
		}
		delay := min(time.Duration(1<<min(n-1, 5))*time.Second, 30*time.Second)
		m.setState(func(s *State) {
			s.Status, s.Error, s.Restarts, s.PID = StatusError, fmt.Sprintf("%s; restarting in %s", reason, delay), n, 0
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		lctx, cancel := context.WithTimeout(ctx, 40*time.Second)
		_ = m.launch(lctx, gen)
		cancel()
	}
}

// Stop stops the core.
func (m *Manager) Stop(ctx context.Context) {
	m.mu.Lock()
	m.wantRun = false
	m.gen++
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	proc, client := m.proc, m.client
	m.proc, m.client = nil, nil
	m.mu.Unlock()
	if proc != nil {
		sctx, cancel := context.WithTimeout(ctx, 6*time.Second)
		_ = proc.Stop(sctx)
		cancel()
	}
	if client != nil {
		client.Close()
	}
	m.setState(func(s *State) { *s = State{Status: StatusStopped, Mode: s.Mode} })
}

// Restart stops the core and starts it again with the same launcher.
func (m *Manager) Restart(ctx context.Context) error {
	m.mu.Lock()
	l := m.launcher
	m.mu.Unlock()
	if l == nil {
		return ErrNotRunning
	}
	return m.Start(ctx, l)
}

// Apply sends a configuration to the core and remembers it for restarts.
func (m *Manager) Apply(ctx context.Context, req coreapi.ApplyRequest) (coreapi.ApplyResponse, error) {
	m.mu.Lock()
	r := req
	m.lastReq = &r
	m.mu.Unlock()
	c, err := m.Must()
	if err != nil {
		return coreapi.ApplyResponse{}, err
	}
	res, err := c.Apply(ctx, req)
	if err != nil {
		return res, err
	}
	m.setState(func(s *State) {
		if strings.HasPrefix(s.Error, "apply: ") {
			s.Error = ""
		}
	})
	return res, nil
}

// LastConfig returns the configuration last applied, or "".
func (m *Manager) LastConfig() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.lastReq == nil {
		return ""
	}
	return m.lastReq.Config
}
