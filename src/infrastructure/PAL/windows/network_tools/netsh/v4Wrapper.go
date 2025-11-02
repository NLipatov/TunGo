//go:build windows

package netsh

import (
	"fmt"
	"net"
	"net/netip"
	"strings"

	"golang.org/x/sys/windows"
	wgwin "golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

// v4Wrapper is an IPv4 implementation of netsh.Contract, now backed by winipcfg.
type v4Wrapper struct {
}

func newV4Wrapper() Contract { return &v4Wrapper{} }

// -------------------- Public API (Contract) --------------------

func (w *v4Wrapper) SetAddressStatic(ifName, ip, mask string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := dottedMaskToPrefix(ip, mask)
	if err != nil {
		return fmt.Errorf("SetAddressStatic: %w", err)
	}
	return luid.SetIPAddressesForFamily(wgwin.AddressFamily(windows.AF_INET), []netip.Prefix{pfx})
}

func (w *v4Wrapper) SetAddressWithGateway(ifName, ip, mask, gw string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := dottedMaskToPrefix(ip, mask)
	if err != nil {
		return fmt.Errorf("SetAddressWithGateway: %w", err)
	}
	if err := luid.SetIPAddressesForFamily(wgwin.AddressFamily(windows.AF_INET), []netip.Prefix{pfx}); err != nil {
		return fmt.Errorf("SetAddressWithGateway: set ip: %w", err)
	}
	gwAddr, gwAddrErr := netip.ParseAddr(gw)
	if gwAddrErr != nil || !gwAddr.Is4() {
		return fmt.Errorf("SetAddressWithGateway: gateway is not IPv4: %q", gw)
	}
	return luid.AddRoute(netip.PrefixFrom(netip.IPv4Unspecified(), 0), gwAddr, atLeast1(metric))
}

func (w *v4Wrapper) DeleteAddress(ifName, interfaceAddress string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	ip, ipErr := netip.ParseAddr(strings.TrimSpace(interfaceAddress))
	if ipErr != nil || !ip.Is4() {
		return fmt.Errorf("DeleteAddress: not an IPv4: %q", interfaceAddress)
	}
	row, err := luid.IPAddress(ip)
	if err != nil {
		return fmt.Errorf("DeleteAddress: lookup failed: %w", err)
	}
	return row.Delete()
}

func (w *v4Wrapper) SetDNS(ifName string, dnsServers []string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	if len(dnsServers) == 0 {
		// "DHCP-like": clear DNS list for IPv4
		setDNSErr := luid.SetDNS(wgwin.AddressFamily(windows.AF_INET), nil, nil)
		if setDNSErr != nil {
			return setDNSErr
		}
		_ = luid.FlushDNS(wgwin.AddressFamily(windows.AF_INET))
		return nil
	}
	addresses := make([]netip.Addr, 0, len(dnsServers))
	for _, s := range dnsServers {
		a, aErr := netip.ParseAddr(strings.TrimSpace(s))
		if aErr != nil || !a.Is4() {
			return fmt.Errorf("SetDNS: bad IPv4 DNS %q", s)
		}
		addresses = append(addresses, a)
	}
	if err := luid.SetDNS(wgwin.AddressFamily(windows.AF_INET), addresses, nil); err != nil {
		return err
	}
	_ = luid.FlushDNS(wgwin.AddressFamily(windows.AF_INET))
	return nil
}

func (w *v4Wrapper) SetMTU(ifName string, mtu int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	if mtu <= 0 {
		return fmt.Errorf("SetMTU: invalid mtu %d", mtu)
	}
	iface, err := luid.IPInterface(wgwin.AddressFamily(windows.AF_INET))
	if err != nil {
		return err
	}
	iface.NLMTU = uint32(mtu)
	// Make metric explicit & low-ish if not set, to avoid auto-metric surprises.
	iface.UseAutomaticMetric = false
	if iface.Metric == 0 {
		iface.Metric = 1
	}
	return iface.Set()
}

func (w *v4Wrapper) AddRoutePrefix(destinationPrefix, ifName string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := parseIPv4Prefix(destinationPrefix)
	if err != nil {
		return err
	}
	// on-link route: nexthop = 0.0.0.0
	return luid.AddRoute(pfx, netip.IPv4Unspecified(), atLeast1(metric))
}

func (w *v4Wrapper) DeleteRoutePrefix(destinationPrefix, ifName string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := parseIPv4Prefix(destinationPrefix)
	if err != nil {
		return err
	}
	return luid.DeleteRoute(pfx, netip.IPv4Unspecified())
}

func (w *v4Wrapper) DeleteDefaultRoute(ifName string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	table, err := wgwin.GetIPForwardTable2(wgwin.AddressFamily(windows.AF_INET))
	if err != nil {
		return err
	}
	var last error
	for i := range table {
		r := &table[i]
		if r.InterfaceLUID == luid && r.DestinationPrefix.PrefixLength == 0 {
			if err := r.Delete(); err != nil {
				last = err
			}
		}
	}
	return last
}

func (w *v4Wrapper) AddHostRouteViaGateway(hostIP, ifName, gateway string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	host := strings.TrimSpace(hostIP)
	ip, ipErr := netip.ParseAddr(host)
	if ipErr != nil || !ip.Is4() {
		return fmt.Errorf("AddHostRouteViaGateway: not an IPv4: %q", hostIP)
	}
	gw, ipErr := netip.ParseAddr(gateway)
	if ipErr != nil || !gw.Is4() {
		return fmt.Errorf("AddHostRouteViaGateway: gateway is not IPv4: %q", gateway)
	}
	return luid.AddRoute(netip.PrefixFrom(ip, 32), gw, atLeast1(metric))
}

func (w *v4Wrapper) AddHostRouteOnLink(hostIP, ifName string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	host := strings.TrimSpace(hostIP)
	ip, ipErr := netip.ParseAddr(host)
	if ipErr != nil || !ip.Is4() {
		return fmt.Errorf("AddHostRouteOnLink: not an IPv4: %q", hostIP)
	}
	return luid.AddRoute(netip.PrefixFrom(ip, 32), netip.IPv4Unspecified(), atLeast1(metric))
}

func (w *v4Wrapper) AddDefaultSplitRoutes(ifName string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	const (
		p1 = "0.0.0.0/1"
		p2 = "128.0.0.0/1"
	)
	for _, s := range []string{p1, p2} {
		pfx, _ := netip.ParsePrefix(s) // valid by const
		if err := luid.AddRoute(pfx, netip.IPv4Unspecified(), atLeast1(metric)); err != nil {
			return fmt.Errorf("AddDefaultSplitRoutes(%s): %w", s, err)
		}
	}
	return nil
}

func (w *v4Wrapper) DeleteDefaultSplitRoutes(ifName string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	var last error
	for _, s := range []string{"0.0.0.0/1", "128.0.0.0/1"} {
		pfx, _ := netip.ParsePrefix(s)
		if err := luid.DeleteRoute(pfx, netip.IPv4Unspecified()); err != nil {
			last = fmt.Errorf("DeleteDefaultSplitRoutes(%s): %w", s, err)
		}
	}
	return last
}

// -------------------- Helpers --------------------

// luidByName resolves interface LUID by FriendlyName (as shown in Windows UI).
func luidByName(ifName string) (wgwin.LUID, error) {
	addrs, err := wgwin.GetAdaptersAddresses(wgwin.AddressFamily(windows.AF_UNSPEC), 0)
	if err != nil {
		return 0, err
	}
	for _, a := range addrs {
		if a.FriendlyName() == ifName {
			return a.LUID, nil
		}
	}
	return 0, fmt.Errorf("interface %q not found", ifName)
}

func dottedMaskToPrefix(ipStr, maskStr string) (netip.Prefix, error) {
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() == nil {
		return netip.Prefix{}, fmt.Errorf("ip is not IPv4: %q", ipStr)
	}
	m := net.ParseIP(maskStr)
	if m == nil || m.To4() == nil {
		return netip.Prefix{}, fmt.Errorf("mask is not dotted IPv4: %q", maskStr)
	}
	ones, bits := net.IPMask(m.To4()).Size()
	if bits != 32 || ones < 0 || ones > 32 {
		return netip.Prefix{}, fmt.Errorf("bad mask: %q", maskStr)
	}
	addr, addrErr := netip.ParseAddr(ip.To4().String())
	if addrErr != nil {
		return netip.Prefix{}, fmt.Errorf("parse addr failed: %q", ipStr)
	}
	return netip.PrefixFrom(addr, ones), nil
}

func parseIPv4Prefix(cidr string) (netip.Prefix, error) {
	pfx, pfxErr := netip.ParsePrefix(strings.TrimSpace(cidr))
	if pfxErr != nil || !pfx.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("bad IPv4 prefix: %q", cidr)
	}
	return pfx, nil
}

func atLeast1(v int) uint32 {
	if v < 1 {
		return 1
	}
	return uint32(v)
}
