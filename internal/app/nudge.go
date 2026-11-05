package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/cloudsync"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

// The devices of a vault that are on the same tailnet tell each other when
// they changed it, so that the others sync at once instead of at their next
// interval. Each lists where it listens in the manifest, which only the
// vault's devices read; word is signed with a key of the vault, and only
// starts a sync, which reads the vault as usual.
const (
	nudgePort = 47393
	nudgePath = "/mygo-clash/sync"
	// nudgeGap is the least time between two syncs that word starts.
	nudgeGap = 10 * time.Second
)

type nudgeMsg struct {
	From  string `json:"from"`
	KeyID string `json:"keyId"`
	At    int64  `json:"at"`
	MAC   string `json:"mac"`
}

type nudger struct {
	a *App

	mu       sync.Mutex
	local    net.Listener // what the core's node forwards its port to
	direct   net.Listener // the tailnet address itself, with the Tailscale app
	directIP string
	last     time.Time
	timer    *time.Timer
}

func newNudger(a *App) *nudger { return &nudger{a: a} }

// start opens the local listener.
func (n *nudger) start() {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Printf("sync word: %v", err)
		return
	}
	n.mu.Lock()
	n.local = ln
	n.mu.Unlock()
	go func() { _ = (&http.Server{Handler: n, ReadHeaderTimeout: 5 * time.Second}).Serve(ln) }()
}

func (n *nudger) stop() {
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, ln := range []net.Listener{n.local, n.direct} {
		if ln != nil {
			_ = ln.Close()
		}
	}
	n.local, n.direct, n.directIP = nil, nil, ""
}

func (n *nudger) enabled() bool {
	st := n.a.settings.Get().Sync
	return st.Enabled && st.Tailnet && n.a.syncer.configured()
}

// target is the local address the core's node forwards to, "" when off.
func (n *nudger) target() string {
	if n == nil || !n.enabled() {
		return ""
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.local == nil {
		return ""
	}
	return n.local.Addr().String()
}

// address is where the other devices reach this one, "" when they don't.
func (n *nudger) address() string {
	if n == nil || !n.enabled() {
		return ""
	}
	s := n.a.ts.current()
	if s.BackendState != "Running" || s.Self == nil {
		return ""
	}
	ip := ""
	for _, a := range s.Self.TailscaleIPs {
		if p, err := net.ResolveIPAddr("ip", a); err == nil && p.IP.To4() != nil {
			ip = a
			break
		}
	}
	if ip == "" {
		return ""
	}
	switch n.a.settings.Get().Tailscale.Mode {
	case "embedded":
		if s.Source != "embedded" || n.target() == "" {
			return ""
		}
	case "system":
		if s.Source != "system" || !n.listenDirect(ip) {
			return ""
		}
	default:
		return ""
	}
	return net.JoinHostPort(ip, strconv.Itoa(nudgePort))
}

// listenDirect listens on the tailnet address, which the Tailscale app
// routes to this computer.
func (n *nudger) listenDirect(ip string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.direct != nil && n.directIP == ip {
		return true
	}
	if n.direct != nil {
		_ = n.direct.Close()
		n.direct, n.directIP = nil, ""
	}
	ln, err := net.Listen("tcp", net.JoinHostPort(ip, strconv.Itoa(nudgePort)))
	if err != nil {
		return false
	}
	n.direct, n.directIP = ln, ip
	go func() { _ = (&http.Server{Handler: n, ReadHeaderTimeout: 5 * time.Second}).Serve(ln) }()
	return true
}

func nudgeMAC(keys *cloudsync.Keys, from string, at int64) string {
	m := hmac.New(sha256.New, keys.Subkey("nudge/v1"))
	fmt.Fprintf(m, "%s|%d|%s", from, at, keys.ID)
	return base64.StdEncoding.EncodeToString(m.Sum(nil))
}

func (n *nudger) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != nudgePath {
		http.NotFound(w, r)
		return
	}
	var msg nudgeMsg
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&msg); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	keys, err := n.a.syncer.keys()
	if err != nil || !n.enabled() {
		http.Error(w, "off", http.StatusServiceUnavailable)
		return
	}
	at := time.Unix(msg.At, 0)
	if msg.KeyID != keys.ID || time.Since(at).Abs() > 5*time.Minute || !hmac.Equal([]byte(msg.MAC), []byte(nudgeMAC(keys, msg.From, msg.At))) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if msg.From == n.a.syncer.state.DeviceID {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	n.kick()
	w.WriteHeader(http.StatusAccepted)
}

// kick syncs soon, but not more often than every nudgeGap.
func (n *nudger) kick() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.timer != nil {
		return // one is coming
	}
	wait := 2 * time.Second
	if since := time.Since(n.last); since < nudgeGap {
		wait = max(wait, nudgeGap-since)
	}
	n.timer = time.AfterFunc(wait, func() {
		n.mu.Lock()
		n.timer, n.last = nil, time.Now()
		n.mu.Unlock()
		select {
		case n.a.syncer.trigger <- struct{}{}:
		default:
		}
	})
}

// tell lets the other devices of the vault that listen know that this one
// changed it.
func (n *nudger) tell(eng *cloudsync.Engine) {
	if n == nil || !n.enabled() {
		return
	}
	m := eng.Current()
	me := eng.State.DeviceID
	at := time.Now().Unix()
	body, _ := json.Marshal(nudgeMsg{From: me, KeyID: eng.Keys.ID, At: at, MAC: nudgeMAC(eng.Keys, me, at)})
	embedded := n.a.settings.Get().Tailscale.Mode == "embedded"
	var wg sync.WaitGroup
	for id, d := range m.Devices {
		if id == me || d.Tailnet == "" {
			continue
		}
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(n.a.ctx, 8*time.Second)
			defer cancel()
			var err error
			if embedded {
				c, cerr := n.a.core.Must()
				if cerr != nil {
					return
				}
				_, err = c.TailscalePost(ctx, coreapi.TailscalePost{Address: addr, Path: nudgePath, Body: body})
			} else {
				err = postDirect(ctx, addr, body)
			}
			if err != nil && !strings.Contains(err.Error(), "context") {
				log.Printf("sync word to %s: %v", addr, err)
			}
		}(d.Tailnet)
	}
	wg.Wait()
}

func postDirect(ctx context.Context, addr string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+nudgePath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}).Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}
