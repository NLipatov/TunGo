package ip

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"tungo/infrastructure/PAL"
)

type Wrapper struct {
	commander PAL.Commander
}

func NewWrapper(commander PAL.Commander) *Wrapper {
	return &Wrapper{commander: commander}
}

// LinkAddrAdd assigns an IP/CIDR to the given interface.
// Example: ifconfig <if> inet 10.0.1.20 10.0.1.20 netmask 255.255.255.0
func (w *Wrapper) LinkAddrAdd(ifName, cidr string) error {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR: %s", cidr)
	}
	ipAddr, prefix := parts[0], parts[1]
	netmask := w.prefixToNetmask(prefix)

	if out, err := w.commander.CombinedOutput("ifconfig", ifName, "inet", ipAddr, ipAddr, "netmask", netmask); err != nil {
		return fmt.Errorf("failed to assign IP to %s: %v (%s)", ifName, err, out)
	}
	return nil
}

func (w *Wrapper) prefixToNetmask(prefix string) string {
	p, err := strconv.Atoi(prefix)
	if err != nil || p < 0 || p > 32 {
		return "255.255.255.255"
	}
	mask := net.CIDRMask(p, 32)
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}
