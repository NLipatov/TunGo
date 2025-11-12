//go:build darwin

package ifconfig

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"tungo/infrastructure/PAL"
)

type v6 struct {
	commander PAL.Commander
}

func newV6(commander PAL.Commander) Contract {
	return &v6{commander: commander}
}

func (v v6) LinkAddrAdd(ifName, cidr string) error {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR: %s", cidr)
	}
	ipStr, pfxStr := parts[0], parts[1]

	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() != nil {
		return fmt.Errorf("not an IPv6 CIDR: %s", cidr)
	}
	p, err := strconv.Atoi(pfxStr)
	if err != nil || p < 0 || p > 128 {
		p = 128
	}

	if out, err := v.commander.CombinedOutput("ifconfig", ifName, "inet6", ipStr, "prefixlen", strconv.Itoa(p), "up"); err != nil {
		return fmt.Errorf("failed to assign IPv6 to %s: %v (%s)", ifName, err, out)
	}
	return nil
}

func (v v6) SetMTU(ifName string, mtu int) error {
	if mtu <= 0 {
		return nil
	}
	if out, err := v.commander.CombinedOutput("ifconfig", ifName, "mtu", strconv.Itoa(mtu)); err != nil {
		return fmt.Errorf("ifconfig set mtu failed: %w; output: %s", err, string(out))
	}
	return nil
}
