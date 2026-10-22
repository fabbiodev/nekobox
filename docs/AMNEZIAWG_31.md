# AmneziaWG 3.1

The desktop client supports `.conf`, `wg://` / `awg://`, sing-box endpoint
JSON, and Amnezia `vpn://` profiles. Import, editing, storage and share-link
export preserve HeaderProtectionKey, ContentPaddingAddition, RekeyAfterTime,
RekeyTimeout, RejectAfterTime, KeepaliveTimeout, MaxHandshakeAttempts,
RandomTrailers, DisableCookies and a PersistentKeepalive range. The editor uses
the protocol's field names, so they match an exported Amnezia configuration.

The local `core/server/internal/awg` endpoint uses the official
`amnezia-vpn/amneziawg-go/v3` library, pinned to `v3.1.20260828`. It overrides the
older AWG endpoint in both RPC and CLI registries. The existing private
sing-box revision and its routing/Reality/XHTTP changes remain in use.

Windows and Linux builders use Go 1.26.10. This includes the Windows asynchronous
socket I/O lifetime fix discussed in [Go issue 77975](https://github.com/golang/go/issues/77975).
The build checks cached core metadata and tracks Go source/module changes to
avoid packaging a stale core. Startup logs identify the Go, core and AWG versions.

AWG startup defers TUN reads and events until IPC configuration is applied.
Otherwise a read started before S4 is configured can use the wrong prefix for
the first packet. Invalid ranges, keys, overlapping headers, and padding/MTU
that exceed the platform's packet buffers are rejected before startup.

Shutdown drains netstack output while closing TCP endpoints. Without a reader,
the stack can block on its final FIN/RST packets after AWG has stopped reading.
The tunnel test closes devices with application sockets still open and checks
that repeated shutdown completes on Windows and Linux.

Regression checks:

```sh
cd core/server
go test -tags with_awg,with_gvisor ./internal/awg ./internal/coreinclude -count=20 -timeout=120s
```

These checks exchange encrypted TCP and UDP payloads in both directions through two
local tunnels using legacy parameters and AWG 3.1 header protection with a
nonzero key, randomized timing, content padding and trailers. They also cover
registry decoding, malformed configurations, failed startup and repeated close.
They run natively on Windows and Linux in `core-regression.yml`.

Protocol details: [official AmneziaWG documentation](https://docs.amnezia.org/documentation/amnezia-wg/).
