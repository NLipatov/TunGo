//go:build windows

package netsh

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"tungo/infrastructure/PAL"
)

// v4Wrapper is an IPv4 implementation of netsh.Contract.
type v4Wrapper struct {
	commander PAL.Commander
}

func newV4Wrapper(commander PAL.Commander) Contract { return &v4Wrapper{commander: commander} }

func (w *v4Wrapper) SetAddressStatic(ifName, ip, mask string) error {
	p := net.ParseIP(ip)
	if p == nil || p.To4() == nil {
		return fmt.Errorf("SetAddressStatic: ip is not IPv4: %q", ip)
	}
	if m := net.ParseIP(mask); m == nil || m.To4() == nil {
		return fmt.Errorf("SetAddressStatic: mask is not dotted IPv4: %q", mask)
	}
	out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"name="+w.q(ifName), "static", ip, mask, "none",
	)
	if err != nil {
		return fmt.Errorf("SetAddressStatic error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v4Wrapper) SetAddressWithGateway(ifName, ip, mask, gw string, metric int) error {
	p := net.ParseIP(ip)
	if p == nil || p.To4() == nil {
		return fmt.Errorf("SetAddressWithGateway: ip is not IPv4: %q", ip)
	}
	if m := net.ParseIP(mask); m == nil || m.To4() == nil {
		return fmt.Errorf("SetAddressWithGateway: mask is not dotted IPv4: %q", mask)
	}
	g := net.ParseIP(gw)
	if g == nil || g.To4() == nil {
		return fmt.Errorf("SetAddressWithGateway: gateway is not IPv4: %q", gw)
	}
	out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "address",
		"name="+w.q(ifName), "static", ip, mask, gw, strconv.Itoa(max(metric, 1)),
	)
	if err != nil {
		return fmt.Errorf("SetAddressWithGateway error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v4Wrapper) DeleteAddress(ifName, interfaceAddress string) error {
	out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ip", "delete", "address",
		"name="+w.q(ifName), "addr="+interfaceAddress,
	)
	if err != nil {
		return fmt.Errorf("DeleteAddress error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v4Wrapper) SetDNS(ifName string, dnsServers []string) error {
	if len(dnsServers) == 0 {
		out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ip", "set", "dns",
			"name="+w.q(ifName), "source=dhcp",
		)
		if err != nil {
			return fmt.Errorf("DNS set DHCP error: %v, output: %s", err, out)
		}
		return nil
	}
	// reset to DHCP first (best-effort)
	_, _ = w.commander.CombinedOutput(
		"netsh", "interface", "ip", "set", "dns",
		"name="+w.q(ifName), "source=dhcp",
	)
	for i, dns := range dnsServers {
		var args []string
		if i == 0 {
			args = []string{"interface", "ip", "set", "dns", "name=" + w.q(ifName), "static", dns, "primary"}
		} else {
			args = []string{"interface", "ip", "add", "dns", "name=" + w.q(ifName), dns, "index=" + strconv.Itoa(i+1)}
		}
		if out, err := w.commander.CombinedOutput("netsh", args...); err != nil {
			return fmt.Errorf("DNS setup error: %v, output: %s", err, out)
		}
	}
	return nil
}

func (w *v4Wrapper) SetMTU(ifName string, mtu int) error {
	out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv4", "set", "subinterface",
		w.q(ifName), "mtu="+strconv.Itoa(mtu), "store=active",
	)
	if err != nil {
		return fmt.Errorf("SetInterfaceMTU error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v4Wrapper) AddRoutePrefix(destinationPrefix, ifName string, metric int) error {
	ip, _, err := net.ParseCIDR(destinationPrefix)
	if err != nil || ip.To4() == nil {
		return fmt.Errorf("bad IPv4 prefix: %q", destinationPrefix)
	}
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv4", "add", "route",
		destinationPrefix,
		"interface="+strconv.Itoa(idx),
		"nexthop=0.0.0.0",
		"metric="+strconv.Itoa(max(metric, 1)),
		"store=active",
	)
	if err != nil {
		return fmt.Errorf("AddRoutePrefix(%s) error: %v, output: %s", destinationPrefix, err, out)
	}
	return nil
}

func (w *v4Wrapper) DeleteRoutePrefix(destinationPrefix, ifName string) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv4", "delete", "route",
		destinationPrefix,
		"interface="+strconv.Itoa(idx),
		"nexthop=0.0.0.0",
	)
	if err != nil {
		return fmt.Errorf("DeleteRoutePrefix(%s) error: %v, output: %s", destinationPrefix, err, out)
	}
	return nil
}

