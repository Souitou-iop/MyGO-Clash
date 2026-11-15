package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
)

// Exit codes of the command line.
const (
	ExitOK         = 0
	ExitError      = 1
	ExitUsage      = 2
	ExitNotRunning = 3
)

// opts are the flags of a command line: --name, --name=value, -n value.
type opts struct {
	pos   []string
	flags map[string]string
}

func (o opts) has(name string) bool   { _, ok := o.flags[name]; return ok }
func (o opts) str(name string) string { return o.flags[name] }
func (o opts) int(name string) (*int, error) {
	v, ok := o.flags[name]
	if !ok {
		return nil, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return nil, usageErr("--%s takes a number", name)
	}
	return &n, nil
}

// valued are the flags that take a value.
var valued = map[string]bool{"limit": true, "n": true, "lines": true, "level": true, "url": true, "group": true, "name": true, "top": true, "port": true, "process": true, "network": true, "timeout": true}

func parseOpts(args []string) (opts, error) {
	o := opts{flags: map[string]string{}}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			o.pos = append(o.pos, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(a, "-") || a == "-" || isNumber(a) {
			o.pos = append(o.pos, a)
			continue
		}
		name := strings.TrimLeft(a, "-")
		if k, v, ok := strings.Cut(name, "="); ok {
			o.flags[k] = v
			continue
		}
		if valued[name] {
			if i+1 >= len(args) {
				return o, usageErr("--%s takes a value", name)
			}
			o.flags[name] = args[i+1]
			i++
			continue
		}
		o.flags[name] = ""
	}
	return o, nil
}

func isNumber(s string) bool { _, err := strconv.ParseFloat(s, 64); return err == nil }

type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usageErr(f string, a ...any) error { return usageError{fmt.Sprintf(f, a...)} }

// command is a command of the command line: the method it calls and the
// arguments it makes of the line, and how it prints the result for people.
type command struct {
	name, args, summary string
	method              string
	build               func(o opts) (any, error)
	human               func(w io.Writer, raw json.RawMessage) error
}

func onOff(o opts) (any, error) {
	if len(o.pos) != 1 || (o.pos[0] != "on" && o.pos[0] != "off") {
		return nil, usageErr("say on or off")
	}
	return map[string]bool{"on": o.pos[0] == "on"}, nil
}

func one(o opts, what string) (string, error) {
	if len(o.pos) != 1 {
		return "", usageErr("give one %s", what)
	}
	return o.pos[0], nil
}

