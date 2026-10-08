package app

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/sysproxy"
)

func TestLogSwitches(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)
	old := config.Defaults()
	cur := old
	cur.Tun.Enabled = !old.Tun.Enabled
	cur.Clash.Mode = "global"
	logSwitches(from(context.Background(), "tray"), old, cur)
	got := buf.String()
	for _, want := range []string{"switch: TUN " + onOff(cur.Tun.Enabled) + " (tray)", "switch: mode rule → global (tray)"} {
		if !strings.Contains(got, want) {
			t.Errorf("log lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "system proxy") {
		t.Errorf("logged a switch that did not change:\n%s", got)
	}
	buf.Reset()
	logSwitches(context.Background(), cur, old)
	if !strings.Contains(buf.String(), "(app)") {
		t.Errorf("no source: %s", buf.String())
	}
}

func TestStaleProxyIsOurs(t *testing.T) {
	st := config.Defaults()
	st.Clash.MixedPort = 7897
	for _, c := range []struct {
		sys  sysproxy.Proxy
		want bool
	}{
		{sysproxy.Proxy{Enabled: true, Host: "127.0.0.1", Port: 7897}, true},
		{sysproxy.Proxy{Enabled: true, Host: st.SystemProxy.Host, Port: 7897}, true},
		{sysproxy.Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080}, false},
		{sysproxy.Proxy{Enabled: true, Host: "10.0.0.2", Port: 7897}, false},
		{sysproxy.Proxy{Enabled: true, PAC: "http://127.0.0.1:33331/commands/pac"}, true},
		{sysproxy.Proxy{Enabled: true, PAC: "http://corp.example/proxy.pac"}, false},
	} {
		if got := ours(c.sys, st); got != c.want {
			t.Errorf("ours(%+v) = %v, want %v", c.sys, got, c.want)
		}
	}
}
