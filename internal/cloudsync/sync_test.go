package cloudsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	stdsync "sync"
	"testing"
	"time"

	xwebdav "golang.org/x/net/webdav"

	"github.com/mygo-clash/mygo-clash/internal/webdav"
)

// server is a WebDAV server for tests. With etags, it enforces If-Match
// and If-None-Match as Nextcloud does; without, it neither sends ETags nor
// honors conditions, as some servers do.
type server struct {
	*httptest.Server
	fs    xwebdav.FileSystem
	mu    stdsync.Mutex
	puts  int
	etags bool
}

func newServer(t *testing.T, etags bool) *server {
	s := &server{fs: xwebdav.NewMemFS(), etags: etags}
	h := &xwebdav.Handler{Prefix: "/dav", FileSystem: s.fs, LockSystem: xwebdav.NewMemLS()}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "ada" || p != "pw" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if r.Method == http.MethodPut {
			s.puts++
			if s.etags {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest(http.MethodHead, r.URL.Path, nil))
				exists := rec.Code == http.StatusOK
				cur := rec.Header().Get("ETag")
				if (r.Header.Get("If-None-Match") == "*" && exists) ||
					(r.Header.Get("If-Match") != "" && r.Header.Get("If-Match") != cur) {
					w.WriteHeader(http.StatusPreconditionFailed)
					return
				}
			}
		}
		if !s.etags {
			w = noETag{w}
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

type noETag struct{ http.ResponseWriter }

func (w noETag) WriteHeader(code int) {
	w.ResponseWriter.Header().Del("ETag")
	w.ResponseWriter.WriteHeader(code)
}

func (w noETag) Write(b []byte) (int, error) {
	w.ResponseWriter.Header().Del("ETag")
	// PROPFIND answers carry ETags in the body too.
	b = bytes.ReplaceAll(b, []byte("getetag"), []byte("notetag")) // same length
	return w.ResponseWriter.Write(b)
}

// allData returns every file on the server.
func (s *server) allData(t *testing.T) map[string][]byte {
	out := map[string][]byte{}
	var walk func(dir string)
	walk = func(dir string) {
		f, err := s.fs.OpenFile(context.Background(), dir, os.O_RDONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		infos, _ := f.Readdir(-1)
		f.Close()
		for _, fi := range infos {
			p := strings.TrimSuffix(dir, "/") + "/" + fi.Name()
			if fi.IsDir() {
				walk(p)
				continue
			}
			f, _ := s.fs.OpenFile(context.Background(), p, os.O_RDONLY, 0)
			data, _ := io.ReadAll(f)
			f.Close()
			out[p] = data
		}
	}
	walk("/")
	return out
}

// memSource is a device's data.
type memSource struct {
	mu    stdsync.Mutex
	files map[string]Item
}

func (m *memSource) Snapshot(context.Context) (map[string]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Clone(m.files), nil
}

func (m *memSource) Apply(_ context.Context, ch map[string][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for p, d := range ch {
		if d == nil {
			delete(m.files, p)
		} else {
			m.files[p] = Item{Data: d, Modified: time.Now()}
		}
	}
	return nil
}

func (m *memSource) Merge(p string, base, local, remote []byte, preferRemote bool) ([]byte, bool) {
	if p != "settings" {
		return nil, false
	}
	return MergeJSON(base, local, remote, preferRemote)
}

func (m *memSource) KeepBoth(_ context.Context, p string, remote []byte, device string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := p + " (conflict)"
	m.files[cp] = Item{Data: remote, Modified: time.Now()}
	return cp, nil
}

func (m *memSource) set(p, data string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.files[p] = Item{Data: []byte(data), Modified: time.Now()}
}

func (m *memSource) get(p string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return string(m.files[p].Data)
}

func (m *memSource) del(p string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.files, p)
}

type device struct {
	src *memSource
	eng *Engine
}

func newDevice(t *testing.T, s *server, name, passphrase, policy string) *device {
	t.Helper()
	c, err := webdav.New(s.URL+"/dav/", "ada", "pw", webdav.Options{AllowInsecure: true})
	if err != nil {
		t.Fatal(err)
	}
	keys, _, err := OpenVault(context.Background(), c, "MyGO-Clash", passphrase, true)
	if err != nil {
		t.Fatal(err)
	}
	src := &memSource{files: map[string]Item{}}
	return &device{src: src, eng: &Engine{
		Client: c, Dir: "MyGO-Clash", Keys: keys, Source: src, Policy: policy, DeviceName: name,
		State:     &State{DeviceID: name},
		Mergeable: func(p string) bool { return p == "settings" },
	}}
}

func (d *device) sync(t *testing.T) Report {
	t.Helper()
	r, err := d.eng.Sync(context.Background())
	if err != nil {
		t.Fatalf("%s: %v", d.eng.DeviceName, err)
	}
	return r
}

func testSync(t *testing.T, etags bool) {
	s := newServer(t, etags)
	a := newDevice(t, s, "laptop", "correct horse battery", "ask")
	b := newDevice(t, s, "desktop", "correct horse battery", "ask")

	a.src.set("settings", `{"theme":"dark","clash":{"mode":"rule","ipv6":true}}`)
	a.src.set("profile/x", "proxies: [x1]")
	a.src.set("profile/y", "proxies: [y1]")
	if r := a.sync(t); len(r.Pushed) != 3 {
		t.Fatalf("first push: %+v", r)
	}
	if r := b.sync(t); len(r.Pulled) != 3 || b.src.get("profile/x") != "proxies: [x1]" {
		t.Fatalf("first pull: %+v", r)
	}

	// Changes on different paths cross.
	a.src.set("profile/x", "proxies: [x2]")
	b.src.set("profile/y", "proxies: [y2]")
	a.sync(t)
	b.sync(t)
	a.sync(t)
	if a.src.get("profile/y") != "proxies: [y2]" || b.src.get("profile/x") != "proxies: [x2]" {
		t.Fatalf("crossed changes: a=%q b=%q", a.src.get("profile/y"), b.src.get("profile/x"))
	}

	// Settings changed on both sides merge field by field.
	a.src.set("settings", `{"theme":"light","clash":{"mode":"rule","ipv6":true}}`)
	b.src.set("settings", `{"theme":"dark","clash":{"mode":"global","ipv6":true}}`)
	a.sync(t)
	if r := b.sync(t); len(r.Merged) != 1 {
		t.Fatalf("merge: %+v", r)
	}
	a.sync(t)
	for _, d := range []*device{a, b} {
		var got map[string]any
		_ = json.Unmarshal([]byte(d.src.get("settings")), &got)
		if got["theme"] != "light" || got["clash"].(map[string]any)["mode"] != "global" {
			t.Fatalf("%s settings %v", d.eng.DeviceName, got)
		}
	}

	// A profile changed on both sides is a conflict to ask about...
	a.src.set("profile/x", "proxies: [x3-laptop]")
	b.src.set("profile/x", "proxies: [x3-desktop]")
	a.sync(t)
	r := b.sync(t)
	if len(r.Conflicts) != 1 || b.src.get("profile/x") != "proxies: [x3-desktop]" {
		t.Fatalf("conflict: %+v %q", r, b.src.get("profile/x"))
	}
	// ...and keeping both copies the server's aside.
	if err := b.eng.State.Resolve("profile/x", KeepBoth); err != nil {
		t.Fatal(err)
	}
	r = b.sync(t)
	if len(r.Copies) != 1 || b.src.get("profile/x (conflict)") != "proxies: [x3-laptop]" {
		t.Fatalf("keep both: %+v", r)
	}
	a.sync(t)
	if a.src.get("profile/x") != "proxies: [x3-desktop]" || a.src.get("profile/x (conflict)") != "proxies: [x3-laptop]" {
		t.Fatalf("after keep both: %q / %q", a.src.get("profile/x"), a.src.get("profile/x (conflict)"))
	}

	// Deleting against a change keeps the change.
	a.src.del("profile/y")
	b.src.set("profile/y", "proxies: [y3]")
	a.sync(t)
	b.sync(t)
	a.sync(t)
	if a.src.get("profile/y") != "proxies: [y3]" {
		t.Fatalf("change lost to a deletion: %q", a.src.get("profile/y"))
	}
	// A plain deletion propagates.
	b.src.del("profile/y")
	b.sync(t)
	a.sync(t)
	if _, ok := a.src.files["profile/y"]; ok {
		t.Fatal("deletion did not propagate")
	}

	// The server sees nothing in the clear.
	for name, data := range s.allData(t) {
		for _, secret := range []string{"x3-laptop", "proxies", "theme", "profile/x", "global"} {
			if bytes.Contains(data, []byte(secret)) {
				t.Fatalf("%s holds %q in the clear", name, secret)
			}
		}
	}
}

func TestSyncWithETags(t *testing.T)    { testSync(t, true) }
func TestSyncWithoutETags(t *testing.T) { testSync(t, false) }

func TestStaleManifestWriteLoses(t *testing.T) {
	s := newServer(t, true)
	a := newDevice(t, s, "a", "passphrase-1", "newest")
	b := newDevice(t, s, "b", "passphrase-1", "newest")
	a.src.set("settings", `{"v":1}`)
	a.sync(t)
	stale, err := a.eng.fetchManifest(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	b.src.set("profile/z", "z")
	b.sync(t) // the server moves on
	next := stale.m
	next.Seq++
	next.Writer = "a"
	next.Updated = time.Now()
	ok, err := a.eng.writeManifest(context.Background(), stale, next)
	if err != nil || ok {
		t.Fatalf("a stale write won: ok=%v err=%v", ok, err)
	}
	// The next sync of a starts over and keeps b's change.
	a.sync(t)
	if a.src.get("profile/z") != "z" {
		t.Fatal("b's change was lost")
	}
}

func TestPassphraseAndRewrap(t *testing.T) {
	s := newServer(t, true)
	a := newDevice(t, s, "a", "first passphrase", "ask")
	c, _ := webdav.New(s.URL+"/dav/", "ada", "pw", webdav.Options{AllowInsecure: true})
	if _, _, err := OpenVault(context.Background(), c, "MyGO-Clash", "wrong passphrase", false); !errors.Is(err, ErrPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	if err := ChangePassphrase(context.Background(), c, "MyGO-Clash", a.eng.Keys, "second passphrase"); err != nil {
		t.Fatal(err)
	}
	keys, _, err := OpenVault(context.Background(), c, "MyGO-Clash", "second passphrase", false)
	if err != nil || keys.ID != a.eng.Keys.ID {
		t.Fatalf("rewrapped vault: %v", err)
	}
	if _, _, err := OpenVault(context.Background(), c, "MyGO-Clash", "first passphrase", false); !errors.Is(err, ErrPassphrase) {
		t.Fatal("the old passphrase still opens the vault")
	}
	if _, err := webdav.New("http://dav.example.com", "u", "p", webdav.Options{}); !errors.Is(err, webdav.ErrInsecure) {
		t.Fatal("plain HTTP was allowed")
	}
}

func TestBackupsAndExport(t *testing.T) {
	s := newServer(t, true)
	a := newDevice(t, s, "laptop", "backup passphrase", "ask")
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		snap := Snapshot{Created: time.Date(2026, 10, 6, 12, i, 0, 0, time.UTC), Files: map[string][]byte{"settings": []byte(fmt.Sprintf(`{"n":%d}`, i))}}
		if _, err := a.eng.UploadBackup(ctx, snap, 3); err != nil {
			t.Fatal(err)
		}
	}
	list, err := a.eng.ListBackups(ctx)
	if err != nil || len(list) != 3 || list[0].Device != "laptop" {
		t.Fatalf("backups %+v %v", list, err)
	}
	snap, err := a.eng.DownloadBackup(ctx, list[0].Name)
	if err != nil || string(snap.Files["settings"]) != `{"n":3}` {
		t.Fatalf("download %+v %v", snap, err)
	}
	file, err := Export(snap, "export password")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(file, []byte(`"n"`)) {
		t.Fatal("export is not encrypted")
	}
	if _, err := Import(file, "nope nope nope"); err == nil {
		t.Fatal("wrong password imported")
	}
	back, err := Import(file, "export password")
	if err != nil || string(back.Files["settings"]) != `{"n":3}` {
		t.Fatalf("import %v", err)
	}
}

func TestMergeJSON(t *testing.T) {
	base := `{"a":1,"b":{"c":1,"d":1},"gone":1,"list":[1]}`
	local := `{"a":2,"b":{"c":1,"d":2},"gone":1,"list":[1,2]}`
	remote := `{"a":1,"b":{"c":3,"d":3},"list":[1,3],"new":true}`
	out, ok := MergeJSON([]byte(base), []byte(local), []byte(remote), false)
	if !ok {
		t.Fatal("no merge")
	}
	var got map[string]any
	_ = json.Unmarshal(out, &got)
	b := got["b"].(map[string]any)
	if got["a"] != 2.0 || b["c"] != 3.0 || b["d"] != 2.0 || got["new"] != true || got["gone"] != nil {
		t.Fatalf("merged %s", out)
	}
	if l := got["list"].([]any); len(l) != 2 || l[1] != 2.0 {
		t.Fatalf("list %v", l)
	}
}
