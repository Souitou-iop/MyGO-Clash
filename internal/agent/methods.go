// Package agent lets scripts and AI agents drive a running MyGO-Clash: the
// methods of its local control socket, a client for them, the command line
// (mygo-clash ctl …) and an MCP server over stdio (mygo-clash mcp).
//
// The app serves the methods on a Unix socket in a directory only the user
// can enter, and asks for a token it writes beside the socket, readable by
// the user alone. Nothing listens on the network.
package agent

// Param is a parameter of a method.
type Param struct {
	Name     string
	Type     string // string, boolean, integer, object, array (of strings)
	Desc     string
	Required bool
	Enum     []string
}

// Method is a method of the control socket, and the tool MCP clients see.
type Method struct {
	Name string
	Desc string
	// ReadOnly methods change nothing; Destructive ones may lose data or
	// cut the network, which their description says.
	ReadOnly    bool
	Destructive bool
	Params      []Param
}

// Tool is the method's name as an MCP tool: "proxies.select" →
// "proxies_select".
func (m Method) Tool() string {
	b := []byte(m.Name)
	for i, c := range b {
		if c == '.' {
			b[i] = '_'
		}
	}
	return string(b)
}

// Methods are what the control socket serves.
var Methods = []Method{
	{Name: "status", ReadOnly: true,
		Desc: "Overall state of MyGO-Clash: core status, system proxy, TUN, outbound mode, current profile, the exit node the main group goes through, mixed port and current speed."},
	{Name: "proxies.list", ReadOnly: true,
		Desc: "Proxy groups with their selected member and members (type, last delay in ms: 0 failed, -1 untested). Pass group to list only that group's nodes.",
		Params: []Param{
			{Name: "group", Type: "string", Desc: "Only this group."},
			{Name: "hidden", Type: "boolean", Desc: "Include groups the profile hides."},
		}},
	{Name: "proxies.select",
		Desc: "Select a node (or a nested group) in a selector group. Takes effect at once; with auto-close on, connections through the group reconnect.",
		Params: []Param{
			{Name: "group", Type: "string", Desc: "Selector group, as proxies.list names it.", Required: true},
			{Name: "node", Type: "string", Desc: "Member of the group to select.", Required: true},
		}},
	{Name: "proxies.delay", ReadOnly: true,
		Desc: "Test latency through one node, or through every member of a group. Returns ms; 0 means the test failed or timed out.",
		Params: []Param{
			{Name: "node", Type: "string", Desc: "A node to test."},
			{Name: "group", Type: "string", Desc: "A group whose members to test at once (instead of node)."},
			{Name: "url", Type: "string", Desc: "Test URL; the settings' latency URL by default."},
		}},
	{Name: "connections.list", ReadOnly: true,
		Desc: "Open connections, newest first: host, process, rule, proxy chain, bytes and speed.",
		Params: []Param{
			{Name: "filter", Type: "string", Desc: "Keep connections whose host, process, rule or chain contains this text."},
			{Name: "limit", Type: "integer", Desc: "At most this many (default 50, 0 for all)."},
		}},
	{Name: "connections.close", Destructive: true,
		Desc: "Close connections by id, or every connection with all=true. Interrupts downloads, calls and streams in progress.",
		Params: []Param{
			{Name: "ids", Type: "array", Desc: "Connection ids from connections.list."},
			{Name: "all", Type: "boolean", Desc: "Close every connection."},
		}},
	{Name: "traffic", ReadOnly: true,
		Desc: "Current upload/download speed (bytes/s), totals since the core started, open connections and core memory."},
	{Name: "stats", ReadOnly: true,
		Desc: "Recorded traffic statistics of this device: totals, by day, top apps, sites, nodes and rules.",
		Params: []Param{
			{Name: "range", Type: "string", Desc: "Period (default today).", Enum: []string{"today", "7d", "30d", "all"}},
			{Name: "top", Type: "integer", Desc: "How many top items per list (default 10)."},
		}},
	{Name: "profiles.list", ReadOnly: true,
		Desc: "Profiles (subscriptions and local configurations) and which is current. Subscription URLs are masked."},
	{Name: "profiles.import",
		Desc: "Add a subscription by URL; it downloads first. A URL already added is reused. The new profile does not become current unless activate is set.",
		Params: []Param{
			{Name: "url", Type: "string", Desc: "Subscription URL (http or https).", Required: true},
			{Name: "name", Type: "string", Desc: "Name for the profile; the provider's by default."},
			{Name: "activate", Type: "boolean", Desc: "Make it the current profile afterwards."},
		}},
	{Name: "profiles.update",
		Desc: "Download remote profiles again: one by uid, or every remote one with all=true. The current one re-applies.",
		Params: []Param{
			{Name: "uid", Type: "string", Desc: "Profile uid or name."},
			{Name: "all", Type: "boolean", Desc: "Update every remote profile."},
		}},
	{Name: "profiles.activate",
		Desc: "Make a profile current and apply it. If the core rejects it, the previous profile stays.",
		Params: []Param{
			{Name: "profile", Type: "string", Desc: "Profile uid or name.", Required: true},
		}},
	{Name: "profiles.delete", Destructive: true,
		Desc: "DANGEROUS: permanently delete a profile and its extensions (merge, script, rules). Cannot be undone; a subscription must be imported again. Requires confirm=true.",
		Params: []Param{
			{Name: "uid", Type: "string", Desc: "Profile uid or name.", Required: true},
			{Name: "confirm", Type: "boolean", Desc: "Must be true.", Required: true},
		}},
	{Name: "settings.get", ReadOnly: true,
		Desc: "The app's settings as JSON, secrets masked as \"[redacted]\". Pass path (dot separated, e.g. clash.mixedPort) for one value.",
		Params: []Param{
			{Name: "path", Type: "string", Desc: "Dot-separated key, e.g. systemProxy.bypass."},
		}},
	{Name: "settings.patch", Destructive: true,
		Desc: "DANGEROUS: change settings with a JSON merge patch, e.g. {\"clash\":{\"allowLan\":true}}. Changing ports (clash.mixedPort…), clash.controller, tun, dns or coreMode reloads or restarts the core and can cut every connection or the network. Masked values (\"[redacted]\") are refused. Prefer sysproxy.set, tun.set and mode.set for those switches.",
		Params: []Param{
			{Name: "patch", Type: "object", Desc: "JSON merge patch of the settings.", Required: true},
		}},
	{Name: "sysproxy.set",
		Desc:   "Turn the system proxy on or off.",
		Params: []Param{{Name: "on", Type: "boolean", Required: true}}},
	{Name: "tun.set",
		Desc:   "Turn TUN mode (all traffic through the core) on or off. Needs the privileged service; if it is not installed, install it from the app first.",
		Params: []Param{{Name: "on", Type: "boolean", Required: true}}},
	{Name: "mode.set",
		Desc:   "Switch the outbound mode: rule (by rules), global (everything through the GLOBAL group) or direct (nothing proxied).",
		Params: []Param{{Name: "mode", Type: "string", Required: true, Enum: []string{"rule", "global", "direct"}}}},
	{Name: "rules.match", ReadOnly: true,
		Desc: "Which rule a connection to a domain, IP or URL would hit, and the proxy it would go through.",
		Params: []Param{
			{Name: "target", Type: "string", Desc: "Domain, IP address or URL.", Required: true},
			{Name: "network", Type: "string", Enum: []string{"tcp", "udp"}},
			{Name: "port", Type: "integer"},
			{Name: "process", Type: "string", Desc: "Program name, for process rules."},
		}},
	{Name: "logs.tail", ReadOnly: true,
		Desc: "The core's latest log lines, oldest first.",
		Params: []Param{
			{Name: "lines", Type: "integer", Desc: "How many (default 50, at most 2000)."},
			{Name: "level", Type: "string", Desc: "Minimum level.", Enum: []string{"debug", "info", "warning", "error"}},
		}},
	{Name: "tailscale.status", ReadOnly: true,
		Desc: "Tailscale: mode, backend state, this node, peers (name, IPs, online, exit node) and the exit node in use."},
	{Name: "core.restart", Destructive: true,
		Desc: "Restart the mihomo core. Every connection drops for a moment."},
}

// MethodByName returns the method of that name, or of that tool name.
func MethodByName(name string) (Method, bool) {
	for _, m := range Methods {
		if m.Name == name || m.Tool() == name {
			return m, true
		}
	}
	return Method{}, false
}

// Schema is the method's input as a JSON Schema.
func (m Method) Schema() map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, p := range m.Params {
		s := map[string]any{"type": p.Type}
		if p.Type == "array" {
			s["items"] = map[string]any{"type": "string"}
		}
		if p.Desc != "" {
			s["description"] = p.Desc
		}
		if len(p.Enum) > 0 {
			s["enum"] = p.Enum
		}
		props[p.Name] = s
		if p.Required {
			required = append(required, p.Name)
		}
	}
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}
