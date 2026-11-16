package corehost

import (
	"os/exec"
	"strings"
	"sync"

	"github.com/metacubex/mihomo/listener"
	"github.com/metacubex/mihomo/log"
)

// tunTrust lets TUN's interface through firewalld while TUN is on.
//
// With the system and mixed stacks the kernel hands the connections that
// come out of TUN to the core's listener on the interface's address, and
// firewalld's default zones (Fedora's among them) refuse new connections
// on an interface they do not know: every TCP connection through TUN was
// refused before it reached the core. The interface goes to the trusted
// zone at runtime only, so nothing outlives the core's run or a reboot.
type tunTrust struct {
	mu    sync.Mutex
	iface string // the interface in the trusted zone, "" for none
}

// sync puts TUN's interface, if TUN is on, in the trusted zone, and takes
// out the one put there before if it changed.
func (t *tunTrust) sync() {
	if !isPrivileged() {
		return
	}
	want := ""
	if conf := listener.GetTunConf(); conf.Enable {
		want = conf.Device
	}
	t.set(want)
}

// release takes TUN's interface out of the trusted zone.
func (t *tunTrust) release() { t.set("") }

func (t *tunTrust) set(want string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if want == t.iface || !firewalldRunning() {
		return
	}
	if t.iface != "" {
		if out, err := firewallCmd("--zone=trusted", "--remove-interface="+t.iface); err != nil {
			log.Warnln("[TUN] firewalld: could not take %s out of the trusted zone: %v: %s", t.iface, err, out)
		}
		t.iface = ""
	}
	if want == "" {
		return
	}
	if out, err := firewallCmd("--zone=trusted", "--change-interface="+want); err != nil {
		log.Warnln("[TUN] firewalld: could not trust %s, connections through TUN may be refused: %v: %s", want, err, out)
		return
	}
	t.iface = want
	log.Infoln("[TUN] firewalld: %s is in the trusted zone while TUN is on", want)
}

func firewalldRunning() bool {
	_, err := firewallCmd("--state")
	return err == nil
}

func firewallCmd(args ...string) (string, error) {
	out, err := exec.Command("firewall-cmd", args...).CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
