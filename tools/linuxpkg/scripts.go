package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// The maintainer scripts of the packages. They do what the packages of
// clash-verge-rev do for its service: remove the app's systemd service when
// the package is removed (not when it is upgraded), restart it on the new
// version when an upgrade finds it running, and refresh the desktop and icon
// databases. scripts/lib.sh holds the shared functions; each format has only
// the glue between its own hooks and them.

//go:embed scripts/lib.sh
var libScript string

// nameRe is what the package name may be, as it goes into shell scripts.
var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9+-]*$`)

// script returns lib.sh for the app called name, followed by body.
func script(name, body string) (string, error) {
	if !nameRe.MatchString(name) {
		return "", fmt.Errorf("package name %q cannot go into a script", name)
	}
	return strings.ReplaceAll(libScript, "@NAME@", name) + "\n" + body, nil
}

// debScripts returns the scripts of a Debian package, by file name. dpkg runs
// postinst with "configure" and, on an upgrade, the old version; prerm and
// postrm with "remove", "purge", "upgrade" and others.
func debScripts(name string) (map[string]string, error) {
	bodies := map[string]string{
		"postinst": `case "$1" in
  configure)
    fix_permissions
    refresh_caches
    if [ -n "${2:-}" ]; then
      service_refresh
    fi
    ;;
esac
exit 0
`,
		"prerm": `case "$1" in
  remove | deconfigure)
    service_remove
    ;;
esac
exit 0
`,
		"postrm": `case "$1" in
  remove | purge)
    service_cleanup
    ;;
esac
refresh_caches
exit 0
`,
	}
	return withShebang(name, bodies)
}

// rpmScripts returns the scriptlets of an RPM package, by hook. rpm passes
// the number of installed versions of the package after the transaction
// step: 1 for a first install and 2 or more for an upgrade in %post, 0 for a
// removal and 1 or more for an upgrade in %preun and %postun.
func rpmScripts(name string) (map[string]string, error) {
	return withShebang(name, map[string]string{
		"post": `fix_permissions
refresh_caches
if [ "${1:-1}" -ge 2 ]; then
  service_refresh
fi
exit 0
`,
		"preun": `if [ "${1:-1}" -eq 0 ]; then
  service_remove
fi
exit 0
`,
		"postun": `if [ "${1:-1}" -eq 0 ]; then
  service_cleanup
fi
refresh_caches
exit 0
`,
	})
}

// archScripts returns the hooks of an Arch package's .INSTALL, by name. nFPM
// wraps each in a function of the same name, which pacman calls in bash; a
// function body cannot exit, and needs no shebang.
func archScripts(name string) (map[string]string, error) {
	out := map[string]string{}
	for hook, body := range map[string]string{
		"post_install": "fix_permissions\nrefresh_caches\n",
		"post_upgrade": "fix_permissions\nrefresh_caches\nservice_refresh\n",
		"pre_remove":   "service_remove\n",
		"post_remove":  "service_cleanup\nrefresh_caches\n",
	} {
		s, err := script(name, body)
		if err != nil {
			return nil, err
		}
		out[hook] = s
	}
	return out, nil
}

func withShebang(name string, bodies map[string]string) (map[string]string, error) {
	out := map[string]string{}
	for hook, body := range bodies {
		s, err := script(name, body)
		if err != nil {
			return nil, err
		}
		out[hook] = "#!/bin/sh\n" + s
	}
	return out, nil
}

// writeScripts writes scripts into dir, one file each, and returns the path
// of every hook.
func writeScripts(dir string, scripts map[string]string) (map[string]string, error) {
	paths := map[string]string{}
	for hook, s := range scripts {
		p := filepath.Join(dir, hook)
		if err := os.WriteFile(p, []byte(s), 0o755); err != nil {
			return nil, err
		}
		paths[hook] = p
	}
	return paths, nil
}

// addDebScripts rewrites the Debian package at path with scripts (postinst,
// prerm, postrm) in its control archive. mygo makes Debian packages without
// maintainer scripts and cannot be given any, so they are added here; the
// rest of the package stays as it is.
func addDebScripts(path string, scripts map[string]string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	const magic = "!<arch>\n"
	if !strings.HasPrefix(string(b), magic) {
		return errors.New("not an ar archive")
	}
	var out bytes.Buffer
	out.WriteString(magic)
	replaced := false
	for rest := b[len(magic):]; len(rest) > 0; {
		if len(rest) < 60 {
			return errors.New("truncated ar header")
		}
		hdr := rest[:60]
		member := strings.TrimSuffix(strings.TrimSpace(string(hdr[:16])), "/")
		size, err := strconv.Atoi(strings.TrimSpace(string(hdr[48:58])))
		if err != nil || size < 0 || 60+size > len(rest) {
			return fmt.Errorf("member %s: bad size", member)
		}
		body := rest[60 : 60+size]
		advance := 60 + size + size%2
		if advance > len(rest) {
			advance = len(rest)
		}
		rest = rest[advance:]
		if member == "control.tar.gz" {
			if body, err = controlWithScripts(body, scripts); err != nil {
				return err
			}
			replaced = true
		}
		// Keep the member's header, with its new size.
		fmt.Fprintf(&out, "%s%-10d`\n", string(hdr[:48]), len(body))
		out.Write(body)
		if len(body)%2 == 1 {
			out.WriteByte('\n')
		}
	}
	if !replaced {
		return errors.New("no control.tar.gz")
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

// controlWithScripts returns the control.tar.gz in gz with scripts added.
func controlWithScripts(gz []byte, scripts map[string]string) ([]byte, error) {
	zr, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	tw := tar.NewWriter(zw)
	tr := tar.NewReader(zr)
	var mtime time.Time
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		if _, ok := scripts[strings.TrimPrefix(h.Name, "./")]; ok {
			continue // replaced below
		}
		if mtime.IsZero() {
			mtime = h.ModTime
		}
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if _, err := io.Copy(tw, tr); err != nil {
			return nil, err
		}
	}
	for _, hook := range []string{"postinst", "prerm", "postrm"} {
		s, ok := scripts[hook]
		if !ok {
			continue
		}
		h := &tar.Header{Typeflag: tar.TypeReg, Name: "./" + hook, Mode: 0o755, Size: int64(len(s)), Uname: "root", Gname: "root"}
		h.ModTime = mtime
		if err := tw.WriteHeader(h); err != nil {
			return nil, err
		}
		if _, err := io.WriteString(tw, s); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
