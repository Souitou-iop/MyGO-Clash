package tools

import "errors"

// Development builds answer the address and reputation lookups with made-up
// data, instead of asking the public services: a demo would send the
// developer's real address to, and through, services that log it. The
// addresses come from the documentation ranges (RFC 5737), which no demo
// can point back at anyone with.

// demoDomestic is what a mainland site sees, demoDomesticIP its address.
const demoDomesticIP = "203.0.113.7"

// DemoIPInfo returns the made-up address of a view: the same views ExitIP
// takes, telling the pages apart the same way.
func DemoIPInfo(view string) (IPInfo, error) {
	switch view {
	case ViewDomestic:
		return IPInfo{
			IP: demoDomesticIP, Country: "中国", CountryCode: "CN", City: "测试节点",
			ISP: "中国电信", ASN: "4134", Timezone: "Asia/Shanghai",
			Source: "IPIP.net",
		}, nil
	case ViewGlobal:
		return IPInfo{
			IP: "198.51.100.21", Country: "Japan", CountryCode: "JP", City: "测试节点",
			ISP: "Amazon.com", ASN: "16509", Timezone: "Asia/Tokyo",
			Source: "ip.sb",
		}, nil
	case ViewCloudflare:
		return IPInfo{
			IP: "198.51.100.21", Country: "Japan", CountryCode: "JP", City: "测试节点",
			ISP: "Amazon.com", ASN: "16509", Timezone: "Asia/Tokyo",
			Colo: "NRT", Source: "Cloudflare",
		}, nil
	}
	return IPInfo{}, errors.New("unknown view " + view)
}

// DemoIPQuality says what the made-up data makes of an address: the
// mainland one a home line, any other a busy data center.
func DemoIPQuality(ip string) IPQuality {
	if ip == demoDomesticIP {
		return IPQuality{IP: ip, Kind: KindResidential, Risk: 0, Org: "Chinanet", Source: "ipapi.is + proxycheck.io"}
	}
	return IPQuality{IP: ip, Kind: KindDatacenter, Proxy: true, VPN: true, Risk: 66, Org: "Amazon.com", Source: "ipapi.is + proxycheck.io"}
}
