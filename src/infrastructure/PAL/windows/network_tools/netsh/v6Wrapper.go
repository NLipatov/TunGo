//go:build windows

package netsh

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"tungo/infrastructure/PAL"
)

// v6Wrapper is an IPv6 implementation of netsh.Contract.
type v6Wrapper struct {
	commander PAL.Commander
}

func newV6Wrapper(commander PAL.Commander) Contract { return &v6Wrapper{commander: commander} }

func dropZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}

func (w *v6Wrapper) SetAddressStatic(ifName, ip, mask string) error {
	pl, err := strconv.Atoi(mask)
	if err != nil || pl < 0 || pl > 128 {
		return fmt.Errorf("SetAddressStatic: bad IPv6 prefix length: %q", mask)
	}
	p := net.ParseIP(dropZone(ip))
	if p == nil || p.To4() != nil {
		return fmt.Errorf("SetAddressStatic: ip is not IPv6: %q", ip)
	}
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	address := dropZone(ip) + "/" + mask
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "add", "address",
		"interface="+strconv.Itoa(idx), address, "store=active",
	); err != nil {
		return fmt.Errorf("SetAddressStatic error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v6Wrapper) SetAddressWithGateway(ifName, ip, mask, gateway string, metric int) error {
	gw := dropZone(strings.TrimSpace(gateway))
	pgw := net.ParseIP(gw)
	if pgw == nil || pgw.To4() != nil {
		return fmt.Errorf("gateway is not IPv6: %q", gateway)
	}
	if err := w.SetAddressStatic(ifName, ip, mask); err != nil {
		return err
	}
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "add", "route",
		"::/0",
		"interface="+strconv.Itoa(idx),
		"nexthop="+gw,
		"metric="+strconv.Itoa(max(metric, 1)),
		"store=active",
	); err != nil {
		return fmt.Errorf("SetAddressWithGateway(add default route) error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v6Wrapper) DeleteAddress(ifName, interfaceAddress string) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	addr := dropZone(interfaceAddress)
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "delete", "address",
		"interface="+strconv.Itoa(idx),
		"address="+addr,
	); err != nil {
		return fmt.Errorf("DeleteAddress error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v6Wrapper) SetDNS(ifName string, dnsServers []string) error {
	_, _ = w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "delete", "dnsservers",
		w.q(ifName), "all",
	)
	if len(dnsServers) == 0 {
		return nil
	}
	for i, raw := range dnsServers {
		dns := dropZone(strings.TrimSpace(raw))
		index := i + 1
		if out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv6", "add", "dnsserver",
			w.q(ifName), dns, "index="+strconv.Itoa(index), "validate=no",
		); err != nil {
			return fmt.Errorf("SetDNS(add %s) error: %v, output: %s", dns, err, out)
		}
	}
	return nil
}

func (w *v6Wrapper) SetMTU(ifName string, mtu int) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "set", "interface",
		"interface="+strconv.Itoa(idx), "mtu="+strconv.Itoa(mtu), "store=active",
	); err != nil {
		return fmt.Errorf("SetMTU error: %v, output: %s", err, out)
	}
	return nil
}

// ---------- routes ----------

func (w *v6Wrapper) AddRoutePrefix(prefix, ifName string, metric int) error {
	p := dropZone(strings.TrimSpace(prefix))
	ip, _, err := net.ParseCIDR(p)
	if err != nil || ip.To4() != nil {
		return fmt.Errorf("bad IPv6 prefix: %q", p)
	}
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "add", "route",
		p,
		"interface="+strconv.Itoa(idx),
		"nexthop=::",
		"metric="+strconv.Itoa(max(metric, 1)),
		"store=active",
	); err != nil {
		return fmt.Errorf("AddRoutePrefix(%s) error: %v, output: %s", p, err, out)
	}
	return nil
}

func (w *v6Wrapper) DeleteRoutePrefix(prefix, ifName string) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	p := dropZone(strings.TrimSpace(prefix))
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "delete", "route",
		p,
		"interface="+strconv.Itoa(idx),
		"nexthop=::",
	); err != nil {
		return fmt.Errorf("DeleteRoutePrefix(%s) error: %v, output: %s", p, err, out)
	}
	return nil
}

