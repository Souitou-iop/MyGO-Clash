// Package service is the app's privileged helper. TUN needs privileges the
// app does not have (root on macOS and Linux, Administrator on Windows), so
// the user installs the service once, and it runs the core for them.
//
// Security:
//
//   - The service runs a copy of the app's executable that it installed
//     where only root can write, never the app's own executable, which the
//     user could replace.
//   - Its socket accepts only the user who installed it (checked with the
//     socket's peer credentials, or the named pipe's security descriptor
//     on Windows) and root.
//   - The core it runs has a home and a socket of its own, which only root
//     can reach; the app talks to the core through the service, which
//     forwards its requests after checking who sends them.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

// Routes of the service's own API; every other path goes to the core.
const (
	PathStatus    = "/__service/status"
	PathCoreStart = "/__service/core/start"
	PathCoreStop  = "/__service/core/stop"
)

// Status describes the service.
type Status struct {
	Version     string    `json:"version"`
	Name        string    `json:"name"`
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"startedAt"`
	CoreRunning bool      `json:"coreRunning"`
	CorePID     int       `json:"corePid,omitempty"`
	CoreError   string    `json:"coreError,omitempty"`
	Home        string    `json:"home"`
}

// StartRequest starts the core.
type StartRequest struct {
	LogLevel string `json:"logLevel,omitempty"`
}

// Client talks to the service.
type Client struct {
	Addr string
	hc   *http.Client
}

// NewClient returns a client of the service of the app named slug.
func NewClient(slug string) *Client {
	addr := Layout(slug).Socket
	return &Client{
		Addr: addr,
		hc: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return coreapi.Dial(ctx, addr)
			}},
		},
	}
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		data, _ := json.Marshal(body)
		rd = bytesReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://service"+path, rd)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		var eb coreapi.ErrorBody
		_ = json.NewDecoder(resp.Body).Decode(&eb)
		if eb.Message == "" {
			eb.Message = resp.Status
		}
		return &coreapi.APIError{Status: resp.StatusCode, Message: eb.Message}
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// Status returns the service's status; an error means it is not installed,
// not running or not ours to use.
func (c *Client) Status(ctx context.Context) (Status, error) {
	var s Status
	err := c.do(ctx, http.MethodGet, PathStatus, nil, &s)
	return s, err
}

// StartCore starts the core, stopping the one running.
func (c *Client) StartCore(ctx context.Context, req StartRequest) (Status, error) {
	var s Status
	err := c.do(ctx, http.MethodPost, PathCoreStart, req, &s)
	return s, err
}

// StopCore stops the core.
func (c *Client) StopCore(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, PathCoreStop, nil, nil)
}

// ErrNotInstalled is returned by Installed when the service is not.
var ErrNotInstalled = errors.New("the service is not installed")

// Check reports the service's status, or why it cannot be used.
func Check(ctx context.Context, slug string) (Status, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	s, err := NewClient(slug).Status(ctx)
	if err != nil {
		var ae *coreapi.APIError
		if errors.As(err, &ae) && ae.Status == http.StatusForbidden {
			return s, fmt.Errorf("the service belongs to another user: %s", ae.Message)
		}
		return s, ErrNotInstalled
	}
	return s, nil
}
