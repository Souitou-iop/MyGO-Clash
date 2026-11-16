package service

import (
	"strings"
	"testing"
)

func TestUnitFile(t *testing.T) {
	p := Layout("mygo-clash")
	cfg := Config{Slug: "mygo-clash", Owner: "1000"}

	plain := unitFile(p, cfg, "")
	if strings.Contains(plain, "SELinuxContext") {
		t.Fatalf("a unit without SELinux has a context:\n%s", plain)
	}
	if !strings.Contains(plain, "LimitNOFILE=1048576\n\n[Install]") {
		t.Fatalf("the unit without SELinux changed:\n%s", plain)
	}

	se := unitFile(p, cfg, seContext)
	want := "LimitNOFILE=1048576\nSELinuxContext=" + seContext + "\n\n[Install]"
	if !strings.Contains(se, want) {
		t.Fatalf("the context is not in [Service]:\n%s", se)
	}
	if !strings.Contains(se, `ExecStart="/usr/local/lib/mygo-clash/mygo-clash-service" service run --name "mygo-clash" --owner "1000"`) {
		t.Fatalf("ExecStart:\n%s", se)
	}
}
