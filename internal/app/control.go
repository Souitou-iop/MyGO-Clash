package app

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/agent"
	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/paths"
	"github.com/mygo-clash/mygo-clash/internal/secure"
)

// The control socket serves the methods of package agent to the command
// line and the MCP server: on a Unix socket in the data directory, which
// only the user can enter, with a token the app writes beside it for the
// user alone and changes at each start. Settings → agent.enabled turns it
// off.

type controlServer struct {
	mu     sync.Mutex
	srv    *http.Server
	sock   string
	tokenF string
	token  string
}

var control controlServer

// applyControl serves the control socket, or stops it, as the settings
// say.
func (a *App) applyControl(st config.Settings) {
	if st.Agent.Enabled {
		if err := control.start(a); err != nil {
			log.Printf("agent control: %v", err)
		}
	} else {
		// Apart: the call that turned it off may be one of the socket's,
		// which the shutdown would wait for.
		go control.stop()
	}
}

func (cs *controlServer) start(a *App) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.srv != nil {
		return nil
	}
	sock, err := paths.ControlSocket(a.dirs.Data, a.slug)
	if err != nil {
		return err
	}
	if conn, err := net.DialTimeout("unix", sock, 500*time.Millisecond); err == nil {
		_ = conn.Close()
		return fmt.Errorf("%s is served by another process", sock)
	}
	_ = os.Remove(sock) // left by a crash
	ln, err := net.Listen("unix", sock)
	if err != nil {
		return err
	}
	_ = os.Chmod(sock, 0o600)
	token := hex.EncodeToString(secure.RandomBytes(32))
	tokenF := paths.ControlToken(a.dirs.Data)
	if err := secure.WriteFileAtomic(tokenF, []byte(token+"\n"), 0o600); err != nil {
		_ = ln.Close()
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+agent.CallPath, func(w http.ResponseWriter, r *http.Request) {
		a.serveControl(w, r, token)
	})
	cs.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	cs.sock, cs.tokenF, cs.token = sock, tokenF, token
	srv := cs.srv
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("agent control: %v", err)
		}
	}()
	log.Printf("agent control on %s", sock)
	return nil
}

func (cs *controlServer) stop() {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if cs.srv == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = cs.srv.Shutdown(ctx)
	_ = os.Remove(cs.sock)
	_ = os.Remove(cs.tokenF)
	cs.srv = nil
	log.Printf("agent control off")
}

func (a *App) serveControl(w http.ResponseWriter, r *http.Request, token string) {
	reply := func(status int, resp agent.Response) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(resp)
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get(agent.TokenHeader)), []byte(token)) != 1 {
		reply(http.StatusUnauthorized, agent.Response{Error: "bad control token"})
		return
	}
	var req agent.Request
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		reply(http.StatusBadRequest, agent.Response{Error: "bad request: " + err.Error()})
		return
	}
	h, ok := controlHandlers[req.Method]
	if !ok {
		reply(http.StatusNotFound, agent.Response{Error: "no method " + req.Method})
		return
	}
	ctx, cancel := context.WithTimeout(from(r.Context(), "agent"), 3*time.Minute)
	defer cancel()
	if err := a.waitReady(ctx); err != nil {
		reply(http.StatusServiceUnavailable, agent.Response{Error: errNotReady.Error()})
		return
	}
	args := req.Args
	if len(bytes.TrimSpace(args)) == 0 || string(bytes.TrimSpace(args)) == "null" {
		args = json.RawMessage("{}")
	}
	v, err := h(ctx, a, args)
	if err != nil {
		reply(http.StatusOK, agent.Response{Error: agent.ScrubURLs(err.Error())})
		return
	}
	result, err := json.Marshal(v)
	if err != nil {
		reply(http.StatusInternalServerError, agent.Response{Error: err.Error()})
		return
	}
	log.Printf("agent: %s", req.Method)
	reply(http.StatusOK, agent.Response{Result: result})
}

// decodeArgs decodes the arguments of a call, refusing unknown ones.
func decodeArgs[T any](raw json.RawMessage) (T, error) {
	var v T
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, fmt.Errorf("arguments: %w", err)
	}
	return v, nil
}
