// Package logx writes the app's logs: files that rotate by size, and the
// last lines of a stream kept in memory to explain failures.
package logx

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RotatingFile is a log file that moves to name.1, name.2, ... when it
// grows past MaxSize, keeping MaxFiles of them.
type RotatingFile struct {
	Path     string
	MaxSize  int64
	MaxFiles int

	mu   sync.Mutex
	f    *os.File
	size int64
}

// Write implements io.Writer.
func (r *RotatingFile) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	if r.MaxSize > 0 && r.size+int64(len(p)) > r.MaxSize {
		r.rotate()
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *RotatingFile) open() error {
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(r.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	fi, _ := f.Stat()
	r.f = f
	if fi != nil {
		r.size = fi.Size()
	}
	return nil
}

func (r *RotatingFile) rotate() {
	_ = r.f.Close()
	keep := max(r.MaxFiles, 1)
	_ = os.Remove(fmt.Sprintf("%s.%d", r.Path, keep))
	for i := keep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", r.Path, i), fmt.Sprintf("%s.%d", r.Path, i+1))
	}
	_ = os.Rename(r.Path, r.Path+".1")
	r.f, r.size = nil, 0
	_ = r.open()
}

// Close closes the file.
func (r *RotatingFile) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.f == nil {
		return nil
	}
	err := r.f.Close()
	r.f = nil
	return err
}

// Tail keeps the last lines written to it.
type Tail struct {
	mu    sync.Mutex
	lines []string
	n     int
	part  []byte
}

// NewTail keeps n lines.
func NewTail(n int) *Tail { return &Tail{n: n} }

// Write implements io.Writer.
func (t *Tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.part = append(t.part, p...)
	for {
		i := bytes.IndexByte(t.part, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSpace(string(t.part[:i]))
		t.part = t.part[i+1:]
		if line == "" {
			continue
		}
		t.lines = append(t.lines, line)
		if len(t.lines) > t.n {
			t.lines = t.lines[len(t.lines)-t.n:]
		}
	}
	if len(t.part) > 4096 {
		t.part = t.part[:0]
	}
	return len(p), nil
}

// Lines returns the lines kept.
func (t *Tail) Lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.lines...)
}

// CleanOld deletes the log files in dir older than days.
func CleanOld(dir string, days int) {
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.Contains(e.Name(), ".log") {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
