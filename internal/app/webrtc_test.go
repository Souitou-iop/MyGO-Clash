package app

import (
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/webrtc"
)

func TestWebRTCHandling(t *testing.T) {
	st := config.Defaults()
	cases := []struct {
		guard, proxy, tun, tunAvailable bool
		want                            webrtc.Handling
	}{
		{false, true, true, true, webrtc.Unset},
		{true, false, false, true, webrtc.Unset},
		{true, true, false, true, webrtc.ProxiedOnly},
		{true, false, true, true, webrtc.PublicOnly},
		{true, true, true, true, webrtc.PublicOnly},
		// TUN waiting for the service: the system proxy is what runs.
		{true, true, true, false, webrtc.ProxiedOnly},
		{true, false, true, false, webrtc.Unset},
	}
	for _, c := range cases {
		st.WebRTCGuard, st.SystemProxy.Enabled, st.Tun.Enabled = c.guard, c.proxy, c.tun
		if got := webrtcHandling(st, c.tunAvailable); got != c.want {
			t.Errorf("%+v: got %q", c, got)
		}
	}
}
