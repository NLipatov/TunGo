//go:build darwin

package ifconfig

import (
	"fmt"
	"net/netip"
	"strconv"

	"tungo/internal/platform/command"
)

type V6 struct {
	commander command.Runner
}

func NewV6(commander command.Runner) *V6 {
	return &V6{commander: commander}
}

func (v V6) LinkAddrAdd(ifName string, prefix netip.Prefix) error {
	addr := prefix.Addr()
	if !prefix.IsValid() || !addr.Is6() || addr.Is4In6() {
		return fmt.Errorf("not an IPv6 prefix: %s", prefix)
	}
	if out, outErr := v.commander.CombinedOutput("ifconfig", ifName, "inet6", addr.String(), "prefixlen", strconv.Itoa(prefix.Bits()), "up"); outErr != nil {
		return fmt.Errorf("failed to assign IPv6 to %s: %v (%s)", ifName, outErr, out)
	}
	return nil
}

func (v V6) SetMTU(ifName string, mtu int) error {
	if out, err := v.commander.CombinedOutput("ifconfig", ifName, "mtu", strconv.Itoa(mtu)); err != nil {
		return fmt.Errorf("ifconfig set mtu failed: %w; output: %s", err, string(out))
	}
	return nil
}
