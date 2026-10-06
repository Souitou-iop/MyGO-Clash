package app

import (
	"time"

	"github.com/egoist/mygo"

	"github.com/mygo-clash/mygo-clash/internal/coreapi"
	"github.com/mygo-clash/mygo-clash/internal/coremgr"
	"github.com/mygo-clash/mygo-clash/internal/config"
)

// AppState is the state of the app the interface shows everywhere.
type AppState struct {
	Ready bool          `json:"ready"`
	Core  coremgr.State `json:"core"`
	// Service is the privileged service, which TUN needs.
	Service ServiceState `json:"service"`
	// SystemProxy is on when the app set the system's proxy.
	SystemProxy      bool   `json:"systemProxy"`
	SystemProxyError string `json:"systemProxyError,omitempty"`
	// Tun is on when the core runs TUN.
	Tun bool `json:"tun"`
	// TunAvailable reports that TUN can be turned on: the core has the
	// privileges, or the service can give them.
	TunAvailable bool   `json:"tunAvailable"`
	Mode         string `json:"mode"`
	ProfileUID   string `json:"profileUid"`
	ProfileName  string `json:"profileName"`
	MixedPort    int    `json:"mixedPort"`
	Lightweight  bool   `json:"lightweight"`
	// ConfigError is why the last configuration did not apply.
	ConfigError string    `json:"configError,omitempty"`
	AppliedAt   time.Time `json:"appliedAt,omitzero"`
}

// ServiceState describes the privileged service.
type ServiceState struct {
	Supported bool   `json:"supported"`
	Installed bool   `json:"installed"`
	Version   string `json:"version,omitempty"`
	// Outdated reports a service of another version than the app's.
	Outdated bool   `json:"outdated"`
	Error    string `json:"error,omitempty"`
}

// Notice is a message for the user, shown as a toast.
type Notice struct {
	Level   string `json:"level"` // info, success, warning, error
	Message string `json:"message"`
	Detail  string `json:"detail,omitempty"`
	// Action is what a button of the toast does: install-service,
	// open-settings, open-sync, ...
	Action string `json:"action,omitempty"`
}

// Events sent to the pages.
var (
	// StateEvent is sent whenever the app's state changes.
	StateEvent = mygo.NewEvent[AppState]("state")
	// SettingsEvent is sent when the settings changed.
	SettingsEvent = mygo.NewEvent[config.Settings]("settings")
	// ProfilesEvent is sent when profiles changed.
	ProfilesEvent = mygo.NewEvent[ProfilesView]("profiles")
	// NoticeEvent is a message for the user.
	NoticeEvent = mygo.NewEvent[Notice]("notice")
	// TailscaleEvent is sent when the tailnet's status changed.
	TailscaleEvent = mygo.NewEvent[coreapi.TailscaleStatus]("tailscale")
	// SyncEvent is sent when the sync's status changed.
	SyncEvent = mygo.NewEvent[SyncStatus]("sync")
	// NavigateEvent asks the page to show a page: home, proxies, settings...
	NavigateEvent = mygo.NewEvent[string]("navigate")
	// RuntimeEvent is sent after a configuration was applied, when proxies,
	// rules and providers may have changed.
	RuntimeEvent = mygo.NewEvent[RuntimeInfo]("runtime")
	// SelectionEvent is sent when a group's selection changed outside the
	// page, from the tray or the quick panel.
	SelectionEvent = mygo.NewEvent[Selection]("selection")
)

// RuntimeInfo describes the configuration last applied.
type RuntimeInfo struct {
	ProfileUID string    `json:"profileUid"`
	AppliedAt  time.Time `json:"appliedAt"`
	// Notes counts the notes extensions left, by extension.
	Notes map[string]int `json:"notes"`
}

// Selection is a group's selection.
type Selection struct {
	Group string `json:"group"`
	Now   string `json:"now"`
}
