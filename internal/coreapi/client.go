package coreapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// APIError is an error response of the core.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("core: HTTP %d", e.Status)
	}
	return e.Message
}

// IsNotFound reports whether err is a 404 of the core, such as a proxy
// that does not exist.
func IsNotFound(err error) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}

// Client talks to a core over its socket.
type Client struct {
	addr   string
	hc     *http.Client
	stream *http.Client
}

// New returns a client of the core serving at addr, a Unix socket path or
// a named pipe.
func New(addr string) *Client {
	transport := func() *http.Transport {
		return &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return Dial(ctx, addr)
			},
			MaxIdleConns:        8,
			MaxIdleConnsPerHost: 8,
			IdleConnTimeout:     30 * time.Second,
			DisableCompression:  true,
		}
	}
	return &Client{
		addr:   addr,
		hc:     &http.Client{Transport: transport(), Timeout: 60 * time.Second},
		stream: &http.Client{Transport: transport()},
	}
}

// Addr returns the socket the client connects to.
func (c *Client) Addr() string { return c.addr }

// Close releases idle connections.
func (c *Client) Close() {
	c.hc.CloseIdleConnections()
	c.stream.CloseIdleConnections()
}

const base = "http://core"

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(data)
	}
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return readError(resp)
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func readError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var eb ErrorBody
	if json.Unmarshal(data, &eb) == nil && eb.Message != "" {
		return &APIError{Status: resp.StatusCode, Message: eb.Message}
	}
	return &APIError{Status: resp.StatusCode, Message: string(bytes.TrimSpace(data))}
}

// streamLines calls fn with each JSON line a streaming endpoint sends,
// until ctx ends, the stream ends or fn returns an error.
func (c *Client) streamLines(ctx context.Context, path string, query url.Values, fn func([]byte) error) error {
	u := base + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.stream.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return readError(resp)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		if err := fn(line); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return io.ErrUnexpectedEOF
}

// ---- Routes of the core ----

// Health returns the core's health.
func (c *Client) Health(ctx context.Context) (Health, error) {
	var h Health
	err := c.do(ctx, http.MethodGet, PathHealth, nil, nil, &h)
	return h, err
}

// Apply replaces the running configuration.
func (c *Client) Apply(ctx context.Context, req ApplyRequest) (ApplyResponse, error) {
	var res ApplyResponse
	err := c.do(ctx, http.MethodPost, PathApply, nil, req, &res)
	return res, err
}

// Validate checks a configuration without applying it.
func (c *Client) Validate(ctx context.Context, config string) error {
	return c.do(ctx, http.MethodPost, PathValidate, nil, ValidateRequest{Config: config}, nil)
}

// PatchGeneral changes the mode or the log level.
func (c *Client) PatchGeneral(ctx context.Context, p GeneralPatch) error {
	return c.do(ctx, http.MethodPatch, PathGeneral, nil, p, nil)
}

// UpdateGeo downloads the GeoIP, GeoSite and ASN databases again.
func (c *Client) UpdateGeo(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, PathGeoUpdate, nil, nil, nil)
}

// DisableRules disables or enables rules by index.
func (c *Client) DisableRules(ctx context.Context, rules map[int]bool) error {
	return c.do(ctx, http.MethodPost, PathRulesDisable, nil, rules, nil)
}

// MatchRule finds the rule a connection to a target would hit, and the
// proxy it would go through.
func (c *Client) MatchRule(ctx context.Context, req MatchRequest) (MatchResult, error) {
	var res MatchResult
	err := c.do(ctx, http.MethodPost, PathRulesMatch, nil, req, &res)
	return res, err
}

// Shutdown stops the core.
func (c *Client) Shutdown(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, PathShutdown, nil, nil, nil)
}

// TailscaleStatus returns the embedded node's status.
func (c *Client) TailscaleStatus(ctx context.Context) (TailscaleStatus, error) {
	var s TailscaleStatus
	err := c.do(ctx, http.MethodGet, PathTSStatus, nil, nil, &s)
	return s, err
}

