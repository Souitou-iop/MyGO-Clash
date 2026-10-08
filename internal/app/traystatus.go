package app

import (
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/mygo-clash/mygo-clash/internal/coremgr"
)

// statusLine is the tray's glance at the app: the mode, the switch that
// carries traffic and the node it goes out of, as "Rule · TUN · HK 01".
// The menu starts with it and the tooltip shows it; the rest is in the app.
func statusLine(a *App, s AppState, node string) string {
	if s.Core.Status != coremgr.StatusRunning {
		return statusLabel(a, s.Core)
	}
	parts := []string{tr(a, s.Mode)}
	switch {
	case s.Tun:
		parts = append(parts, "TUN")
	case s.SystemProxy:
		parts = append(parts, tr(a, "systemProxy"))
	default:
		parts = append(parts, tr(a, "proxyOff"))
	}
	if node != "" && s.Mode != "direct" {
		parts = append(parts, node)
	}
	return strings.Join(parts, " · ")
}

// leafNode is the node the mode's main group ends at, through nested
// groups. Rule mode's main group is the first selector the profile shows.
func leafNode(v *ProxiesView, mode string) string {
	if v == nil {
		return ""
	}
	var g *ProxyGroup
	if mode == "global" {
		g = v.Global
	} else {
		for i := range v.Groups {
			if v.Groups[i].Type == "Selector" && !v.Groups[i].Hidden {
				g = &v.Groups[i]
				break
			}
		}
	}
	if g == nil {
		return ""
	}
	byName := make(map[string]*ProxyGroup, len(v.Groups))
	for i := range v.Groups {
		byName[v.Groups[i].Name] = &v.Groups[i]
	}
	node := g.Now
	for range 8 { // groups can't loop, but a broken view shouldn't hang us
		next, ok := byName[node]
		if !ok || next.Now == "" {
			break
		}
		node = next.Now
	}
	return node
}

// toolTip puts the line under the app's name. Windows keeps 127 UTF-16
// units of it; Linux shows the title on one line, if at all.
func toolTip(name, line string) string {
	if runtime.GOOS == "linux" {
		return name + " · " + line
	}
	tip := name + "\n" + line
	if runtime.GOOS == "windows" {
		tip = clipUTF16(tip, 127)
	}
	return tip
}

// clipUTF16 cuts s to n UTF-16 units, between characters.
func clipUTF16(s string, n int) string {
	if len(utf16.Encode([]rune(s))) <= n {
		return s
	}
	used := 0
	for i, r := range s {
		used += utf16.RuneLen(r)
		if used > n-1 { // room for the ellipsis
			return s[:i] + "…"
		}
	}
	return s
}
