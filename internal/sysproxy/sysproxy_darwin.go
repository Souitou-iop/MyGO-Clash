package sysproxy

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

// DefaultBypass is what skips the proxy on macOS.
var DefaultBypass = []string{
	"127.0.0.1", "192.168.0.0/16", "10.0.0.0/8", "172.16.0.0/12", "localhost", "*.local", "*.crashlytics.com", "<local>",
}

const networksetup = "/usr/sbin/networksetup"

// services lists the enabled network services (Wi-Fi, Ethernet, ...).
func services() ([]string, error) {
	out, err := exec.Command(networksetup, "-listallnetworkservices").Output()
	if err != nil {
		return nil, fmt.Errorf("networksetup: %w", err)
	}
	var list []string
	sc := bufio.NewScanner(bytes.NewReader(out))
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first { // "An asterisk (*) denotes that a network service is disabled."
			first = false
			continue
		}
		if line == "" || strings.HasPrefix(line, "*") {
			continue
		}
		list = append(list, line)
	}
	if len(list) == 0 {
		return nil, errors.New("no network service is enabled")
	}
	return list, nil
}

// forEach runs fn for every service at once and joins their errors.
func forEach(fn func(svc string) error) error {
	list, err := services()
	if err != nil {
		return err
	}
	var wg sync.WaitGroup
	errs := make([]error, len(list))
	for i, svc := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := fn(svc); err != nil {
				errs[i] = fmt.Errorf("%s: %w", svc, err)
			}
		}()
	}
	wg.Wait()
	return errors.Join(errs...)
}

func run(args ...string) error {
	out, err := exec.Command(networksetup, args...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if strings.Contains(msg, "requires admin") || strings.Contains(msg, "privileges") {
			return fmt.Errorf("macOS refused to change the proxy (%s); an administrator may need to allow it", msg)
		}
		return fmt.Errorf("%v: %s", err, msg)
	}
	return nil
}

func setManual(p Proxy) error {
	port := strconv.Itoa(p.Port)
	return forEach(func(svc string) error {
		steps := [][]string{
			{"-setautoproxystate", svc, "off"},
			{"-setwebproxy", svc, p.Host, port},
			{"-setsecurewebproxy", svc, p.Host, port},
			{"-setsocksfirewallproxy", svc, p.Host, port},
			append([]string{"-setproxybypassdomains", svc}, bypassOrEmpty(p.Bypass)...),
		}
		for _, s := range steps {
			if err := run(s...); err != nil {
				return err
			}
		}
		return nil
	})
}

func bypassOrEmpty(b []string) []string {
	if len(b) == 0 {
		return []string{"Empty"}
	}
	return b
}

func setPAC(url string) error {
	return forEach(func(svc string) error {
		for _, s := range [][]string{
			{"-setwebproxystate", svc, "off"},
			{"-setsecurewebproxystate", svc, "off"},
			{"-setsocksfirewallproxystate", svc, "off"},
			{"-setautoproxyurl", svc, url},
			{"-setautoproxystate", svc, "on"},
		} {
			if err := run(s...); err != nil {
				return err
			}
		}
		return nil
	})
}

func setDirect() error {
	return forEach(func(svc string) error {
		for _, s := range [][]string{
			{"-setwebproxystate", svc, "off"},
			{"-setsecurewebproxystate", svc, "off"},
			{"-setsocksfirewallproxystate", svc, "off"},
			{"-setautoproxystate", svc, "off"},
		} {
			if err := run(s...); err != nil {
				return err
			}
		}
		return nil
	})
}

// get reads the proxy the system uses now, with scutil, which reports the
// settings of the primary service.
func get() (Proxy, error) {
	out, err := exec.Command("/usr/sbin/scutil", "--proxy").Output()
	if err != nil {
		return Proxy{}, err
	}
	kv := map[string]string{}
	var exceptions []string
	inExceptions := false
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "ExceptionsList") {
			inExceptions = true
			continue
		}
		if inExceptions {
			if line == "}" {
				inExceptions = false
				continue
			}
			if _, v, ok := strings.Cut(line, " : "); ok {
				exceptions = append(exceptions, v)
			}
			continue
		}
		if k, v, ok := strings.Cut(line, " : "); ok {
			kv[k] = v
		}
	}
	p := Proxy{Bypass: exceptions}
	if kv["ProxyAutoConfigEnable"] == "1" {
		p.Enabled, p.PAC = true, kv["ProxyAutoConfigURLString"]
		return p, nil
	}
	if kv["HTTPEnable"] == "1" {
		p.Enabled, p.Host = true, kv["HTTPProxy"]
		p.Port, _ = strconv.Atoi(kv["HTTPPort"])
	} else if kv["SOCKSEnable"] == "1" {
		p.Enabled, p.Host = true, kv["SOCKSProxy"]
		p.Port, _ = strconv.Atoi(kv["SOCKSPort"])
	}
	return p, nil
}
