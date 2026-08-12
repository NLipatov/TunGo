//go:build windows

package ipcfg

import (
	"tungo/internal/tun/internal/windows/ipcfg/network_interface/resolver"
)

func NewV4() Contract {
	return newV4(
		resolver.NewResolver(),
	)
}

func NewV6() Contract {
	return newV6(
		resolver.NewResolver(),
	)
}
