package main

import (
	"runtime"
	"runtime/debug"
	"strings"
)

func coreBuildDescription() string {
	parts := []string{runtime.Version(), runtime.GOOS + "/" + runtime.GOARCH}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			switch dep.Path {
			case "github.com/sagernet/sing-box":
				if dep.Replace != nil {
					dep = dep.Replace
				}
				parts = append(parts, "core="+dep.Path+"@"+dep.Version)
			case "github.com/amnezia-vpn/amneziawg-go/v3":
				parts = append(parts, "AWG="+dep.Version)
			}
		}
	}
	return strings.Join(parts, "; ")
}
