package profiles

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/metacubex/mihomo/common/convert"
)

// ConvertLinks turns a list of share links, plain or in base64, into
// proxies, and returns the lines it could not read. mihomo's converter
// reads most schemes. wireguard://, snell:// and ssh://, which mihomo runs
// but does not read as links, are read here. Proxies without a name are
// named after their type and server, and names are made unique.
func ConvertLinks(data []byte) (proxies []map[string]any, skipped []string) {
	data = convert.DecodeBase64(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")))
	for _, raw := range bytes.Split(data, []byte("\n")) {
		line := strings.TrimSpace(string(raw))
		if line == "" {
			continue
		}
		read, err := convertLink(line)
		if err != nil {
			skipped = append(skipped, line)
			continue
		}
		proxies = append(proxies, read...)
	}
	nameProxies(proxies)
	return proxies, skipped
}

// linkTypes are the schemes read here, and the type of proxy each is.
var linkTypes = map[string]string{
	"wireguard": "wireguard",
	"wg":        "wireguard",
	"snell":     "snell",
	"ssh":       "ssh",
}

// linkReaders fill in what a proxy of each type needs beyond its name,
// server and port.
var linkReaders = map[string]func(u *url.URL, p map[string]any) error{
	"wireguard": readWireGuard,
	"snell":     readSnell,
	"ssh":       readSSH,
}

func convertLink(line string) ([]map[string]any, error) {
	scheme, _, _ := strings.Cut(line, "://")
	typ, ok := linkTypes[strings.ToLower(scheme)]
	if !ok {
		return convert.ConvertsV2Ray([]byte(line))
	}
	u, err := url.Parse(line)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(u.Port())
	if u.Hostname() == "" || err != nil || port <= 0 || port > 65535 {
		return nil, errors.New("the link names no server and port")
	}
	p := map[string]any{"name": u.Fragment, "type": typ, "server": u.Hostname(), "port": port}
	if err := linkReaders[typ](u, p); err != nil {
		return nil, err
	}
	return []map[string]any{p}, nil
}

// readWireGuard reads wireguard://privatekey@server:port?publickey=...
// &address=10.0.0.2/32,fd00::2/128&reserved=1,2,3&mtu=1280, as v2rayN,
// sing-box and clash-verge-rev share it.
func readWireGuard(u *url.URL, p map[string]any) error {
	q := u.Query()
	private := u.User.Username()
	if private == "" {
		private = queryKey(q, "privatekey", "private-key")
	}
	public := queryKey(q, "publickey", "public-key", "peer-public-key")
	if private == "" || public == "" {
		return errors.New("the link has no keys")
	}
	p["private-key"], p["public-key"], p["udp"] = private, public, true
	if shared := queryKey(q, "presharedkey", "pre-shared-key", "psk"); shared != "" {
		p["pre-shared-key"] = shared
	}
	addresses := strings.Split(firstOf(q, "address", "ip", "local-address")+","+q.Get("ipv6"), ",")
	for _, addr := range addresses {
		addr = strings.TrimSpace(addr)
		ip, _, _ := strings.Cut(addr, "/")
		switch {
		case addr == "":
		case strings.Contains(ip, ":"):
			if _, set := p["ipv6"]; !set {
				p["ipv6"] = addr
			}
		default:
			if _, set := p["ip"]; !set {
				p["ip"] = addr
			}
		}
	}
	if reserved := readReserved(q.Get("reserved")); reserved != nil {
		p["reserved"] = reserved
	}
	if mtu, err := strconv.Atoi(q.Get("mtu")); err == nil && mtu > 0 {
		p["mtu"] = mtu
	}
	return nil
}

// readReserved reads the three reserved bytes of WireGuard, written
// "1,2,3" or in base64.
func readReserved(s string) []any {
	if s == "" {
		return nil
	}
	if parts := strings.Split(s, ","); len(parts) == 3 {
		out := []any{}
		for _, part := range parts {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 0 || n > 255 {
				return nil
			}
			out = append(out, n)
		}
		return out
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != 3 {
		return nil
	}
	return []any{int(b[0]), int(b[1]), int(b[2])}
}

// readSnell reads snell://psk@server:port?version=4&obfs=http
// &obfs-host=bing.com.
func readSnell(u *url.URL, p map[string]any) error {
	q := u.Query()
	psk := u.User.Username()
	if psk == "" {
		psk = q.Get("psk")
	}
	if psk == "" {
		return errors.New("the link has no pre-shared key")
	}
	p["psk"] = psk
	if version, err := strconv.Atoi(q.Get("version")); err == nil {
		p["version"] = version
		// Snell carries UDP from its version 3.
		p["udp"] = version >= 3
	}
	if mode := firstOf(q, "obfs", "obfs-mode"); mode != "" && mode != "none" {
		p["obfs-opts"] = map[string]any{"mode": mode, "host": firstOf(q, "obfs-host", "host")}
	}
	return nil
}

// readSSH reads ssh://user:secret@server:port?host-key=....
func readSSH(u *url.URL, p map[string]any) error {
	user := u.User.Username()
	if user == "" {
		return errors.New("the link has no user")
	}
	p["username"] = user
	if secret, ok := u.User.Password(); ok {
		p["password"] = secret
	}
	if hostKey := u.Query().Get("host-key"); hostKey != "" {
		p["host-key"] = strings.Split(hostKey, ",")
	}
	return nil
}

// firstOf returns the first of the query parameters that is set, as
// links name the same thing differently.
func firstOf(q url.Values, names ...string) string {
	for _, name := range names {
		if v := strings.TrimSpace(q.Get(name)); v != "" {
			return v
		}
	}
	return ""
}

// queryKey is firstOf for a key in base64, whose "+" links often leave
// unescaped and the query then reads as a space.
func queryKey(q url.Values, names ...string) string {
	return strings.ReplaceAll(firstOf(q, names...), " ", "+")
}

// nameProxies names the proxies without a name after their type and
// server, as "vmess 1.2.3.4:443", and makes the names unique as mihomo
// does, "name-01", "name-02".
func nameProxies(proxies []map[string]any) {
	seen := map[string]bool{}
	count := map[string]int{}
	for _, p := range proxies {
		name, _ := p["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			name = fmt.Sprintf("%v %s", p["type"], net.JoinHostPort(fmt.Sprint(p["server"]), fmt.Sprint(p["port"])))
		}
		base := name
		for seen[name] {
			count[base]++
			name = fmt.Sprintf("%s-%02d", base, count[base])
		}
		seen[name] = true
		p["name"] = name
	}
}