// TailscaleLogin starts a login of the embedded node.
func (c *Client) TailscaleLogin(ctx context.Context, l TailscaleLogin) error {
	return c.do(ctx, http.MethodPost, PathTSLogin, nil, l, nil)
}

// TailscaleLogout logs the embedded node out.
func (c *Client) TailscaleLogout(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, PathTSLogout, nil, nil, nil)
}

// TailscalePrefs changes preferences of the embedded node.
func (c *Client) TailscalePrefs(ctx context.Context, p TailscalePrefs) error {
	return c.do(ctx, http.MethodPost, PathTSPrefs, nil, p, nil)
}

// TailscalePing pings a peer of the embedded node.
func (c *Client) TailscalePing(ctx context.Context, p TailscalePing) (TailscalePingResult, error) {
	var r TailscalePingResult
	err := c.do(ctx, http.MethodPost, PathTSPing, nil, p, &r)
	return r, err
}

// TailscaleState returns the embedded node's saved state.
func (c *Client) TailscaleState(ctx context.Context) (TailscaleState, error) {
	var s TailscaleState
	err := c.do(ctx, http.MethodGet, PathTSStateExport, nil, nil, &s)
	return s, err
}

// WatchTailscale calls fn with the embedded node's events until ctx ends
// or the stream breaks.
func (c *Client) WatchTailscale(ctx context.Context, fn func(TailscaleEvent)) error {
	return c.streamLines(ctx, PathTSWatch, nil, func(line []byte) error {
		var e TailscaleEvent
		if err := json.Unmarshal(line, &e); err != nil {
			return err
		}
		fn(e)
		return nil
	})
}

// ---- mihomo's API ----

// Version returns mihomo's version.
func (c *Client) Version(ctx context.Context) (Version, error) {
	var v Version
	err := c.do(ctx, http.MethodGet, "/version", nil, nil, &v)
	return v, err
}

// Configs returns the running general configuration.
func (c *Client) Configs(ctx context.Context) (RuntimeConfig, error) {
	var rc RuntimeConfig
	err := c.do(ctx, http.MethodGet, "/configs", nil, nil, &rc)
	return rc, err
}

// Proxies returns every proxy and group, by name.
func (c *Client) Proxies(ctx context.Context) (map[string]Proxy, error) {
	var r ProxiesResponse
	err := c.do(ctx, http.MethodGet, "/proxies", nil, nil, &r)
	return r.Proxies, err
}

// SelectProxy selects a member of a selector group.
func (c *Client) SelectProxy(ctx context.Context, group, name string) error {
	return c.do(ctx, http.MethodPut, "/proxies/"+url.PathEscape(group), nil, map[string]string{"name": name}, nil)
}

// UnfixProxy clears the fixed selection of a url-test or fallback group.
func (c *Client) UnfixProxy(ctx context.Context, group string) error {
	return c.do(ctx, http.MethodDelete, "/proxies/"+url.PathEscape(group), nil, nil, nil)
}

func delayQuery(testURL string, timeout time.Duration) url.Values {
	q := url.Values{}
	q.Set("url", testURL)
	q.Set("timeout", strconv.Itoa(int(timeout/time.Millisecond)))
	return q
}

// ProxyDelay tests the delay of a proxy, in milliseconds.
func (c *Client) ProxyDelay(ctx context.Context, name, testURL string, timeout time.Duration) (int, error) {
	var r struct {
		Delay int `json:"delay"`
	}
	err := c.do(ctx, http.MethodGet, "/proxies/"+url.PathEscape(name)+"/delay", delayQuery(testURL, timeout), nil, &r)
	return r.Delay, err
}

// GroupDelay tests the delay of every member of a group.
func (c *Client) GroupDelay(ctx context.Context, group, testURL string, timeout time.Duration) (map[string]int, error) {
	r := map[string]int{}
	err := c.do(ctx, http.MethodGet, "/group/"+url.PathEscape(group)+"/delay", delayQuery(testURL, timeout), nil, &r)
	return r, err
}

