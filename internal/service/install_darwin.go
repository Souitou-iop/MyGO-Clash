package service

import (
	"context"
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
	Unit       string // the launchd plist
	Home       string // the core's home
	Socket     string // the service's socket
	CoreSocket string // the core's socket, for the service alone
	Logs       string
}

// Layout returns the paths of the service of the app named slug.
func Layout(slug string) Paths {
	label := "io.mygo." + slug + ".service"
	return Paths{
		Label:      label,
		Bin:        "/Library/PrivilegedHelperTools/" + label,
		Unit:       "/Library/LaunchDaemons/" + label + ".plist",
		Home:       "/Library/Application Support/" + label,
		Socket:     "/var/run/" + label + ".sock",
		CoreSocket: "/var/run/" + label + "/core.sock",
		Logs:       "/Library/Logs/" + label,
	}
}

const plist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>service</string>
    <string>run</string>
    <string>--name</string><string>%s</string>
    <string>--owner</string><string>%s</string>
  </array>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>ProcessType</key><string>Interactive</string>
  <key>StandardOutPath</key><string>%s/service.log</string>
  <key>StandardErrorPath</key><string>%s/service.log</string>
</dict>
</plist>
`

// install installs and starts the service; it runs as root.
func install(cfg Config) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("installing the service needs root")
	}
	p := Layout(cfg.Slug)
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	_ = exec.Command("launchctl", "bootout", "system/"+p.Label).Run()
	if err := copyExecutable(exe, p.Bin); err != nil {
		return err
	}
	if err := os.MkdirAll(p.Logs, 0o755); err != nil {
		return err
	}
	unit := fmt.Sprintf(plist, xmlEscape(p.Label), xmlEscape(p.Bin), xmlEscape(cfg.Slug), xmlEscape(cfg.Owner), xmlEscape(p.Logs), xmlEscape(p.Logs))
	if err := os.WriteFile(p.Unit, []byte(unit), 0o644); err != nil {
		return err
	}
	if out, err := exec.Command("launchctl", "bootstrap", "system", p.Unit).CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl bootstrap: %v: %s", err, out)
	}
	_ = exec.Command("launchctl", "enable", "system/"+p.Label).Run()
	_ = exec.Command("launchctl", "kickstart", "-k", "system/"+p.Label).Run()
	return nil
}

// uninstall stops and removes the service; it runs as root.
func uninstall(slug string) error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("removing the service needs root")
	}
	p := Layout(slug)
	_ = exec.Command("launchctl", "bootout", "system/"+p.Label).Run()
	for _, path := range []string{p.Unit, p.Bin, p.Socket} {
		_ = os.Remove(path)
	}
	_ = os.RemoveAll(filepath.Dir(p.CoreSocket))
	_ = os.RemoveAll(p.Home)
	return nil
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// Elevate runs the app's executable as root with args, asking the user for
// an administrator's password in the system's dialog.
func Elevate(ctx context.Context, prompt string, args ...string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	parts := []string{shellQuote(exe)}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	command := strings.Join(parts, " ")
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	script := fmt.Sprintf(`do shell script "%s" with prompt "%s" with administrator privileges`, esc.Replace(command), esc.Replace(prompt))
	out, err := exec.CommandContext(ctx, "/usr/bin/osascript", "-e", script).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "-128") {
			return ErrCanceled
		}
		return fmt.Errorf("%v: %s", err, msg)
	}
	return nil
}

// CurrentOwner identifies the user running the app to the service.
func CurrentOwner() string { return strconv.Itoa(os.Getuid()) }
