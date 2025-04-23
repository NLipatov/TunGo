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

// RouteAddToServer selects the same gateway/interface the system would use for destIP and installs a specific route.
func RouteAddToServer(destIP string) error {
	out, err := exec.Command("route", "get", destIP).CombinedOutput()
	if err != nil {
		return fmt.Errorf("route get %s failed: %w", destIP, err)
	}

	var gateway, iface string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(strings.TrimSpace(line))
		if len(f) < 2 {
			continue
		}
		switch f[0] {
		case "gateway:":
			gateway = f[1]
		case "interface:":
			iface = f[1]
		}
	}

	if gateway != "" {
		return RouteAddViaGateway(destIP, gateway)
	}
	if iface != "" {
		return RouteAdd(destIP, iface)
	}
	return fmt.Errorf("no route found for %s", destIP)
}

func RouteAdd(ip, iface string) error {
	cmd := exec.Command("route", "add", ip, "-interface", iface)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route add %s via interface %s failed: %v (%s)", ip, iface, err, out)
	}
	return nil
}

func RouteAddViaGateway(ip, gw string) error {
	cmd := exec.Command("route", "add", ip, gw)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route add %s via %s failed: %v (%s)", ip, gw, err, out)
	}
	return nil
}

func RouteAddSplit(dev string) error {
	if out, err := exec.Command("route", "-q", "add", "-net", "0.0.0.0/1", "-interface", dev).CombinedOutput(); err != nil {
		return fmt.Errorf("route add 0.0.0.0/1 failed: %v (%s)", err, out)
	}
	if out, err := exec.Command("route", "-q", "add", "-net", "128.0.0.0/1", "-interface", dev).CombinedOutput(); err != nil {
		return fmt.Errorf("route add 128.0.0.0/1 failed: %v (%s)", err, out)
	}
	return nil
}

func RouteDelSplit(dev string) {
	_ = exec.Command("route", "-q", "delete", "-net", "0.0.0.0/1", "-interface", dev).Run()
	_ = exec.Command("route", "-q", "delete", "-net", "128.0.0.0/1", "-interface", dev).Run()
}

func RouteDel(destIP string) error {
	cmd := exec.Command("route", "delete", destIP)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("route delete %s failed: %v (%s)", destIP, err, out)
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
