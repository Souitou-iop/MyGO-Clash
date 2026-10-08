// Linuxpkg packages the Linux builds of `mygo build` for more
// distributions. Next to the Debian package in each build directory, such as
// build/linux-amd64, it writes an RPM package (Fedora, openSUSE and the
// like), an Arch Linux package and an AppImage:
//
//	go run -C tools/linuxpkg . "$PWD/build/linux-amd64" "$PWD/build/linux-arm64"
//
// All of the packages get maintainer scripts (scripts.go): mygo's Debian
// package has none, so they are added to it. They remove the app's systemd
// service with the package, restart it on an upgrade, and refresh the
// desktop and icon caches.
//
// The RPM and Arch packages hold the files of the Debian package: the app in
// /opt, its command in /usr/bin, its desktop entry and icons. The AppImage
// holds the app's directory as the build left it. It needs mksquashfs
// (squashfs-tools), and downloads the AppImage runtime, whose checksum it
// knows, once.
//
// It is a module of its own so that the app's module does not take on the
// dependencies of nFPM.
package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goreleaser/nfpm/v2"
	_ "github.com/goreleaser/nfpm/v2/arch"
	"github.com/goreleaser/nfpm/v2/files"
	_ "github.com/goreleaser/nfpm/v2/rpm"
)

const (
	homepage = "https://github.com/Souitou-iop/MyGO-Clash"
	license  = "GPL-3.0-or-later"
)

// depends lists what the app loads at run time, by format: GTK, WebKitGTK
// and the AppIndicator library of the tray. RPM names them by the shared
// libraries, which Fedora and openSUSE package under different names.
var depends = map[string][]string{
	"rpm":       {"libgtk-3.so.0()(64bit)", "libwebkit2gtk-4.1.so.0()(64bit)", "libayatana-appindicator3.so.1()(64bit)"},
	"archlinux": {"gtk3", "webkit2gtk-4.1", "libayatana-appindicator"},
}

// The AppImage runtime, from https://github.com/AppImage/type2-runtime.
const runtimeRelease = "20251108"

var runtimeSHA256 = map[string]string{
	"x86_64":  "2fca8b443c92510f1483a883f60061ad09b46b978b2631c807cd873a47ec260d",
	"aarch64": "00cbdfcf917cc6c0ff6d3347d59e0ca1f7f45a6df1a428a0d6d8a78664d87444",
}

var appImageArch = map[string]string{"amd64": "x86_64", "arm64": "aarch64"}

func main() {
	log.SetFlags(0)
	log.SetPrefix("linuxpkg: ")
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: linuxpkg BUILD-DIR...")
		os.Exit(2)
	}
	for _, dir := range os.Args[1:] {
		if err := packageDir(dir); err != nil {
			log.Fatalf("%s: %v", dir, err)
		}
	}
}

// packageDir writes the packages of the build in dir.
func packageDir(dir string) error {
	debs, err := filepath.Glob(filepath.Join(dir, "*.deb"))
	if err != nil {
		return err
	}
	if len(debs) != 1 {
		return fmt.Errorf("want one Debian package, found %d", len(debs))
	}
	tmp, err := os.MkdirTemp("", "linuxpkg")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	root := filepath.Join(tmp, "root")
	control, err := readDeb(debs[0], root)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(debs[0]), err)
	}
	name, goarch := control["Package"], control["Architecture"]
	// Debian sorts a pre-release before its release with "~"; semantic
	// versions, which nFPM takes, with "-".
	version := strings.Replace(control["Version"], "~", "-", 1)
	contents, err := contentsOf(root)
	if err != nil {
		return err
	}
	description, _, _ := strings.Cut(control["Description"], "\n")

	// mygo's Debian package has no maintainer scripts; add them. The other
	// formats get the same ones.
	debHooks, err := debScripts(name)
	if err != nil {
		return err
	}
	if err := addDebScripts(debs[0], debHooks); err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(debs[0]), err)
	}
	log.Printf("added maintainer scripts to %s", debs[0])
	hooks := map[string]map[string]string{}
	for format, build := range map[string]func(string) (map[string]string, error){"rpm": rpmScripts, "archlinux": archScripts} {
		s, err := build(name)
		if err != nil {
			return err
		}
		dir := filepath.Join(tmp, format+"-scripts")
		if err := os.Mkdir(dir, 0o755); err != nil {
			return err
		}
		if hooks[format], err = writeScripts(dir, s); err != nil {
			return err
		}
	}

	for _, format := range []string{"rpm", "archlinux"} {
		info := nfpm.WithDefaults(&nfpm.Info{
			Name:        name,
			Arch:        goarch,
			Version:     version,
			Release:     "1",
			Maintainer:  control["Maintainer"],
			Description: description,
			Homepage:    homepage,
			License:     license,
			Overridables: nfpm.Overridables{
				Depends:   depends[format],
				Contents:  contents,
				RPM:       nfpm.RPM{Compression: "zstd", Summary: description, Group: "Applications/Internet", Packager: control["Maintainer"]},
				ArchLinux: nfpm.ArchLinux{Packager: control["Maintainer"]},
			},
		})
		h := hooks[format]
		if format == "rpm" {
			info.Scripts = nfpm.Scripts{PostInstall: h["post"], PreRemove: h["preun"], PostRemove: h["postun"]}
		} else {
			info.Scripts = nfpm.Scripts{PostInstall: h["post_install"], PreRemove: h["pre_remove"], PostRemove: h["post_remove"]}
			info.ArchLinux.Scripts = nfpm.ArchLinuxScripts{PostUpgrade: h["post_upgrade"]}
		}
		if err := writePackage(info, format, dir); err != nil {
			return fmt.Errorf("%s: %w", format, err)
		}
	}
	return writeAppImage(dir, name, version, goarch, tmp)
}

