//go:build !with_awg

package coreinclude

import "github.com/sagernet/sing-box/adapter/endpoint"

func registerAWG(*endpoint.Registry) {}
