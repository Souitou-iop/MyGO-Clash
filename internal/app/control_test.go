package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mygo-clash/mygo-clash/internal/agent"
	"github.com/mygo-clash/mygo-clash/internal/config"
	"github.com/mygo-clash/mygo-clash/internal/coreapi"
)

func TestControlHandlersMatchMethods(t *testing.T) {
	for _, m := range agent.Methods {
		if controlHandlers[m.Name] == nil {
			t.Errorf("no handler for %s", m.Name)
		}
	}
	for name := range controlHandlers {
		if _, ok := agent.MethodByName(name); !ok {
			t.Errorf("handler %s is not in agent.Methods", name)
		}
	}
}

func TestExitOf(t *testing.T) {
	v := ProxiesView{Mode: "rule", Groups: []ProxyGroup{
		{Name: "Hidden", Hidden: true, Now: "X"},
		{Name: "Proxy", Now: "Auto", All: []ProxyItem{{Name: "Auto", Type: "URLTest", Delay: -1}}},
		{Name: "Auto", Now: "HK", All: []ProxyItem{{Name: "HK", Type: "Trojan", Delay: 88}}},
	}, Global: &ProxyGroup{Name: "GLOBAL", Now: "JP", All: []ProxyItem{{Name: "JP", Type: "Vmess", Delay: 120}}}}
	e := exitOf(v)
	if strings.Join(e.Chain, ">") != "Proxy>Auto" || e.Node != "HK" || e.Delay != 88 || e.Type != "Trojan" {
		t.Fatalf("rule: %+v", e)
	}
	v.Mode = "global"
	if e := exitOf(v); e.Node != "JP" || e.Delay != 120 {
		t.Fatalf("global: %+v", e)
	}
	v.Mode = "direct"
	if e := exitOf(v); e.Node != "DIRECT" {
		t.Fatalf("direct: %+v", e)
	}
	// A loop of groups ends.
	loop := ProxiesView{Mode: "rule", Groups: []ProxyGroup{{Name: "A", Now: "B"}, {Name: "B", Now: "A"}}}
	if e := exitOf(loop); len(e.Chain) != 2 {
		t.Fatalf("loop: %+v", e)
	}
}

func TestFilterConns(t *testing.T) {
	now := time.Now()
	all := []coreapi.Connection{
		{ID: "1", Metadata: coreapi.ConnectionMetadata{Host: "github.com", Process: "git"}, Chains: []string{"HK", "Proxy"}, Start: now.Add(-time.Minute)},
		{ID: "2", Metadata: coreapi.ConnectionMetadata{DestinationIP: "1.1.1.1"}, Rule: "Match", Start: now},
	}
	got := filterConns(all, "")
	if len(got) != 2 || got[0].ID != "2" || got[0].Host != "1.1.1.1" {
		t.Fatalf("%+v", got)
	}
	if got := filterConns(all, "HK"); len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("%+v", got)
	}
	if got := filterConns(all, "GIT"); len(got) != 1 {
		t.Fatalf("%+v", got)
	}
}

func TestTailLogs(t *testing.T) {
	logs := []coreapi.LogEvent{
		{Type: "debug", Payload: "a"}, {Type: "info", Payload: "b"},
		{Type: "warning", Payload: "fetch https://sub.example.com/x?token=SECRET failed"}, {Type: "error", Payload: "d"},
	}
	got := tailLogs(logs, 2, 1)
	if len(got) != 2 || got[1].Payload != "d" || strings.Contains(got[0].Payload, "SECRET") {
		t.Fatalf("%+v", got)
	}
	if got := tailLogs(logs, 10, 0); len(got) != 4 {
		t.Fatalf("%+v", got)
	}
}

func TestMaskSettings(t *testing.T) {
	st := config.Defaults()
	st.Clash.Controller.Secret = "s3cret"
	st.Clash.LANAuth.Password = "pw"
	st.Sync.URL, st.Sync.Username = "https://dav.example.com/u", "me"
	st.Tailscale.ControlURL = "https://hs.example.com"
	b, _ := json.Marshal(maskSettings(st))
	for _, s := range []string{"s3cret", `"pw"`, "dav.example.com", `"me"`, "hs.example.com"} {
		if strings.Contains(string(b), s) {
			t.Errorf("%s leaked", s)
		}
	}
	if !st.Agent.Enabled {
		t.Error("agent control is off by default")
	}
}

func TestPatchChecks(t *testing.T) {
	data, _ := json.Marshal(config.Defaults())
	var doc any
	_ = json.Unmarshal(data, &doc)
	if err := unknownKeys(doc, map[string]any{"clash": map[string]any{"mixedPort": 1.0}}, ""); err != nil {
		t.Fatal(err)
	}
	if err := unknownKeys(doc, map[string]any{"clash": map[string]any{"mixdPort": 1.0}}, ""); err == nil || !strings.Contains(err.Error(), "clash.mixdPort") {
		t.Fatalf("err = %v", err)
	}
	if err := unknownKeys(doc, map[string]any{"dns": map[string]any{"profiles": map[string]any{"uid1": true}}}, ""); err != nil {
		t.Fatal(err)
	}
	v, err := lookupPath(doc, "clash.mixedPort")
	if err != nil || v.(float64) != float64(config.Defaults().Clash.MixedPort) {
		t.Fatalf("%v %v", v, err)
	}
	if _, err := lookupPath(doc, "clash.nothing"); err == nil {
		t.Fatal("no error")
	}
	if v, err := lookupPath(doc, "webUis.0"); err != nil || !strings.HasPrefix(v.(string), "https://") {
		t.Fatalf("%v %v", v, err)
	}
}

func TestServeControlToken(t *testing.T) {
	a := &App{ready: make(chan struct{})}
	close(a.ready)
	call := func(token, body string) (int, agent.Response) {
		r := httptest.NewRequest(http.MethodPost, agent.CallPath, strings.NewReader(body))
		if token != "" {
			r.Header.Set(agent.TokenHeader, token)
		}
		w := httptest.NewRecorder()
		a.serveControl(w, r, "good")
		var resp agent.Response
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		return w.Code, resp
	}
	if code, _ := call("", `{"method":"status"}`); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
	if code, _ := call("bad", `{"method":"status"}`); code != http.StatusUnauthorized {
		t.Fatalf("bad token: %d", code)
	}
	if code, resp := call("good", `{"method":"nope"}`); code != http.StatusNotFound || resp.Error == "" {
		t.Fatalf("unknown method: %d %+v", code, resp)
	}
	if code, _ := call("good", `{`); code != http.StatusBadRequest {
		t.Fatalf("bad body: %d", code)
	}
}

func TestDecodeArgsRefusesUnknown(t *testing.T) {
	if _, err := decodeArgs[struct {
		On bool `json:"on"`
	}](json.RawMessage(`{"of":true}`)); err == nil {
		t.Fatal("an unknown argument passed")
	}
}

func TestAgentIsDeviceSetting(t *testing.T) {
	st := config.Defaults()
	if _, ok := syncable(st)["agent"]; ok {
		t.Fatal("agent control syncs between devices")
	}
}
