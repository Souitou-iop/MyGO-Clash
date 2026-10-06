package app

import (
	"context"
	"sync"
	"time"
)

// hub fans a stream out to subscribers. It runs one upstream (a connection
// to the core) while anyone listens, restarting it when it breaks, and
// stops it when the last subscriber leaves.
type hub[T any] struct {
	run func(ctx context.Context, emit func(T)) error

	mu      sync.Mutex
	subs    map[int]chan T
	next    int
	cancel  context.CancelFunc
	last    T
	hasLast bool
}

func newHub[T any](run func(ctx context.Context, emit func(T)) error) *hub[T] {
	return &hub[T]{run: run, subs: map[int]chan T{}}
}

// Subscribe returns a channel of the stream's values, starting with the
// last one, and a function that ends the subscription.
func (h *hub[T]) Subscribe() (<-chan T, func()) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan T, 16)
	id := h.next
	h.next++
	h.subs[id] = ch
	if h.hasLast {
		ch <- h.last
	}
	if h.cancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		h.cancel = cancel
		go h.loop(ctx)
	}
	return ch, func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		if _, ok := h.subs[id]; !ok {
			return
		}
		delete(h.subs, id)
		close(ch) // sends happen under the lock too
		if len(h.subs) == 0 && h.cancel != nil {
			h.cancel()
			h.cancel = nil
		}
	}
}

// Last returns the last value and whether there is one.
func (h *hub[T]) Last() (T, bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.last, h.hasLast
}

func (h *hub[T]) loop(ctx context.Context) {
	for ctx.Err() == nil {
		_ = h.run(ctx, func(v T) {
			h.mu.Lock()
			h.last, h.hasLast = v, true
			for _, ch := range h.subs {
				select {
				case ch <- v:
				default: // a slow subscriber skips a value
				}
			}
			h.mu.Unlock()
		})
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// pump sends the values of a subscription to send until ctx ends or send
// fails (the page stopped listening).
func pump[T any](ctx context.Context, h *hub[T], send func(T) error) error {
	ch, stop := h.Subscribe()
	defer stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case v, ok := <-ch:
			if !ok {
				return nil
			}
			if err := send(v); err != nil {
				return nil
			}
		}
	}
}

// restart reconnects the upstream, for a change of what it asks for.
func (h *hub[T]) restart() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cancel == nil {
		return
	}
	h.cancel()
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	go h.loop(ctx)
}