// writePackage writes the package of info in format into dir.
func writePackage(info *nfpm.Info, format, dir string) error {
	p, err := nfpm.Get(format)
	if err != nil {
		return err
	}
	if err := nfpm.PrepareForPackager(info, format); err != nil {
		return err
	}
	out := filepath.Join(dir, p.ConventionalFileName(info))
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	if err := p.Package(info, f); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	log.Printf("built %s", out)
	return nil
}

// contentsOf lists the files under root for nFPM, at their paths from
// root. Of the directories, it lists only the app's own in /opt: the others
// belong to the system.
func contentsOf(root string) (files.Contents, error) {
	var cs files.Contents
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == root {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		dst := "/" + filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			cs = append(cs, &files.Content{Type: files.TypeSymlink, Source: target, Destination: dst})
		case d.IsDir():
			if strings.HasPrefix(dst, "/opt/") {
				cs = append(cs, &files.Content{Type: files.TypeDir, Destination: dst, FileInfo: &files.ContentFileInfo{Mode: 0o755}})
			}
		default:
			cs = append(cs, &files.Content{Source: p, Destination: dst, FileInfo: &files.ContentFileInfo{Mode: info.Mode().Perm()}})
		}
		return nil
	})
	return cs, err
}

// readDeb unpacks the data of the Debian package at path into root and
// returns the fields of its control file.
func readDeb(path, root string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	magic := make([]byte, 8)
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != "!<arch>\n" {
		return nil, errors.New("not an ar archive")
	}
	var control map[string]string
	unpacked := false
	for {
		hdr := make([]byte, 60)
		if _, err := io.ReadFull(r, hdr); err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		member := strings.TrimSuffix(strings.TrimSpace(string(hdr[:16])), "/")
		size, err := strconv.ParseInt(strings.TrimSpace(string(hdr[48:58])), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", member, err)
		}
		body := io.LimitReader(r, size)
		switch member {
		case "control.tar.gz":
			if control, err = readControl(body); err != nil {
				return nil, err
			}
		case "data.tar.gz":
			if err := untar(body, root); err != nil {
				return nil, err
			}
			unpacked = true
		}
		if _, err := io.Copy(io.Discard, body); err != nil {
			return nil, err
		}
		if size%2 == 1 {
			if _, err := r.Discard(1); err != nil {
				return nil, err
			}
		}
	}
	if control == nil || !unpacked {
		return nil, errors.New("no control.tar.gz or data.tar.gz")
	}
	return control, nil
}

// readControl returns the fields of the control file in a control.tar.gz,
// continuation lines joined to their field with newlines.
func readControl(r io.Reader) (map[string]string, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err != nil {
			return nil, fmt.Errorf("control: %w", err)
		}
		if strings.TrimPrefix(h.Name, "./") != "control" {
			continue
		}
		b, err := io.ReadAll(tr)
		if err != nil {
			return nil, err
		}
		fields := map[string]string{}
		last := ""
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, " ") && last != "" {
				fields[last] += "\n" + strings.TrimSpace(line)
				continue
			}
			if k, v, ok := strings.Cut(line, ":"); ok {
				last = strings.TrimSpace(k)
				fields[last] = strings.TrimSpace(v)
			}
		}
		return fields, nil
	}
}