func (w *v4Wrapper) DeleteDefaultRoute(ifName string) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	_, _ = w.commander.CombinedOutput(
		"netsh", "interface", "ipv4", "delete", "route",
		"0.0.0.0/0", "interface="+strconv.Itoa(idx),
	)
	out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv4", "delete", "route",
		"0.0.0.0", "interface="+strconv.Itoa(idx),
	)
	if err != nil {
		return fmt.Errorf("DeleteDefaultRoute error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v4Wrapper) AddHostRouteViaGateway(hostIP, ifName, gateway string, metric int) error {
	g := net.ParseIP(gateway)
	if g == nil || g.To4() == nil {
		return fmt.Errorf("AddHostRouteViaGateway: gateway is not IPv4: %q", gateway)
	}
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	host := strings.TrimSpace(hostIP)
	if net.ParseIP(host).To4() == nil {
		return fmt.Errorf("AddHostRouteViaGateway: not an IPv4: %q", hostIP)
	}
	args := []string{
		"interface", "ipv4", "add", "route",
		host + "/32",
		"interface=" + strconv.Itoa(idx),
		"nexthop=" + gateway,
		"metric=" + strconv.Itoa(max(metric, 1)),
		"store=active",
	}
	out, err := w.commander.CombinedOutput("netsh", args...)
	if err != nil {
		return fmt.Errorf("AddHostRouteViaGateway error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v4Wrapper) AddHostRouteOnLink(hostIP, ifName string, metric int) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	host := strings.TrimSpace(hostIP)
	if net.ParseIP(host).To4() == nil {
		return fmt.Errorf("AddHostRouteOnLink: not an IPv4: %q", hostIP)
	}
	args := []string{
		"interface", "ipv4", "add", "route",
		host + "/32",
		"interface=" + strconv.Itoa(idx),
		"nexthop=0.0.0.0",
		"metric=" + strconv.Itoa(max(metric, 1)),
		"store=active",
	}
	out, err := w.commander.CombinedOutput("netsh", args...)
	if err != nil {
		return fmt.Errorf("AddHostRouteOnLink(v4) error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v4Wrapper) AddDefaultSplitRoutes(ifName string, metric int) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	halves := []string{"0.0.0.0/1", "128.0.0.0/1"}
	for _, p := range halves {
		out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv4", "add", "route",
			p, "interface="+strconv.Itoa(idx),
			"nexthop=0.0.0.0",
			"metric="+strconv.Itoa(max(metric, 1)),
			"store=active",
		)
		if err != nil {
			return fmt.Errorf("AddDefaultSplitRoutes(v4 %s) error: %v, output: %s", p, err, out)
		}
	}
	return nil
}

func (w *v4Wrapper) DeleteDefaultSplitRoutes(ifName string) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	var last error
	halves := []string{"0.0.0.0/1", "128.0.0.0/1"}
	for _, p := range halves {
		if out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv4", "delete", "route",
			p, "interface="+strconv.Itoa(idx),
		); err != nil {
			last = fmt.Errorf("DeleteDefaultSplitRoutes(v4 %s) error: %v, output: %s", p, err, out)
		}
	}
	return last
}

func (w *v4Wrapper) ifIndexOf(name string) (int, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return 0, fmt.Errorf("InterfaceByName(%q): %w", name, err)
	}
	if iface.Index <= 0 {
		return 0, fmt.Errorf("interface %q has invalid index: %d", name, iface.Index)
	}
	return iface.Index, nil
}

func (w *v4Wrapper) q(s string) string { return `"` + s + `"` }
