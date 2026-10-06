// Package profiles manages the user's profiles, the configurations of the
// core, and their extensions, and turns the current one into the runtime
// configuration.
//
// A profile is remote (downloaded from a subscription URL, updated on a
// schedule) or local (written by the user). Each can have extensions,
// applied in order when the runtime configuration is generated:
//
//	rules, proxies, groups   prepend, append or delete list items
//	merge                    YAML merged over the configuration
//	script                   JavaScript main(config, profileName)
//
// and the global Merge and Script items apply to every profile. Profiles
// and the index (which holds subscription URLs) are sealed on disk.
package profiles

import (
	"crypto/rand"
	"encoding/base32"
	"strings"
)

// Types of profile items.
const (
	TypeRemote  = "remote"
	TypeLocal   = "local"
	TypeMerge   = "merge"
	TypeScript  = "script"
	TypeRules   = "rules"
	TypeProxies = "proxies"
	TypeGroups  = "groups"
)

// UIDs of the global extensions.
const (
	GlobalMerge  = "Merge"
	GlobalScript = "Script"
)

// Profile is a profile or an extension.
type Profile struct {
	UID  string `json:"uid"`
	Type string `json:"type"`
	Name string `json:"name"`
	Desc string `json:"desc,omitempty"`
	// URL is the subscription of a remote profile.
	URL string `json:"url,omitempty"`
	// Home is the provider's web page, from its profile-web-page-url header.
	Home string `json:"home,omitempty"`
	// Updated is when the content last changed, in Unix seconds.
	Updated int64  `json:"updated"`
	Usage   *Usage `json:"usage,omitempty"`
	Option  Option `json:"option"`
	// Selected remembers the selection of each selector group.
	Selected []Selected `json:"selected,omitempty"`
	// LastError is why the last update failed, "" when it worked.
	LastError string `json:"lastError,omitempty"`
	// Size is the size of the content in bytes.
	Size int `json:"size"`
	// Proxies and Groups count what the profile defines.
	Proxies int `json:"proxies"`
	Groups  int `json:"groups"`
	// Converted reports a subscription of share links that the app turned
	// into a configuration.
	Converted bool `json:"converted,omitempty"`
}

// Usage is the traffic and expiry a subscription reports.
type Usage struct {
	Upload   int64 `json:"upload"`
	Download int64 `json:"download"`
	Total    int64 `json:"total"`
	Expire   int64 `json:"expire"` // Unix seconds, 0 for never
}

// Option configures how a profile updates and which extensions it uses.
type Option struct {
	UserAgent string `json:"userAgent,omitempty"`
	// WithProxy downloads through the core's proxy.
	WithProxy bool `json:"withProxy,omitempty"`
	// UpdateInterval updates the subscription every so many minutes; 0
	// uses the provider's suggestion, -1 never.
	UpdateInterval int `json:"updateInterval,omitempty"`
	// TimeoutSeconds bounds a download; 0 is 60 seconds.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`
	// Insecure accepts invalid TLS certificates. Dangerous.
	Insecure bool `json:"insecure,omitempty"`
	// NoAutoUpdate stops scheduled updates.
	NoAutoUpdate bool `json:"noAutoUpdate,omitempty"`
	// UIDs of the profile's extensions.
	Merge   string `json:"merge,omitempty"`
	Script  string `json:"script,omitempty"`
	Rules   string `json:"rules,omitempty"`
	Proxies string `json:"proxies,omitempty"`
	Groups  string `json:"groups,omitempty"`
}

// Selected is the selection of a group.
type Selected struct {
	Name string `json:"name"`
	Now  string `json:"now"`
}

// IsProfile reports whether the item is a profile rather than an extension.
func (p *Profile) IsProfile() bool { return p.Type == TypeRemote || p.Type == TypeLocal }

// Extension returns the UID of the profile's extension of a kind.
func (o *Option) Extension(kind string) string {
	switch kind {
	case TypeMerge:
		return o.Merge
	case TypeScript:
		return o.Script
	case TypeRules:
		return o.Rules
	case TypeProxies:
		return o.Proxies
	case TypeGroups:
		return o.Groups
	}
	return ""
}

func (o *Option) setExtension(kind, uid string) {
	switch kind {
	case TypeMerge:
		o.Merge = uid
	case TypeScript:
		o.Script = uid
	case TypeRules:
		o.Rules = uid
	case TypeProxies:
		o.Proxies = uid
	case TypeGroups:
		o.Groups = uid
	}
}

// ExtensionKinds are the kinds of extension a profile has, in the order
// they apply.
var ExtensionKinds = []string{TypeRules, TypeProxies, TypeGroups, TypeMerge, TypeScript}

var uidPrefix = map[string]string{
	TypeRemote: "R", TypeLocal: "L", TypeMerge: "m", TypeScript: "s",
	TypeRules: "r", TypeProxies: "p", TypeGroups: "g",
}

func newUID(kind string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return uidPrefix[kind] + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b))
}

// Templates of new items.
const (
	templateLocal = `# A profile of MyGO-Clash. See https://wiki.metacubex.one/config/

proxies: []

proxy-groups: []

rules:
  - MATCH,DIRECT
`
	templateMerge = `# Merge: these keys replace or merge into the profile.
# prepend-rules, append-rules, prepend-proxies, append-proxies,
# prepend-proxy-groups and append-proxy-groups add to lists.

profile:
  store-selected: true
`
	templateSeq = `# Items to add before (prepend) or after (append) the profile's list,
# and names (or rules) to delete from it.

prepend: []

append: []

delete: []
`
	templateScript = `// Script: change the configuration in JavaScript.
// config is the configuration, profileName the profile's name.

function main(config, profileName) {
  return config;
}
`
)

func template(kind string) string {
	switch kind {
	case TypeLocal:
		return templateLocal
	case TypeMerge:
		return templateMerge
	case TypeScript:
		return templateScript
	case TypeRules, TypeProxies, TypeGroups:
		return templateSeq
	}
	return ""
}
