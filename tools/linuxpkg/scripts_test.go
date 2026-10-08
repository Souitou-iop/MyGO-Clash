package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Every script is POSIX sh, apart from the Arch hooks, which are function
// bodies of bash: check that they parse.
func TestScriptsParse(t *testing.T) {
	deb, _ := debScripts("mygo-clash")
	rpm, _ := rpmScripts("mygo-clash")
	arch, _ := archScripts("mygo-clash")
	for kind, set := range map[string]map[string]string{"deb": deb, "rpm": rpm, "arch": arch} {
		for hook, s := range set {
			shell := "sh"
			if kind == "arch" {
				shell = "bash"
				s = "function " + hook + "() {\n" + s + "\n}\n"
			}
			cmd := exec.Command(shell, "-n")
			cmd.Stdin = strings.NewReader(s)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("%s %s: %v\n%s", kind, hook, err, out)
			}
			if strings.Contains(s, "@NAME@") {
				t.Errorf("%s %s: name not filled in", kind, hook)
			}
		}
	}
}

func TestScriptRejectsOddNames(t *testing.T) {
	if _, err := script("a b; rm -rf /", ""); err == nil {
		t.Fatal("accepted a name that is not a package name")
	}
}

func TestAddDebScripts(t *testing.T) {
	tarGz := func(files map[string]string, order []string) []byte {
		var buf bytes.Buffer
		zw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(zw)
		for _, n := range order {
			tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "./" + n, Mode: 0o644, Size: int64(len(files[n]))})
			io.WriteString(tw, files[n])
		}
		tw.Close()
		zw.Close()
		return buf.Bytes()
	}
	// An odd-sized control archive checks the padding.
	control := tarGz(map[string]string{"control": "Package: x\nVersion: 1\nArchitecture: amd64\n", "md5sums": ""}, []string{"control", "md5sums"})
	data := tarGz(map[string]string{"hello": "hi"}, []string{"hello"})
	var deb bytes.Buffer
	deb.WriteString("!<arch>\n")
	for _, m := range []struct {
		name string
		b    []byte
	}{{"debian-binary", []byte("2.0\n")}, {"control.tar.gz", control}, {"data.tar.gz", data}} {
		fmt.Fprintf(&deb, "%-16s%-12d%-6d%-6d%-8s%-10d`\n", m.name, 1, 0, 0, "100644", len(m.b))
		deb.Write(m.b)
		if len(m.b)%2 == 1 {
			deb.WriteByte('\n')
		}
	}
	path := filepath.Join(t.TempDir(), "x.deb")
	if err := os.WriteFile(path, deb.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	scripts, _ := debScripts("mygo-clash")
	// Twice: the second run replaces the scripts of the first.
	for range 2 {
		if err := addDebScripts(path, scripts); err != nil {
			t.Fatal(err)
		}
	}
	root := filepath.Join(t.TempDir(), "root")
	fields, err := readDeb(path, root)
	if err != nil {
		t.Fatal(err)
	}
	if fields["Package"] != "x" {
		t.Errorf("control fields lost: %v", fields)
	}
	if b, err := os.ReadFile(filepath.Join(root, "hello")); err != nil || string(b) != "hi" {
		t.Errorf("data lost: %q, %v", b, err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := controlFiles(t, b)
	for _, want := range []string{"control", "md5sums", "postinst", "prerm", "postrm"} {
		if n := got[want]; n != 1 {
			t.Errorf("control archive has %d of %s, want 1", n, want)
		}
	}
}

// controlFiles counts the files of the control archive in a Debian package.
func controlFiles(t *testing.T, deb []byte) map[string]int {
	t.Helper()
	i := bytes.Index(deb, []byte("control.tar.gz"))
	var size int
	fmt.Sscanf(string(deb[i+48:i+58]), "%d", &size)
	zr, err := gzip.NewReader(bytes.NewReader(deb[i+60 : i+60+size]))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err != nil {
			return counts
		}
		counts[strings.TrimPrefix(h.Name, "./")]++
	}
}
