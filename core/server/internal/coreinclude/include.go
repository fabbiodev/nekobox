package coreinclude

import (
	"context"

	box "github.com/sagernet/sing-box"
	"github.com/sagernet/sing-box/adapter/endpoint"
	"github.com/sagernet/sing-box/include"
)

func EndpointRegistry() *endpoint.Registry {
	registry := include.EndpointRegistry()
	registerAWG(registry)
	return registry
}

func Context(ctx context.Context) context.Context {
	return box.Context(ctx, include.InboundRegistry(), include.OutboundRegistry(), EndpointRegistry(), include.DNSTransportRegistry(), include.ServiceRegistry())
}
