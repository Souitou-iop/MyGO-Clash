package app

import (
	"runtime"
	"strings"
	"unicode/utf16"

	"github.com/mygo-clash/mygo-clash/internal/coremgr"
)

// statusLines are what the tray says the app is doing: the mode and the
// switches that carry traffic, the profile, and the node it goes out of.
// The menu starts with them and the tooltip shows them.
func statusLines(a *App, s AppState, node string) []string {
	if s.Core.Status != coremgr.StatusRunning {
		return []string{statusLabel(a, s.Core)}
	}
	state := []string{tr(a, s.Mode)}
	if s.SystemProxy {
		state = append(state, tr(a, "systemProxy"))
	}
	if s.Tun {
		state = append(state, "TUN")
	}
	if len(state) == 1 {
		state = append(state, tr(a, "proxyOff"))
	}
	lines := []string{strings.Join(state, " · ")}
	if s.ProfileName != "" {
		lines = append(lines, tr(a, "profile")+": "+s.ProfileName)
	}
	if node != "" && s.Mode != "direct" {
		lines = append(lines, node)
	}
	return lines
}

// nodePath is the way out of the mode's main group, down to the node it
// ends at: "Proxy › Auto › HK 01". Rule mode's main group is the first
// selector the profile shows.
func nodePath(v *ProxiesView, mode string) string {
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
	if g == nil || g.Now == "" {
		return ""
	}
	byName := make(map[string]*ProxyGroup, len(v.Groups))
	for i := range v.Groups {
		byName[v.Groups[i].Name] = &v.Groups[i]
	}
	path := []string{g.Name, g.Now}
	for range 8 { // groups can't loop, but a broken view shouldn't hang us
		next, ok := byName[path[len(path)-1]]
		if !ok || next.Now == "" {
			break
		}
		path = append(path, next.Now)
	}
	if len(path) > 3 {
		path = []string{path[0], "…", path[len(path)-1]}
	}
	return strings.Join(path, " › ")
}

// toolTip joins the lines under the app's name. Windows keeps 127 UTF-16
// units of it; Linux shows the title on one line, if at all.
func toolTip(name string, lines []string) string {
	if runtime.GOOS == "linux" {
		return name + " · " + strings.Join(lines, " · ")
	}
	tip := name + "\n" + strings.Join(lines, "\n")
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
