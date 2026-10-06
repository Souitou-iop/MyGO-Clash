package coreapi

import "time"

// Version is mihomo's version.
type Version struct {
	Meta    bool   `json:"meta"`
	Version string `json:"version"`
}

// DelayHistory is the result of a delay test; Delay 0 means it failed.
type DelayHistory struct {
	Time  time.Time `json:"time"`
	Delay int       `json:"delay"`
}

// ProxyState is a proxy's state for one test URL.
type ProxyState struct {
	Alive   bool           `json:"alive"`
	History []DelayHistory `json:"history"`
}

// Proxy is a proxy or a proxy group of the running configuration.
type Proxy struct {
	Name           string                `json:"name"`
	Type           string                `json:"type"`
	UDP            bool                  `json:"udp"`
	XUDP           bool                  `json:"xudp,omitempty"`
	TFO            bool                  `json:"tfo,omitempty"`
	MPTCP          bool                  `json:"mptcp,omitempty"`
	Alive          bool                  `json:"alive"`
	History        []DelayHistory        `json:"history"`
	Extra          map[string]ProxyState `json:"extra,omitempty"`
	ID             string                `json:"id,omitempty"`
	ProviderName   string                `json:"provider-name,omitempty"`
	DialerProxy    string                `json:"dialer-proxy,omitempty"`
	Interface      string                `json:"interface,omitempty"`
	All            []string              `json:"all,omitempty"` // a group's members
	Now            string                `json:"now,omitempty"` // a group's selection
	Hidden         bool                  `json:"hidden,omitempty"`
	Icon           string                `json:"icon,omitempty"`
	TestURL        string                `json:"testUrl,omitempty"`
	ExpectedStatus string                `json:"expectedStatus,omitempty"`
	Fixed          string                `json:"fixed,omitempty"`
}

// ProxiesResponse is GET /proxies.
type ProxiesResponse struct {
	Proxies map[string]Proxy `json:"proxies"`
}

// SubscriptionInfo is the traffic and expiry a provider reports.
type SubscriptionInfo struct {
	Upload   int64 `json:"Upload"`
	Download int64 `json:"Download"`
	Total    int64 `json:"Total"`
	Expire   int64 `json:"Expire"`
}

// ProxyProvider is a provider of proxies.
type ProxyProvider struct {
	Name             string            `json:"name"`
	Type             string            `json:"type"`
	VehicleType      string            `json:"vehicleType"`
	Proxies          []Proxy           `json:"proxies"`
	TestURL          string            `json:"testUrl,omitempty"`
	ExpectedStatus   string            `json:"expectedStatus,omitempty"`
	UpdatedAt        *time.Time        `json:"updatedAt,omitempty"`
	SubscriptionInfo *SubscriptionInfo `json:"subscriptionInfo,omitempty"`
}

// ProxyProvidersResponse is GET /providers/proxies.
type ProxyProvidersResponse struct {
	Providers map[string]ProxyProvider `json:"providers"`
}

// RuleExtra is the statistics of a rule.
type RuleExtra struct {
	Disabled  bool      `json:"disabled"`
	HitCount  uint64    `json:"hitCount"`
	HitAt     time.Time `json:"hitAt"`
	MissCount uint64    `json:"missCount"`
	MissAt    time.Time `json:"missAt"`
}

// Rule is a rule of the running configuration.
type Rule struct {
	Index   int        `json:"index"`
	Type    string     `json:"type"`
	Payload string     `json:"payload"`
	Proxy   string     `json:"proxy"`
	Size    int        `json:"size"`
	Extra   *RuleExtra `json:"extra,omitempty"`
}

// RulesResponse is GET /rules.
type RulesResponse struct {
	Rules []Rule `json:"rules"`
}

// RuleProvider is a provider of rules.
type RuleProvider struct {
	Name        string     `json:"name"`
	Type        string     `json:"type"`
	VehicleType string     `json:"vehicleType"`
	Behavior    string     `json:"behavior"`
	Format      string     `json:"format"`
	RuleCount   int        `json:"ruleCount"`
	UpdatedAt   *time.Time `json:"updatedAt,omitempty"`
}

// RuleProvidersResponse is GET /providers/rules.
type RuleProvidersResponse struct {
	Providers map[string]RuleProvider `json:"providers"`
}

