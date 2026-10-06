// Package webdav is a small WebDAV client: what sync needs (PROPFIND, GET,
// PUT with conditions, DELETE, MKCOL), over TLS that can pin a server's key.
package webdav

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// Errors of requests.
var (
	ErrNotFound     = errors.New("not found on the server")
	ErrPrecondition = errors.New("the file changed on the server")
	ErrAuth         = errors.New("the server refused the user name or password")
	ErrInsecure     = errors.New("the address uses http://, which sends the password and data unencrypted")
)

// Options configure a client.
type Options struct {
	// AllowInsecure allows http:// URLs.
	AllowInsecure bool
	// PinnedKey, the base64 SHA-256 of a certificate's public key, trusts
	// the server that presents it, as a self-signed one, instead of
	// certificate authorities.
	PinnedKey string
	// Proxy routes requests through a proxy, such as the core's; nil goes
	// direct.
	Proxy func(*http.Request) (*url.URL, error)
	Timeout time.Duration
}

// Client is a WebDAV client rooted at a URL.
type Client struct {
	base     *url.URL
	user     string
	password string
	hc       *http.Client
}

// New returns a client of the collection at rawURL.
func New(rawURL, user, password string, opts Options) (*Client, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("%q is not a URL", rawURL)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !opts.AllowInsecure {
			return nil, ErrInsecure
		}
	default:
		return nil, fmt.Errorf("%q is not an http(s) URL", rawURL)
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if opts.PinnedKey != "" {
		want := opts.PinnedKey
		tlsCfg.InsecureSkipVerify = true // replaced by the pin below
		tlsCfg.VerifyConnection = func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 {
				return errors.New("the server sent no certificate")
			}
			if got := KeyFingerprint(cs.PeerCertificates[0]); got != want {
				return fmt.Errorf("the server's key %s is not the pinned one", got)
			}
			return nil
		}
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	tr := &http.Transport{
		Proxy:               opts.Proxy,
		DialContext:         (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSClientConfig:     tlsCfg,
		TLSHandshakeTimeout: 15 * time.Second,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     60 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	return &Client{base: u, user: user, password: password, hc: &http.Client{Transport: tr, Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if req.URL.Scheme != "https" && !opts.AllowInsecure {
				return ErrInsecure // never follow a redirect to plain HTTP
			}
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			return nil
		}}}, nil
}

// KeyFingerprint returns the base64 SHA-256 of a certificate's public key.
func KeyFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}

// ServerKey connects to the server of rawURL and returns the fingerprint
// of its key and whether certificate authorities trust it, so that the
// user can decide to pin it.
func ServerKey(ctx context.Context, rawURL string) (fingerprint string, trusted bool, err error) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return "", false, errors.New("only https:// servers have keys")
	}
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "443")
	}
	d := tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true, ServerName: u.Hostname()}}
	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return "", false, err
	}
	defer conn.Close()
	cs := conn.(*tls.Conn).ConnectionState()
	if len(cs.PeerCertificates) == 0 {
		return "", false, errors.New("no certificate")
	}
	leaf := cs.PeerCertificates[0]
	inter := x509.NewCertPool()
	for _, c := range cs.PeerCertificates[1:] {
		inter.AddCert(c)
	}
	_, verr := leaf.Verify(x509.VerifyOptions{DNSName: u.Hostname(), Intermediates: inter})
	return KeyFingerprint(leaf), verr == nil, nil
}

func (c *Client) url(p string) string {
	u := *c.base
	segments := strings.Split(strings.Trim(p, "/"), "/")
	for i, s := range segments {
		segments[i] = url.PathEscape(s)
	}
	u.RawPath = ""
	u.Path = c.base.Path + strings.Join(segments, "/")
	if strings.HasSuffix(p, "/") {
		u.Path += "/"
	}
	return u.String()
}

func (c *Client) do(ctx context.Context, method, p string, body []byte, hdr map[string]string) (*http.Response, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(p), rd)
	if err != nil {
		return nil, err
	}
	if c.user != "" || c.password != "" {
		req.SetBasicAuth(c.user, c.password)
	}
	req.Header.Set("User-Agent", "MyGO-Clash")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	if body != nil {
		req.ContentLength = int64(len(body))
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, ErrAuth
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, ErrNotFound
	case http.StatusPreconditionFailed:
		resp.Body.Close()
		return nil, ErrPrecondition
	}
	return resp, nil
}

func statusError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	msg := strings.TrimSpace(string(data))
	if len(msg) > 200 || strings.HasPrefix(msg, "<") {
		msg = ""
	}
	if msg != "" {
		return fmt.Errorf("the server answered %s: %s", resp.Status, msg)
	}
	return fmt.Errorf("the server answered %s", resp.Status)
}

// Info describes a file on the server.
type Info struct {
	Name    string
	Size    int64
	ModTime time.Time
	ETag    string
	IsDir   bool
}

