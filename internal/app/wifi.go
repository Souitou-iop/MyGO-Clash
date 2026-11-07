package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/config"
)

// The parsers of each system's answer, platform-neutral so the tests see
// them everywhere. The readers live in the wifi_*.go of their platform.

// wifiDeviceDarwin finds the device of the Wi-Fi port, en1 rather than en0
// more often than one would think.
func wifiDeviceDarwin(hardwarePorts string) string {
	lines := strings.Split(hardwarePorts, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "Hardware Port: Wi-Fi" && i+1 < len(lines) {
			fields := strings.Fields(lines[i+1])
			if len(fields) == 2 && fields[0] == "Device:" {
				return fields[1]
			}
		}
	}
	return ""
}

// parseAirportNetwork reads the answer of -getairportnetwork: the network's
// name after "Current Wi-Fi Network: ", and nothing when not associated.
func parseAirportNetwork(out string) string {
	line := strings.TrimSpace(out)
	const prefix = "Current Wi-Fi Network: "
	if strings.HasPrefix(line, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(line, prefix))
	}
	return ""
}

// parseNetshInterfaces reads the SSID out of netsh's answer. The labels are
// localized, but the field keeps the literal SSID on every locale met so
// far; only a field NAMED SSID counts, so BSSID and networks whose name
// mentions it stay out of the way.
func parseNetshInterfaces(out string) string {
	for _, line := range strings.Split(out, "\n") {
		label, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(label) != "SSID" {
			continue
		}
		if v := strings.TrimSpace(value); v != "" {
			return v
		}
	}
	return ""
}

// parseNmcliWiFi reads the ssid of the active network: the line that starts
// with "yes:", unescaping the colons nmcli escapes.
func parseNmcliWiFi(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "yes:") {
			continue
		}
		ssid := strings.TrimPrefix(line, "yes:")
		ssid = strings.ReplaceAll(ssid, `\:`, ":")
		ssid = strings.ReplaceAll(ssid, `\\`, `\`)
		return strings.TrimSpace(ssid)
	}
	return ""
}

// watchWiFi acts on the Wi-Fi networks the rules name: when the computer
// joins one of them, the rule's actions run once. The watcher sleeps while
// there are no rules, and the network is asked every few seconds while
// there are. The first read of a session counts as a join, so the state
// fits the network the computer woke up on.
func (a *App) watchWiFi() {
	go func() {
		last := ""
		for {
			time.Sleep(5 * time.Second)
			rules := a.settings.Get().WiFiRules
			if len(rules) == 0 {
				continue
			}
			ssid, err := wifiSSID()
			if err != nil || ssid == "" || ssid == last {
				continue
			}
			if r := matchWiFiRule(rules, ssid); r != nil {
				a.applyWiFiRule(ssid, r)
			}
			last = ssid
		}
	}()
}

// matchWiFiRule finds the rule of a network, the first that names it.
func matchWiFiRule(rules []config.WiFiRule, ssid string) *config.WiFiRule {
	if ssid == "" {
		return nil
	}
	for i := range rules {
		if rules[i].SSID == ssid {
			return &rules[i]
		}
	}
	return nil
}

// applyWiFiRule runs the actions of a rule: the profile to switch to, the
// system proxy and TUN to turn on or off, the mode to set. The proxy and
// TUN go through the settings, whose paths apply them; a TUN the service
// cannot carry is undone there, with its notice.
func (a *App) applyWiFiRule(ssid string, r *config.WiFiRule) {
	ctx := context.Background()
	var parts []string
	if r.ProfileUID != "" {
		if err := (Profiles{a}).Activate(ctx, r.ProfileUID); err == nil {
			parts = append(parts, tr(a, "profile"))
		}
	}
	if r.SysProxy != nil || r.Tun != nil {
		_, _ = a.updateSettings(ctx, func(s *config.Settings) {
			if r.SysProxy != nil {
				s.SystemProxy.Enabled = *r.SysProxy
			}
			if r.Tun != nil {
				s.Tun.Enabled = *r.Tun
			}
		})
		if r.SysProxy != nil {
			parts = append(parts, tr(a, "systemProxy"))
		}
		if r.Tun != nil {
			parts = append(parts, tr(a, "tun"))
		}
	}
	if r.Mode != "" {
		a.setMode("wifi", r.Mode)
		parts = append(parts, tr(a, r.Mode))
	}
	if len(parts) == 0 {
		return
	}
	a.notify(Notice{Level: "info", Message: fmt.Sprintf(tr(a, "wifiApplied"), ssid) + " · " + strings.Join(parts, " · ")})
}
