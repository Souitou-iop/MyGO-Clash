// Package coreapi is the contract between the app and its core: the
// requests of the routes the core adds under /mygo, the types of mihomo's
// REST API the app uses, and a client that speaks both over the core's
// private socket (a Unix socket, or a named pipe on Windows).
package coreapi

import (
	"encoding/json"
	"time"
)

// Routes the core adds to mihomo's REST API.
const (
	PathHealth        = "/mygo/health"
	PathApply         = "/mygo/apply"
	PathValidate      = "/mygo/validate"
	PathGeneral       = "/mygo/general"
	PathGeoUpdate     = "/mygo/geo/update"
	PathRulesDisable  = "/mygo/rules/disable"
	PathRulesMatch    = "/mygo/rules/match"
	PathShutdown      = "/mygo/shutdown"
	PathTSStatus      = "/mygo/tailscale/status"
	PathTSWatch       = "/mygo/tailscale/watch"
	PathTSLogin       = "/mygo/tailscale/login"
	PathTSLogout      = "/mygo/tailscale/logout"
	PathTSPrefs       = "/mygo/tailscale/prefs"
	PathTSPing        = "/mygo/tailscale/ping"
	PathTSStateExport = "/mygo/tailscale/state"
)

// Bootstrap is what the core needs to start, which the app or the service
// writes to its standard input as one JSON line.
type Bootstrap struct {
	// Home is mihomo's home directory: caches, GeoIP data, providers.
	Home string `json:"home"`
	// Socket is where the core serves its API: a Unix socket path, or a
	// named pipe (\\.\pipe\...) on Windows.
	Socket string `json:"socket"`
	// PipeSDDL is the security descriptor of the named pipe on Windows.
	PipeSDDL string `json:"pipeSddl,omitempty"`
	// LogLevel is the core's log level until a configuration sets one.
	LogLevel string `json:"logLevel,omitempty"`
	// Mode is "sidecar" or "service", reported by health checks.
	Mode string `json:"mode"`
	// OwnerUID, on Unix, is the user the core's files belong to when the
	// service runs it as root; -1 to leave them.
	OwnerUID int `json:"ownerUid"`
	OwnerGID int `json:"ownerGid"`
	// ExitWithStdin stops the core when its standard input closes, so
	// that a sidecar ends with the app that started it.
	ExitWithStdin bool `json:"exitWithStdin"`
}

