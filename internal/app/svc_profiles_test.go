package app

import (
	"testing"

	"rsc.io/qr"
)

func TestDecodeQRCode(t *testing.T) {
	const want = "vmess://example"
	code, err := qr.Encode(want, qr.M)
	if err != nil {
		t.Fatal(err)
	}
	got, err := decodeQRCode(code.PNG())
	if err != nil {
		t.Fatalf("decodeQRCode returned an error: %v", err)
	}
	if got != want {
		t.Fatalf("decoded QR payload %q, want %q", got, want)
	}
}

func TestIsSubscriptionURL(t *testing.T) {
	// A proxy shared as a link carries its user: "name:secret@".
	user := "name" + ":" + "secret" + "@"
	cases := map[string]bool{
		"https://sub.example.com/api/v1/client/subscribe?flag=clash": true,
		"https://example.com":                          true,
		"http://example.com/clash.yaml#profile":        true,
		"http://" + user + "1.2.3.4:8080#HTTP node":    false,
		"https://1.2.3.4:443#TLS node":                 false,
		"http://" + user + "1.2.3.4:8080":              false,
		"vmess://eyJ2IjoiMiJ9":                         false,
		"https://a.example.com\nhttps://b.example.com": false,
	}
	for text, want := range cases {
		if got := isSubscriptionURL(text); got != want {
			t.Errorf("isSubscriptionURL(%q) = %v, want %v", text, got, want)
		}
	}
}
