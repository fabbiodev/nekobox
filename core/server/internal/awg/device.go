//go:build with_awg

// AWG integration adapted from the pinned NekoBox sing-box fork (GPL-3.0).
// The protocol engine is the official AmneziaWG v3 library.
package awg

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"

	"github.com/amnezia-vpn/amneziawg-go/v3/conn"
	"github.com/amnezia-vpn/amneziawg-go/v3/device"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing/common/exceptions"
	E "github.com/sagernet/sing/common/exceptions"
	"github.com/sagernet/sing/common/logger"
	"github.com/sagernet/sing/common/metadata"
	"github.com/sagernet/sing/common/network"
)

type DeviceOpts struct {
	UseIntegratedTun bool
	Address          []netip.Prefix
	AllowedIps       []netip.Prefix
	ExcludedIps      []netip.Prefix
	MTU              uint32
}

type Device struct {
	awgDevice *device.Device
	tun       tunAdapter
	bind      conn.Bind
	logger    *device.Logger
	ipcConfig string
	closeOnce sync.Once
}

func NewDevice(ctx context.Context, logger logger.ContextLogger, dial network.Dialer, ipcConfig string, opts DeviceOpts) (*Device, error) {
	var (
		tun tunAdapter
		err error
	)

	if opts.UseIntegratedTun {
		tun, err = newSystemTun(ctx, opts.Address, opts.AllowedIps, opts.ExcludedIps, opts.MTU, logger)
		if err != nil {
			return nil, exceptions.Cause(err, "create tunnel")
		}
	} else {
		tun, err = newNetworkTun(opts.Address, opts.MTU)
		if err != nil {
			return nil, err
		}
	}

	awgLogger := &device.Logger{
		Verbosef: func(format string, args ...interface{}) {
			logger.Debug(fmt.Sprintf(strings.ToLower(format), args...))
		},
		Errorf: func(format string, args ...interface{}) {
			logger.Error(fmt.Sprintf(strings.ToLower(format), args...))
		},
	}

	return &Device{
		tun:       tun,
		bind:      newBind(ctx, dial),
		logger:    awgLogger,
		ipcConfig: ipcConfig,
	}, nil
}

func (d *Device) Start(stage adapter.StartStage) error {
	if stage != adapter.StartStateStart {
		return nil
	}

	startup := newStartupTun(d.tun)
	d.awgDevice = device.NewDevice(startup, d.bind, d.logger)
	err := d.awgDevice.IpcSet(d.ipcConfig)
	startup.release()
	if err != nil {
		d.Close()
		return E.Cause(err, "set ipc config")
	}

	if err := d.tun.Start(); err != nil {
		d.Close()
		return E.Cause(err, "tun start")
	}

	if err := d.awgDevice.Up(); err != nil {
		d.Close()
		return err
	}
	return nil
}

func (d *Device) Close() error {
	d.closeOnce.Do(func() {
		if d.awgDevice != nil {
			d.awgDevice.Close()
		} else {
			d.tun.Close()
			d.bind.Close()
		}
	})
	return nil
}

func (d *Device) DialContext(ctx context.Context, network string, destination metadata.Socksaddr) (net.Conn, error) {
	return d.tun.DialContext(ctx, network, destination)
}

func (d *Device) ListenPacket(ctx context.Context, destination metadata.Socksaddr) (net.PacketConn, error) {
	return d.tun.ListenPacket(ctx, destination)
}
