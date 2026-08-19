//go:build darwin

package ifconfig

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"

	"tungo/internal/platform/command"
)

type V4 struct {
	commander command.Runner
}

func NewV4(commander command.Runner) *V4 {
	return &V4{commander: commander}
}

func (v V4) LinkAddrAdd(ifName string, prefix netip.Prefix) error {
	if !prefix.IsValid() || !prefix.Addr().Is4() {
		return fmt.Errorf("not an IPv4 prefix: %s", prefix)
	}
	ip := prefix.Addr().String()
	mask := net.CIDRMask(prefix.Bits(), 32)
	netmask := fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
	if out, outErr := v.commander.CombinedOutput("ifconfig", ifName, "inet", ip, ip, "netmask", netmask, "up"); outErr != nil {
		return fmt.Errorf("failed to assign IPv4 to %s: %v (%s)", ifName, outErr, out)
	}
	return nil
}

func (v V4) SetMTU(ifName string, mtu int) error {
	if out, err := v.commander.CombinedOutput("ifconfig", ifName, "mtu", strconv.Itoa(mtu)); err != nil {
		return fmt.Errorf("ifconfig set mtu failed: %w; output: %s", err, string(out))
	}
	return nil
}
