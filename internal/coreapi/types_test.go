package coreapi

import "testing"

func TestTailscaleConfigSameNode(t *testing.T) {
	base := &TailscaleConfig{Enabled: true, Hostname: "a", ControlURL: "", Ephemeral: false, ControlVia: ""}

	renamed := &TailscaleConfig{Enabled: true, Hostname: "b", ControlURL: "", Ephemeral: false, ControlVia: ""}
	if !base.SameNode(renamed) {
		t.Error("a hostname change renamed the node instead of keeping it")
	}

	otherControl := &TailscaleConfig{Enabled: true, Hostname: "a", ControlURL: "https://example.com", Ephemeral: false, ControlVia: ""}
	if base.SameNode(otherControl) {
		t.Error("a control server change must restart the node")
	}

	ephemeral := &TailscaleConfig{Enabled: true, Hostname: "a", ControlURL: "", Ephemeral: true, ControlVia: ""}
	if base.SameNode(ephemeral) {
		t.Error("an ephemeral change must restart the node")
	}

	via := &TailscaleConfig{Enabled: true, Hostname: "a", ControlURL: "", Ephemeral: false, ControlVia: "Proxy"}
	if base.SameNode(via) {
		t.Error("a control proxy change must restart the node")
	}

	if base.SameNode(nil) || (&TailscaleConfig{}).SameNode(base) {
		t.Error("nil is only the same node as nil")
	}
}
