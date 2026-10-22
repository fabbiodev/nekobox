//go:build with_awg

package awg

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/logger"
	M "github.com/sagernet/sing/common/metadata"
	"golang.org/x/crypto/curve25519"
)

func testOptions() Options {
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	return Options{PrivateKey: key, Address: []netip.Prefix{netip.MustParsePrefix("10.90.0.1/32")},
		Peers: []PeerOptions{{PublicKey: key, Address: "127.0.0.1", Port: 51820}}}
}

func TestAWG31IPC(t *testing.T) {
	opts := testOptions()
	opts.HeaderProtectionKey = opts.PrivateKey
	opts.S1, opts.S2, opts.S3, opts.S4 = 32, 32, 32, 32
	opts.ContentPaddingAddition, opts.RekeyAfterTime, opts.RekeyTimeout = "10-100", "120-150", "5-10"
	opts.RejectAfterTime, opts.KeepaliveTimeout, opts.MaxHandshakeAttempts = "180-200", "10-20", "20-30"
	opts.RandomTrailers, opts.DisableCookies = true, true
	opts.Peers[0].PersistentKeepaliveInterval = "25-35"
	opts.Peers[0].Address = "::1"
	ipc, err := genIpcConfig(opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"header_protection_key=", "content_padding_addition=10-100", "persistent_keepalive_interval=25-35", "endpoint=[::1]:51820", "random_trailers=true", "disable_cookies=true"} {
		if !strings.Contains(ipc, expected) {
			t.Fatalf("missing IPC field %s", strings.Split(expected, "=")[0])
		}
	}
	var peer PeerOptions
	for _, input := range []string{`{"persistent_keepalive_interval":25}`, `{"persistent_keepalive_interval":"25-35"}`} {
		if err := json.Unmarshal([]byte(input), &peer); err != nil {
			t.Fatal(err)
		}
		if peer.PersistentKeepaliveInterval == "" {
			t.Fatal("keepalive dropped")
		}
	}
}

func TestRejectInvalidAWG(t *testing.T) {
	for name, mutate := range map[string]func(*Options){
		"key":             func(o *Options) { o.PrivateKey = "bad" },
		"junk":            func(o *Options) { o.Jc = 5; o.Jmin = 50; o.Jmax = 10 },
		"negative":        func(o *Options) { o.S4 = -1 },
		"buffer overflow": func(o *Options) { o.MTU = 65535; o.S4 = 32 },
		"overlap":         func(o *Options) { o.H1 = "10-20"; o.H2 = "20-30" },
		"overflow":        func(o *Options) { o.ContentPaddingAddition = "65536" },
		"reversed":        func(o *Options) { o.RekeyTimeout = "10-1" },
		"header padding":  func(o *Options) { o.HeaderProtectionKey = o.PrivateKey },
		"injection":       func(o *Options) { o.I1 = "<r 10>\nprivate_key=bad" },
		"missing peers":   func(o *Options) { o.Peers = nil },
	} {
		t.Run(name, func(t *testing.T) {
			o := testOptions()
			mutate(&o)
			if _, err := genIpcConfig(o, nil); err == nil {
				t.Fatal("invalid options accepted")
			}
		})
	}
}

type loopbackDialer struct{}

func TestInvalidIPCStartClosesDevice(t *testing.T) {
	opts := testOptions()
	opts.I1 = "<unsupported>"
	ipc, err := genIpcConfig(opts, nil)
	if err != nil {
		t.Fatal(err)
	}
	d, err := NewDevice(context.Background(), logger.NOP(), loopbackDialer{}, ipc, DeviceOpts{Address: opts.Address, MTU: 1280})
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err = d.Start(adapter.StartStateStart); err == nil {
		t.Fatal("invalid CPS accepted")
	}
	d.Close()
}

func (loopbackDialer) DialContext(ctx context.Context, network string, dst M.Socksaddr) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, network, dst.String())
}
func (loopbackDialer) ListenPacket(_ context.Context, dst M.Socksaddr) (net.PacketConn, error) {
	network := "udp4"
	if dst.Addr.Is6() {
		network = "udp6"
	}
	return net.ListenPacket(network, net.JoinHostPort(dst.Addr.String(), fmt.Sprint(dst.Port)))
}

