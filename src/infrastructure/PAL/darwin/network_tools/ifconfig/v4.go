//go:build darwin

package ifconfig

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"tungo/infrastructure/PAL"
)

type v4 struct {
	commander PAL.Commander
}

func newV4(commander PAL.Commander) Contract {
	return &v4{commander: commander}
}

func (v v4) LinkAddrAdd(ifName, cidr string) error {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR: %s", cidr)
	}
	ipStr, pfxStr := parts[0], parts[1]

	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() == nil {
		return fmt.Errorf("not an IPv4 CIDR: %s", cidr)
	}
	p, err := strconv.Atoi(pfxStr)
	if err != nil || p < 0 || p > 32 {
		return fmt.Errorf("invalid IPv4 prefix: %q", pfxStr)
	}
	mask := net.CIDRMask(p, 32)
	netmask := fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])

	if out, err := v.commander.CombinedOutput("ifconfig", ifName, "inet", ipStr, ipStr, "netmask", netmask, "up"); err != nil {
		return fmt.Errorf("failed to assign IPv4 to %s: %v (%s)", ifName, err, out)
	}
	return nil
}

func (v v4) SetMTU(ifName string, mtu int) error {
	if mtu <= 0 {
		return nil
	}
	if out, err := v.commander.CombinedOutput("ifconfig", ifName, "mtu", strconv.Itoa(mtu)); err != nil {
		return fmt.Errorf("ifconfig set mtu failed: %w; output: %s", err, string(out))
	}
	return nil
}