var commands = []command{
	{name: "status", summary: "core, system proxy, TUN, mode, profile and the exit node", method: "status", human: printStatus},
	{name: "groups", args: "[group]", summary: "proxy groups, or the nodes of one", method: "proxies.list",
		build: func(o opts) (any, error) {
			m := map[string]any{}
			if len(o.pos) > 0 {
				m["group"] = strings.Join(o.pos, " ")
			}
			if o.has("hidden") {
				m["hidden"] = true
			}
			return m, nil
		}, human: printGroups},
	{name: "select", args: "<group> <node>", summary: "select a node in a selector group", method: "proxies.select",
		build: func(o opts) (any, error) {
			if len(o.pos) != 2 {
				return nil, usageErr("give the group and the node (quote names with spaces)")
			}
			return map[string]string{"group": o.pos[0], "node": o.pos[1]}, nil
		}},
	{name: "delay", args: "<node> | --group <group> [--url u]", summary: "test the latency of a node or of a whole group", method: "proxies.delay",
		build: func(o opts) (any, error) {
			m := map[string]string{}
			if o.has("url") {
				m["url"] = o.str("url")
			}
			if o.has("group") {
				m["group"] = o.str("group")
				return m, nil
			}
			n, err := one(o, "node, or --group")
			m["node"] = n
			return m, err
		}, human: printDelay},
	{name: "conns", args: "[filter] [--limit n]", summary: "open connections, newest first", method: "connections.list",
		build: func(o opts) (any, error) {
			m := map[string]any{}
			if len(o.pos) > 0 {
				m["filter"] = strings.Join(o.pos, " ")
			}
			n, err := o.int("limit")
			if n != nil {
				m["limit"] = *n
			}
			return m, err
		}, human: printConns},
	{name: "close", args: "<id>... | --all", summary: "close connections", method: "connections.close",
		build: func(o opts) (any, error) {
			if o.has("all") {
				return map[string]bool{"all": true}, nil
			}
			if len(o.pos) == 0 {
				return nil, usageErr("give connection ids, or --all")
			}
			return map[string]any{"ids": o.pos}, nil
		}},
	{name: "traffic", summary: "current speed and totals", method: "traffic", human: printTraffic},
	{name: "stats", args: "[today|7d|30d|all] [--top n]", summary: "recorded traffic statistics", method: "stats",
		build: func(o opts) (any, error) {
			m := map[string]any{}
			if len(o.pos) > 0 {
				m["range"] = o.pos[0]
			}
			n, err := o.int("top")
			if n != nil {
				m["top"] = *n
			}
			return m, err
		}, human: printStats},
	{name: "profiles", summary: "profiles, the current one marked (URLs masked)", method: "profiles.list", human: printProfiles},
	{name: "import", args: "<url> [--name n] [--activate]", summary: "add a subscription", method: "profiles.import",
		build: func(o opts) (any, error) {
			u, err := one(o, "URL")
			m := map[string]any{"url": u}
			if o.has("name") {
				m["name"] = o.str("name")
			}
			if o.has("activate") {
				m["activate"] = true
			}
			return m, err
		}},
	{name: "update", args: "<profile> | --all", summary: "download subscriptions again", method: "profiles.update",
		build: func(o opts) (any, error) {
			if o.has("all") {
				return map[string]bool{"all": true}, nil
			}
			p, err := one(o, "profile (uid or name), or --all")
			return map[string]string{"uid": p}, err
		}},
	{name: "use", args: "<profile>", summary: "make a profile current", method: "profiles.activate",
		build: func(o opts) (any, error) {
			p, err := one(o, "profile (uid or name)")
			return map[string]string{"profile": p}, err
		}},
	{name: "delete", args: "<profile> --yes", summary: "delete a profile for good", method: "profiles.delete",
		build: func(o opts) (any, error) {
			p, err := one(o, "profile (uid or name)")
			if err == nil && !o.has("yes") {
				err = usageErr("deleting %s cannot be undone: add --yes", p)
			}
			return map[string]any{"uid": p, "confirm": true}, err
		}},
	{name: "settings", args: "[path]", summary: "settings, secrets masked; a dot path for one value", method: "settings.get",
		build: func(o opts) (any, error) {
			if len(o.pos) > 0 {
				return map[string]string{"path": o.pos[0]}, nil
			}
			return nil, nil
		}},
	{name: "set", args: "<path>=<value>...", summary: "change settings (values are JSON, else text)", method: "settings.patch",
		build: func(o opts) (any, error) {
			if len(o.pos) == 0 {
				return nil, usageErr("give path=value, e.g. clash.allowLan=true")
			}
			patch, err := patchOf(o.pos)
			return map[string]any{"patch": patch}, err
		}, human: func(w io.Writer, _ json.RawMessage) error { _, err := fmt.Fprintln(w, "ok"); return err }},
	{name: "proxy", args: "on|off", summary: "the system proxy", method: "sysproxy.set", build: onOff},
	{name: "tun", args: "on|off", summary: "TUN mode", method: "tun.set", build: onOff},
	{name: "mode", args: "rule|global|direct", summary: "the outbound mode", method: "mode.set",
		build: func(o opts) (any, error) {
			m, err := one(o, "mode: rule, global or direct")
			return map[string]string{"mode": m}, err
		}},
	{name: "match", args: "<target> [--udp] [--port n] [--process p]", summary: "the rule and proxy a destination would take", method: "rules.match",
		build: func(o opts) (any, error) {
			t, err := one(o, "domain, IP or URL")
			m := map[string]any{"target": t}
			if o.has("udp") {
				m["network"] = "udp"
			}
			if o.has("process") {
				m["process"] = o.str("process")
			}
			if n, perr := o.int("port"); perr != nil {
				err = perr
			} else if n != nil {
				m["port"] = *n
			}
			return m, err
		}},
	{name: "logs", args: "[-n lines] [--level l]", summary: "the core's latest logs", method: "logs.tail",
		build: func(o opts) (any, error) {
			m := map[string]any{}
			n, err := o.int("n")
			if n == nil && err == nil {
				n, err = o.int("lines")
			}
			if n != nil {
				m["lines"] = *n
			}
			if o.has("level") {
				m["level"] = o.str("level")
			}
			return m, err
		}, human: printLogs},
	{name: "tailscale", summary: "Tailscale's state and peers", method: "tailscale.status", human: printTailscale},
	{name: "restart", summary: "restart the core", method: "core.restart"},
}

