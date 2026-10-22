//go:build with_awg

package awg

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/sagernet/sing-box/option"
	"github.com/sagernet/sing/common/json/badoption"
)

// Range accepts both legacy JSON numbers and AWG 3.1's "min-max" strings.
type Range string

func (r *Range) UnmarshalJSON(data []byte) error {
	var value string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
	} else {
		value = string(data)
	}
	*r = Range(strings.TrimSpace(value))
	return nil
}

func parseRange(value string, bits int) (uint64, uint64, error) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) > 2 {
		return 0, 0, fmt.Errorf("expected an unsigned integer or min-max range")
	}
	first, err := strconv.ParseUint(parts[0], 10, bits)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid unsigned %d-bit range", bits)
	}
	last := first
	if len(parts) == 2 {
		last, err = strconv.ParseUint(parts[1], 10, bits)
		if err != nil || last < first {
			return 0, 0, fmt.Errorf("invalid unsigned %d-bit range", bits)
		}
	}
	return first, last, nil
}

type Options struct {
	UseIntegratedTun       bool                             `json:"useIntegratedTun,omitempty"`
	PrivateKey             string                           `json:"private_key"`
	Address                badoption.Listable[netip.Prefix] `json:"address"`
	MTU                    uint32                           `json:"mtu,omitempty"`
	ListenPort             uint16                           `json:"listen_port,omitempty"`
	Jc                     int                              `json:"jc,omitempty"`
	Jmin                   int                              `json:"jmin,omitempty"`
	Jmax                   int                              `json:"jmax,omitempty"`
	S1                     int                              `json:"s1,omitempty"`
	S2                     int                              `json:"s2,omitempty"`
	S3                     int                              `json:"s3,omitempty"`
	S4                     int                              `json:"s4,omitempty"`
	H1                     Range                            `json:"h1,omitempty"`
	H2                     Range                            `json:"h2,omitempty"`
	H3                     Range                            `json:"h3,omitempty"`
	H4                     Range                            `json:"h4,omitempty"`
	I1                     string                           `json:"i1,omitempty"`
	I2                     string                           `json:"i2,omitempty"`
	I3                     string                           `json:"i3,omitempty"`
	I4                     string                           `json:"i4,omitempty"`
	I5                     string                           `json:"i5,omitempty"`
	HeaderProtectionKey    string                           `json:"header_protection_key,omitempty"`
	ContentPaddingAddition Range                            `json:"content_padding_addition,omitempty"`
	RekeyAfterTime         Range                            `json:"rekey_after_time,omitempty"`
	RekeyTimeout           Range                            `json:"rekey_timeout,omitempty"`
	RejectAfterTime        Range                            `json:"reject_after_time,omitempty"`
	KeepaliveTimeout       Range                            `json:"keepalive_timeout,omitempty"`
	MaxHandshakeAttempts   Range                            `json:"max_handshake_attempts,omitempty"`
	RandomTrailers         bool                             `json:"random_trailers,omitempty"`
	DisableCookies         bool                             `json:"disable_cookies,omitempty"`
	Peers                  []PeerOptions                    `json:"peers"`
	option.DialerOptions
}

type PeerOptions struct {
	Address                     string                           `json:"address,omitempty"`
	Port                        uint16                           `json:"port,omitempty"`
	PublicKey                   string                           `json:"public_key"`
	PresharedKey                string                           `json:"pre_shared_key,omitempty"`
	AllowedIPs                  badoption.Listable[netip.Prefix] `json:"allowed_ips,omitempty"`
	PersistentKeepaliveInterval Range                            `json:"persistent_keepalive_interval,omitempty"`
}
