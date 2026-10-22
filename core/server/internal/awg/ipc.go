//go:build with_awg

package awg

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/amnezia-vpn/amneziawg-go/v3/device"
)

func keyHex(value, field string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) != 32 {
		return "", fmt.Errorf("%s must be a base64-encoded 32-byte key", field)
	}
	return hex.EncodeToString(key), nil
}

// genIpcConfig validates before creating a TUN or starting protocol workers.
// Values are never included in errors or logs because IPC contains private keys.
func genIpcConfig(opts Options, resolvePeer func(string) (netip.Addr, error)) (string, error) {
	if len(opts.Address) == 0 || len(opts.Peers) == 0 {
		return "", fmt.Errorf("AWG requires a local address and at least one peer")
	}
	if opts.MTU != 0 && (opts.MTU < 576 || opts.MTU > 65535) {
		return "", fmt.Errorf("AWG MTU must be between 576 and 65535")
	}
	private, err := keyHex(opts.PrivateKey, "private_key")
	if err != nil {
		return "", err
	}
	var ipc strings.Builder
	add := func(key, value string) { fmt.Fprintf(&ipc, "%s=%s\n", key, value) }
	add("private_key", private)
	if opts.ListenPort != 0 {
		add("listen_port", strconv.Itoa(int(opts.ListenPort)))
	}
	for _, field := range []struct {
		name  string
		value int
	}{
		{"jc", opts.Jc}, {"jmin", opts.Jmin}, {"jmax", opts.Jmax},
		{"s1", opts.S1}, {"s2", opts.S2}, {"s3", opts.S3}, {"s4", opts.S4},
	} {
		if field.value < 0 || field.value > 65535 {
			return "", fmt.Errorf("%s must be between 0 and 65535", field.name)
		}
		if field.value != 0 {
			add(field.name, strconv.Itoa(field.value))
		}
	}
	if opts.Jc > 0 && (opts.Jmin <= 0 || opts.Jmax < opts.Jmin) {
		return "", fmt.Errorf("junk packets require 0 < jmin <= jmax")
	}
	mtu := opts.MTU
	if mtu == 0 {
		mtu = 1408
	}
	if uint64(mtu)+uint64(opts.S4)+device.MessageTransportSize > device.MaxMessageSize {
		return "", fmt.Errorf("MTU plus S4 exceeds the AWG packet buffer on this platform")
	}
	if opts.S1+device.MessageInitiationSize > device.MaxMessageSize ||
		opts.S2+device.MessageResponseSize > device.MaxMessageSize ||
		opts.S3+device.MessageCookieReplySize > device.MaxMessageSize {
		return "", fmt.Errorf("handshake padding exceeds the AWG packet buffer on this platform")
	}
	headers := []Range{opts.H1, opts.H2, opts.H3, opts.H4}
	var lower, upper [4]uint64
	for i, header := range headers {
		lower[i], upper[i] = uint64(i+1), uint64(i+1)
		if header == "" {
			continue
		}
		lower[i], upper[i], err = parseRange(string(header), 32)
		if err != nil {
			return "", fmt.Errorf("h%d: %w", i+1, err)
		}
		add(fmt.Sprintf("h%d", i+1), string(header))
	}
	for i := range headers {
		for j := i + 1; j < len(headers); j++ {
			if lower[i] <= upper[j] && lower[j] <= upper[i] {
				return "", fmt.Errorf("h%d and h%d ranges overlap", i+1, j+1)
			}
		}
	}
	for i, signature := range []string{opts.I1, opts.I2, opts.I3, opts.I4, opts.I5} {
		if strings.ContainsAny(signature, "\r\n\x00") {
			return "", fmt.Errorf("i%d must be a single CPS expression", i+1)
		}
		if signature != "" {
			add(fmt.Sprintf("i%d", i+1), signature)
		}
	}
	if opts.HeaderProtectionKey != "" {
		key, keyErr := keyHex(opts.HeaderProtectionKey, "header_protection_key")
		if keyErr != nil {
			return "", keyErr
		}
		if opts.S1 < 12 || opts.S2 < 12 || opts.S3 < 12 || opts.S4 < 12 {
			return "", fmt.Errorf("header protection requires s1, s2, s3 and s4 >= 12")
		}
		add("header_protection_key", key)
	}
	for _, field := range []struct {
		name  string
		value Range
	}{
		{"content_padding_addition", opts.ContentPaddingAddition}, {"rekey_after_time", opts.RekeyAfterTime},
		{"rekey_timeout", opts.RekeyTimeout}, {"reject_after_time", opts.RejectAfterTime},
		{"keepalive_timeout", opts.KeepaliveTimeout}, {"max_handshake_attempts", opts.MaxHandshakeAttempts},
	} {
		if field.value == "" {
			continue
		}
		if _, _, err := parseRange(string(field.value), 16); err != nil {
			return "", fmt.Errorf("%s: %w", field.name, err)
		}
		add(field.name, string(field.value))
	}
	add("random_trailers", strconv.FormatBool(opts.RandomTrailers))
	add("disable_cookies", strconv.FormatBool(opts.DisableCookies))
	for _, peer := range opts.Peers {
		key, err := keyHex(peer.PublicKey, "public_key")
		if err != nil {
			return "", err
		}
		add("public_key", key)
		if peer.PresharedKey != "" {
			key, err = keyHex(peer.PresharedKey, "pre_shared_key")
			if err != nil {
				return "", err
			}
			add("preshared_key", key)
		}
		if peer.Address != "" {
			address, err := netip.ParseAddr(peer.Address)
			if err != nil {
				if resolvePeer == nil {
					return "", fmt.Errorf("AWG peer endpoint requires a resolver")
				}
				address, err = resolvePeer(peer.Address)
				if err != nil {
					return "", fmt.Errorf("resolve AWG peer: %w", err)
				}
			}
			if !address.IsValid() || peer.Port == 0 {
				return "", fmt.Errorf("invalid AWG peer endpoint")
			}
			add("endpoint", net.JoinHostPort(address.String(), strconv.Itoa(int(peer.Port))))
		}
		if peer.PersistentKeepaliveInterval != "" {
			if _, _, err := parseRange(string(peer.PersistentKeepaliveInterval), 16); err != nil {
				return "", fmt.Errorf("persistent_keepalive_interval: %w", err)
			}
			add("persistent_keepalive_interval", string(peer.PersistentKeepaliveInterval))
		}
		for _, prefix := range peer.AllowedIPs {
			add("allowed_ip", prefix.String())
		}
	}
	return ipc.String(), nil
}
