package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/paths"
)

// TokenHeader carries the control token.
const TokenHeader = "X-MyGO-Clash-Token"

// CallPath is where the socket takes calls: POST {"method", "args"}.
const CallPath = "/v1/call"

// Request is a call of a method.
type Request struct {
	Method string          `json:"method"`
	Args   json.RawMessage `json:"args,omitempty"`
}

// Response is the answer to a call: a result or an error.
type Response struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// ErrNotRunning reports that no app answers.
var ErrNotRunning = errors.New("MyGO-Clash is not running, or its agent control is off (Settings → agent.enabled)")

// Target is an app to talk to.
type Target struct {
	Name   string // MyGO-Clash or MyGO-Clash Dev
	Socket string
	Token  string // the token's file
}

// Targets returns where the apps would serve, the release first. dev
// keeps the development build only; MYGO_CLASH_SOCKET names a socket
// explicitly (its token beside it, as control.token).
func Targets(dev bool) []Target {
	if s := os.Getenv("MYGO_CLASH_SOCKET"); s != "" {
		return []Target{{Name: "MyGO-Clash", Socket: s, Token: strings.TrimSuffix(s, "control.sock") + "control.token"}}
	}
	var ts []Target
	for _, name := range paths.Names {
		if dev && !strings.HasSuffix(name, " Dev") {
			continue
		}
		data, err := paths.DataDir(name)
		if err != nil {
			continue
		}
		sock, err := paths.ControlSocket(data, paths.Slug(name))
		if err != nil {
			continue
		}
		ts = append(ts, Target{Name: name, Socket: sock, Token: paths.ControlToken(data)})
	}
	return ts
}

// Client calls the methods of a running app.
type Client struct {
	Target Target
	token  string
	hc     *http.Client
}

// Dial finds a running app among the targets.
func Dial(ctx context.Context, targets []Target) (*Client, error) {
	for _, t := range targets {
		var d net.Dialer
		cctx, cancel := context.WithTimeout(ctx, time.Second)
		conn, err := d.DialContext(cctx, "unix", t.Socket)
		cancel()
		if err != nil {
			continue
		}
		_ = conn.Close()
		token, err := os.ReadFile(t.Token)
		if err != nil {
			return nil, fmt.Errorf("read the control token: %w", err)
		}
		sock := t.Socket
		return &Client{Target: t, token: strings.TrimSpace(string(token)), hc: &http.Client{
			Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return d.DialContext(ctx, "unix", sock)
			}},
		}}, nil
	}
	return nil, ErrNotRunning
}

// Call calls a method with args (any JSON value, nil for none) and decodes
// its result into out, unless nil.
func (c *Client) Call(ctx context.Context, method string, args any, out any) error {
	raw, err := c.CallRaw(ctx, method, args)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// CallRaw calls a method and returns its result as JSON.
func (c *Client) CallRaw(ctx context.Context, method string, args any) (json.RawMessage, error) {
	req := Request{Method: method}
	if args != nil {
		b, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		req.Args = b
	}
	body, _ := json.Marshal(req)
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://mygo-clash"+CallPath, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set(TokenHeader, c.token)
	resp, err := c.hc.Do(hr)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var r Response
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if r.Error != "" {
		return nil, errors.New(r.Error)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New(resp.Status)
	}
	return r.Result, nil
}
