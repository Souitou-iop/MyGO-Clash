//go:build windows

package coreapi

import (
	"context"
	"net"
	"strings"

	"github.com/metacubex/wireguard-go/ipc/namedpipe"
)

// Dial connects to a core's named pipe, or to a Unix socket path.
func Dial(ctx context.Context, addr string) (net.Conn, error) {
	if strings.HasPrefix(addr, `\\.\pipe\`) {
		return namedpipe.DialContext(ctx, addr)
	}
	var d net.Dialer
	return d.DialContext(ctx, "unix", addr)
}
