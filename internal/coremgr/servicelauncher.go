package coremgr

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/service"
)

// ServiceLauncher starts the core through the service, with the
// privileges TUN needs. The app then reaches the core through the
// service's socket, which forwards its requests: the bootstrap's socket
// must be the service's.
type ServiceLauncher struct {
	Client *service.Client
}

// Mode implements Launcher.
func (ServiceLauncher) Mode() string { return "service" }

// Start implements Launcher.
func (l ServiceLauncher) Start(ctx context.Context, boot coreapi.Bootstrap) (Process, error) {
	st, err := l.Client.StartCore(ctx, service.StartRequest{LogLevel: boot.LogLevel})
	if err != nil {
		return nil, err
	}
	p := &serviceProc{client: l.Client, pid: st.CorePID, done: make(chan struct{})}
	go p.watch()
	return p, nil
}

type serviceProc struct {
	client *service.Client
	pid    int
	done   chan struct{}

	mu       sync.Mutex
	err      error
	stopping bool
	once     sync.Once
}

// watch asks the service about the core until it is gone.
func (p *serviceProc) watch() {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	misses := 0
	for range t.C {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		st, err := p.client.Status(ctx)
		cancel()
		p.mu.Lock()
		stopping := p.stopping
		p.mu.Unlock()
		if stopping {
			p.finish(nil)
			return
		}
		switch {
		case err != nil:
			if misses++; misses >= 3 {
				p.finish(errors.New("the service stopped answering: " + err.Error()))
				return
			}
		case !st.CoreRunning || st.CorePID != p.pid:
			msg := st.CoreError
			if msg == "" {
				msg = "the service's core stopped"
			}
			p.finish(errors.New(msg))
			return
		default:
			misses = 0
		}
	}
}

func (p *serviceProc) finish(err error) {
	p.once.Do(func() {
		p.mu.Lock()
		p.err = err
		p.mu.Unlock()
		close(p.done)
	})
}

func (p *serviceProc) Done() <-chan struct{} { return p.done }

func (p *serviceProc) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

func (p *serviceProc) PID() int { return p.pid }

func (p *serviceProc) Stop(ctx context.Context) error {
	p.mu.Lock()
	p.stopping = true
	p.mu.Unlock()
	err := p.client.StopCore(ctx)
	p.finish(nil)
	return err
}
