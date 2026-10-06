package sysproxy

import (
	"errors"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// DefaultBypass is what skips the proxy on Windows.
var DefaultBypass = []string{
	"localhost", "127.*", "192.168.*", "10.*",
	"172.16.*", "172.17.*", "172.18.*", "172.19.*", "172.20.*", "172.21.*", "172.22.*", "172.23.*",
	"172.24.*", "172.25.*", "172.26.*", "172.27.*", "172.28.*", "172.29.*", "172.30.*", "172.31.*",
	"<local>",
}

var (
	wininet            = windows.NewLazySystemDLL("wininet.dll")
	internetSetOptionW = wininet.NewProc("InternetSetOptionW")
)

const (
	optionRefresh            = 37
	optionSettingsChanged    = 39
	optionPerConnection      = 75
	perConnFlags             = 1
	perConnProxyServer       = 2
	perConnProxyBypass       = 3
	perConnAutoconfigURL     = 4
	proxyTypeDirect          = 0x1
	proxyTypeProxy           = 0x2
	proxyTypeAutoProxyURL    = 0x4
	internetSettingsRegistry = `Software\Microsoft\Windows\CurrentVersion\Internet Settings`
)

// perConnOption is INTERNET_PER_CONN_OPTIONW: a DWORD and a union of a
// DWORD, a string pointer or a FILETIME, aligned to 8 bytes.
type perConnOption struct {
	option uint32
	_      uint32
	value  uintptr
}

// perConnOptionList is INTERNET_PER_CONN_OPTION_LISTW.
type perConnOptionList struct {
	size        uint32
	connection  *uint16
	optionCount uint32
	optionError uint32
	options     *perConnOption
}

// apply sets the options of the LAN connection and tells applications.
func apply(flags uint32, server, bypass, pac string) error {
	strPtr := func(s string) uintptr {
		p, _ := windows.UTF16PtrFromString(s)
		return uintptr(unsafe.Pointer(p))
	}
	opts := []perConnOption{
		{option: perConnFlags, value: uintptr(flags)},
		{option: perConnProxyServer, value: strPtr(server)},
		{option: perConnProxyBypass, value: strPtr(bypass)},
		{option: perConnAutoconfigURL, value: strPtr(pac)},
	}
	list := perConnOptionList{optionCount: uint32(len(opts)), options: &opts[0]}
	list.size = uint32(unsafe.Sizeof(list))
	if r, _, err := internetSetOptionW.Call(0, optionPerConnection, uintptr(unsafe.Pointer(&list)), uintptr(list.size)); r == 0 {
		return err
	}
	internetSetOptionW.Call(0, optionSettingsChanged, 0, 0)
	internetSetOptionW.Call(0, optionRefresh, 0, 0)
	return nil
}

func setManual(p Proxy) error {
	return apply(proxyTypeDirect|proxyTypeProxy, p.Server(), strings.Join(p.Bypass, ";"), "")
}

func setPAC(url string) error { return apply(proxyTypeDirect|proxyTypeAutoProxyURL, "", "", url) }

func setDirect() error { return apply(proxyTypeDirect, "", "", "") }

func get() (Proxy, error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, internetSettingsRegistry, registry.QUERY_VALUE)
	if err != nil {
		return Proxy{}, err
	}
	defer k.Close()
	var p Proxy
	if pac, _, err := k.GetStringValue("AutoConfigURL"); err == nil && pac != "" {
		p.Enabled, p.PAC = true, pac
		return p, nil
	}
	enabled, _, err := k.GetIntegerValue("ProxyEnable")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return p, err
	}
	p.Enabled = enabled == 1
	server, _, _ := k.GetStringValue("ProxyServer")
	// "host:port", or "http=host:port;https=...;socks=..."
	if strings.Contains(server, "=") {
		for _, part := range strings.Split(server, ";") {
			if k, v, ok := strings.Cut(part, "="); ok && (k == "http" || k == "https") {
				server = v
				break
			}
		}
	}
	if i := strings.LastIndexByte(server, ':'); i > 0 {
		p.Host = server[:i]
		p.Port, _ = strconv.Atoi(server[i+1:])
	}
	if b, _, err := k.GetStringValue("ProxyOverride"); err == nil {
		p.Bypass = ParseBypass(b)
	}
	return p, nil
}
