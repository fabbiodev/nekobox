//go:build with_awg

package coreinclude

import (
	"github.com/sagernet/sing-box/adapter/endpoint"
	"nekobox_core/internal/awg"
)

func registerAWG(registry *endpoint.Registry) { awg.RegisterEndpoint(registry) }
