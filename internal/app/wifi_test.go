package app

import (
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/config"
)

func TestParseAirportNetwork(t *testing.T) {
	if got := parseAirportNetwork("Current Wi-Fi Network: Home 5G\n"); got != "Home 5G" {
		t.Fatalf("got %q", got)
	}
	if got := parseAirportNetwork("en1 is not associated with an AirPort network."); got != "" {
		t.Fatalf("got %q", got)
	}
	if got := parseAirportNetwork("** Error: Error obtaining wireless information."); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestWifiDeviceDarwin(t *testing.T) {
	out := "Hardware Port: Wi-Fi\nDevice: en1\nEthernet Address: 1c:f6:4c:4e:e6:67\n\nHardware Port: Ethernet\nDevice: en0\n"
	if got := wifiDeviceDarwin(out); got != "en1" {
		t.Fatalf("got %q", got)
	}
	if got := wifiDeviceDarwin("Hardware Port: Ethernet\nDevice: en0\n"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestParseNetshInterfaces(t *testing.T) {
	out := " There is no wireless interface on the system.\n\n"
	if got := parseNetshInterfaces(out); got != "" {
		t.Fatalf("got %q", got)
	}
	out = "    Name                   : Wi-Fi\n    SSID                   : Office Net\n    BSSID                  : aa:bb:cc:dd:ee:ff\n    State                  : connected\n"
	if got := parseNetshInterfaces(out); got != "Office Net" {
		t.Fatalf("got %q", got)
	}
	// A network whose name names another field stays whole.
	out = "    SSID                   : My BSSID Network\n    BSSID                  : aa:bb:cc:dd:ee:ff\n"
	if got := parseNetshInterfaces(out); got != "My BSSID Network" {
		t.Fatalf("got %q", got)
	}
}

func TestParseNmcliWiFi(t *testing.T) {
	out := "no:Home\nno:Office\nyes:Cafe \\: with colon\n"
	if got := parseNmcliWiFi(out); got != "Cafe : with colon" {
		t.Fatalf("got %q", got)
	}
	if got := parseNmcliWiFi("no:Home\n"); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestMatchWiFiRule(t *testing.T) {
	rules := []config.WiFiRule{{SSID: "Home"}, {SSID: "Office"}}
	if matchWiFiRule(nil, "Home") != nil {
		t.Fatal("no rules, no match")
	}
	if matchWiFiRule(rules, "") != nil {
		t.Fatal("no network, no match")
	}
	if r := matchWiFiRule(rules, "Office"); r == nil || r.SSID != "Office" {
		t.Fatalf("got %v", r)
	}
	if matchWiFiRule(rules, "Cafe") != nil {
		t.Fatal("matched a network no rule names")
	}
}
