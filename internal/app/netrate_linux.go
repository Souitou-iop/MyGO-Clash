package app

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

// ifCounters reads /proc/net/dev for the physical interfaces: the ones
// with a device behind them in /sys/class/net.
func ifCounters() (map[string]ifBytes, error) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]ifBytes{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if _, err := os.Stat("/sys/class/net/" + name + "/device"); err != nil {
			continue
		}
		v := strings.Fields(rest)
		if len(v) < 9 {
			continue
		}
		in, e1 := strconv.ParseUint(v[0], 10, 64)
		sent, e2 := strconv.ParseUint(v[8], 10, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		out[name] = ifBytes{in, sent}
	}
	return out, sc.Err()
}
