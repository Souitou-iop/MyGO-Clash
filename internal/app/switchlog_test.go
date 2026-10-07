package app

import (
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/config"
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
