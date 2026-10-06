package service

import (
	"context"
	"errors"
	"fmt"
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

[Install]
WantedBy=multi-user.target
`

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
	unit := fmt.Sprintf(unitTemplate, p.Bin, cfg.Slug, cfg.Owner)
	if err := os.WriteFile(p.Unit, []byte(unit), 0o644); err != nil {
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