// ConnectionMetadata describes the endpoints of a connection.
type ConnectionMetadata struct {
	Network           string   `json:"network"`
	Type              string   `json:"type"`
	SourceIP          string   `json:"sourceIP"`
	DestinationIP     string   `json:"destinationIP"`
	SourcePort        string   `json:"sourcePort"`
	DestinationPort   string   `json:"destinationPort"`
	InboundIP         string   `json:"inboundIP,omitempty"`
	InboundPort       string   `json:"inboundPort,omitempty"`
	InboundName       string   `json:"inboundName,omitempty"`
	InboundUser       string   `json:"inboundUser,omitempty"`
	Host              string   `json:"host"`
	SniffHost         string   `json:"sniffHost,omitempty"`
	DNSMode           string   `json:"dnsMode,omitempty"`
	UID               uint32   `json:"uid,omitempty"`
	Process           string   `json:"process,omitempty"`
	ProcessPath       string   `json:"processPath,omitempty"`
	SpecialProxy      string   `json:"specialProxy,omitempty"`
	SpecialRules      string   `json:"specialRules,omitempty"`
	RemoteDestination string   `json:"remoteDestination,omitempty"`
	DSCP              uint8    `json:"dscp,omitempty"`
	DestinationGeoIP  []string `json:"destinationGeoIP,omitempty"`
	DestinationIPASN  string   `json:"destinationIPASN,omitempty"`
}

// Connection is an open connection.
type Connection struct {
	ID          string             `json:"id"`
	Metadata    ConnectionMetadata `json:"metadata"`
	Upload      int64              `json:"upload"`
	Download    int64              `json:"download"`
	Start       time.Time          `json:"start"`
	Chains      []string           `json:"chains"`
	Rule        string             `json:"rule"`
	RulePayload string             `json:"rulePayload"`
	// UploadSpeed and DownloadSpeed are computed by the app between two
	// snapshots, in bytes per second.
	UploadSpeed   int64 `json:"uploadSpeed,omitempty"`
	DownloadSpeed int64 `json:"downloadSpeed,omitempty"`
}

// Connections is GET /connections.
type Connections struct {
	DownloadTotal int64        `json:"downloadTotal"`
	UploadTotal   int64        `json:"uploadTotal"`
	Connections   []Connection `json:"connections"`
	Memory        uint64       `json:"memory"`
}

// Traffic is a sample of /traffic, in bytes per second and in total.
type Traffic struct {
	Up        int64 `json:"up"`
	Down      int64 `json:"down"`
	UpTotal   int64 `json:"upTotal"`
	DownTotal int64 `json:"downTotal"`
}

// Memory is a sample of /memory, in bytes.
type Memory struct {
	Inuse   uint64 `json:"inuse"`
	OSLimit uint64 `json:"oslimit"`
}

// LogEvent is a line of /logs.
type LogEvent struct {
	Type    string    `json:"type"` // debug, info, warning, error
	Payload string    `json:"payload"`
	Time    time.Time `json:"time"` // set by the app when it receives the line
}

// RuntimeConfig is the part of GET /configs the app shows.
type RuntimeConfig struct {
	Port        int    `json:"port"`
	SocksPort   int    `json:"socks-port"`
	RedirPort   int    `json:"redir-port"`
	TProxyPort  int    `json:"tproxy-port"`
	MixedPort   int    `json:"mixed-port"`
	AllowLan    bool   `json:"allow-lan"`
	BindAddress string `json:"bind-address"`
	Mode        string `json:"mode"`
	LogLevel    string `json:"log-level"`
	IPv6        bool   `json:"ipv6"`
	Tun         struct {
		Enable bool   `json:"enable"`
		Device string `json:"device"`
		Stack  string `json:"stack"`
	} `json:"tun"`
}

// DNSAnswer is a record of a DNS query.
type DNSAnswer struct {
	Name string `json:"name"`
	Type int    `json:"type"`
	TTL  int    `json:"TTL"`
	Data string `json:"data"`
}

// DNSQueryResult is GET /dns/query.
type DNSQueryResult struct {
	Status   int         `json:"Status"`
	Question []any       `json:"Question"`
	Answer   []DNSAnswer `json:"Answer,omitempty"`
}
