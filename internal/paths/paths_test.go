package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanRuns(t *testing.T) {
	support := t.TempDir()
	data := filepath.Join(support, "MyGO-Clash")
	if err := os.MkdirAll(data, 0o700); err != nil {
		t.Fatal(err)
	}
	mk := func(name string, files ...string) {
		t.Helper()
		dir := filepath.Join(support, name)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	mk("MyGO-Clash-101")         // an empty run: goes
	mk("MyGO-Clash-202", "keep") // a run with data: stays
	mk("MyGO-Clash Dev", "keep") // the development data: stays
	mk("MyGO-Clash-abc")         // not a pid: stays
	mk("MyGO-Clash Dev-303")     // a run of a development app: goes
	mk("Other-404")              // another app's directory: stays

	CleanRuns(data, "MyGO-Clash Dev")

	for _, name := range []string{"MyGO-Clash", "MyGO-Clash-202", "MyGO-Clash Dev", "MyGO-Clash-abc", "Other-404"} {
		if _, err := os.Stat(filepath.Join(support, name)); err != nil {
			t.Fatalf("%s was removed: %v", name, err)
		}
	}
	for _, name := range []string{"MyGO-Clash-101", "MyGO-Clash Dev-303"} {
		if _, err := os.Stat(filepath.Join(support, name)); !os.IsNotExist(err) {
			t.Fatalf("%s stayed: %v", name, err)
		}
	}
}
