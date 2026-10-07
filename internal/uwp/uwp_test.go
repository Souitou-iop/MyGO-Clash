package uwp

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestExemptions(t *testing.T) {
	listed := []StoreApp{{SID: "S-1-15-2-1"}, {SID: "S-1-15-2-2"}, {SID: "S-1-15-2-3"}}
	// 2 is unchecked, 3 newly checked, 9 belongs to no listed app (another
	// user's) and stays; case and duplicates do not matter.
	got := Exemptions([]string{"S-1-15-2-2", "s-1-15-2-9"}, listed, []string{"S-1-15-2-1", "S-1-15-2-3", "s-1-15-2-1", "S-1-15-2-7"})
	want := []string{"s-1-15-2-9", "S-1-15-2-1", "S-1-15-2-3"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := Exemptions(nil, listed, nil); len(got) != 0 {
		t.Fatalf("none selected: %v", got)
	}
}

func TestList(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sids")
	in := []string{"S-1-15-2-1", "S-1-15-2-2"}
	if err := WriteList(p, in); err != nil {
		t.Fatal(err)
	}
	out, err := ReadList(p)
	if err != nil || !slices.Equal(in, out) {
		t.Fatalf("%v %v", out, err)
	}
	if err := WriteList(p, nil); err != nil {
		t.Fatal(err)
	}
	if out, _ := ReadList(p); len(out) != 0 {
		t.Fatalf("empty list read as %v", out)
	}
}
