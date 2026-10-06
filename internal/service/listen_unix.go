//go:build !windows

package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
)

// coreSDDL is the security descriptor of the core's pipe (Windows only).
const coreSDDL = ""

// listen listens on the service's socket. Everyone may connect; authorize
// lets only the owner and root through.
func listen(path, owner string) (net.Listener, error) {
	if atoi(owner) < 0 {
		return nil, fmt.Errorf("invalid owner %q", owner)
	}
	_ = os.Remove(path) // root's own path, in a directory only root writes
	l, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o666); err != nil {
		l.Close()
		return nil, err
	}
	return l, nil
}

// connContext records the user on the other side of the connection.
func connContext(ctx context.Context, c net.Conn) context.Context {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return ctx
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return ctx
	}
	peer := Peer{}
	_ = raw.Control(func(fd uintptr) {
		if uid, err := peerUID(int(fd)); err == nil {
			peer = Peer{UID: uid, Known: true}
		}
	})
	return context.WithValue(ctx, peerKey{}, peer)
}

func allowed(ctx context.Context, owner string) error {
	p := peerOf(ctx)
	if !p.Known {
		return errors.New("the service cannot tell who is connecting")
	}
	if p.UID == 0 || p.UID == atoi(owner) {
		return nil
	}
	return fmt.Errorf("user %d may not use this service, which belongs to user %s", p.UID, owner)
}

func hideWindow(*exec.Cmd) {}
