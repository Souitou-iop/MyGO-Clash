package app

import (
	"encoding/binary"
	"net"
	"strings"
	"syscall"
)

// ifCounters reads the 64-bit counters of the physical interfaces (en*,
// the cellular pdp_ip*) from the routing table's interface list.
func ifCounters() (map[string]ifBytes, error) {
	const netRtIflist2, rtmIfinfo2 = 6, 0x12
	rib, err := syscall.RouteRIB(netRtIflist2, 0)
	if err != nil {
		return nil, err
	}
	names := map[int]string{}
	if ifs, err := net.Interfaces(); err == nil {
		for _, i := range ifs {
			names[i.Index] = i.Name
		}
	}
	out := map[string]ifBytes{}
	for len(rib) >= 4 {
		n := int(binary.NativeEndian.Uint16(rib))
		if n < 4 || n > len(rib) {
			break
		}
		msg := rib[:n]
		rib = rib[n:]
		// if_msghdr2: the index at 12, if_data64 at 32 with its byte
		// counters at 64 and 72 into it.
		if msg[3] != rtmIfinfo2 || n < 112 {
			continue
		}
		name := names[int(binary.NativeEndian.Uint16(msg[12:]))]
		if !strings.HasPrefix(name, "en") && !strings.HasPrefix(name, "pdp_ip") {
			continue
		}
		out[name] = ifBytes{
			in:  binary.NativeEndian.Uint64(msg[32+64:]),
			out: binary.NativeEndian.Uint64(msg[32+72:]),
		}
	}
	return out, nil
}
