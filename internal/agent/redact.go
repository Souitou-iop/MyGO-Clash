package agent

import (
	"net/url"
	"regexp"
	"strings"
)

// Redacted stands in for a secret, as in the app's diagnostics.
const Redacted = "[redacted]"

// MaskURL keeps what tells a subscription apart, its scheme and host, and
// hides the rest, where providers put their tokens:
// "https://sub.example.com/api/v1/client/subscribe?token=…" →
// "https://sub.example.com/[redacted]".
func MaskURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return Redacted
	}
	host := u.Hostname()
	if p := u.Port(); p != "" {
		host += ":" + p
	}
	if u.Path == "" && u.RawQuery == "" && u.Fragment == "" && u.User == nil {
		return u.Scheme + "://" + host
	}
	return u.Scheme + "://" + host + "/" + Redacted
}

var urlPattern = regexp.MustCompile(`[a-zA-Z][a-zA-Z0-9+.-]*://[^\s"'<>]+`)

// ScrubURLs masks every URL in a text, such as an error that quotes the
// address it failed to fetch.
func ScrubURLs(s string) string {
	return urlPattern.ReplaceAllStringFunc(s, func(m string) string {
		trail := ""
		for strings.HasSuffix(m, ")") || strings.HasSuffix(m, ".") || strings.HasSuffix(m, ",") || strings.HasSuffix(m, ";") {
			trail = m[len(m)-1:] + trail
			m = m[:len(m)-1]
		}
		return MaskURL(m) + trail
	})
}

// HasRedacted reports whether a decoded JSON value holds the mask, which
// must never be written back as a value.
func HasRedacted(v any) bool {
	switch v := v.(type) {
	case string:
		return strings.Contains(v, Redacted)
	case map[string]any:
		for _, x := range v {
			if HasRedacted(x) {
				return true
			}
		}
	case []any:
		for _, x := range v {
			if HasRedacted(x) {
				return true
			}
		}
	}
	return false
}
