package app

import (
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ifCounters reads GetIfTable2 for the hardware interfaces that are up.
func ifCounters() (map[string]ifBytes, error) {
	var t *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfTableNormal, &t); err != nil {
		return nil, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(t))
	rows := unsafe.Slice(&t.Table[0], t.NumEntries)
	out := map[string]ifBytes{}
	for i := range rows {
		r := &rows[i]
		const hardware, operUp = 1, 1
		if r.InterfaceAndOperStatusFlags&hardware == 0 || r.OperStatus != operUp {
			continue
		}
		out[strconv.FormatUint(r.InterfaceLuid, 16)] = ifBytes{r.InOctets, r.OutOctets}
	}
	return out, nil
}
