package service

import (
	"strings"
	"testing"
)

func TestUnitFile(t *testing.T) {
	p := Layout("mygo-clash")
	cfg := Config{Slug: "mygo-clash", Owner: "1000"}
	u := unitFile(cfg, p.Bin)
	want := `ExecStart="/usr/local/sbin/mygo-clash/mygo-clash-service" service run --name "mygo-clash" --owner "1000"`
	if !strings.Contains(u, want) {
		t.Fatalf("ExecStart:\n%s", u)
	}
	env := unitFile(cfg, envBin, oldBin("mygo-clash"))
	if !strings.Contains(env, `ExecStart="/usr/bin/env" "/usr/local/lib/mygo-clash/mygo-clash-service" service run`) {
		t.Fatalf("ExecStart through env:\n%s", env)
	}
	if p.Bin == oldBin("mygo-clash") {
		t.Fatal("the executable is still where SELinux labels it lib_t")
	}
}
