//go:build with_awg

// AWG integration adapted from the pinned NekoBox sing-box fork (GPL-3.0).
// The protocol engine is the official AmneziaWG v3 library.
package awg

import (
	"context"
	"net"
	"net/netip"
	"sync"
	"syscall"

	"github.com/amnezia-vpn/amneziawg-go/v3/tun"
	"github.com/amnezia-vpn/amneziawg-go/v3/tun/netstack"
	"github.com/sagernet/sing/common/metadata"
)

type networkTun struct {
	tun.Device
	conn      *netstack.Net
	addresses []netip.Addr
	closeOnce sync.Once
	closeErr  error
}

func newNetworkTun(address []netip.Prefix, mtu uint32) (tunAdapter, error) {
	var localAddresses []netip.Addr
	for _, prefix := range address {
		localAddresses = append(localAddresses, prefix.Addr())
	}

	tun, conn, err := netstack.CreateNetTUN(localAddresses, []netip.Addr{}, int(mtu))
	if err != nil {
		return nil, err
	}

	return &networkTun{
		Device:    tun,
		conn:      conn,
		addresses: localAddresses,
	}, nil
}

func (t *networkTun) Start() error {
	return nil
}

func (t *networkTun) Close() error {
	t.closeOnce.Do(func() {
		// netstack emits FIN/RST synchronously while aborting TCP endpoints.
		// AWG may already have stopped its TUN reader at this point. Without
		// a consumer, netstack's unbuffered WriteNotify blocks Stack.Close.
		drained := make(chan struct{})
		go func() {
			defer close(drained)
			buffers := [][]byte{make([]byte, 65535)}
			sizes := make([]int, 1)
			for {
				if _, err := t.Device.Read(buffers, sizes, 0); err != nil {
					return
				}
			}
		}()
		t.closeErr = t.Device.Close()
		<-drained
	})
	return t.closeErr
}

func (t *networkTun) DialContext(ctx context.Context, network string, destination metadata.Socksaddr) (net.Conn, error) {
	return t.conn.DialContext(ctx, network, destination.String())
}

func (t *networkTun) ListenPacket(ctx context.Context, destination metadata.Socksaddr) (net.PacketConn, error) {
	// destination is the remote address requested by the routed connection.
	// Binding a local socket to it works only by accident for unspecified
	// addresses and otherwise either fails or makes parallel UDP flows collide
	// (Discord voice opens several such flows at once).  The userspace AWG
	// stack must instead use its own virtual address and let it allocate an
	// ephemeral source port.
	local, ok := t.localAddress(destination.Addr.Is4())
	if !ok {
		return nil, syscall.EAFNOSUPPORT
	}
	return t.conn.ListenUDPAddrPort(netip.AddrPortFrom(local, 0))
}

func (t *networkTun) localAddress(is4 bool) (netip.Addr, bool) {
	for _, addr := range t.addresses {
		if addr.Is4() == is4 {
			return addr, true
		}
	}
	return netip.Addr{}, false
}
