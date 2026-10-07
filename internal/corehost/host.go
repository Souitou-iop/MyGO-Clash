// Package corehost runs the core: mihomo, embedded as a library, serving
// its REST API on a private socket, with the app's routes under /mygo and
// the embedded Tailscale node.
//
// The app starts the core as a child process (the sidecar) or asks the
// service to start it with the privileges TUN needs. Either way the
// configuration arrives over the API and is never written to disk.
package corehost

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"slices"
	"sync"
	"syscall"
	"time"

	"github.com/metacubex/chi"
	"github.com/metacubex/mihomo/adapter"
	"github.com/metacubex/mihomo/component/updater"
	"github.com/metacubex/mihomo/config"
	C "github.com/metacubex/mihomo/constant"
	"github.com/metacubex/mihomo/dns"
	"github.com/metacubex/mihomo/hub/executor"
	"github.com/metacubex/mihomo/hub/route"
	"github.com/metacubex/mihomo/log"
	"github.com/metacubex/mihomo/tunnel"

	mhttp "github.com/metacubex/http"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/tailnet"
)

// AppVersion is the version of the app, set by main.
var AppVersion = "dev"

func init() {
	C.Version = MihomoVersion()
}

// MihomoVersion returns the version of the embedded mihomo module.
func MihomoVersion() string {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, m := range bi.Deps {
			if m.Path == "github.com/metacubex/mihomo" {
				v := m.Version
				if m.Replace != nil && m.Replace.Version != "" {
					v = m.Replace.Version
				}
				if len(v) > 1 && v[0] == 'v' {
					v = v[1:]
				}
				return v
			}
		}
	}
	return C.Version
}

type controller struct {
	Addr    string
	Secret  string
	Origins []string
	Private bool
}

type host struct {
	boot    coreapi.Bootstrap
	started time.Time
	node    *tailnet.Node

	applyMu   sync.Mutex
	hash      string
	appliedAt time.Time
	tsName    string
	unregDNS  func()
	ctrl      controller

	stop     chan struct{}
	stopOnce sync.Once
}

// Run reads the bootstrap from r and runs the core until it is told to
// stop, receives SIGINT or SIGTERM, or (for a sidecar) r closes.
func Run(r io.Reader) error {
	br := bufio.NewReader(r)
	line, err := br.ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return fmt.Errorf("read bootstrap: %w", err)
	}
	var boot coreapi.Bootstrap
	if err := json.Unmarshal(line, &boot); err != nil {
		return fmt.Errorf("parse bootstrap: %w", err)
	}
	return Serve(boot, br)
}

