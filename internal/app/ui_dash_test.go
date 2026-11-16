package app

import "testing"

func TestFlagOf(t *testing.T) {
	tests := []struct{ name, want string }{
		{"🇯🇵 Tokyo", "JP"},
		{"JP 01", "JP"},
		{"tokyo-hysteria2", "JP"},
		{"us-hysteria2", "US"},
		{"Tokyo", "JP"},
		{"Osaka", "JP"},
		{"Hong Kong 01", "HK"},
		{"hongkong-vless", "HK"},
		{"Los Angeles", "US"},
		{"San Jose 01", "US"},
		{"New York", "US"},
		{"NRT 01", "JP"},
		{"JPN-02", "JP"},
		{"HKG-01", "HK"},
		{"SJC", "US"},
		{"GR 01", "GR"},
		{"东京 01", "JP"},
		{"圣何塞", "US"},
		{"UK London", "GB"},
		{"Frankfurt", "DE"},
		{"Singapore", "SG"},
		{"Seoul", "KR"},
		{"Taipei", "TW"},
		{"Sydney", "AU"},
		{"Toronto", "CA"},
		{"Dubai", "AE"},
		{"Node 01", ""},
		{"Auto", ""},
		{"DIRECT", ""},
	}
	for _, c := range tests {
		if got := flagOf(c.name); got != c.want {
			t.Errorf("flagOf(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}
