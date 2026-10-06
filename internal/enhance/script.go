package enhance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/dop251/goja"

	"github.com/mygo-clash/mygo-clash/internal/yamlx"
)

// ScriptTemplate is the script of a new script extension, which changes
// nothing.
const ScriptTemplate = `// Define main function (script entry)

function main(config, profileName) {
  return config;
}
`

const (
	scriptTimeout  = 5 * time.Second
	maxScriptLogs  = 1000
	maxScriptOut   = 1 << 20  // bytes of console output
	maxScriptJSON  = 10 << 20 // bytes of configuration in and out
	maxScriptStack = 4096
)

// ErrScript is wrapped by the errors of scripts that threw, timed out or
// returned something other than a configuration.
var ErrScript = errors.New("script failed")

// RunScript calls the script's main(config, profileName) with a copy of the
// configuration and returns the configuration it returned, along with what
// it printed. A script that fails leaves the configuration as it was and
// reports why in the error and the logs.
//
// The script runs in a fresh goja runtime with no access to files, the
// network or the process, a 5 second budget and a bounded console.
func RunScript(ctx context.Context, script string, cfg *yamlx.Map, profileName string) (*yamlx.Map, []LogEntry, error) {
	if strings.TrimSpace(script) == strings.TrimSpace(ScriptTemplate) || strings.TrimSpace(script) == "" {
		return LowercaseKeys(cfg), nil, nil
	}
	in, err := json.Marshal(LowercaseKeys(cfg))
	if err != nil {
		return cfg, nil, err
	}
	if len(in) > maxScriptJSON {
		return cfg, nil, fmt.Errorf("%w: the configuration is too large for scripts", ErrScript)
	}

	vm := goja.New()
	vm.SetMaxCallStackSize(maxScriptStack)
	var logs []LogEntry
	outSize := 0
	logFn := func(level string) func(goja.FunctionCall) goja.Value {
		return func(call goja.FunctionCall) goja.Value {
			parts := make([]string, 0, len(call.Arguments))
			for _, a := range call.Arguments {
				parts = append(parts, formatJS(vm, a))
			}
			msg := strings.Join(parts, " ")
			outSize += len(msg)
			if len(logs) >= maxScriptLogs || outSize > maxScriptOut {
				panic(vm.NewGoError(errors.New("the script printed too much")))
			}
			logs = append(logs, LogEntry{Level: level, Message: msg})
			return goja.Undefined()
		}
	}
	console := vm.NewObject()
	for _, level := range []string{"log", "info", "warn", "error", "debug", "table"} {
		_ = console.Set(level, logFn(level))
	}
	_ = vm.Set("console", console)

	timer := time.AfterFunc(scriptTimeout, func() { vm.Interrupt("timeout") })
	defer timer.Stop()
	stop := context.AfterFunc(ctx, func() { vm.Interrupt("canceled") })
	defer stop()

	fail := func(format string, a ...any) (*yamlx.Map, []LogEntry, error) {
		err := fmt.Errorf("%w: "+format, append([]any{ErrScript}, a...)...)
		logs = append(logs, LogEntry{Level: "exception", Message: err.Error()})
		return cfg, logs, err
	}

	if _, err := vm.RunScript("script.js", script); err != nil {
		return fail("%s", jsError(err))
	}
	mainFn, ok := goja.AssertFunction(vm.Get("main"))
	if !ok {
		return fail("the script defines no main function")
	}
	parsed, err := vm.RunString("JSON.parse")
	if err != nil {
		return fail("%s", jsError(err))
	}
	parse, _ := goja.AssertFunction(parsed)
	arg, err := parse(goja.Undefined(), vm.ToValue(string(in)))
	if err != nil {
		return fail("%s", jsError(err))
	}
	res, err := mainFn(goja.Undefined(), arg, vm.ToValue(profileName))
	if err != nil {
		return fail("%s", jsError(err))
	}
	if res == nil || goja.IsUndefined(res) || goja.IsNull(res) {
		return fail("main should return the configuration")
	}
	if _, isObj := res.(*goja.Object); !isObj {
		return fail("main should return an object")
	}
	stringified, err := vm.RunString("(v) => JSON.stringify(v)")
	if err != nil {
		return fail("%s", jsError(err))
	}
	stringify, _ := goja.AssertFunction(stringified)
	out, err := stringify(goja.Undefined(), res)
	if err != nil {
		return fail("%s", jsError(err))
	}
	text := out.String()
	if len(text) > maxScriptJSON {
		return fail("the returned configuration is too large")
	}
	tree, err := yamlx.DecodeJSON([]byte(text))
	if err != nil {
		return fail("the returned value is not a configuration: %v", err)
	}
	result, ok := tree.(*yamlx.Map)
	if !ok {
		return fail("main should return an object")
	}
	return LowercaseKeys(result), logs, nil
}

func jsError(err error) string {
	var ex *goja.Exception
	if errors.As(err, &ex) {
		return ex.Error()
	}
	var interrupted *goja.InterruptedError
	if errors.As(err, &interrupted) {
		if interrupted.Value() == "timeout" {
			return fmt.Sprintf("the script ran longer than %s", scriptTimeout)
		}
		return "the script was canceled"
	}
	return err.Error()
}

// formatJS prints a value as console.log would: strings as they are,
// others as indented JSON.
func formatJS(vm *goja.Runtime, v goja.Value) string {
	if v == nil || goja.IsUndefined(v) {
		return "undefined"
	}
	if s, ok := v.Export().(string); ok {
		return s
	}
	stringified, err := vm.RunString("(v) => JSON.stringify(v, null, 2)")
	if err == nil {
		if f, ok := goja.AssertFunction(stringified); ok {
			if r, err := f(goja.Undefined(), v); err == nil && !goja.IsUndefined(r) {
				return r.String()
			}
		}
	}
	return v.String()
}

// CheckScript reports syntax errors in a script, and whether it defines
// main, without running main.
func CheckScript(script string) error {
	if _, err := goja.Compile("script.js", script, false); err != nil {
		return errors.New(jsError(err))
	}
	if !strings.Contains(script, "main") {
		return errors.New("the script defines no main function")
	}
	return nil
}
