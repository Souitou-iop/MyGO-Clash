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
//
// The executable is in /usr/local/sbin, whose files SELinux labels bin_t
// by default: systemd starts a bin_t program in unconfined_service_t,
// where the service may reach the network and open /dev/net/tun. Its
// updates are written in its directory and so are labeled bin_t too.
func Layout(slug string) Paths {
	name := slug + "-service"
	return Paths{
		Label:      name,
		Bin:        "/usr/local/sbin/" + slug + "/" + name,
		Unit:       "/etc/systemd/system/" + name + ".service",
		Home:       "/var/lib/" + name,
		Socket:     "/run/" + name + ".sock",
		CoreSocket: "/run/" + name + "/core.sock",
		Logs:       "/var/log/" + name,
	}
}

// oldBin is where versions up to 1.0.0 put the executable. SELinux labels
// it lib_t, which systemd starts in its own init_t, where the service may
// neither connect to the network nor open /dev/net/tun.
func oldBin(slug string) string {
	return "/usr/local/lib/" + slug + "/" + slug + "-service"
}

// removeOldBin removes the executable at oldBin and its directory.
func removeOldBin(slug string) {
	old := oldBin(slug)
	_ = os.Remove(old)
	_ = os.Remove(old + ".new")
	_ = os.Remove(filepath.Dir(old))
}

const unitTemplate = `[Unit]
Description=MyGO-Clash service (TUN mode)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=%s service run --name "%s" --owner "%s"
Restart=on-failure
RestartSec=2
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
`

// unitFile is the service's systemd unit, which starts the service with
// cmd: its executable, after what runs it if anything does.
func unitFile(cfg Config, cmd ...string) string {
	q := make([]string, len(cmd))
	for i, w := range cmd {
		q[i] = `"` + w + `"`
	}
	return fmt.Sprintf(unitTemplate, strings.Join(q, " "), cfg.Slug, cfg.Owner)
}

// envBin starts the service through env during a relocation (see
// relocate): env is labeled bin_t, so systemd starts it, and the service
// it runs, in unconfined_service_t.
const envBin = "/usr/bin/env"

// sameFile reports whether a and b name the same file, through links:
// /usr/local/sbin is a link to bin on Fedora.
func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}

// relocate moves a service that runs from oldBin, which its own update
// brings to this version, to Layout's Bin: it copies itself there,
// points the unit at the copy and exits for systemd to start that.
//
// Under SELinux the service at oldBin runs in init_t, which may not create
// files labeled bin_t: it then points the unit at itself through env,
// which brings it back in unconfined_service_t, where the copy succeeds.
// Should a step fail it stays where it is, as before.
func relocate(cfg Config) {
	p := Layout(cfg.Slug)
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if sameFile(exe, p.Bin) {
		removeOldBin(cfg.Slug)
		return
	}
	if !sameFile(exe, oldBin(cfg.Slug)) {
		return
	}
	old, err := os.ReadFile(p.Unit)
	if err != nil {
		return
	}
	unit := unitFile(cfg, p.Bin)
	if err := copyExecutable(exe, p.Bin); err != nil {
		throughEnv := unitFile(cfg, envBin, exe)
		if string(old) == throughEnv {
			log.Printf("relocate: copy to %s: %v", p.Bin, err)
			return
		}
		log.Printf("relocate: copy to %s: %v; restarting through %s", p.Bin, err, envBin)
		unit = throughEnv
	}
	if err := os.WriteFile(p.Unit, []byte(unit), 0o644); err != nil {
		log.Printf("relocate: write the unit: %v", err)
		_ = os.Remove(p.Bin)
		return
	}
	if out, err := exec.Command("systemctl", "daemon-reload").CombinedOutput(); err != nil {
		log.Printf("relocate: systemctl daemon-reload: %v: %s", err, out)
		_ = os.WriteFile(p.Unit, old, 0o644)
		_ = os.Remove(p.Bin)
		return
	}
	log.Printf("relocate: restarting")
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
	removeOldBin(cfg.Slug)
	if err := os.WriteFile(p.Unit, []byte(unitFile(cfg, p.Bin)), 0o644); err != nil {
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
	removeOldBin(slug)
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