// patchOf turns path=value pairs into a merge patch.
func patchOf(pairs []string) (map[string]any, error) {
	patch := map[string]any{}
	for _, p := range pairs {
		k, v, ok := strings.Cut(p, "=")
		if !ok || k == "" {
			return nil, usageErr("%q is not path=value", p)
		}
		var val any
		if err := json.Unmarshal([]byte(v), &val); err != nil {
			val = v
		}
		keys := strings.Split(k, ".")
		m := patch
		for _, key := range keys[:len(keys)-1] {
			next, ok := m[key].(map[string]any)
			if !ok {
				next = map[string]any{}
				m[key] = next
			}
			m = next
		}
		m[keys[len(keys)-1]] = val
	}
	return patch, nil
}

func cliUsage(w io.Writer, prog string) {
	fmt.Fprintf(w, "Usage: %s ctl [--json] [--dev] <command> [arguments]\n\n", prog)
	fmt.Fprintln(w, "Drives the running MyGO-Clash through its local control socket.")
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, c := range commands {
		fmt.Fprintf(tw, "  %s %s\t%s\n", c.name, c.args, c.summary)
	}
	fmt.Fprintf(tw, "  launch\tstart the app if it is not running\n")
	fmt.Fprintf(tw, "  call <method> [json]\tcall a method with JSON arguments\n")
	fmt.Fprintf(tw, "  methods\tlist the methods\n")
	_ = tw.Flush()
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Flags:")
	fmt.Fprintln(w, "  --json   print the result as JSON, for scripts")
	fmt.Fprintln(w, "  --dev    talk to the development build")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Exit codes: 0 ok, 1 error, 2 usage, 3 the app is not running.\n")
	fmt.Fprintf(w, "MCP: %s mcp, a Model Context Protocol server over stdio.\n", prog)
}

