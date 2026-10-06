package service

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
)

// ErrCanceled is returned when the user declined to authorize.
var ErrCanceled = errors.New("the authorization was canceled")

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,40}$`)

// Main runs the service command line: run, install or uninstall, with
// --name (the app's slug) and --owner (a UID, or a SID on Windows).
func Main(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: service run|install|uninstall --name NAME --owner USER")
	}
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	name := fs.String("name", "mygo-clash", "the app's name")
	owner := fs.String("owner", "", "the user the service serves")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if !slugRe.MatchString(*name) {
		return fmt.Errorf("invalid name %q", *name)
	}
	cfg := Config{Slug: *name, Owner: *owner}
	switch args[0] {
	case "run":
		if cfg.Owner == "" {
			return errors.New("--owner is required")
		}
		return runService(cfg)
	case "install":
		if cfg.Owner == "" {
			return errors.New("--owner is required")
		}
		return install(cfg)
	case "uninstall":
		return uninstall(cfg.Slug)
	}
	return fmt.Errorf("unknown service command %q", args[0])
}

// copyExecutable copies the running executable to dst, where only the
// administrator can write it.
func copyExecutable(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	_ = os.Remove(tmp)
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	_ = os.Remove(dst) // Windows cannot rename over a file
	return os.Rename(tmp, dst)
}