// Serve runs the core with a bootstrap. When boot.ExitWithStdin is set it
// stops once stdin reaches EOF.
func Serve(boot coreapi.Bootstrap, stdin io.Reader) error {
	if boot.Home == "" || boot.Socket == "" {
		return errors.New("bootstrap needs a home and a socket")
	}
	if err := os.MkdirAll(boot.Home, 0o700); err != nil {
		return err
	}
	C.SetHomeDir(boot.Home)
	C.SetConfig(filepath.Join(boot.Home, "config.yaml")) // never written; anchors relative paths
	if boot.LogLevel != "" {
		if lvl, ok := log.LogLevelMapping[boot.LogLevel]; ok {
			log.SetLevel(lvl)
		}
	}
	if runtime.GOOS == "windows" && boot.PipeSDDL != "" {
		// mihomo reads the pipe's security descriptor from the environment.
		_ = os.Setenv("LISTEN_NAMEDPIPE_SDDL", boot.PipeSDDL)
	}

	h := &host{
		boot:    boot,
		started: time.Now(),
		node:    tailnet.NewNode(boot.Home),
		stop:    make(chan struct{}),
	}
	// Embed mode keeps the API from replacing the configuration, the
	// binary (upgrade) or the process (restart) behind the app's back.
	route.SetEmbedMode(true)
	route.Register(h.routes)
	h.startController(controller{})
	log.Infoln("MyGO-Clash core %s (mihomo %s) serving %s", AppVersion, C.Version, boot.Socket)

	if boot.ExitWithStdin && stdin != nil {
		go func() {
			_, _ = io.Copy(io.Discard, stdin)
			log.Warnln("the app went away; stopping")
			h.shutdown()
		}()
	}
	if boot.OwnerUID >= 0 && os.Geteuid() == 0 {
		go h.chownLoop()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	select {
	case s := <-sig:
		log.Warnln("received %v; stopping", s)
	case <-h.stop:
	}
	h.cleanup()
	return nil
}

func (h *host) shutdown() { h.stopOnce.Do(func() { close(h.stop) }) }

func (h *host) cleanup() {
	done := make(chan struct{})
	go func() {
		executor.Shutdown() // removes TUN routes and firewall rules
		h.node.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
	if runtime.GOOS != "windows" {
		_ = os.Remove(h.boot.Socket)
	}
	h.chown()
}

func (h *host) startController(c controller) {
	cfg := &route.Config{
		Addr:   c.Addr,
		Secret: c.Secret,
		Cors:   route.Cors{AllowOrigins: c.Origins, AllowPrivateNetwork: c.Private},
	}
	if runtime.GOOS == "windows" {
		cfg.PipeAddr = h.boot.Socket
	} else {
		cfg.UnixAddr = h.boot.Socket
	}
	route.ReCreateServer(cfg)
	h.ctrl = c
}

func (h *host) routes(r chi.Router) {
	r.Get(coreapi.PathHealth, h.health)
	r.Post(coreapi.PathApply, h.apply)
	r.Post(coreapi.PathValidate, h.validate)
	r.Patch(coreapi.PathGeneral, h.general)
	r.Post(coreapi.PathGeoUpdate, h.geoUpdate)
	r.Post(coreapi.PathRulesDisable, h.rulesDisable)
	r.Post(coreapi.PathShutdown, func(w mhttp.ResponseWriter, r *mhttp.Request) {
		w.WriteHeader(http.StatusNoContent)
		go func() {
			time.Sleep(100 * time.Millisecond)
			h.shutdown()
		}()
	})
	r.Get(coreapi.PathTSStatus, func(w mhttp.ResponseWriter, r *mhttp.Request) {
		writeJSON(w, http.StatusOK, h.node.Status())
	})
	r.Get(coreapi.PathTSStateExport, func(w mhttp.ResponseWriter, r *mhttp.Request) {
		writeJSON(w, http.StatusOK, h.node.State())
	})
	r.Get(coreapi.PathTSWatch, h.tailscaleWatch)
	r.Post(coreapi.PathTSLogin, func(w mhttp.ResponseWriter, r *mhttp.Request) {
		var req coreapi.TailscaleLogin
		if !readJSON(w, r, &req) {
			return
		}
		respond(w, h.node.Login(r.Context(), req.AuthKey))
	})
	r.Post(coreapi.PathTSLogout, func(w mhttp.ResponseWriter, r *mhttp.Request) {
		respond(w, h.node.Logout(r.Context()))
	})
	r.Post(coreapi.PathTSPrefs, func(w mhttp.ResponseWriter, r *mhttp.Request) {
		var req coreapi.TailscalePrefs
		if !readJSON(w, r, &req) {
			return
		}
		respond(w, h.node.SetPrefs(r.Context(), req))
	})
	r.Post(coreapi.PathTSPing, func(w mhttp.ResponseWriter, r *mhttp.Request) {
		var req coreapi.TailscalePing
		if !readJSON(w, r, &req) {
			return
		}
		res, err := h.node.Ping(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
}

func (h *host) health(w mhttp.ResponseWriter, r *mhttp.Request) {
	h.applyMu.Lock()
	hash, at := h.hash, h.appliedAt
	h.applyMu.Unlock()
	writeJSON(w, http.StatusOK, coreapi.Health{
		Version:     AppVersion,
		CoreVersion: C.Version,
		PID:         os.Getpid(),
		StartedAt:   h.started,
		Mode:        h.boot.Mode,
		Privileged:  isPrivileged(),
		ConfigHash:  hash,
		AppliedAt:   at,
		Home:        h.boot.Home,
	})
}

func (h *host) apply(w mhttp.ResponseWriter, r *mhttp.Request) {
	var req coreapi.ApplyRequest
	if !readJSON(w, r, &req) {
		return
	}
	h.applyMu.Lock()
	defer h.applyMu.Unlock()

	var warnings []string
	if err := h.node.Configure(req.Tailscale); err != nil {
		warnings = append(warnings, "tailscale: "+err.Error())
	}
	ensureGeo(r.Context(), req.Config)
	cfg, err := executor.ParseWithBytes([]byte(req.Config))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	h.bindTailscale(cfg, req.Tailscale)
	// force makes mihomo (re)create the listeners of ports; it leaves those
	// whose address did not change alone.
	executor.ApplyConfig(cfg, true)

	sum := sha256.Sum256([]byte(req.Config))
	h.hash = hex.EncodeToString(sum[:])
	h.appliedAt = time.Now()

	var c controller // without a TCP address, secret and CORS do not matter
	if cfg.Controller.ExternalController != "" {
		c = controller{
			Addr:    cfg.Controller.ExternalController,
			Secret:  cfg.Controller.Secret,
			Origins: cfg.Controller.Cors.AllowOrigins,
			Private: cfg.Controller.Cors.AllowPrivateNetwork,
		}
	}
	if c.Addr != h.ctrl.Addr || c.Secret != h.ctrl.Secret || !slices.Equal(c.Origins, h.ctrl.Origins) || c.Private != h.ctrl.Private {
		h.ctrl = c
		// Restarting the servers closes this request's connection: answer first.
		go func() {
			time.Sleep(200 * time.Millisecond)
			h.applyMu.Lock()
			defer h.applyMu.Unlock()
			h.startController(c)
		}()
	}
	writeJSON(w, http.StatusOK, coreapi.ApplyResponse{ConfigHash: h.hash, Warnings: warnings})
}

// bindTailscale swaps the placeholder outbound of the tailnet, a reject
// proxy, for the node's adapter, and points ts:// name servers at the node.
// A placeholder left unswapped rejects, so tailnet traffic never leaks.
func (h *host) bindTailscale(cfg *config.Config, ts *coreapi.TailscaleConfig) {
	name := ""
	if ts != nil && ts.Enabled {
		name = ts.ProxyName
	}
	if name != h.tsName && h.unregDNS != nil {
		h.unregDNS()
		h.unregDNS = nil
	}
	h.tsName = name
	if name == "" {
		return
	}
	p, ok := cfg.Proxies[name].(*adapter.Proxy)
	if !ok || p.Type() != C.Reject {
		log.Warnln("[Tailscale] the configuration has no placeholder %q", name)
		return
	}
	_ = p.ProxyAdapter.Close()
	p.ProxyAdapter = tailnet.NewAdapter(h.node, name)
	if h.unregDNS == nil {
		h.unregDNS = dns.RegisterTailscaleDnsClient(name, tailnet.DNSClient{Name: name, Node: h.node})
	}
}

func (h *host) validate(w mhttp.ResponseWriter, r *mhttp.Request) {
	var req coreapi.ValidateRequest
	if !readJSON(w, r, &req) {
		return
	}
	ensureGeo(r.Context(), req.Config)
	cfg, err := executor.ParseWithBytes([]byte(req.Config))
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	// Parsing made proxies and providers; close them now rather than at
	// the next garbage collection.
	for _, p := range cfg.Proxies {
		_ = p.Close()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *host) general(w mhttp.ResponseWriter, r *mhttp.Request) {
	var req coreapi.GeneralPatch
	if !readJSON(w, r, &req) {
		return
	}
	if req.Mode != "" {
		var m tunnel.TunnelMode
		if err := m.UnmarshalText([]byte(req.Mode)); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		tunnel.SetMode(m)
		log.Infoln("mode is now %s", m)
	}
	if req.LogLevel != "" {
		lvl, ok := log.LogLevelMapping[req.LogLevel]
		if !ok {
			writeError(w, http.StatusBadRequest, fmt.Errorf("unknown log level %q", req.LogLevel))
			return
		}
		log.SetLevel(lvl)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *host) geoUpdate(w mhttp.ResponseWriter, r *mhttp.Request) {
	if err := updater.UpdateGeoDatabases(); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *host) rulesDisable(w mhttp.ResponseWriter, r *mhttp.Request) {
	var req map[int]bool
	if !readJSON(w, r, &req) {
		return
	}
	rules := tunnel.Rules()
	for i, off := range req {
		if i < 0 || i >= len(rules) {
			continue
		}
		if rw, ok := rules[i].(C.RuleWrapper); ok {
			rw.SetDisabled(off)
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *host) tailscaleWatch(w mhttp.ResponseWriter, r *mhttp.Request) {
	ch, stop := h.node.Watch()
	defer stop()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(mhttp.Flusher)
	enc := json.NewEncoder(w)
	keepalive := time.NewTicker(15 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e := <-ch:
			if enc.Encode(e) != nil {
				return
			}
		case <-keepalive.C:
			if _, err := w.Write([]byte("\n")); err != nil {
				return
			}
		}
		if flusher != nil {
			flusher.Flush()
		}
	}
}

// chownLoop gives the files a root core writes in its home back to the
// user it runs for.
func (h *host) chownLoop() {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-t.C:
			h.chown()
		}
	}
}

func (h *host) chown() {
	if h.boot.OwnerUID < 0 || runtime.GOOS == "windows" || os.Geteuid() != 0 {
		return
	}
	_ = filepath.WalkDir(h.boot.Home, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		_ = os.Lchown(p, h.boot.OwnerUID, h.boot.OwnerGID)
		return nil
	})
}

func readJSON(w mhttp.ResponseWriter, r *mhttp.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<20)).Decode(v); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func respond(w mhttp.ResponseWriter, err error) {
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, tailnet.ErrOff) {
			code = http.StatusConflict
		}
		writeError(w, code, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w mhttp.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w mhttp.ResponseWriter, code int, err error) {
	writeJSON(w, code, coreapi.ErrorBody{Message: err.Error()})
}
