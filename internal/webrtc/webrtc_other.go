//go:build !windows

package webrtc

// Apply sets the policy of the browsers; only Windows has it per user.
func Apply(Handling, string) error { return nil }
