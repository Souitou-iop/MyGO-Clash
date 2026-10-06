package sysproxy

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// DefaultBypass is what skips the proxy on Linux.
var DefaultBypass = []string{"localhost", "127.0.0.1", "::1", "192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12"}

// Linux has no system proxy, only what desktops set: GNOME (and desktops
// built on it) in GSettings, KDE in kioslaverc. Both are set when their
// tools exist.

func gsettings(args ...string) error {
	out, err := exec.Command("gsettings", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("gsettings %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func hasGnome() bool {
	_, err := exec.LookPath("gsettings")
	return err == nil
}

func kdeTool() string {
	if !strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "KDE") {
		return ""
	}
	for _, t := range []string{"kwriteconfig6", "kwriteconfig5"} {
		if _, err := exec.LookPath(t); err == nil {
			return t
		}
	}
	return ""
}

func kde(tool string, values map[string]string) error {
	for k, v := range values {
		if out, err := exec.Command(tool, "--file", "kioslaverc", "--group", "Proxy Settings", "--key", k, v).CombinedOutput(); err != nil {
			return fmt.Errorf("%s: %v: %s", tool, err, out)
		}
	}
	// Tell running KDE applications.
	_ = exec.Command("dbus-send", "--type=signal", "/KIO/Scheduler", "org.kde.KIO.Scheduler.reparseSlaveConfiguration", "string:").Run()
	return nil
}

func gnomeList(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = "'" + strings.ReplaceAll(s, "'", "") + "'"
	}
	return "[" + strings.Join(q, ", ") + "]"
}

func setManual(p Proxy) error {
	var errs []error
	applied := false
	port := strconv.Itoa(p.Port)
	if hasGnome() {
		applied = true
		for _, schema := range []string{"http", "https", "socks"} {
			errs = append(errs,
				gsettings("set", "org.gnome.system.proxy."+schema, "host", p.Host),
				gsettings("set", "org.gnome.system.proxy."+schema, "port", port))
		}
		errs = append(errs,
			gsettings("set", "org.gnome.system.proxy", "ignore-hosts", gnomeList(p.Bypass)),
			gsettings("set", "org.gnome.system.proxy", "mode", "manual"))
	}
	if tool := kdeTool(); tool != "" {
		applied = true
		addr := p.Host + " " + port
		errs = append(errs, kde(tool, map[string]string{
			"ProxyType": "1", "httpProxy": "http://" + addr, "httpsProxy": "http://" + addr,
			"socksProxy": "socks://" + addr, "NoProxyFor": strings.Join(p.Bypass, ","),
		}))
	}
	if !applied {
		return errors.New("this desktop has no proxy setting the app knows (GNOME or KDE); set http_proxy in your shell, or use TUN mode")
	}
	return errors.Join(errs...)
}

func setPAC(url string) error {
	var errs []error
	if hasGnome() {
		errs = append(errs, gsettings("set", "org.gnome.system.proxy", "autoconfig-url", url),
			gsettings("set", "org.gnome.system.proxy", "mode", "auto"))
	}
	if tool := kdeTool(); tool != "" {
		errs = append(errs, kde(tool, map[string]string{"ProxyType": "2", "Proxy Config Script": url}))
	}
	return errors.Join(errs...)
}

func setDirect() error {
	var errs []error
	if hasGnome() {
		errs = append(errs, gsettings("set", "org.gnome.system.proxy", "mode", "none"))
	}
	if tool := kdeTool(); tool != "" {
		errs = append(errs, kde(tool, map[string]string{"ProxyType": "0"}))
	}
	return errors.Join(errs...)
}

func get() (Proxy, error) {
	if !hasGnome() {
		return Proxy{}, errors.New("unknown desktop")
	}
	read := func(args ...string) string {
		out, _ := exec.Command("gsettings", append([]string{"get"}, args...)...).Output()
		return strings.Trim(strings.TrimSpace(string(out)), "'")
	}
	var p Proxy
	switch read("org.gnome.system.proxy", "mode") {
	case "manual":
		p.Enabled = true
		p.Host = read("org.gnome.system.proxy.http", "host")
		p.Port, _ = strconv.Atoi(read("org.gnome.system.proxy.http", "port"))
	case "auto":
		p.Enabled, p.PAC = true, read("org.gnome.system.proxy", "autoconfig-url")
	}
	return p, nil
}
