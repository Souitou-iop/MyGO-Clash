package agent

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Launch starts the app this executable is, unless one already answers,
// and waits until its control socket does.
func Launch(ctx context.Context, targets []Target) (*Client, error) {
	if c, err := Dial(ctx, targets); err == nil {
		return c, nil
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	var cmd *exec.Cmd
	if bundle := appBundle(exe); bundle != "" {
		// Through Launch Services, as from the Finder.
		cmd = exec.Command("open", "-g", bundle)
	} else {
		cmd = exec.Command(exe)
		detach(cmd)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	go func() { _ = cmd.Wait() }()
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	for {
		if c, err := Dial(ctx, targets); err == nil {
			return c, nil
		}
		select {
		case <-ctx.Done():
			return nil, ErrNotRunning
		case <-time.After(300 * time.Millisecond):
		}
	}
}

// appBundle returns the .app bundle of a macOS executable, "" otherwise.
func appBundle(exe string) string {
	if runtime.GOOS != "darwin" {
		return ""
	}
	dir := filepath.Dir(exe)
	if filepath.Base(dir) != "MacOS" || filepath.Base(filepath.Dir(dir)) != "Contents" {
		return ""
	}
	bundle := filepath.Dir(filepath.Dir(dir))
	if !strings.HasSuffix(bundle, ".app") {
		return ""
	}
	return bundle
}