// untar unpacks a tar.gz into dir.
func untar(r io.Reader, dir string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		name := filepath.Clean(strings.TrimPrefix(h.Name, "./"))
		if name == "." {
			continue
		}
		if !filepath.IsLocal(name) {
			return fmt.Errorf("%s leaves the package", h.Name)
		}
		p := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(p, 0o755)
		case tar.TypeSymlink:
			if err = os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
				err = os.Symlink(h.Linkname, p)
			}
		case tar.TypeReg:
			if err = os.MkdirAll(filepath.Dir(p), 0o755); err == nil {
				err = writeFile(p, tr, fs.FileMode(h.Mode).Perm())
			}
		default:
			err = fmt.Errorf("%s: unexpected type %c", h.Name, h.Typeflag)
		}
		if err != nil {
			return err
		}
	}
}

func writeFile(path string, r io.Reader, mode fs.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// writeAppImage writes the app of the build in dir as an AppImage: the
// AppImage runtime followed by a SquashFS image of the app's directory.
func writeAppImage(dir, name, version, goarch, tmp string) error {
	arch, ok := appImageArch[goarch]
	if !ok {
		log.Printf("no AppImage for %s", goarch)
		return nil
	}
	if _, err := exec.LookPath("mksquashfs"); err != nil {
		return errors.New("the AppImage needs mksquashfs: install squashfs-tools")
	}
	runtime, err := appImageRuntime(arch)
	if err != nil {
		return err
	}
	desktop, err := os.ReadFile(filepath.Join(dir, name+".desktop"))
	if err != nil {
		return err
	}
	title := name
	for _, line := range strings.Split(string(desktop), "\n") {
		if v, ok := strings.CutPrefix(line, "Name="); ok {
			title = v
			break
		}
	}

	app := filepath.Join(tmp, "AppDir")
	if err := os.Mkdir(app, 0o755); err != nil {
		return err
	}
	// The executable, with the resources it finds next to it.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || n == name+".desktop" || strings.HasSuffix(n, ".deb") || strings.HasSuffix(n, ".rpm") ||
			strings.Contains(n, ".pkg.tar") || strings.HasSuffix(n, ".AppImage") || strings.HasSuffix(n, ".tar.gz") ||
			strings.HasSuffix(n, ".delta") || strings.HasSuffix(n, ".json") || n == "install.sh" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		src, err := os.Open(filepath.Join(dir, n))
		if err != nil {
			return err
		}
		err = writeFile(filepath.Join(app, n), src, info.Mode().Perm())
		src.Close()
		if err != nil {
			return err
		}
	}
	desktop = append(bytes.TrimRight(desktop, "\n"), []byte("\nX-AppImage-Version="+version+"\n")...)
	if err := os.WriteFile(filepath.Join(app, name+".desktop"), desktop, 0o644); err != nil {
		return err
	}
	for link, target := range map[string]string{"AppRun": name, ".DirIcon": name + ".png"} {
		if err := os.Symlink(target, filepath.Join(app, link)); err != nil {
			return err
		}
	}

	image := filepath.Join(tmp, "app.squashfs")
	cmd := exec.Command("mksquashfs", app, image, "-root-owned", "-noappend", "-no-xattrs", "-comp", "zstd", "-b", "1M", "-quiet")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mksquashfs: %w", err)
	}
	out := filepath.Join(dir, fmt.Sprintf("%s-%s-%s.AppImage", title, version, arch))
	f, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	img, err := os.Open(image)
	if err != nil {
		f.Close()
		return err
	}
	defer img.Close()
	if _, err := f.Write(runtime); err != nil {
		f.Close()
		return err
	}
	if _, err := io.Copy(f, img); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	log.Printf("built %s", out)
	return nil
}

// appImageRuntime returns the AppImage runtime for arch, which it downloads
// into the user's cache the first time.
func appImageRuntime(arch string) ([]byte, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(cache, "mygo-clash-linuxpkg", "appimage-runtime-"+runtimeRelease, "runtime-"+arch)
	if b, err := os.ReadFile(path); err == nil && sum(b) == runtimeSHA256[arch] {
		return b, nil
	}
	url := "https://github.com/AppImage/type2-runtime/releases/download/" + runtimeRelease + "/runtime-" + arch
	log.Printf("downloading %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if got := sum(b); got != runtimeSHA256[arch] {
		return nil, fmt.Errorf("%s: SHA-256 %s, want %s", url, got, runtimeSHA256[arch])
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return b, os.WriteFile(path, b, 0o644)
}

func sum(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
