//go:build !windows

package coreapi

import (
	"context"
	"net"
)

// Dial connects to a core's socket.
func Dial(ctx context.Context, addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", addr)
}