// ProxyProviders returns the providers of proxies, by name.
func (c *Client) ProxyProviders(ctx context.Context) (map[string]ProxyProvider, error) {
	var r ProxyProvidersResponse
	err := c.do(ctx, http.MethodGet, "/providers/proxies", nil, nil, &r)
	return r.Providers, err
}

// UpdateProxyProvider downloads a provider again.
func (c *Client) UpdateProxyProvider(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPut, "/providers/proxies/"+url.PathEscape(name), nil, nil, nil)
}

// HealthcheckProxyProvider tests the proxies of a provider.
func (c *Client) HealthcheckProxyProvider(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodGet, "/providers/proxies/"+url.PathEscape(name)+"/healthcheck", nil, nil, nil)
}

// Rules returns the running rules.
func (c *Client) Rules(ctx context.Context) ([]Rule, error) {
	var r RulesResponse
	err := c.do(ctx, http.MethodGet, "/rules", nil, nil, &r)
	return r.Rules, err
}

// RuleProviders returns the providers of rules, by name.
func (c *Client) RuleProviders(ctx context.Context) (map[string]RuleProvider, error) {
	var r RuleProvidersResponse
	err := c.do(ctx, http.MethodGet, "/providers/rules", nil, nil, &r)
	return r.Providers, err
}

// UpdateRuleProvider downloads a rule provider again.
func (c *Client) UpdateRuleProvider(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodPut, "/providers/rules/"+url.PathEscape(name), nil, nil, nil)
}

// Connections returns the open connections.
func (c *Client) Connections(ctx context.Context) (Connections, error) {
	var r Connections
	err := c.do(ctx, http.MethodGet, "/connections", nil, nil, &r)
	return r, err
}

// CloseConnection closes a connection.
func (c *Client) CloseConnection(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/connections/"+url.PathEscape(id), nil, nil, nil)
}

// CloseAllConnections closes every connection.
func (c *Client) CloseAllConnections(ctx context.Context) error {
	return c.do(ctx, http.MethodDelete, "/connections", nil, nil, nil)
}

// FlushFakeIP empties the fake-ip cache.
func (c *Client) FlushFakeIP(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/cache/fakeip/flush", nil, nil, nil)
}

// FlushDNS empties the DNS cache.
func (c *Client) FlushDNS(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, "/cache/dns/flush", nil, nil, nil)
}

// DNSQuery resolves a name with the core's DNS.
func (c *Client) DNSQuery(ctx context.Context, name, typ string) (DNSQueryResult, error) {
	q := url.Values{}
	q.Set("name", name)
	if typ != "" {
		q.Set("type", typ)
	}
	var r DNSQueryResult
	err := c.do(ctx, http.MethodGet, "/dns/query", q, nil, &r)
	return r, err
}

// StreamTraffic calls fn with a traffic sample every second.
func (c *Client) StreamTraffic(ctx context.Context, fn func(Traffic)) error {
	return c.streamLines(ctx, "/traffic", nil, func(line []byte) error {
		var t Traffic
		if err := json.Unmarshal(line, &t); err != nil {
			return err
		}
		fn(t)
		return nil
	})
}

// StreamMemory calls fn with the core's memory use every second.
func (c *Client) StreamMemory(ctx context.Context, fn func(Memory)) error {
	return c.streamLines(ctx, "/memory", nil, func(line []byte) error {
		var m Memory
		if err := json.Unmarshal(line, &m); err != nil {
			return err
		}
		fn(m)
		return nil
	})
}

// StreamLogs calls fn with each log line of at least level (debug, info,
// warning, error).
func (c *Client) StreamLogs(ctx context.Context, level string, fn func(LogEvent)) error {
	q := url.Values{}
	if level != "" {
		q.Set("level", level)
	}
	return c.streamLines(ctx, "/logs", q, func(line []byte) error {
		var e LogEvent
		if err := json.Unmarshal(line, &e); err != nil {
			return err
		}
		e.Time = time.Now()
		fn(e)
		return nil
	})
}
