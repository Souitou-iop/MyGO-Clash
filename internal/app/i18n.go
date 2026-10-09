package app

import (
	"strings"

	"github.com/egoist/mygo"
)

// languages are the languages of the interface, as settings.language
// takes them; src/lib/i18n.ts has the same.
var languages = []string{"en", "zh-CN", "zh-TW", "ja", "ko", "ru", "es", "pt-BR", "de", "fr", "tr", "id", "vi", "fa", "ar"}

// messages are the strings of the Go side, in English: the tray, the quick
// panel, notifications and errors. The page has its own. translations has
// them in the other languages, from i18n_*.go.
var messages = map[string]string{
	"importing":       "Importing profile…",
	"imported":        "Profile imported",
	"importFailed":    "Could not import the profile",
	"importFile":      "Import a profile",
	"serviceFallback": "The service could not start the core; running it without privileges (no TUN)",
	"coreStartFailed": "The core did not start",
	"coreCrashed":     "The core stopped unexpectedly; restarting it",
	"applyFailed":     "Could not apply the configuration",
	"updateFailed":    "Could not update %s",
	"tunNeedsService": "TUN mode needs the service: install it first",
	"serviceOutdated": "The service is older than the app: update it to use TUN mode again",
	"serviceFailed":   "Could not install the service",
	"updateReady":     "MyGO-Clash %s is installed: restart to use it",
	"modeFailed":      "Could not switch the mode",
	"sysproxyFailed":  "Could not set the system proxy",
	"guardStopped":    "The system proxy guard stopped after repeated failures",
	"hotkeyFailed":    "Could not register the shortcut",
	"invalidProfile":  "The profile is invalid",
	"noRuntime":       "No configuration runs yet",
	"controllerOff":   "Turn the external controller on first",
	"servicePrompt":   "MyGO-Clash needs to install its service, which TUN mode requires.",
	"uwpPrompt":       "MyGO-Clash needs to let Store apps reach the proxy.",
	"tsNoLoginURL":    "Tailscale gave no login link; check the control server",
	"tsOff":           "Tailscale is off",
	"syncNotSetUp":    "Sync is not set up",
	"syncBusy":        "A sync is running",
	"syncConflicts":   "%d items changed on two devices; choose which to keep",
	"syncFailed":      "Sync failed",
	"conflictFrom":    "from",

	// Tray and panel.
	"running":      "Running",
	"stopped":      "Stopped",
	"starting":     "Starting…",
	"error":        "Error",
	"dashboard":    "Open Dashboard",
	"quickPanel":   "Quick Panel",
	"mode":         "Mode",
	"rule":         "Rule",
	"global":       "Global",
	"direct":       "Direct",
	"proxies":      "Proxies",
	"profiles":     "Profiles",
	"systemProxy":  "System Proxy",
	"tun":          "TUN Mode",
	"tunNeeds":     "TUN Mode (install service)",
	"tunUpdate":    "TUN Mode (update service)",
	"copyEnv":      "Copy Proxy Command",
	"openDir":      "Open Folder",
	"dataDir":      "Data",
	"coreDir":      "Core",
	"logsDir":      "Logs",
	"more":         "More",
	"restartCore":  "Restart Core",
	"reapply":      "Reload Configuration",
	"updateGeo":    "Update GeoData",
	"lightweight":  "Lightweight Mode",
	"checkUpdates": "Check for Updates…",
	"restartApp":   "Restart App",
	"quit":         "Quit",
	"tailscale":    "Tailscale",
	"exitNode":     "Exit Node",
	"none":         "None",
	"copyIP":       "Copy My Address",
	"adminConsole": "Admin Console",
	"tsNeedsLogin": "Needs login",
	"testDelay":    "Test Delays",
	"timeout":      "Timeout",
	"on":           "On",
	"off":          "Off",
	"noProfile":    "No profile",
	"upload":       "Up",
	"download":     "Down",
	"group":        "Group",
	"profile":      "Profile",
	"notRunning":   "The core is not running",
	"settings":     "Settings…",
	"connected":    "Connected",
	"disconnected": "Not connected",
	"syncNow":      "Sync Now",
	"proxyOff":     "Proxy off",
}

// translations are messages in the other languages, by language.
var translations = map[string]map[string]string{}

// matchLanguage returns the language of the interface that fits tag best:
// a language tag such as "zh-Hant-TW" or a POSIX locale such as
// "pt_PT.UTF-8". It is English without one.
func matchLanguage(tag string) string {
	tag, _, _ = strings.Cut(tag, ".")
	tag, _, _ = strings.Cut(tag, "@")
	parts := strings.FieldsFunc(strings.ToLower(tag), func(r rune) bool { return r == '-' || r == '_' })
	if len(parts) == 0 {
		return "en"
	}
	switch parts[0] {
	case "zh":
		for _, p := range parts[1:] {
			switch p {
			case "hans":
				return "zh-CN"
			case "hant", "tw", "hk", "mo":
				return "zh-TW"
			}
		}
		return "zh-CN"
	case "pt":
		return "pt-BR"
	}
	for _, l := range languages {
		if l == parts[0] {
			return l
		}
	}
	return "en"
}

// lang returns the language of the interface, from the settings or the
// system.
func lang(a *App) string {
	l := ""
	if a != nil && a.settings != nil {
		l = a.settings.Get().Language
	}
	if l == "" {
		l = mygo.App.Locale()
	}
	return matchLanguage(l)
}

// tr translates a message of the Go side.
func tr(a *App, key string) string {
	if s := translations[lang(a)][key]; s != "" {
		return s
	}
	if s, ok := messages[key]; ok {
		return s
	}
	return key
}
