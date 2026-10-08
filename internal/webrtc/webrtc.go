// Package webrtc keeps WebRTC in Chromium browsers from showing the address
// behind the proxy: a page can learn it from the STUN requests WebRTC sends
// outside the proxy, whatever the system proxy says. On Windows it sets the
// browsers' own policy for the user, which they apply at once, without an
// administrator; Firefox has no such policy per user.
package webrtc

// Handling is what the policy lets WebRTC use (RFC 8828).
type Handling string

const (
	// Unset removes the policy the app set.
	Unset Handling = ""
	// PublicOnly keeps WebRTC to the interface of the default route:
	// in TUN mode, the tunnel, so UDP still flows, through the proxy.
	PublicOnly Handling = "default_public_interface_only"
	// ProxiedOnly keeps WebRTC from UDP the proxy does not carry: with a
	// system proxy, it falls back to TCP through the proxy.
	ProxiedOnly Handling = "disable_non_proxied_udp"
)
