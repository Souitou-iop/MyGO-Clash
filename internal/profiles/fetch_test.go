package profiles

import (
	"strings"
	"testing"
)

func TestNormalizeVmessShareLink(t *testing.T) {
	const link = "vmess://eyJ2IjoiMiIsInBzIjoiXHU3ZjhlXHU1NmZkMTY4LjE0My4xMDIuMTgwIiwiYWRkIjoiNjY2Ni41OTk5LnBybyIsInBvcnQiOiI0MDE4MCIsImlkIjoiMTFiMWM4MzAtMTE5OS00OTIwLWI3ZmItOTFkYzk3ZDU3MzkzIiwiYWlkIjoiMCIsIm5ldCI6InRjcCIsInR5cGUiOiJub25lIiwiaG9zdCI6IiIsInBhdGgiOiIiLCJ0bHMiOiIifQ=="

	content, converted, err := Normalize([]byte(link))
	if err != nil {
		t.Fatalf("Normalize returned an error: %v", err)
	}
	if !converted {
		t.Fatal("Normalize did not mark the share link as converted")
	}
	text := string(content)
	for _, want := range []string{"proxies:", "6666.5999.pro", "40180", "11b1c830-1199-4920-b7fb-91dc97d57393"} {
		if !strings.Contains(text, want) {
			t.Fatalf("converted profile does not contain %q:\n%s", want, text)
		}
	}
}

func TestSkippedLinks(t *testing.T) {
	list := strings.Join([]string{
		"trojan://secret@example.com:443#ok",
		"wireguard://key@example.com:51820#unsupported",
		"not a link",
		"",
	}, "\n")
	skipped := SkippedLinks([]byte(list))
	if len(skipped) != 2 {
		t.Fatalf("SkippedLinks returned %q, want the wireguard link and the text", skipped)
	}
	if _, converted, err := Normalize([]byte(list)); err != nil || !converted {
		t.Fatalf("Normalize = converted %v, error %v; want the trojan link converted", converted, err)
	}
}
