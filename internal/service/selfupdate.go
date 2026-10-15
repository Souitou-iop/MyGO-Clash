package service

import (
	"archive/tar"
	"bytes"
	"cmp"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// The service updates itself to the app's version without asking for an
// administrator again: the app hands it the archive of that version, as
// published for the app's own updates, and the service takes the
// executable from it only when the archive is signed with the key the
// service was built with, and the executable says it is of the version
// asked for, newer than the service's. So the service runs only what the
// app's releases ship, never an older one.
//
//  1. POST PathUpdateStage?version=V&signature=S with the archive as the
//     body: the service checks it and keeps the executable next to its own,
//     while its core goes on running.
//  2. POST PathUpdateApply: the service stops its core, puts the executable
//     in place of its own and exits, and the system starts it again.
const (
	PathUpdateStage = "/__service/update/stage"
	PathUpdateApply = "/__service/update/apply"
)

// UpdateKey is the public key the app's updates are signed with, in
// base64, set by main from mygo.json. Without it the service updates only
// through the administrator.
var UpdateKey string

// maxArchive bounds the archive the service reads.
const maxArchive = 1 << 30

var versionRe = regexp.MustCompile(`^v?[0-9][0-9A-Za-z.+-]{0,63}$`)

// stagedPath is where the service keeps the executable of an update: next
// to its own, where only the administrator writes, so that it can be
// renamed into place. It keeps the extension, for Windows to run it.
func stagedPath(p Paths) string {
	ext := filepath.Ext(p.Bin)
	return strings.TrimSuffix(p.Bin, ext) + ".update" + ext
}

// stage checks the archive in r, signed with signature, and keeps the
// executable of version from it at stagedPath.
func stage(ctx context.Context, p Paths, version, signature string, r io.Reader) error {
	if UpdateKey == "" {
		return errors.New("this build of the service cannot update itself")
	}
	pub, err := base64.StdEncoding.DecodeString(strings.TrimSpace(UpdateKey))
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return errors.New("invalid update key")
	}
	if !versionRe.MatchString(version) {
		return fmt.Errorf("invalid version %q", version)
	}
	if compareVersions(version, Version) <= 0 {
		return fmt.Errorf("the service is already of version %s", Version)
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return errors.New("invalid signature")
	}

	dir := filepath.Dir(p.Bin)
	archive, err := os.CreateTemp(dir, ".update-*.tar.gz")
	if err != nil {
		return err
	}
	defer func() {
		archive.Close()
		os.Remove(archive.Name())
	}()
	sum := sha256.New()
	n, err := io.Copy(io.MultiWriter(archive, sum), io.LimitReader(r, maxArchive+1))
	if err != nil {
		return fmt.Errorf("receiving the update: %w", err)
	}
	if n > maxArchive {
		return errors.New("the update is too large")
	}
	if !ed25519.Verify(pub, sum.Sum(nil), sig) {
		return errors.New("the update is not signed with the app's key")
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return err
	}

	dst := stagedPath(p)
	_ = os.Remove(dst)
	if err := extractExecutable(archive, dst); err != nil {
		os.Remove(dst)
		return err
	}
	if runtime.GOOS == "darwin" {
		// Out of its bundle, as when installed (see install).
		if out, err := exec.Command("/usr/bin/codesign", "--force", "--sign", "-", dst).CombinedOutput(); err != nil {
			os.Remove(dst)
			return fmt.Errorf("codesign: %v: %s", err, out)
		}
	}
	got, err := executableVersion(ctx, dst)
	if err != nil || got != strings.TrimPrefix(version, "v") {
		os.Remove(dst)
		if err != nil {
			return fmt.Errorf("the update's version: %w", err)
		}
		return fmt.Errorf("the update is of version %s, not %s", got, version)
	}
	return nil
}