// RunCLI runs a command line (what follows "ctl") and returns the exit
// code.
func RunCLI(args []string, stdout, stderr io.Writer, prog string) int {
	o, err := parseOpts(args)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitUsage
	}
	asJSON, dev := o.has("json"), o.has("dev")
	if len(o.pos) == 0 || o.has("h") || o.has("help") || o.pos[0] == "help" {
		cliUsage(stdout, prog)
		if len(o.pos) == 0 && !o.has("h") && !o.has("help") {
			return ExitUsage
		}
		return ExitOK
	}
	timeout := 3 * time.Minute
	if o.has("timeout") {
		d, err := time.ParseDuration(o.str("timeout"))
		if err != nil {
			fmt.Fprintln(stderr, "--timeout takes a duration, e.g. 30s")
			return ExitUsage
		}
		timeout = d
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	name, rest := o.pos[0], opts{pos: o.pos[1:], flags: o.flags}
	targets := Targets(dev)

	fail := func(err error) int {
		var ue usageError
		switch {
		case errors.As(err, &ue):
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
			return ExitUsage
		case errors.Is(err, ErrNotRunning):
			fmt.Fprintf(stderr, "%v\nStart it, or run: %s ctl launch\n", err, prog)
			return ExitNotRunning
		}
		if asJSON {
			_ = json.NewEncoder(stdout).Encode(map[string]string{"error": err.Error()})
		} else {
			fmt.Fprintf(stderr, "%s: %v\n", name, err)
		}
		return ExitError
	}
	print := func(raw json.RawMessage, human func(io.Writer, json.RawMessage) error) int {
		if asJSON || human == nil {
			if err := printJSON(stdout, raw); err != nil {
				return fail(err)
			}
			return ExitOK
		}
		if err := human(stdout, raw); err != nil {
			return fail(err)
		}
		return ExitOK
	}

	switch name {
	case "methods":
		if asJSON {
			b, _ := json.Marshal(methodList())
			return print(b, nil)
		}
		tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
		for _, m := range Methods {
			fmt.Fprintf(tw, "%s\t%s\n", m.Name, firstSentence(m.Desc))
		}
		_ = tw.Flush()
		return ExitOK
	case "launch":
		c, err := Launch(ctx, targets)
		if err != nil {
			return fail(err)
		}
		b, _ := json.Marshal(map[string]string{"running": c.Target.Name})
		return print(b, func(w io.Writer, _ json.RawMessage) error {
			_, err := fmt.Fprintf(w, "%s is running\n", c.Target.Name)
			return err
		})
	case "call":
		if len(rest.pos) == 0 {
			return fail(usageErr("give a method; see: %s ctl methods", prog))
		}
		var callArgs any
		if len(rest.pos) > 1 {
			if err := json.Unmarshal([]byte(strings.Join(rest.pos[1:], " ")), &callArgs); err != nil {
				return fail(usageErr("the arguments are not JSON: %v", err))
			}
		}
		c, err := Dial(ctx, targets)
		if err != nil {
			return fail(err)
		}
		raw, err := c.CallRaw(ctx, rest.pos[0], callArgs)
		if err != nil {
			return fail(err)
		}
		return print(raw, nil)
	}
	var cmd *command
	for i := range commands {
		if commands[i].name == name {
			cmd = &commands[i]
		}
	}
	if cmd == nil {
		fmt.Fprintf(stderr, "unknown command %q; see: %s ctl --help\n", name, prog)
		return ExitUsage
	}
	var callArgs any
	if cmd.build != nil {
		if callArgs, err = cmd.build(rest); err != nil {
			return fail(err)
		}
	}
	c, err := Dial(ctx, targets)
	if err != nil {
		return fail(err)
	}
	raw, err := c.CallRaw(ctx, cmd.method, callArgs)
	if err != nil {
		return fail(err)
	}
	human := cmd.human
	if human == nil {
		human = printOK
	}
	return print(raw, human)
}

func firstSentence(s string) string {
	if i := strings.Index(s, ". "); i > 0 {
		return s[:i+1]
	}
	return s
}

func methodList() []map[string]any {
	var out []map[string]any
	for _, m := range Methods {
		out = append(out, map[string]any{"name": m.Name, "description": m.Desc, "readOnly": m.ReadOnly, "destructive": m.Destructive, "input": m.Schema()})
	}
	return out
}

func printJSON(w io.Writer, raw json.RawMessage) error {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// printOK prints a small result as key: value lines, a larger one as JSON.
func printOK(w io.Writer, raw json.RawMessage) error {
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil || len(m) > 6 {
		return printJSON(w, raw)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		switch v := m[k].(type) {
		case map[string]any, []any:
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "%s: %s\n", k, b)
		case nil:
		case string:
			if v != "" {
				fmt.Fprintf(w, "%s: %s\n", k, v)
			}
		default:
			fmt.Fprintf(w, "%s: %v\n", k, v)
		}
	}
	return nil
}

// --- human output