// Health describes a running core.
type Health struct {
	Version     string    `json:"version"`     // the app version of the core binary
	CoreVersion string    `json:"coreVersion"` // mihomo's version
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"startedAt"`
	Mode        string    `json:"mode"`
	Privileged  bool      `json:"privileged"`
	ConfigHash  string    `json:"configHash"`
	AppliedAt   time.Time `json:"appliedAt"`
	Home        string    `json:"home"`
}

// ApplyRequest replaces the running configuration.
type ApplyRequest struct {
	Config    string           `json:"config"` // YAML
	Tailscale *TailscaleConfig `json:"tailscale,omitempty"`
}

// ApplyResponse reports how applying went.
type ApplyResponse struct {
	ConfigHash string   `json:"configHash"`
	Warnings   []string `json:"warnings,omitempty"`
}

// ValidateRequest checks a configuration without applying it.
type ValidateRequest struct {
	Config string `json:"config"`
}

// GeneralPatch changes settings that need no new configuration.
type GeneralPatch struct {
	Mode     string `json:"mode,omitempty"`
	LogLevel string `json:"logLevel,omitempty"`
}

// MatchRequest asks which rule a connection would hit.
type MatchRequest struct {
	// Target is a domain, an IP address or a URL.
	Target string `json:"target"`
	// Network is "tcp" (the default) or "udp".
	Network string `json:"network,omitempty"`
	// Port is the destination port; 0 takes the URL's, else 443.
	Port int `json:"port,omitempty"`
	// Process is the name of the program that connects, for process rules.
	Process string `json:"process,omitempty"`
}

// MatchHop is a step of the path a connection takes: a group, then the
// member it selects, down to the proxy that dials.
type MatchHop struct {
	Name string `json:"name"`
	Type string `json:"type"` // Selector, URLTest, Shadowsocks, Direct...
}

// MatchResult is the rule a connection would hit and where it would go.
type MatchResult struct {
	Host    string `json:"host"` // the domain or address tested, as normalized
	Network string `json:"network"`
	Port    int    `json:"port"`
	Mode    string `json:"mode"` // rule, global or direct
	// Source says what decided: "rule", "mode" (global or direct mode, so
	// no rule was consulted) or "none" (no rule matched and the core
	// falls back to DIRECT).
	Source string `json:"source"`
	// Index is the position in the rules list; -1 without a rule.
	Index    int    `json:"index"`
	RuleType string `json:"ruleType,omitempty"`
	Payload  string `json:"payload,omitempty"`
	// Policy is the proxy or group the rule names.
	Policy string `json:"policy"`
	// Chain is the path from Policy down to the proxy that dials.
	Chain []MatchHop `json:"chain"`
	// IPs are the addresses the name resolved to while matching, or the
	// address that was given.
	IPs []string `json:"ips"`
	// Disabled counts the rules switched off by hand, which are skipped.
	Disabled int `json:"disabled"`
}

// ErrorBody is the body of the core's error responses.
type ErrorBody struct {
	Message string `json:"message"`
}

// TailscaleConfig configures the tailnet node the core runs. The core
// keeps the node across configuration changes and restarts it only when
// these settings change.
type TailscaleConfig struct {
	Enabled bool `json:"enabled"`
	// ProxyName is the outbound of the configuration the node serves.
	ProxyName  string `json:"proxyName"`
	Hostname   string `json:"hostname"`
	ControlURL string `json:"controlUrl,omitempty"`
	Ephemeral  bool   `json:"ephemeral"`
	// AcceptRoutes accepts the subnets peers advertise.
	AcceptRoutes bool `json:"acceptRoutes"`
	// ExitNode is the IP or stable ID of a peer, "auto:any", or "".
	ExitNode         string `json:"exitNode,omitempty"`
	ExitNodeAllowLAN bool   `json:"exitNodeAllowLan"`
	// ControlVia names the proxy the node reaches the coordination
	// server and relays through; "" goes direct.
	ControlVia string `json:"controlVia,omitempty"`
	// ShareProxyPort, when not 0, serves the core's mixed proxy to the
	// tailnet on this port of the node's addresses.
	ShareProxyPort int `json:"shareProxyPort,omitempty"`
	// ShareProxyTarget is the address of the mixed proxy, e.g. 127.0.0.1:7897.
	ShareProxyTarget string `json:"shareProxyTarget,omitempty"`
	// State is the node's saved state, keyed by name: its keys and
	// profile. The app stores it encrypted and hands it back at start.
	State map[string][]byte `json:"state,omitempty"`
}

// SameNode reports whether two configurations run the same node, so that
// switching between them needs no restart.
func (c *TailscaleConfig) SameNode(o *TailscaleConfig) bool {
	if c == nil || o == nil {
		return c == o
	}
	return c.Enabled == o.Enabled && c.Hostname == o.Hostname && c.ControlURL == o.ControlURL &&
		c.Ephemeral == o.Ephemeral && c.ControlVia == o.ControlVia
}

// TailscaleLogin starts a login: with an auth key, or interactively, in
// which case the status gets an AuthURL to open.
type TailscaleLogin struct {
	AuthKey string `json:"authKey,omitempty"`
}

// TailscalePrefs changes preferences of a running node.
type TailscalePrefs struct {
	AcceptRoutes     *bool   `json:"acceptRoutes,omitempty"`
	ExitNode         *string `json:"exitNode,omitempty"`
	ExitNodeAllowLAN *bool   `json:"exitNodeAllowLan,omitempty"`
	ShieldsUp        *bool   `json:"shieldsUp,omitempty"`
}

// TailscalePing pings a peer.
type TailscalePing struct {
	IP   string `json:"ip"`
	Type string `json:"type,omitempty"` // disco (default), TSMP, ICMP
}

// TailscalePingResult is the result of a ping.
type TailscalePingResult struct {
	LatencyMs float64 `json:"latencyMs"`
	Endpoint  string  `json:"endpoint,omitempty"` // the direct address, if any
	DERP      string  `json:"derp,omitempty"`     // the relay region, if relayed
	Err       string  `json:"err,omitempty"`
}

// TailscaleStatus is the state of a tailnet node: the embedded node of the
// core, or the Tailscale client installed on the computer.
type TailscaleStatus struct {
	// Source is "embedded" or "system".
	Source string `json:"source"`
	// Available reports whether there is a node to talk to.
	Available bool `json:"available"`
	// BackendState is Tailscale's: NoState, NeedsLogin, NeedsMachineAuth,
	// Stopped, Starting or Running.
	BackendState   string          `json:"backendState"`
	AuthURL        string          `json:"authUrl,omitempty"`
	Self           *TailscalePeer  `json:"self,omitempty"`
	Peers          []TailscalePeer `json:"peers"`
	User           *TailscaleUser  `json:"user,omitempty"`
	TailnetName    string          `json:"tailnetName,omitempty"`
	MagicDNSSuffix string          `json:"magicDnsSuffix,omitempty"`
	MagicDNS       bool            `json:"magicDns"`
	ExitNodeID     string          `json:"exitNodeId,omitempty"`
	AcceptRoutes   bool            `json:"acceptRoutes"`
	ShieldsUp      bool            `json:"shieldsUp"`
	Routes         []string        `json:"routes,omitempty"` // subnets peers advertise, primary routes
	Health         []string        `json:"health,omitempty"`
	Version        string          `json:"version,omitempty"`
	Interface      string          `json:"interface,omitempty"` // system client's interface
	ShareProxy     string          `json:"shareProxy,omitempty"`
	Error          string          `json:"error,omitempty"`
	UpdatedAt      time.Time       `json:"updatedAt"`
	// StateVersion counts the writes of the embedded node's state; the app
	// fetches the state again when it moves.
	StateVersion uint64           `json:"stateVersion,omitempty"`
	Extra        *json.RawMessage `json:"extra,omitempty"`
}

// TailscaleState is the embedded node's saved state.
type TailscaleState struct {
	Version uint64            `json:"version"`
	State   map[string][]byte `json:"state"`
}

// TailscaleUser is the account a node belongs to.
type TailscaleUser struct {
	LoginName     string `json:"loginName"`
	DisplayName   string `json:"displayName"`
	ProfilePicURL string `json:"profilePicUrl,omitempty"`
}

// TailscalePeer is a node of the tailnet.
type TailscalePeer struct {
	ID             string     `json:"id"`
	HostName       string     `json:"hostName"`
	DNSName        string     `json:"dnsName"`
	OS             string     `json:"os"`
	User           string     `json:"user,omitempty"`
	TailscaleIPs   []string   `json:"tailscaleIps"`
	Online         bool       `json:"online"`
	Active         bool       `json:"active"`
	LastSeen       *time.Time `json:"lastSeen,omitempty"`
	ExitNode       bool       `json:"exitNode"`       // the exit node in use
	ExitNodeOption bool       `json:"exitNodeOption"` // can be one
	Relay          string     `json:"relay,omitempty"`
	CurAddr        string     `json:"curAddr,omitempty"`
	RxBytes        int64      `json:"rxBytes"`
	TxBytes        int64      `json:"txBytes"`
	PrimaryRoutes  []string   `json:"primaryRoutes,omitempty"`
	Tags           []string   `json:"tags,omitempty"`
	KeyExpiry      *time.Time `json:"keyExpiry,omitempty"`
	Expired        bool       `json:"expired"`
}

// TailscaleEvent is a line of the watch stream.
type TailscaleEvent struct {
	// Status is set when the node's status changed.
	Status *TailscaleStatus `json:"status,omitempty"`
	// StateVersion is set when the node saved state, which the app then
	// fetches and persists.
	StateVersion uint64 `json:"stateVersion,omitempty"`
}
