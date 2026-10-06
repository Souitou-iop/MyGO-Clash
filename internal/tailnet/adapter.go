package tailnet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"

	"github.com/metacubex/mihomo/adapter/outbound"
	C "github.com/metacubex/mihomo/constant"
	D "github.com/miekg/dns"
)

// Adapter is the outbound of the embedded node: what mihomo sends to it
// enters the tailnet. Destinations the tailnet does not route (no peer, no
// subnet router, no exit node) fail rather than leave by another way.
//
// Adapters are cheap and made for every configuration; they share the node,
// so closing one leaves the node running.
type Adapter struct {
	*outbound.Base
	node *Node
}

// NewAdapter returns the outbound named name of the node.
func NewAdapter(node *Node, name string) *Adapter {
	return &Adapter{
		Base: outbound.NewBase(outbound.BaseOption{Name: name, Addr: "tailscale", Type: C.Tailscale, UDP: true}),
		node: node,
	}
}

// DialContext implements C.ProxyAdapter.
func (a *Adapter) DialContext(ctx context.Context, metadata *C.Metadata) (C.Conn, error) {
	srv, err := a.node.ready(ctx)
	if err != nil {
		return nil, err
	}
	dst, err := a.destination(ctx, metadata)
	if err != nil {
		return nil, err
	}
	ns, err := srv.Netstack(ctx)
	if err != nil {
		return nil, err
	}
	v4, v6 := srv.TailscaleIPs()
	src := v4
	if dst.Addr().Is6() {
		src = v6
	}
	if !src.IsValid() {
		return nil, fmt.Errorf("tailscale has no address for %s", dst)
	}
	conn, err := ns.DialContextTCPWithBind(ctx, src, dst)
	if err != nil {
		return nil, err
	}
	return outbound.NewConn(conn, a), nil
}

// ListenPacketContext implements C.ProxyAdapter.
func (a *Adapter) ListenPacketContext(ctx context.Context, metadata *C.Metadata) (C.PacketConn, error) {
	srv, err := a.node.ready(ctx)
	if err != nil {
		return nil, err
	}
	dst, err := a.destination(ctx, metadata)
	if err != nil {
		return nil, err
	}
	metadata.DstIP = dst.Addr()
	v4, v6 := srv.TailscaleIPs()
	src := v4
	if dst.Addr().Is6() {
		src = v6
	}
	if !src.IsValid() {
		return nil, fmt.Errorf("tailscale has no address for %s", dst)
	}
	pc, err := srv.ListenPacket("udp", net.JoinHostPort(src.String(), "0"))
	if err != nil {
		return nil, err
	}
	return outbound.NewPacketConn(pc, a), nil
}

// destination resolves the target: its IP, or its name through
// Tailscale's DNS (MagicDNS, then the tailnet's resolvers).
func (a *Adapter) destination(ctx context.Context, metadata *C.Metadata) (netip.AddrPort, error) {
	port := metadata.DstPort
	if metadata.DstIP.IsValid() {
		return netip.AddrPortFrom(metadata.DstIP.Unmap(), port), nil
	}
	if metadata.Host == "" {
		return netip.AddrPort{}, errors.New("no destination")
	}
	ip, err := a.node.lookup(ctx, metadata.Host)
	if err != nil {
		return netip.AddrPort{}, fmt.Errorf("resolve %s through tailscale: %w", metadata.Host, err)
	}
	return netip.AddrPortFrom(ip, port), nil
}

// IsL3Protocol implements C.ProxyAdapter: DNS through this outbound needs
// names resolved first, as for WireGuard.
func (a *Adapter) IsL3Protocol(*C.Metadata) bool { return true }

// SupportUOT implements C.ProxyAdapter.
func (a *Adapter) SupportUOT() bool { return false }

// Close implements C.ProxyAdapter; the node outlives the configuration.
func (a *Adapter) Close() error { return nil }

// lookup resolves a name to its first IPv4 address, else IPv6.
func (n *Node) lookup(ctx context.Context, host string) (netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip, nil
	}
	name := D.Fqdn(strings.ToLower(host))
	var lastErr error = errors.New("no address")
	for _, qt := range []string{"A", "AAAA"} {
		resp, err := n.QueryDNS(ctx, name, qt)
		if err != nil {
			lastErr = err
			continue
		}
		var msg D.Msg
		if err := msg.Unpack(resp); err != nil {
			lastErr = err
			continue
		}
		for _, rr := range msg.Answer {
			switch r := rr.(type) {
			case *D.A:
				if ip, ok := netip.AddrFromSlice(r.A.To4()); ok {
					return ip, nil
				}
			case *D.AAAA:
				if ip, ok := netip.AddrFromSlice(r.AAAA); ok {
					return ip, nil
				}
			}
		}
	}
	return netip.Addr{}, lastErr
}

// DNSClient answers mihomo's ts:// name servers with the node's DNS.
type DNSClient struct {
	Name string
	Node *Node
}

// Address implements mihomo's DNS client.
func (c DNSClient) Address() string { return "tailscale://" + c.Name }

// ResetConnection implements mihomo's DNS client.
func (c DNSClient) ResetConnection() {}

// ExchangeContext implements mihomo's DNS client.
func (c DNSClient) ExchangeContext(ctx context.Context, msg *D.Msg) (*D.Msg, error) {
	if len(msg.Question) == 0 {
		return nil, errors.New("a DNS query needs a question")
	}
	q := msg.Question[0]
	qtype, ok := D.TypeToString[q.Qtype]
	if !ok {
		return nil, fmt.Errorf("unsupported query type %d", q.Qtype)
	}
	resp, err := c.Node.QueryDNS(ctx, q.Name, qtype)
	if err != nil {
		return nil, err
	}
	var out D.Msg
	if err := out.Unpack(resp); err != nil {
		return nil, err
	}
	out.Id = msg.Id
	return &out, nil
}