func (w *v6Wrapper) DeleteDefaultRoute(ifName string) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	if out, err := w.commander.CombinedOutput(
		"netsh", "interface", "ipv6", "delete", "route",
		"::/0",
		"interface="+strconv.Itoa(idx),
	); err != nil {
		return fmt.Errorf("DeleteDefaultRoute error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v6Wrapper) AddHostRouteViaGateway(hostIP, ifName, gateway string, metric int) error {
	gw := dropZone(strings.TrimSpace(gateway))
	pgw := net.ParseIP(gw)
	if pgw == nil || pgw.To4() != nil {
		return fmt.Errorf("gateway is not IPv6: %q", gateway)
	}
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	host := dropZone(strings.TrimSpace(hostIP))
	if net.ParseIP(host) == nil || net.ParseIP(host).To4() != nil {
		return fmt.Errorf("AddHostRouteViaGateway(v6): not an IPv6: %q", hostIP)
	}
	args := []string{
		"interface", "ipv6", "add", "route",
		host + "/128",
		"interface=" + strconv.Itoa(idx),
		"nexthop=" + dropZone(gateway),
		"metric=" + strconv.Itoa(max(metric, 1)),
		"store=active",
	}
	out, err := w.commander.CombinedOutput("netsh", args...)
	if err != nil {
		return fmt.Errorf("AddHostRouteViaGateway(v6) error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v6Wrapper) AddHostRouteOnLink(hostIP, ifName string, metric int) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	host := dropZone(strings.TrimSpace(hostIP))
	if net.ParseIP(host) == nil || net.ParseIP(host).To4() != nil {
		return fmt.Errorf("AddHostRouteOnLink(v6): not an IPv6: %q", hostIP)
	}
	args := []string{
		"interface", "ipv6", "add", "route",
		host + "/128",
		"interface=" + strconv.Itoa(idx),
		"nexthop=::",
		"metric=" + strconv.Itoa(max(metric, 1)),
		"store=active",
	}
	out, err := w.commander.CombinedOutput("netsh", args...)
	if err != nil {
		return fmt.Errorf("AddHostRouteOnLink(v6) error: %v, output: %s", err, out)
	}
	return nil
}

func (w *v6Wrapper) AddDefaultSplitRoutes(ifName string, metric int) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	halves := []string{"::/1", "8000::/1"}
	for _, p := range halves {
		out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv6", "add", "route",
			p, "interface="+strconv.Itoa(idx), "nexthop=::", "metric="+strconv.Itoa(max(metric, 1)), "store=active",
		)
		if err != nil {
			return fmt.Errorf("AddDefaultSplitRoutes(v6 %s) error: %v, output: %s", p, err, out)
		}
	}
	return nil
}

func (w *v6Wrapper) DeleteDefaultSplitRoutes(ifName string) error {
	idx, idxErr := w.ifIndexOf(ifName)
	if idxErr != nil {
		return idxErr
	}
	var last error
	halves := []string{"::/1", "8000::/1"}
	for _, p := range halves {
		if out, err := w.commander.CombinedOutput(
			"netsh", "interface", "ipv6", "delete", "route",
			p, "interface="+strconv.Itoa(idx),
		); err != nil {
			last = fmt.Errorf("DeleteDefaultSplitRoutes(v6 %s) error: %v, output: %s", p, err, out)
		}
	}
	return last
}

func (w *v6Wrapper) ifIndexOf(name string) (int, error) {
	iface, err := net.InterfaceByName(name)
	if err != nil {
		return 0, fmt.Errorf("InterfaceByName(%q): %w", name, err)
	}
	if iface.Index <= 0 {
		return 0, fmt.Errorf("interface %q has invalid index: %d", name, iface.Index)
	}
	return iface.Index, nil
}

func (w *v6Wrapper) q(s string) string { return `"` + s + `"` }
