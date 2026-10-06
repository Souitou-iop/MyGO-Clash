//go:build windows

package service

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"syscall"

	"github.com/metacubex/wireguard-go/ipc/namedpipe"
	"golang.org/x/sys/windows"
)

// coreSDDL lets only SYSTEM and Administrators reach the core's pipe; the
// app reaches the core through the service.
const coreSDDL = "D:PAI(A;OICI;GWGR;;;SY)(A;OICI;GWGR;;;BA)"

var sidRe = regexp.MustCompile(`^S-1-[0-9]+(-[0-9]+)+$`)

// listen listens on the service's pipe, which only the owner, SYSTEM and
// Administrators may open.
func listen(path, owner string) (net.Listener, error) {
	if !sidRe.MatchString(owner) {
		return nil, fmt.Errorf("invalid owner %q", owner)
	}
	sd, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:PAI(A;OICI;GWGR;;;%s)(A;OICI;GWGR;;;SY)(A;OICI;GWGR;;;BA)", owner))
	if err != nil {
		return nil, err
	}
	return (&namedpipe.ListenConfig{SecurityDescriptor: sd}).Listen(path)
}

func connContext(ctx context.Context, _ net.Conn) context.Context { return ctx }

// allowed lets everyone the pipe let in.
func allowed(context.Context, string) error { return nil }

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
