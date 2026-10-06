package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/logx"
)

func bytesReader(b []byte) io.Reader { return bytes.NewReader(b) }

// Version is the app version of the service's executable, set by main.
var Version = "dev"

// Config is what the service runs with.
type Config struct {
	Slug string
	// Owner is the user allowed to use the service: a UID on Unix, a SID on
	// Windows.
	Owner string
}

type server struct {
	cfg     Config
	paths   Paths
	started time.Time

	mu       sync.Mutex
	core     *exec.Cmd
	coreIn   io.WriteCloser
	coreDone chan struct{}
	coreErr  string
	tail     *logx.Tail
	logFile  *logx.RotatingFile
	proxy    *httputil.ReverseProxy
}

// Run serves the service until ctx ends.
func Run(ctx context.Context, cfg Config) error {
	p := Layout(cfg.Slug)
	for _, dir := range []string{p.Home, p.Logs, filepath.Dir(p.CoreSocket)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		_ = os.Chmod(dir, 0o700)
	}
	s := &server{
		cfg:     cfg,
		paths:   p,
		started: time.Now(),
		tail:    logx.NewTail(30),
		logFile: &logx.RotatingFile{Path: filepath.Join(p.Logs, "core.log"), MaxSize: 8 << 20, MaxFiles: 3},
	}
	coreAddr := p.CoreSocket
	s.proxy = &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(&url.URL{Scheme: "http", Host: "core"})
		},
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return coreapi.Dial(ctx, coreAddr)
			},
			MaxIdleConnsPerHost: 8,
		},
		FlushInterval: -1, // streams (traffic, logs) flush every write
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadGateway, fmt.Errorf("the core is not running: %v", err))
		},
	}
	ln, err := listen(p.Socket, cfg.Owner)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", p.Socket, err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+PathStatus, func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.status()) })
	mux.HandleFunc("POST "+PathCoreStart, s.handleStart)
	mux.HandleFunc("POST "+PathCoreStop, func(w http.ResponseWriter, r *http.Request) {
		s.stopCore()
		w.WriteHeader(http.StatusNoContent)
	})
	mux.Handle("/", s.proxy)
	srv := &http.Server{
		Handler:     s.authorize(mux),
		ConnContext: connContext,
	}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Printf("service %s %s listening on %s for %s", cfg.Slug, Version, p.Socket, cfg.Owner)
	err = srv.Serve(ln)
	s.stopCore()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// authorize lets only the owner and the administrator in.
func (s *server) authorize(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := allowed(r.Context(), s.cfg.Owner); err != nil {
			writeError(w, http.StatusForbidden, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) status() Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := Status{Version: Version, Name: s.cfg.Slug, PID: os.Getpid(), StartedAt: s.started, CoreError: s.coreErr, Home: s.paths.Home}
	if s.core != nil {
		select {
		case <-s.coreDone:
		default:
			st.CoreRunning = true
			st.CorePID = s.core.Process.Pid
		}
	}
	return st
}

func (s *server) handleStart(w http.ResponseWriter, r *http.Request) {
	var req StartRequest
	_ = json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req)
	s.stopCore()
	if err := s.startCore(req); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// Wait until the core answers, so that the app can use it at once.
	c := coreapi.New(s.paths.CoreSocket)
	defer c.Close()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	for {
		if _, err := c.Health(ctx); err == nil {
			break
		}
		s.mu.Lock()
		done := s.coreDone
		s.mu.Unlock()
		select {
		case <-done:
			writeError(w, http.StatusInternalServerError, fmt.Errorf("the core exited: %s", s.status().CoreError))
			return
		case <-ctx.Done():
			writeError(w, http.StatusGatewayTimeout, errors.New("the core did not answer"))
			return
		case <-time.After(100 * time.Millisecond):
		}
	}
	writeJSON(w, s.status())
}

func (s *server) startCore(req StartRequest) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	boot := coreapi.Bootstrap{
		Home:          s.paths.Home,
		Socket:        s.paths.CoreSocket,
		PipeSDDL:      coreSDDL,
		LogLevel:      req.LogLevel,
		Mode:          "service",
		OwnerUID:      -1,
		OwnerGID:      -1,
		ExitWithStdin: true,
	}
	line, _ := json.Marshal(boot)
	cmd := exec.Command(exe, "core")
	hideWindow(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	out := io.MultiWriter(s.logFile, s.tail)
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return err
	}
	if _, err := stdin.Write(append(line, '\n')); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	done := make(chan struct{})
	s.mu.Lock()
	s.core, s.coreIn, s.coreDone, s.coreErr = cmd, stdin, done, ""
	s.mu.Unlock()
	go func() {
		err := cmd.Wait()
		s.mu.Lock()
		if s.core == cmd {
			msg := "exited"
			if err != nil {
				msg = err.Error()
			}
			if lines := s.tail.Lines(); len(lines) > 0 {
				msg += ": " + lines[len(lines)-1]
			}
			s.coreErr = msg
		}
		s.mu.Unlock()
		close(done)
	}()
	return nil
}

// stopCore stops the core: closing its input makes it clean its routes up
// and exit; it is killed if it has not within five seconds.
func (s *server) stopCore() {
	s.mu.Lock()
	cmd, in, done := s.core, s.coreIn, s.coreDone
	s.core, s.coreIn = nil, nil
	s.mu.Unlock()
	if cmd == nil {
		return
	}
	_ = in.Close()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(coreapi.ErrorBody{Message: err.Error()})
}

type peerKey struct{}

// Peer is who is on the other side of a connection to the service.
type Peer struct {
	UID   int
	Known bool
}

func peerOf(ctx context.Context) Peer {
	p, _ := ctx.Value(peerKey{}).(Peer)
	return p
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return -2
	}
	return n
}