// extractExecutable writes the app's executable in the archive r to dst:
// the one in the bundle's Contents/MacOS on macOS, the .exe on Windows,
// the executable file elsewhere, at the archive's top.
func extractExecutable(r io.Reader, dst string) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	found := false
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("reading the update: %w", err)
		}
		if h.Typeflag != tar.TypeReg || !isAppExecutable(path.Clean(h.Name), h.FileInfo().Mode()) {
			continue
		}
		if found {
			return errors.New("the update holds more than one executable")
		}
		found = true
		if h.Size <= 0 || h.Size > maxArchive {
			return fmt.Errorf("invalid executable size %d", h.Size)
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, io.LimitReader(tr, h.Size))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	if !found {
		return errors.New("the update holds no executable")
	}
	return nil
}

func isAppExecutable(name string, mode os.FileMode) bool {
	parts := strings.Split(name, "/")
	switch runtime.GOOS {
	case "darwin":
		return len(parts) == 4 && strings.HasSuffix(parts[0], ".app") && parts[1] == "Contents" && parts[2] == "MacOS"
	case "windows":
		return len(parts) == 1 && strings.HasSuffix(strings.ToLower(name), ".exe")
	}
	return len(parts) == 1 && mode&0o111 != 0
}

// executableVersion asks the executable at path for its version.
func executableVersion(ctx context.Context, path string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version")
	hideWindow(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.String()), nil
}

// apply puts the staged executable in place of the service's own. The
// running one keeps its file: renamed over on Unix, moved aside on
// Windows, which removes it at the next start (see cleanUpdate).
func apply(p Paths) error {
	staged := stagedPath(p)
	if _, err := os.Stat(staged); err != nil {
		return errors.New("no update is staged")
	}
	if runtime.GOOS == "windows" {
		old := p.Bin + ".old"
		_ = os.Remove(old)
		if err := os.Rename(p.Bin, old); err != nil {
			return err
		}
		if err := os.Rename(staged, p.Bin); err != nil {
			_ = os.Rename(old, p.Bin)
			return err
		}
		return nil
	}
	return os.Rename(staged, p.Bin)
}

// cleanUpdate removes what an update left: the executable it replaced on
// Windows, and one staged but not applied.
func cleanUpdate(p Paths) {
	_ = os.Remove(p.Bin + ".old")
	_ = os.Remove(stagedPath(p))
}

// compareVersions compares versions such as 0.2.7 and 0.3.0-beta as
// semantic versioning does: numbers numerically, a pre-release before its
// release.
func compareVersions(a, b string) int {
	a, b = strings.TrimPrefix(a, "v"), strings.TrimPrefix(b, "v")
	a, _, _ = strings.Cut(a, "+")
	b, _, _ = strings.Cut(b, "+")
	ar, apre, _ := strings.Cut(a, "-")
	br, bpre, _ := strings.Cut(b, "-")
	if c := compareFields(ar, br); c != 0 {
		return c
	}
	switch {
	case apre == bpre:
		return 0
	case apre == "":
		return 1
	case bpre == "":
		return -1
	}
	return compareFields(apre, bpre)
}

func compareFields(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range max(len(as), len(bs)) {
		if i >= len(as) {
			return -1
		}
		if i >= len(bs) {
			return 1
		}
		x, xerr := strconv.Atoi(as[i])
		y, yerr := strconv.Atoi(bs[i])
		switch {
		case xerr == nil && yerr == nil:
			if x != y {
				return cmp.Compare(x, y)
			}
		case xerr == nil:
			return -1
		case yerr == nil:
			return 1
		default:
			if c := strings.Compare(as[i], bs[i]); c != 0 {
				return c
			}
		}
	}
	return 0
}

// Update stages the archive of version in r, signed with signature, in
// the service, which goes on running meanwhile; ApplyUpdate then restarts
// it on the new version.
func (c *Client) Update(ctx context.Context, version, signature string, r io.Reader) error {
	q := url.Values{"version": {version}, "signature": {signature}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://service"+PathUpdateStage+"?"+q.Encode(), r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/gzip")
	// The archive takes longer than the client's timeout to check.
	hc := *c.hc
	hc.Timeout = 0
	return c.send(&hc, req, nil)
}

// ApplyUpdate restarts the service on the version Update staged; its core
// stops.
func (c *Client) ApplyUpdate(ctx context.Context) error {
	return c.do(ctx, http.MethodPost, PathUpdateApply, nil, nil)
}
