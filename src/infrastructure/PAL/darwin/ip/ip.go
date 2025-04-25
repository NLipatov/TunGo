package ip

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

// LinkAddrAdd assigns an IP/CIDR to the given interface.
// Example: ifconfig <if> inet 10.0.1.20 10.0.1.20 netmask 255.255.255.0
func LinkAddrAdd(ifName, cidr string) error {
	parts := strings.Split(cidr, "/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid CIDR: %s", cidr)
	}
	ipAddr, prefix := parts[0], parts[1]
	netmask := prefixToNetmask(prefix)

	cmd := exec.Command("ifconfig", ifName, "inet", ipAddr, ipAddr, "netmask", netmask)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to assign IP to %s: %v (%s)", ifName, err, out)
	}
	return nil
}
func LinkDel(ifName string) error {
	return exec.Command("ifconfig", ifName, "destroy").Run()
}

func prefixToNetmask(prefix string) string {
	p, err := strconv.Atoi(prefix)
	if err != nil || p < 0 || p > 32 {
		return "255.255.255.255"
	}
	mask := net.CIDRMask(p, 32)
	return fmt.Sprintf("%d.%d.%d.%d", mask[0], mask[1], mask[2], mask[3])
}
