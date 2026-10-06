package cloudsync

import (
	"encoding/json"
	"reflect"
)

// MergeJSON merges two JSON objects changed from a base, field by field:
// a field changed on one side takes that side's value, a field changed on
// both takes the preferred side's. Lists and other values are atomic.
func MergeJSON(base, local, remote []byte, preferRemote bool) ([]byte, bool) {
	var b, l, r any
	if json.Unmarshal(base, &b) != nil || json.Unmarshal(local, &l) != nil || json.Unmarshal(remote, &r) != nil {
		return nil, false
	}
	merged := merge3(b, l, r, preferRemote)
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return nil, false
	}
	return out, true
}

func merge3(base, local, remote any, preferRemote bool) any {
	switch {
	case reflect.DeepEqual(local, remote):
		return local
	case reflect.DeepEqual(base, local):
		return remote
	case reflect.DeepEqual(base, remote):
		return local
	}
	bm, bok := base.(map[string]any)
	lm, lok := local.(map[string]any)
	rm, rok := remote.(map[string]any)
	if lok && rok {
		if !bok {
			bm = map[string]any{}
		}
		out := map[string]any{}
		keys := map[string]bool{}
		for k := range lm {
			keys[k] = true
		}
		for k := range rm {
			keys[k] = true
		}
		for k := range bm {
			keys[k] = true
		}
		for k := range keys {
			lv, lhas := lm[k]
			rv, rhas := rm[k]
			bv, bhas := bm[k]
			switch {
			case lhas && rhas:
				out[k] = merge3(bv, lv, rv, preferRemote)
			case lhas && !rhas:
				if bhas && reflect.DeepEqual(bv, lv) {
					continue // removed remotely, unchanged here
				}
				out[k] = lv
			case !lhas && rhas:
				if bhas && reflect.DeepEqual(bv, rv) {
					continue // removed here, unchanged remotely
				}
				out[k] = rv
			}
		}
		return out
	}
	if preferRemote {
		return remote
	}
	return local
}