// Bytes formats a size: 1.5 MB.
func Bytes(n int64) string {
	f := float64(n)
	units := []string{"B", "KB", "MB", "GB", "TB"}
	i := 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

func delayText(d int) string {
	switch {
	case d < 0:
		return "-"
	case d == 0:
		return "timeout"
	}
	return fmt.Sprintf("%d ms", d)
}

func onText(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

func printStatus(w io.Writer, raw json.RawMessage) error {
	var s struct {
		App, Version string
		Core         struct{ Status, Mode, Error, CoreVersion string }
		SystemProxy  bool
		SysErr       string `json:"systemProxyError"`
		Tun          bool
		TunAvailable bool
		Mode         string
		Profile      struct{ UID, Name string }
		MixedPort    int
		Exit         *struct {
			Chain []string
			Node  string
			Type  string
			Delay int
		}
		Speed       *struct{ Up, Down int64 }
		ConfigError string
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "app\t%s %s\n", s.App, s.Version)
	core := s.Core.Status
	if s.Core.Mode != "" {
		core += " (" + s.Core.Mode + ")"
	}
	if s.Core.Error != "" {
		core += ": " + s.Core.Error
	}
	fmt.Fprintf(tw, "core\t%s\n", core)
	sp := onText(s.SystemProxy)
	if s.SysErr != "" {
		sp += " (" + s.SysErr + ")"
	}
	fmt.Fprintf(tw, "system proxy\t%s\n", sp)
	tun := onText(s.Tun)
	if !s.TunAvailable {
		tun += " (service not installed)"
	}
	fmt.Fprintf(tw, "tun\t%s\n", tun)
	fmt.Fprintf(tw, "mode\t%s\n", s.Mode)
	fmt.Fprintf(tw, "profile\t%s\n", s.Profile.Name)
	fmt.Fprintf(tw, "mixed port\t%d\n", s.MixedPort)
	if s.Exit != nil {
		via := ""
		if len(s.Exit.Chain) > 0 {
			via = strings.Join(s.Exit.Chain, " → ") + " → "
		}
		fmt.Fprintf(tw, "exit\t%s%s (%s)\n", via, s.Exit.Node, delayText(s.Exit.Delay))
	}
	if s.Speed != nil {
		fmt.Fprintf(tw, "speed\t↑ %s/s  ↓ %s/s\n", Bytes(s.Speed.Up), Bytes(s.Speed.Down))
	}
	if s.ConfigError != "" {
		fmt.Fprintf(tw, "config error\t%s\n", s.ConfigError)
	}
	return tw.Flush()
}

type groupJSON struct {
	Name, Type, Now string
	Hidden          bool
	All             []struct {
		Name, Type string
		Delay      int
		IsGroup    bool
	}
}

func printGroups(w io.Writer, raw json.RawMessage) error {
	var list struct {
		Mode   string
		Groups []groupJSON
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if list.Groups == nil { // one group
		var g groupJSON
		if err := json.Unmarshal(raw, &g); err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s (%s), now %s\n", g.Name, g.Type, g.Now)
		for _, m := range g.All {
			mark := " "
			if m.Name == g.Now {
				mark = "*"
			}
			typ := m.Type
			if m.IsGroup {
				typ += " group"
			}
			fmt.Fprintf(tw, "%s %s\t%s\t%s\n", mark, m.Name, typ, delayText(m.Delay))
		}
		return tw.Flush()
	}
	fmt.Fprintf(tw, "GROUP\tTYPE\tNOW\tMEMBERS\n")
	for _, g := range list.Groups {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", g.Name, g.Type, g.Now, len(g.All))
	}
	return tw.Flush()
}

func printDelay(w io.Writer, raw json.RawMessage) error {
	var r struct {
		Node   string
		Delay  int
		Group  string
		Delays map[string]int
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	if r.Group == "" {
		_, err := fmt.Fprintf(w, "%s: %s\n", r.Node, delayText(r.Delay))
		return err
	}
	names := make([]string, 0, len(r.Delays))
	for n := range r.Delays {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		a, b := r.Delays[names[i]], r.Delays[names[j]]
		if (a == 0) != (b == 0) {
			return b == 0
		}
		return a < b
	})
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, n := range names {
		fmt.Fprintf(tw, "%s\t%s\n", n, delayText(r.Delays[n]))
	}
	return tw.Flush()
}

func printConns(w io.Writer, raw json.RawMessage) error {
	var r struct {
		Total, Matched int
		Connections    []struct {
			ID, Network, Host, Port, Process, Rule string
			Chains                                 []string
			Upload, Download                       int64
		}
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "ID\tHOST\tPROCESS\tRULE\tCHAIN\t↑\t↓\n")
	for _, c := range r.Connections {
		id := c.ID
		if len(id) > 8 {
			id = id[:8]
		}
		chain := strings.Join(c.Chains, " ← ")
		fmt.Fprintf(tw, "%s\t%s:%s\t%s\t%s\t%s\t%s\t%s\n", id, c.Host, c.Port, c.Process, c.Rule, chain, Bytes(c.Upload), Bytes(c.Download))
	}
	_ = tw.Flush()
	_, err := fmt.Fprintf(w, "%d shown, %d matched, %d open (ids shortened; --json for full ids)\n", len(r.Connections), r.Matched, r.Total)
	return err
}

func printTraffic(w io.Writer, raw json.RawMessage) error {
	var t struct {
		Up, Down, UpTotal, DownTotal int64
		Connections                  int
		Memory                       uint64
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "speed  ↑ %s/s  ↓ %s/s\ntotal  ↑ %s  ↓ %s\nconnections %d, core memory %s\n",
		Bytes(t.Up), Bytes(t.Down), Bytes(t.UpTotal), Bytes(t.DownTotal), t.Connections, Bytes(int64(t.Memory)))
	return err
}

func printStats(w io.Writer, raw json.RawMessage) error {
	type item struct {
		Name     string
		Other    bool
		Up, Down int64
	}
	var r struct {
		From, To        string
		Up, Down, Conns int64
		Apps, Sites     []item
		Nodes, Policies []item
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s – %s: ↑ %s  ↓ %s, %d connections\n", r.From, r.To, Bytes(r.Up), Bytes(r.Down), r.Conns)
	for _, sec := range []struct {
		title string
		items []item
	}{{"apps", r.Apps}, {"sites", r.Sites}, {"nodes", r.Nodes}} {
		if len(sec.items) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s\n", sec.title)
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, it := range sec.items {
			name := it.Name
			if it.Other {
				name = "(other)"
			}
			fmt.Fprintf(tw, "  %s\t%s\n", name, Bytes(it.Up+it.Down))
		}
		_ = tw.Flush()
	}
	return nil
}

func printProfiles(w io.Writer, raw json.RawMessage) error {
	var r struct {
		Profiles []struct {
			UID, Name, Type, URL, LastError string
			Current                         bool
			Updated                         time.Time
			Proxies                         int
		}
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, " \tUID\tNAME\tTYPE\tNODES\tUPDATED\tURL\n")
	for _, p := range r.Profiles {
		mark := " "
		if p.Current {
			mark = "*"
		}
		upd := "-"
		if !p.Updated.IsZero() {
			upd = p.Updated.Local().Format("2006-01-02 15:04")
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\t%s\n", mark, p.UID, p.Name, p.Type, p.Proxies, upd, p.URL)
		if p.LastError != "" {
			fmt.Fprintf(tw, " \t\t  last update failed: %s\t\t\t\t\n", p.LastError)
		}
	}
	return tw.Flush()
}

func printLogs(w io.Writer, raw json.RawMessage) error {
	var logs []struct {
		Type, Payload string
		Time          time.Time
	}
	if err := json.Unmarshal(raw, &logs); err != nil {
		return err
	}
	for _, l := range logs {
		fmt.Fprintf(w, "%s %-7s %s\n", l.Time.Local().Format("15:04:05"), l.Type, l.Payload)
	}
	return nil
}

func printTailscale(w io.Writer, raw json.RawMessage) error {
	var t struct {
		Mode, BackendState, Tailnet, ExitNode, Error string
		Self                                         *struct {
			Name string
			IPs  []string
		}
		Peers []struct {
			Name     string
			IPs      []string
			OS       string
			Online   bool
			ExitNode bool
		}
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "mode\t%s\nstate\t%s\n", t.Mode, t.BackendState)
	if t.Tailnet != "" {
		fmt.Fprintf(tw, "tailnet\t%s\n", t.Tailnet)
	}
	if t.Self != nil {
		fmt.Fprintf(tw, "this device\t%s %s\n", t.Self.Name, strings.Join(t.Self.IPs, ", "))
	}
	if t.ExitNode != "" {
		fmt.Fprintf(tw, "exit node\t%s\n", t.ExitNode)
	}
	if t.Error != "" {
		fmt.Fprintf(tw, "error\t%s\n", t.Error)
	}
	_ = tw.Flush()
	if len(t.Peers) > 0 {
		fmt.Fprintln(w)
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintf(tw, "PEER\tIP\tOS\tONLINE\n")
		for _, p := range t.Peers {
			ip := ""
			if len(p.IPs) > 0 {
				ip = p.IPs[0]
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Name, ip, p.OS, onText(p.Online))
		}
		_ = tw.Flush()
	}
	return nil
}

// Stdout returns where the command line writes: os.Stdout, made usable
// first where the executable has no console of its own (Windows).
func Stdout() (io.Writer, io.Writer) {
	attachConsole()
	return os.Stdout, os.Stderr
}
