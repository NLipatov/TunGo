//go:build windows

package resolver

import "golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"

type Contract interface {
	NetworkInterfaceByName(ifName string) (winipcfg.LUID, error)
	NetworkInterfaceName(luid winipcfg.LUID) string
}
