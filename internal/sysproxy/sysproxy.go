// Package sysproxy sets the operating system's proxy: the setting
// browsers and most applications follow.
package sysproxy

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Proxy is a system proxy setting.
type Proxy struct {
	Enabled bool   `json:"enabled"`
	Host    string `json:"host"`
	Port    int    `json:"port"`
	// Bypass lists hosts and networks that skip the proxy.
	Bypass []string `json:"bypass"`
	// PAC is the URL of a proxy auto-configuration script, used instead of
	// Host and Port when set.
	PAC string `json:"pac,omitempty"`
}

// Server returns host:port.
func (p Proxy) Server() string { return net.JoinHostPort(p.Host, strconv.Itoa(p.Port)) }

// Matches reports whether the system's setting is p, as far as the
// system can tell.
func (p Proxy) Matches(sys Proxy) bool {
	if p.Enabled != sys.Enabled {
		return false
	}
	if !p.Enabled {
		return true
	}
	if p.PAC != "" {
		return sys.PAC == p.PAC
	}
	return sys.PAC == "" && strings.EqualFold(sys.Host, p.Host) && sys.Port == p.Port
}

// ParseBypass splits a list of hosts written with commas, semicolons or
// new lines.
func ParseBypass(s string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' || r == '\n' || r == '\r' }) {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// Set applies a setting; a disabled one turns the proxy off.
func Set(p Proxy) error {
	if !p.Enabled {
		return setDirect()
	}
	if p.PAC != "" {
		return setPAC(p.PAC)
	}
	return setManual(p)
}

// Get reads the system's setting.
func Get() (Proxy, error) { return get() }

// Guard sets the proxy again whenever the system's setting stops matching,
// as when another app changed it.
type Guard struct {
	mu     sync.Mutex
	want   Proxy
	cancel context.CancelFunc
	// OnFix is called after the guard set the proxy again; OnFail when it
	// gave up after failing repeatedly.
	OnFix  func()
	OnFail func(error)
}

// Watch starts guarding want every interval; it replaces a previous watch.
func (g *Guard) Watch(want Proxy, interval time.Duration) {
	g.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	g.mu.Lock()
	g.want, g.cancel = want, cancel
	g.mu.Unlock()
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		failures := 0
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			cur, err := Get()
			if err == nil && want.Matches(cur) {
				failures = 0
				continue
			}
			if err := Set(want); err != nil {
				if failures++; failures >= 5 {
					if g.OnFail != nil {
						g.OnFail(err)
					}
					return
				}
				continue
			}
			failures = 0
			if g.OnFix != nil {
				g.OnFix()
			}
		}
	}()
}

// Stop stops guarding.
func (g *Guard) Stop() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.cancel != nil {
		g.cancel()
		g.cancel = nil
	}
}

// PACServer serves a proxy auto-configuration script on the loopback.
type PACServer struct {
	mu     sync.Mutex
	script string
	srv    *http.Server
	url    string
}

// Serve starts serving script (or updates it) and returns its URL.
func (s *PACServer) Serve(script string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.script = script
	if s.srv != nil {
		return s.url, nil
	}
	ln, err := net.Listen("tcp", "127.0.0.1:33331")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return "", err
		}
	}
	s.srv = &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.mu.Lock()
			script := s.script
			s.mu.Unlock()
			w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = fmt.Fprint(w, script)
		}),
	}
	s.url = fmt.Sprintf("http://%s/commands/pac", ln.Addr())
	go func() { _ = s.srv.Serve(ln) }()
	return s.url, nil
}

// Close stops serving.
func (s *PACServer) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.srv != nil {
		_ = s.srv.Close()
		s.srv = nil
	}
}

// RenderPAC fills a script's placeholders.
func RenderPAC(script, host string, port int) string {
	return strings.NewReplacer("%mixed-port%", strconv.Itoa(port), "%proxy-host%", host).Replace(script)
}
