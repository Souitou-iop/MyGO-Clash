package app

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// githubMirror serves GitHub's downloads where GitHub is out of reach, as
// Clash Verge uses it. What comes through it is signed, so it can't change
// an update, only hold one back.
const githubMirror = "https://gh-proxy.org/"

// githubTransport carries the app's own requests to GitHub, the update
// checks and downloads of mygo's updater: through the core when it runs,
// which may be the only way out, then directly, then for downloads through
// a mirror. Go's default would go directly only, ignoring the system proxy.
type githubTransport struct {
	a      *App
	core   *http.Transport
	direct *http.Transport
}

func newGitHubTransport(a *App) *githubTransport {
	return &githubTransport{
		a: a,
		core: quickTransport(func(*http.Request) (*url.URL, error) {
			return &url.URL{Scheme: "http", Host: a.mixedAddr()}, nil
		}),
		direct: quickTransport(http.ProxyFromEnvironment),
	}
}

// quickTransport gives up on a route that does not answer soon, leaving
// the time to the next.
func quickTransport(proxy func(*http.Request) (*url.URL, error)) *http.Transport {
	return &http.Transport{
		Proxy:                 proxy,
		DialContext:           (&net.Dialer{Timeout: 8 * time.Second}).DialContext,
		TLSHandshakeTimeout:   8 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		IdleConnTimeout:       30 * time.Second,
	}
}

func isGitHub(host string) bool {
	return host == "github.com" || host == "api.github.com" || strings.HasSuffix(host, ".githubusercontent.com")
}

func (t *githubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if !isGitHub(req.URL.Hostname()) || (req.Method != http.MethodGet && req.Method != http.MethodHead) {
		return http.DefaultTransport.RoundTrip(req)
	}
	type route struct {
		name string
		rt   http.RoundTripper
		req  *http.Request
	}
	var routes []route
	if t.a.mixedAddr() != "" {
		routes = append(routes, route{"core", t.core, req})
	}
	routes = append(routes, route{"direct", t.direct, req})
	// The mirror fetches downloads, not the API.
	if req.URL.Hostname() != "api.github.com" {
		if u, err := url.Parse(githubMirror + req.URL.String()); err == nil {
			r := req.Clone(req.Context())
			r.URL, r.Host = u, ""
			routes = append(routes, route{"mirror", t.direct, r})
		}
	}
	var errs []error
	for _, r := range routes {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		resp, err := r.rt.RoundTrip(r.req)
		if err == nil && resp.StatusCode < 500 {
			return resp, nil
		}
		if err == nil {
			resp.Body.Close()
			err = fmt.Errorf("HTTP %s", resp.Status)
		}
		errs = append(errs, fmt.Errorf("%s: %w", r.name, err))
	}
	return nil, errors.Join(errs...)
}