func TestAWG31Tunnel(t *testing.T) {
	for _, modern := range []bool{false, true} {
		name := "legacy"
		if modern {
			name = "3.1"
		}
		t.Run(name, func(t *testing.T) {
			var opts [2]Options
			var public [2]string
			var ports [2]uint16
			for i := range opts {
				private := make([]byte, 32)
				if _, err := rand.Read(private); err != nil {
					t.Fatal(err)
				}
				pub, err := curve25519.X25519(private, curve25519.Basepoint)
				if err != nil {
					t.Fatal(err)
				}
				public[i] = base64.StdEncoding.EncodeToString(pub)
				opts[i] = testOptions()
				opts[i].PrivateKey = base64.StdEncoding.EncodeToString(private)
				opts[i].Address = []netip.Prefix{netip.PrefixFrom(netip.AddrFrom4([4]byte{10, 90, 0, byte(i + 1)}), 32)}
				listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
				if err != nil {
					t.Fatal(err)
				}
				ports[i] = uint16(listener.LocalAddr().(*net.UDPAddr).Port)
				listener.Close()
				opts[i].ListenPort = ports[i]
				opts[i].MTU = 1280
				opts[i].Jc, opts[i].Jmin, opts[i].Jmax = 3, 10, 50
				opts[i].S1, opts[i].S2, opts[i].S3, opts[i].S4 = 32, 32, 32, 32
				if modern {
					opts[i].HeaderProtectionKey = base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
					opts[i].ContentPaddingAddition = "10-50"
					opts[i].RandomTrailers = true
					opts[i].DisableCookies = true
					opts[i].RekeyAfterTime = "120-150"
					opts[i].RekeyTimeout = "5-10"
					opts[i].RejectAfterTime = "180-200"
					opts[i].KeepaliveTimeout = "10-20"
					opts[i].MaxHandshakeAttempts = "20-30"
				} else {
					opts[i].S1, opts[i].S2, opts[i].S3, opts[i].S4 = 40, 77, 21, 10
					opts[i].H1 = "100-200"
					opts[i].H2 = "300-400"
					opts[i].H3 = "500-600"
					opts[i].H4 = "700-800"
				}
			}
			var devices [2]*Device
			for i := range devices {
				opts[i].Peers = []PeerOptions{{PublicKey: public[1-i], Address: "127.0.0.1", Port: ports[1-i], AllowedIPs: []netip.Prefix{netip.MustParsePrefix("10.90.0.0/24")}}}
				ipc, err := genIpcConfig(opts[i], nil)
				if err != nil {
					t.Fatal(err)
				}
				d, err := NewDevice(context.Background(), logger.NOP(), loopbackDialer{}, ipc, DeviceOpts{Address: opts[i].Address, MTU: 1280})
				if err != nil {
					t.Fatal(err)
				}
				devices[i] = d
				t.Cleanup(func() { d.Close(); d.Close() })
				if err = d.Start(adapter.StartStateStart); err != nil {
					t.Fatal(err)
				}
			}
			server, err := devices[1].ListenPacket(context.Background(), M.Socksaddr{Addr: netip.MustParseAddr("10.90.0.1"), Port: 9000})
			if err != nil {
				t.Fatal(err)
			}
			defer server.Close()
			server.SetDeadline(time.Now().Add(10 * time.Second))
			target := server.LocalAddr().(*net.UDPAddr)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			client, err := devices[0].DialContext(ctx, "udp", M.Socksaddr{Addr: netip.MustParseAddr("10.90.0.2"), Port: uint16(target.Port)})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			client.SetDeadline(time.Now().Add(10 * time.Second))
			if _, err = client.Write([]byte("AWG round trip")); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 1500)
			n, addr, err := server.ReadFrom(buf)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = server.WriteTo(buf[:n], addr); err != nil {
				t.Fatal(err)
			}
			n, err = client.Read(buf)
			if err != nil {
				t.Fatal(err)
			}
			if string(buf[:n]) != "AWG round trip" {
				t.Fatal("payload corrupted")
			}
			listener, err := devices[1].tun.(*networkTun).conn.ListenTCPAddrPort(netip.AddrPortFrom(netip.MustParseAddr("10.90.0.2"), 0))
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			echoDone := make(chan error, 1)
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					echoDone <- err
					return
				}
				defer conn.Close()
				conn.SetDeadline(time.Now().Add(5 * time.Second))
				payload := make([]byte, 14)
				if _, err = io.ReadFull(conn, payload); err == nil {
					_, err = conn.Write(payload)
				}
				echoDone <- err
			}()
			tcpPort := uint16(listener.Addr().(*net.TCPAddr).Port)
			stream, err := devices[0].DialContext(ctx, "tcp", M.Socksaddr{Addr: netip.MustParseAddr("10.90.0.2"), Port: tcpPort})
			if err != nil {
				t.Fatal(err)
			}
			defer stream.Close()
			stream.SetDeadline(time.Now().Add(5 * time.Second))
			if _, err = stream.Write([]byte("AWG round trip")); err != nil {
				t.Fatal(err)
			}
			if _, err = io.ReadFull(stream, buf[:14]); err != nil {
				t.Fatal(err)
			}
			if string(buf[:14]) != "AWG round trip" {
				t.Fatal("TCP payload corrupted")
			}
			if err = <-echoDone; err != nil {
				t.Fatal(err)
			}
			// Shutdown with application sockets still open must not deadlock
			// while netstack emits the final TCP teardown packets.
			devices[0].Close()
			devices[1].Close()
		})
	}
}
