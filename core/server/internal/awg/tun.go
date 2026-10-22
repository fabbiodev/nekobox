//go:build with_awg

// AWG integration adapted from the pinned NekoBox sing-box fork (GPL-3.0).
// The protocol engine is the official AmneziaWG v3 library.
package awg

import (
	awgTun "github.com/amnezia-vpn/amneziawg-go/v3/tun"
	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/network"
)

type tunAdapter interface {
	network.Dialer
	awgTun.Device
	adapter.SimpleLifecycle
}
