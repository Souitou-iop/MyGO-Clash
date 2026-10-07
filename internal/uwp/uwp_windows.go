package uwp

import (
	"fmt"
	"runtime"
	"slices"
	"sort"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	firewallAPI   = windows.NewLazySystemDLL("firewallapi.dll")
	procEnum      = firewallAPI.NewProc("NetworkIsolationEnumAppContainers")
	procFree      = firewallAPI.NewProc("NetworkIsolationFreeAppContainers")
	procGetConfig = firewallAPI.NewProc("NetworkIsolationGetAppContainerConfig")
	procSetConfig = firewallAPI.NewProc("NetworkIsolationSetAppContainerConfig")

	procLoadIndirectString = windows.NewLazySystemDLL("shlwapi.dll").NewProc("SHLoadIndirectString")
)

// appContainer is INET_FIREWALL_APP_CONTAINER.
type appContainer struct {
	sid              *windows.SID
	userSid          *windows.SID
	appContainerName *uint16
	displayName      *uint16
	description      *uint16
	capabilities     struct {
		count uint32
		sids  *windows.SIDAndAttributes
	}
	binaries struct {
		count uint32
		paths **uint16
	}
	workingDirectory *uint16
	packageFullName  *uint16
}

// List returns the app containers of the user, with their exemptions.
func List() ([]StoreApp, error) {
	exempt, err := Exempted()
	if err != nil {
		return nil, err
	}
	var n uint32
	var arr *appContainer
	if r, _, _ := procEnum.Call(0, uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&arr))); r != 0 {
		return nil, fmt.Errorf("listing Store apps: %w", windows.Errno(r))
	}
	if arr == nil {
		return nil, nil
	}
	defer procFree.Call(uintptr(unsafe.Pointer(arr)))
	seen := map[string]bool{}
	var out []StoreApp
	for _, c := range unsafe.Slice(arr, n) {
		if c.sid == nil {
			continue
		}
		sid := c.sid.String()
		if seen[sid] {
			continue
		}
		seen[sid] = true
		a := StoreApp{SID: sid, Name: windows.UTF16PtrToString(c.appContainerName), DisplayName: resolve(windows.UTF16PtrToString(c.displayName))}
		if a.DisplayName == "" {
			a.DisplayName = a.Name
		}
		a.Exempt = slices.ContainsFunc(exempt, func(s string) bool { return strings.EqualFold(s, sid) })
		out = append(out, a)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].DisplayName) < strings.ToLower(out[j].DisplayName) })
	return out, nil
}

// resolve turns an indirect name, "@{Package?ms-resource://...}", into the
// name it stands for.
func resolve(s string) string {
	if !strings.HasPrefix(s, "@") {
		return s
	}
	src, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return ""
	}
	buf := make([]uint16, 1024)
	if hr, _, _ := procLoadIndirectString.Call(uintptr(unsafe.Pointer(src)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0); hr != 0 {
		return ""
	}
	return windows.UTF16ToString(buf)
}

// Exempted returns the SIDs exempted from the loopback restriction.
func Exempted() ([]string, error) {
	var n uint32
	var arr *windows.SIDAndAttributes
	if r, _, _ := procGetConfig.Call(uintptr(unsafe.Pointer(&n)), uintptr(unsafe.Pointer(&arr))); r != 0 {
		return nil, fmt.Errorf("reading the loopback exemptions: %w", windows.Errno(r))
	}
	if arr == nil {
		return nil, nil
	}
	var out []string
	for _, s := range unsafe.Slice(arr, n) {
		if s.Sid != nil {
			out = append(out, s.Sid.String())
		}
	}
	return out, nil
}

// SetExempted replaces the exemptions; it takes an administrator.
func SetExempted(sids []string) error {
	list := make([]windows.SIDAndAttributes, 0, len(sids))
	for _, s := range sids {
		sid, err := windows.StringToSid(s)
		if err != nil {
			return fmt.Errorf("%q is not a SID: %w", s, err)
		}
		list = append(list, windows.SIDAndAttributes{Sid: sid})
	}
	var p uintptr
	if len(list) > 0 {
		p = uintptr(unsafe.Pointer(&list[0]))
	}
	r, _, _ := procSetConfig.Call(uintptr(len(list)), p)
	runtime.KeepAlive(list)
	if r != 0 {
		return fmt.Errorf("setting the loopback exemptions: %w", windows.Errno(r))
	}
	return nil
}
