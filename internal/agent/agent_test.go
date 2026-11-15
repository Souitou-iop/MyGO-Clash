package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaskURL(t *testing.T) {
	for in, want := range map[string]string{
		"": "",
		"https://sub.example.com/api/v1/client/subscribe?token=abc": "https://sub.example.com/" + Redacted,
		"https://user:pass@example.com":                             "https://example.com/" + Redacted,
		"http://example.com:8080":                                   "http://example.com:8080",
		"not a url":                                                 Redacted,
	} {
		if got := MaskURL(in); got != want {
			t.Errorf("MaskURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScrubURLs(t *testing.T) {
	in := `Get "https://sub.example.com/s?token=SECRET": timeout (see https://a.example/x?k=1).`
	got := ScrubURLs(in)
	if strings.Contains(got, "SECRET") || strings.Contains(got, "k=1") {
		t.Fatalf("leaked: %s", got)
	}
	want := `Get "https://sub.example.com/[redacted]": timeout (see https://a.example/[redacted]).`
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestHasRedacted(t *testing.T) {
	if HasRedacted(map[string]any{"a": map[string]any{"b": []any{1.0, "x"}}}) {
		t.Fatal("false positive")
	}
	if !HasRedacted(map[string]any{"a": map[string]any{"b": []any{Redacted}}}) {
		t.Fatal("missed")
	}
}

func TestParseOpts(t *testing.T) {
	o, err := parseOpts([]string{"--json", "conns", "google", "--limit", "5", "-n=3", "--all", "-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !o.has("json") || !o.has("all") || o.str("limit") != "5" || o.str("n") != "3" {
		t.Fatalf("flags %v", o.flags)
	}
	if strings.Join(o.pos, ",") != "conns,google,-1" {
		t.Fatalf("pos %v", o.pos)
	}
	if _, err := parseOpts([]string{"--limit"}); err == nil {
		t.Fatal("a missing value passed")
	}
}

func TestPatchOf(t *testing.T) {
	p, err := patchOf([]string{"clash.allowLan=true", "clash.mixedPort=7890", "theme=dark", "systemProxy.bypass=[\"a\"]"})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(p)
	want := `{"clash":{"allowLan":true,"mixedPort":7890},"systemProxy":{"bypass":["a"]},"theme":"dark"}`
	if string(b) != want {
		t.Fatalf("got %s", b)
	}
	if _, err := patchOf([]string{"novalue"}); err == nil {
		t.Fatal("no error")
	}
}

func TestMethods(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Methods {
		if seen[m.Tool()] {
			t.Errorf("duplicate tool %s", m.Tool())
		}
		seen[m.Tool()] = true
		if m.Desc == "" {
			t.Errorf("%s has no description", m.Name)
		}
		if m.ReadOnly && m.Destructive {
			t.Errorf("%s is read-only and destructive", m.Name)
		}
		s := m.Schema()
		if s["type"] != "object" {
			t.Errorf("%s: schema %v", m.Name, s)
		}
		if _, err := json.Marshal(s); err != nil {
			t.Error(err)
		}
	}
	for _, name := range []string{"profiles.delete", "settings.patch"} {
		m, _ := MethodByName(name)
		if !m.Destructive || !strings.Contains(m.Desc, "DANGEROUS") {
			t.Errorf("%s is not marked dangerous", name)
		}
	}
	if m, ok := MethodByName("proxies_select"); !ok || m.Name != "proxies.select" {
		t.Fatal("tool name lookup")
	}
	// Every command of the command line calls a method that exists.
	for _, c := range commands {
		if _, ok := MethodByName(c.method); !ok {
			t.Errorf("command %s calls unknown method %s", c.name, c.method)
		}
	}
}

// serveFake serves a fake app on a socket in a temporary directory.
func serveFake(t *testing.T, token string, h func(Request) Response) Target {
	t.Helper()
	dir, err := os.MkdirTemp("", "agent")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "control.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	tokenF := filepath.Join(dir, "control.token")
	_ = os.WriteFile(tokenF, []byte(token+"\n"), 0o600)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(TokenHeader) != token {
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(Response{Error: "bad control token"})
			return
		}
		var req Request
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(h(req))
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return Target{Name: "fake", Socket: sock, Token: tokenF}
}

func TestClient(t *testing.T) {
	tg := serveFake(t, "tok", func(r Request) Response {
		if r.Method == "fail" {
			return Response{Error: "it failed"}
		}
		return Response{Result: json.RawMessage(`{"method":"` + r.Method + `","args":` + string(r.Args) + `}`)}
	})
	ctx := context.Background()
	if _, err := Dial(ctx, []Target{{Socket: filepath.Join(t.TempDir(), "none.sock")}}); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("err = %v", err)
	}
	c, err := Dial(ctx, []Target{{Socket: "/nonexistent/x.sock"}, tg})
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Method string
		Args   map[string]int
	}
	if err := c.Call(ctx, "status", map[string]int{"n": 1}, &out); err != nil || out.Method != "status" || out.Args["n"] != 1 {
		t.Fatalf("%v %+v", err, out)
	}
	if err := c.Call(ctx, "fail", nil, nil); err == nil || err.Error() != "it failed" {
		t.Fatalf("err = %v", err)
	}
	c.token = "wrong"
	if err := c.Call(ctx, "status", nil, nil); err == nil || !strings.Contains(err.Error(), "token") {
		t.Fatalf("a wrong token passed: %v", err)
	}
}

func TestRunCLI(t *testing.T) {
	tg := serveFake(t, "tok", func(r Request) Response {
		switch r.Method {
		case "status":
			return Response{Result: json.RawMessage(`{"app":"MyGO-Clash","version":"1","core":{"status":"running"},"mode":"rule","profile":{"name":"P"},"exit":{"chain":["Proxy"],"node":"HK","delay":42}}`)}
		case "sysproxy.set":
			return Response{Result: r.Args}
		}
		return Response{Error: "no method " + r.Method}
	})
	t.Setenv("MYGO_CLASH_SOCKET", tg.Socket)
	run := func(args ...string) (int, string, string) {
		var out, errb bytes.Buffer
		code := RunCLI(args, &out, &errb, "mygo-clash")
		return code, out.String(), errb.String()
	}
	if code, out, _ := run("status"); code != 0 || !strings.Contains(out, "Proxy → HK (42 ms)") {
		t.Fatalf("%d %s", code, out)
	}
	if code, out, _ := run("--json", "proxy", "on"); code != 0 || !strings.Contains(out, `"on": true`) {
		t.Fatalf("%d %s", code, out)
	}
	if code, _, _ := run("proxy", "maybe"); code != ExitUsage {
		t.Fatalf("code %d", code)
	}
	if code, out, _ := run("--json", "restart"); code != ExitError || !strings.Contains(out, `"error"`) {
		t.Fatalf("%d %s", code, out)
	}
	if code, _, _ := run("delete", "x"); code != ExitUsage {
		t.Fatalf("delete without --yes: %d", code)
	}
	t.Setenv("MYGO_CLASH_SOCKET", filepath.Join(t.TempDir(), "none.sock"))
	if code, _, errs := run("status"); code != ExitNotRunning || !strings.Contains(errs, "launch") {
		t.Fatalf("%d %s", code, errs)
	}
}

type fakeCaller struct{ calls []string }

func (f *fakeCaller) call(_ context.Context, method string, args json.RawMessage) (json.RawMessage, error) {
	f.calls = append(f.calls, method)
	if method == "status" {
		return json.RawMessage(`{"mode":"rule"}`), nil
	}
	return nil, ErrNotRunning
}

func (f *fakeCaller) launch(context.Context) (string, error) { return "MyGO-Clash", nil }

func TestMCP(t *testing.T) {
	in := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"proxies_list","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"nope"}`,
		`{"jsonrpc":"2.0","id":6,"method":"ping"}`,
		`not json`,
	}, "\n")
	var out bytes.Buffer
	f := &fakeCaller{}
	if err := serveMCP(strings.NewReader(in), &out, "1.0", f); err != nil {
		t.Fatal(err)
	}
	replies := map[string]map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var r map[string]any
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		if r["jsonrpc"] != "2.0" {
			t.Fatalf("no jsonrpc: %s", line)
		}
		id, _ := json.Marshal(r["id"])
		replies[string(id)] = r
	}
	if len(replies) != 7 { // every request but the notification
		t.Fatalf("%d replies: %s", len(replies), out.String())
	}
	init := replies["1"]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" || init["capabilities"].(map[string]any)["tools"] == nil {
		t.Fatalf("initialize: %v", init)
	}
	tools := replies["2"]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != len(Methods)+1 {
		t.Fatalf("%d tools", len(tools))
	}
	for _, tl := range tools {
		m := tl.(map[string]any)
		if m["inputSchema"].(map[string]any)["type"] != "object" || m["description"] == "" {
			t.Fatalf("tool %v", m)
		}
	}
	ok := replies["3"]["result"].(map[string]any)
	if ok["isError"] != false || ok["structuredContent"].(map[string]any)["mode"] != "rule" {
		t.Fatalf("status: %v", ok)
	}
	bad := replies["4"]["result"].(map[string]any)
	if bad["isError"] != true || !strings.Contains(bad["content"].([]any)[0].(map[string]any)["text"].(string), "app_launch") {
		t.Fatalf("not running: %v", bad)
	}
	if replies["5"]["error"].(map[string]any)["code"] != -32601.0 {
		t.Fatalf("unknown method: %v", replies["5"])
	}
	if replies["null"]["error"].(map[string]any)["code"] != -32700.0 {
		t.Fatalf("parse error: %v", replies["null"])
	}
}
