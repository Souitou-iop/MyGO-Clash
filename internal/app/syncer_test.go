package app

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/mygo-clash/mygo-clash/internal/profiles"
)

// itemJSON is a profile item as sync stores it, with content.
func itemJSON(name string, updated, traffic int64, content string) []byte {
	meta := profiles.Profile{
		UID: "x", Type: profiles.TypeRemote, Name: name,
		URL: "https://example.com/sub", Updated: updated,
		Usage: &profiles.Usage{Download: traffic}, Size: len(content),
	}
	return canonical(itemFile{Meta: meta, Content: []byte(content)})
}

// Two devices that refresh the same subscription at about the same time hold
// the same content and differ only in what the fetch reported; that is not a
// conflict, but edits on either side still merge field by field.
func TestProfileItemMerge(t *testing.T) {
	base := itemJSON("Sub", 100, 1, "cfg")
	local := itemJSON("My Sub", 200, 2, "cfg")
	remote := itemJSON("Sub", 210, 3, "cfg")
	out, ok := (appSource{}).Merge(itemPrefix+"x", base, local, remote, true)
	if !ok {
		t.Fatal("the versions did not merge")
	}
	var got itemFile
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	if got.Meta.Name != "My Sub" || got.Meta.Updated != 210 || got.Meta.Usage.Download != 3 || string(got.Content) != "cfg" {
		t.Fatalf("merged %s", out)
	}
	// The merged data is what the next snapshot hashes: same bytes, so the
	// merge does not read as a change of its own.
	if !bytes.Equal(canonical(got), out) {
		t.Fatalf("the merge is not canonical:\n%s\n%s", out, canonical(got))
	}

	// Contents that differ are a real conflict: the user chooses.
	if _, ok := (appSource{}).Merge(itemPrefix+"x", base, local, itemJSON("Sub", 210, 3, "other"), true); ok {
		t.Fatal("versions with different contents merged")
	}
}