type multistatus struct {
	Responses []struct {
		Href     string `xml:"href"`
		Propstat []struct {
			Prop struct {
				Length       string `xml:"getcontentlength"`
				LastModified string `xml:"getlastmodified"`
				ETag         string `xml:"getetag"`
				ResourceType struct {
					Collection *struct{} `xml:"collection"`
				} `xml:"resourcetype"`
			} `xml:"prop"`
			Status string `xml:"status"`
		} `xml:"propstat"`
	} `xml:"response"`
}

const propfindBody = `<?xml version="1.0" encoding="utf-8"?>
<d:propfind xmlns:d="DAV:"><d:prop><d:getcontentlength/><d:getlastmodified/><d:getetag/><d:resourcetype/></d:prop></d:propfind>`

func (c *Client) propfind(ctx context.Context, p, depth string) ([]Info, error) {
	resp, err := c.do(ctx, "PROPFIND", p, []byte(propfindBody), map[string]string{"Depth": depth, "Content-Type": "application/xml; charset=utf-8"})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMultiStatus && resp.StatusCode != http.StatusOK {
		return nil, statusError(resp)
	}
	var ms multistatus
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&ms); err != nil {
		return nil, fmt.Errorf("the server's answer is not WebDAV: %w", err)
	}
	var out []Info
	for _, r := range ms.Responses {
		href, err := url.PathUnescape(r.Href)
		if err != nil {
			href = r.Href
		}
		if hu, err := url.Parse(href); err == nil && hu.Path != "" {
			href = hu.Path
		}
		info := Info{Name: path.Base(strings.TrimSuffix(href, "/"))}
		info.IsDir = strings.HasSuffix(href, "/")
		for _, ps := range r.Propstat {
			if !strings.Contains(ps.Status, " 200") {
				continue
			}
			pr := ps.Prop
			info.Size, _ = strconv.ParseInt(pr.Length, 10, 64)
			if t, err := http.ParseTime(pr.LastModified); err == nil {
				info.ModTime = t
			}
			info.ETag = strings.TrimSpace(pr.ETag)
			info.IsDir = info.IsDir || pr.ResourceType.Collection != nil
		}
		out = append(out, info)
	}
	return out, nil
}

// Stat describes a file.
func (c *Client) Stat(ctx context.Context, p string) (Info, error) {
	infos, err := c.propfind(ctx, p, "0")
	if err != nil {
		return Info{}, err
	}
	if len(infos) == 0 {
		return Info{}, ErrNotFound
	}
	return infos[0], nil
}

// List returns the files of a directory, without the directory itself.
func (c *Client) List(ctx context.Context, dir string) ([]Info, error) {
	dir = strings.TrimSuffix(dir, "/") + "/"
	infos, err := c.propfind(ctx, dir, "1")
	if err != nil {
		return nil, err
	}
	self := path.Base(strings.TrimSuffix(c.url(dir), "/"))
	out := infos[:0]
	for i, info := range infos {
		if i == 0 && info.IsDir && info.Name == self {
			continue
		}
		out = append(out, info)
	}
	return out, nil
}

// Get downloads a file and returns it with its ETag.
func (c *Client) Get(ctx context.Context, p string) ([]byte, string, error) {
	resp, err := c.do(ctx, http.MethodGet, p, nil, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", statusError(resp)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	return data, resp.Header.Get("ETag"), err
}

// Condition makes a write conditional.
type Condition struct {
	// IfMatch writes only if the file still has this ETag.
	IfMatch string
	// IfNoneMatch writes only if the file does not exist.
	IfNoneMatch bool
}

// Put uploads a file and returns its new ETag, if the server says.
func (c *Client) Put(ctx context.Context, p string, data []byte, cond Condition) (string, error) {
	hdr := map[string]string{"Content-Type": "application/octet-stream"}
	if cond.IfMatch != "" {
		hdr["If-Match"] = cond.IfMatch
	}
	if cond.IfNoneMatch {
		hdr["If-None-Match"] = "*"
	}
	resp, err := c.do(ctx, http.MethodPut, p, data, hdr)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return resp.Header.Get("ETag"), nil
	case http.StatusConflict:
		return "", fmt.Errorf("%w: the folder does not exist", ErrNotFound)
	}
	return "", statusError(resp)
}

// Delete deletes a file; a missing one is not an error.
func (c *Client) Delete(ctx context.Context, p string, ifMatch string) error {
	hdr := map[string]string{}
	if ifMatch != "" {
		hdr["If-Match"] = ifMatch
	}
	resp, err := c.do(ctx, http.MethodDelete, p, nil, hdr)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return statusError(resp)
	}
	return nil
}

// MkdirAll creates a directory and its parents.
func (c *Client) MkdirAll(ctx context.Context, dir string) error {
	parts := strings.Split(strings.Trim(dir, "/"), "/")
	for i := range parts {
		p := strings.Join(parts[:i+1], "/") + "/"
		if _, err := c.Stat(ctx, p); err == nil {
			continue
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		resp, err := c.do(ctx, "MKCOL", p, nil, nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
		// 405: it exists already (some servers do not answer PROPFIND for it).
		if resp.StatusCode >= 300 && resp.StatusCode != http.StatusMethodNotAllowed {
			return statusError(resp)
		}
	}
	return nil
}
