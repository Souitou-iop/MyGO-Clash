package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// The MCP server speaks JSON-RPC 2.0 over stdio, one message a line, as
// the Model Context Protocol's stdio transport says. It serves the
// control socket's methods as tools, and connects to the app at each call,
// so it can start before the app does.

var mcpVersions = []string{"2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"}

const mcpInstructions = `Controls MyGO-Clash, a mihomo (Clash Meta) proxy client running on this computer.
Start with status. Use proxies_list to find group and node names before proxies_select; proxies_delay tests nodes.
sysproxy_set, tun_set and mode_set are the main switches. Tools marked DANGEROUS (profiles_delete, settings_patch)
can lose data or cut the network: ask the user first. Secrets and subscription URLs come back masked.
If a tool says the app is not running, call app_launch.`

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcReply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// caller is what the MCP server calls methods with: a running app, found
// anew when it went away.
type caller interface {
	call(ctx context.Context, method string, args json.RawMessage) (json.RawMessage, error)
	launch(ctx context.Context) (string, error)
}

type appCaller struct {
	targets []Target
	mu      sync.Mutex
	c       *Client
}

func (a *appCaller) client(ctx context.Context) (*Client, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.c != nil {
		return a.c, nil
	}
	c, err := Dial(ctx, a.targets)
	if err != nil {
		return nil, err
	}
	a.c = c
	return c, nil
}

func (a *appCaller) call(ctx context.Context, method string, args json.RawMessage) (json.RawMessage, error) {
	c, err := a.client(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := c.CallRaw(ctx, method, args)
	var ue *url.Error
	if err != nil && (errors.As(err, &ue) || strings.Contains(err.Error(), "bad control token")) {
		// The app restarted (a new socket, a new token): find it again.
		a.mu.Lock()
		a.c = nil
		a.mu.Unlock()
		if c, derr := a.client(ctx); derr == nil {
			return c.CallRaw(ctx, method, args)
		}
	}
	return raw, err
}

func (a *appCaller) launch(ctx context.Context) (string, error) {
	c, err := Launch(ctx, a.targets)
	if err != nil {
		return "", err
	}
	a.mu.Lock()
	a.c = c
	a.mu.Unlock()
	return c.Target.Name, nil
}

// RunMCP serves MCP over in and out until in ends.
func RunMCP(in io.Reader, out io.Writer, version string, dev bool) error {
	return serveMCP(in, out, version, &appCaller{targets: Targets(dev)})
}

func serveMCP(in io.Reader, out io.Writer, version string, app caller) error {
	var wmu sync.Mutex
	enc := json.NewEncoder(out)
	write := func(r rpcReply) {
		r.JSONRPC = "2.0"
		wmu.Lock()
		defer wmu.Unlock()
		_ = enc.Encode(r)
	}
	var wg sync.WaitGroup
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg rpcMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			write(rpcReply{ID: json.RawMessage("null"), Error: &rpcError{-32700, "parse error"}})
			continue
		}
		if len(msg.ID) == 0 || msg.Method == "" {
			continue // a notification, or an answer we did not ask for
		}
		if msg.Method == "tools/call" {
			// Calls can take long (a delay test, a download): answer the
			// rest meanwhile.
			wg.Add(1)
			go func(msg rpcMessage) {
				defer wg.Done()
				write(rpcReply{ID: msg.ID, Result: callTool(app, msg.Params)})
			}(msg)
			continue
		}
		result, rerr := handleMCP(msg, version)
		write(rpcReply{ID: msg.ID, Result: result, Error: rerr})
	}
	wg.Wait()
	return sc.Err()
}

func handleMCP(msg rpcMessage, version string) (any, *rpcError) {
	switch msg.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		v := mcpVersions[1]
		if slices.Contains(mcpVersions, p.ProtocolVersion) {
			v = p.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": v,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "mygo-clash", "title": "MyGO-Clash", "version": version},
			"instructions":    mcpInstructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolList()}, nil
	case "resources/list":
		return map[string]any{"resources": []any{}}, nil
	case "prompts/list":
		return map[string]any{"prompts": []any{}}, nil
	}
	return nil, &rpcError{-32601, "method not found: " + msg.Method}
}

const launchTool = "app_launch"

func toolList() []map[string]any {
	tools := []map[string]any{}
	for _, m := range Methods {
		tools = append(tools, map[string]any{
			"name":        m.Tool(),
			"title":       m.Name,
			"description": m.Desc,
			"inputSchema": m.Schema(),
			"annotations": map[string]any{
				"readOnlyHint":    m.ReadOnly,
				"destructiveHint": m.Destructive,
				"openWorldHint":   false,
			},
		})
	}
	tools = append(tools, map[string]any{
		"name":        launchTool,
		"title":       "app.launch",
		"description": "Start MyGO-Clash if it is not running (in the background, without its window) and wait until it answers.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false},
		"annotations": map[string]any{"readOnlyHint": false, "destructiveHint": false, "idempotentHint": true, "openWorldHint": false},
	})
	return tools
}

func toolText(text string, isErr bool) map[string]any {
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}, "isError": isErr}
}

func callTool(app caller, params json.RawMessage) map[string]any {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return toolText("bad parameters: "+err.Error(), true)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	if p.Name == launchTool {
		name, err := app.launch(ctx)
		if err != nil {
			return toolText(err.Error(), true)
		}
		return toolText(name+" is running.", false)
	}
	m, ok := MethodByName(p.Name)
	if !ok {
		return toolText("unknown tool "+p.Name, true)
	}
	raw, err := app.call(ctx, m.Name, p.Arguments)
	if err != nil {
		if errors.Is(err, ErrNotRunning) {
			return toolText(err.Error()+". Call app_launch to start it.", true)
		}
		return toolText(err.Error(), true)
	}
	var v any
	if len(raw) == 0 || json.Unmarshal(raw, &v) != nil {
		return toolText("ok", false)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	res := toolText(string(b), false)
	if obj, ok := v.(map[string]any); ok {
		res["structuredContent"] = obj
	}
	return res
}

// MCPUsage is shown for mcp --help.
func MCPUsage(w io.Writer, prog string) {
	fmt.Fprintf(w, "Usage: %s mcp [--dev]\n\nServes MyGO-Clash's tools to MCP clients over stdio (JSON-RPC, one message a line).\n", prog)
}
