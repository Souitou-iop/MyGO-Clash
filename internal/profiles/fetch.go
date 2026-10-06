package profiles

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/metacubex/mihomo/common/convert"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// DefaultUserAgent is sent with subscription requests. Providers look for
// "clash" to answer with a Clash profile, and for "mihomo" or "verge" to
// include the protocols only mihomo supports.
var DefaultUserAgent = "mihomo/1.19 clash-verge/v2.4 MyGO-Clash"

const maxSubscription = 32 << 20

// FetchOptions configures a download.
type FetchOptions struct {
	UserAgent string
	Timeout   time.Duration
	Insecure  bool
	// Proxy is the proxy to download through, such as
	// http://127.0.0.1:7897, or "" for direct.
	Proxy string
}

// Fetched is a downloaded subscription.
type Fetched struct {
	Content []byte
	// Name is the file name the provider suggested.
	Name           string
	Usage          *Usage
	UpdateInterval int // minutes, 0 when not suggested
	Home           string
	Converted      bool
	Proxies        int
	Groups         int
}

// Fetch downloads a subscription and checks that it is a profile. Lists
// of share links (vmess://, ss://, trojan://, ...; plain or base64) become
// a profile with a selector and an automatic group.
func Fetch(ctx context.Context, rawURL string, opts FetchOptions) (*Fetched, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%q is not an http(s) URL", rawURL)
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 60 * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: opts.Insecure, MinVersion: tls.VersionTLS12}
	if opts.Proxy != "" {
		pu, err := url.Parse(opts.Proxy)
		if err != nil {
			return nil, err
		}
		transport.Proxy = http.ProxyURL(pu)
	} else {
		transport.Proxy = nil // not the system proxy, which may be the app itself
	}
	client := &http.Client{Transport: transport, Timeout: opts.Timeout}
	defer client.CloseIdleConnections()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, describeNetError(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("the server answered %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSubscription+1))
	if err != nil {
		return nil, describeNetError(err)
	}
	if len(body) > maxSubscription {
		return nil, fmt.Errorf("the subscription is larger than %d MiB", maxSubscription>>20)
	}

	f := &Fetched{
		Name:  filenameOf(resp.Header.Get("Content-Disposition")),
		Usage: parseUserinfo(resp.Header.Get("Subscription-Userinfo")),
		Home:  strings.TrimSpace(resp.Header.Get("Profile-Web-Page-Url")),
	}
	if h, err := strconv.ParseFloat(strings.TrimSpace(resp.Header.Get("Profile-Update-Interval")), 64); err == nil && h > 0 {
		f.UpdateInterval = int(h * 60)
	}
	if f.Name == "" {
		f.Name = u.Hostname()
	}
	content, converted, err := Normalize(body)
	if err != nil {
		return nil, err
	}
	f.Content, f.Converted = content, converted
	if cfg, err := yamlx.Parse(content); err == nil {
		f.Proxies, f.Groups = countOf(cfg)
	}
	return f, nil
}

func describeNetError(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		if ue.Timeout() {
			return errors.New("the download timed out; try again, or download through the proxy")
		}
		return fmt.Errorf("the download failed: %v", ue.Err)
	}
	return err
}

// Normalize checks that data is a profile: a Clash configuration, or a
// list of share links, which it converts.
func Normalize(data []byte) (content []byte, converted bool, err error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if cfg, perr := yamlx.Parse(data); perr == nil {
		if cfg.Has("proxies") || cfg.Has("proxy-providers") || cfg.Has("proxy-groups") || cfg.Has("rules") {
			return data, false, nil
		}
		if cfg.Len() == 0 {
			return nil, false, errors.New("the subscription is empty")
		}
	}
	proxies, cerr := convert.ConvertsV2Ray(data)
	if cerr != nil || len(proxies) == 0 {
		return nil, false, errors.New("the subscription is not a Clash profile nor a list of share links; check its URL, or that your provider supports Clash")
	}
	return FromProxies(proxies), true, nil
}

// FromProxies builds a profile around proxies: a selector, an automatic
// group picking the fastest, and a rule sending everything to the
// selector.
func FromProxies(proxies []map[string]any) []byte {
	cfg := yamlx.NewMap()
	names := []any{}
	list := []any{}
	for _, p := range proxies {
		m := yamlx.FromStd(p).(*yamlx.Map)
		name := m.String("name")
		m.SetFirst("name", name)
		list = append(list, m)
		names = append(names, name)
	}
	cfg.Set("proxies", list)
	cfg.Set("proxy-groups", []any{
		yamlx.MapOf("name", "Proxy", "type", "select", "proxies", append([]any{"Auto", "DIRECT"}, names...)),
		yamlx.MapOf("name", "Auto", "type", "url-test", "proxies", names,
			"url", "https://www.gstatic.com/generate_204", "interval", int64(300), "tolerance", int64(50)),
	})
	cfg.Set("rules", []any{"GEOIP,private,DIRECT,no-resolve", "MATCH,Proxy"})
	out, _ := yamlx.Marshal(cfg)
	return append([]byte("# Converted from a list of share links by MyGO-Clash\n"), out...)
}

func countOf(cfg *yamlx.Map) (proxies, groups int) {
	return len(cfg.Slice("proxies")), len(cfg.Slice("proxy-groups"))
}

// parseUserinfo reads "upload=1; download=2; total=3; expire=4".
func parseUserinfo(h string) *Usage {
	if h == "" {
		return nil
	}
	var u Usage
	found := false
	for _, part := range strings.FieldsFunc(h, func(r rune) bool { return r == ';' || r == ',' }) {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			continue
		}
		found = true
		switch strings.ToLower(strings.TrimSpace(k)) {
		case "upload":
			u.Upload = int64(n)
		case "download":
			u.Download = int64(n)
		case "total":
			u.Total = int64(n)
		case "expire":
			u.Expire = int64(n)
		}
	}
	if !found {
		return nil
	}
	return &u
}

// filenameOf reads the file name of a Content-Disposition header,
// preferring filename* (RFC 5987, which carries non-ASCII names).
func filenameOf(h string) string {
	if h == "" {
		return ""
	}
	_, params, err := mime.ParseMediaType(h)
	name := ""
	if err == nil {
		name = params["filename"] // mime decodes filename* into filename
	}
	if name == "" {
		// Malformed headers are common; look for the parameter by hand.
		for _, part := range strings.Split(h, ";") {
			k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
			if !ok {
				continue
			}
			switch strings.ToLower(k) {
			case "filename*":
				if _, enc, ok := strings.Cut(v, "''"); ok {
					if dec, err := url.QueryUnescape(enc); err == nil {
						name = dec
					}
				}
			case "filename":
				if name == "" {
					name = strings.Trim(v, `"`)
				}
			}
		}
	}
	name = strings.TrimSpace(name)
	for _, ext := range []string{".yaml", ".yml", ".txt"} {
		if strings.HasSuffix(strings.ToLower(name), ext) {
			name = name[:len(name)-len(ext)]
		}
	}
	return name
}
