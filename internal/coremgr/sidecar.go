package coremgr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/logx"
)

// Sidecar starts the core as a child process of the app, with the user's
// privileges: everything but TUN works.
type Sidecar struct {
	// Exe is the executable to run with the "core" argument; "" is the
	// app's own.
	Exe string
	// Log receives the core's output.
	Log io.Writer
}

// Mode implements Launcher.
func (Sidecar) Mode() string { return "sidecar" }

// Start implements Launcher.
func (s Sidecar) Start(ctx context.Context, boot coreapi.Bootstrap) (Process, error) {
	exe := s.Exe
	if exe == "" {
		var err error
		if exe, err = os.Executable(); err != nil {
			return nil, err
		}
	}
	boot.ExitWithStdin = true
	boot.OwnerUID, boot.OwnerGID = -1, -1
	line, err := json.Marshal(boot)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(exe, "core")
	hideWindow(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	tail := logx.NewTail(20)
	out := io.Writer(tail)
	if s.Log != nil {
		out = io.MultiWriter(s.Log, tail)
	}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start the core: %w", err)
	}
	if _, err := stdin.Write(append(line, '\n')); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("start the core: %w", err)
	}
	p := &sidecarProc{cmd: cmd, stdin: stdin, tail: tail, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		if err != nil || !p.stopping {
			p.err = exitError(err, tail.Lines())
		}
		p.mu.Unlock()
		close(p.done)
	}()
	return p, nil
}

type sidecarProc struct {
	cmd   *exec.Cmd
	stdin io.WriteCloser
	tail  *logx.Tail
	done  chan struct{}

	mu       sync.Mutex
	err      error
	stopping bool
}

func (p *sidecarProc) Done() <-chan struct{} { return p.done }

func (p *sidecarProc) Err() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

func (p *sidecarProc) PID() int { return p.cmd.Process.Pid }

// Stop closes the core's input, which makes it clean up (routes, DNS) and
// exit, and kills it if it has not by the deadline.
func (p *sidecarProc) Stop(ctx context.Context) error {
	p.mu.Lock()
	p.stopping = true
	p.mu.Unlock()
	_ = p.stdin.Close()
	select {
	case <-p.done:
		return nil
	case <-ctx.Done():
		_ = p.cmd.Process.Kill()
		<-p.done
		return ctx.Err()
	}
}

// exitError explains why the core exited, with its last lines of output,
// which usually say.
func exitError(err error, lines []string) error {
	var msg []string
	for _, l := range lines {
		if strings.Contains(l, "level=fatal") || strings.Contains(l, "level=error") || strings.HasPrefix(l, "core:") || strings.HasPrefix(l, "panic") {
			msg = append(msg, l)
		}
	}
	if len(msg) == 0 && len(lines) > 0 {
		msg = lines[max(0, len(lines)-3):]
	}
	if err == nil {
		err = errors.New("exited")
	}
	if len(msg) == 0 {
		return err
	}
	return fmt.Errorf("%v: %s", err, strings.Join(msg, " | "))
}
