package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Paths are where the service lives.
type Paths struct {
	Label      string
	Bin        string
	Unit       string // the systemd unit
	Home       string // the core's home
	Socket     string // the service's socket
	CoreSocket string // the core's socket, for the service alone
	Logs       string
}

// Layout returns the paths of the service of the app named slug.
func Layout(slug string) Paths {
	name := slug + "-service"
	return Paths{
		Label:      name,
		Bin:        "/usr/local/lib/" + slug + "/" + name,
		Unit:       "/etc/systemd/system/" + name + ".service",
		Home:       "/var/lib/" + name,
		Socket:     "/run/" + name + ".sock",
		CoreSocket: "/run/" + name + "/core.sock",
		Logs:       "/var/log/" + name,
	}
}

const unitTemplate = `[Unit]
Description=MyGO-Clash service (TUN mode)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart="%s" service run --name "%s" --owner "%s"
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576
%s
[Install]
WantedBy=multi-user.target
`

// unitFile is the service's systemd unit; seCtx is its SELinux context, or
// "" for none (see selinuxContext).
func unitFile(p Paths, cfg Config, seCtx string) string {
	line := ""
	if seCtx != "" {
		line = "SELinuxContext=" + seCtx + "\n"
	}
	return fmt.Sprintf(unitTemplate, p.Bin, cfg.Slug, cfg.Owner, line)
}

// seContext is the SELinux domain the service runs in. systemd moves a
// program it starts into it by itself only when the program is labeled
// bin_t, and the service's executable in /usr/local/lib is labeled lib_t:
// it would stay in systemd's own init_t, which may neither connect to
// the network nor open /dev/net/tun. A label given to the file would not
// last: the service's updates replace the file.
const seContext = "system_u:system_r:unconfined_service_t:s0"

// selinuxContext returns seContext where SELinux is on and its policy has
// that domain, and "" otherwise, since systemd refuses to start a unit
// whose context the policy does not know. The kernel checks the context
// when it is written to selinuxfs, as libselinux's security_check_context
// does.
func selinuxContext() string {
	f, err := os.OpenFile("/sys/fs/selinux/context", os.O_RDWR, 0)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Write([]byte(seContext + "\x00")); err != nil {
		return ""
	}
	return seContext
}

// fixUnit gives the unit of a service installed before it had a SELinux
// context that context, and restarts the service in it. It does so only
// when the service runs in init_t, where it cannot work; the service
// reaches this version through its own update, without the user
// installing it again.
func fixUnit(cfg Config) {
	cur, err := os.ReadFile("/proc/self/attr/current")
	if err != nil || !strings.Contains(string(cur), ":init_t:") {
		return
	}
	seCtx := selinuxContext()
	if seCtx == "" {
		return
	}
	p := Layout(cfg.Slug)
	old, err := os.ReadFile(p.Unit)
	if err != nil || strings.Contains(string(old), "SELinuxContext=") {
		return
	}
	if err := os.WriteFile(p.Unit, []byte(unitFile(p, cfg, seCtx)), 0o644); err != nil {
		log.Printf("selinux: write the unit: %v", err)
		return
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		log.Printf("selinux: systemctl daemon-reload: %v: %s", err, out)
		_ = os.WriteFile(p.Unit, old, 0o644)
		return
	}
	log.Printf("selinux: restarting in %s", seCtx)
	os.Exit(exitRestart)
}

func install(cfg Config) error {
	if os.Geteuid() != 0 {
		return errors.New("installing the service needs root")
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		return errors.New("the service needs systemd")
	}
	p := Layout(cfg.Slug)
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = exec.Command("systemctl", "stop", p.Label).Run()
	if err := copyExecutable(exe, p.Bin); err != nil {
		return err
	}
	if err := os.WriteFile(p.Unit, []byte(unitFile(p, cfg, selinuxContext())), 0o644); err != nil {
		return err
	}
	for _, args := range [][]string{{"daemon-reload"}, {"enable", "--now", p.Label}, {"restart", p.Label}} {
		if out, err := exec.Command("systemctl", args...).CombinedOutput(); err != nil {
			return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	return nil
}

func uninstall(slug string) error {
	if os.Geteuid() != 0 {
		return errors.New("removing the service needs root")
	}
	p := Layout(slug)
	_ = exec.Command("systemctl", "disable", "--now", p.Label).Run()
	_ = os.Remove(p.Unit)
	_ = exec.Command("systemctl", "daemon-reload").Run()
	_ = os.Remove(p.Bin)
	_ = os.Remove(filepath.Dir(p.Bin))
	_ = os.Remove(p.Socket)
	_ = os.RemoveAll(filepath.Dir(p.CoreSocket))
	_ = os.RemoveAll(p.Home)
	return nil
}

// Elevate runs the app's executable as root with args through polkit,
// which asks the user for a password.
func Elevate(ctx context.Context, prompt string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	pkexec, err := exec.LookPath("pkexec")
	if err != nil {
		return fmt.Errorf("pkexec is not installed; run this as root instead: sudo %q %s", exe, strings.Join(args, " "))
	}
	out, err := exec.CommandContext(ctx, pkexec, append([]string{exe}, args...)...).CombinedOutput()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) && (ee.ExitCode() == 126 || ee.ExitCode() == 127) {
			return ErrCanceled
		}
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CurrentOwner identifies the user running the app to the service.
func CurrentOwner() string { return strconv.Itoa(os.Getuid()) }
