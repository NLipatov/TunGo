//go:build windows

package netsh

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
	wgwin "golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

type v6Wrapper struct{}

func newV6Wrapper() Contract { return &v6Wrapper{} }

// -------------------- Contract (IPv6) --------------------

func (w *v6Wrapper) SetAddressStatic(ifName, ip, mask string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := ipv6PrefixFromIPMask(ip, mask)
	if err != nil {
		return fmt.Errorf("SetAddressStatic(v6): %w", err)
	}
	return luid.SetIPAddressesForFamily(wgwin.AddressFamily(windows.AF_INET6), []netip.Prefix{pfx})
}

func (w *v6Wrapper) SetAddressWithGateway(ifName, ip, mask, gateway string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := ipv6PrefixFromIPMask(ip, mask)
	if err != nil {
		return fmt.Errorf("SetAddressWithGateway(v6): %w", err)
	}
	if err := luid.SetIPAddressesForFamily(wgwin.AddressFamily(windows.AF_INET6), []netip.Prefix{pfx}); err != nil {
		return fmt.Errorf("SetAddressWithGateway(v6): set ip: %w", err)
	}
	gw, gwErr := netip.ParseAddr(strings.TrimSpace(gateway))
	if gwErr != nil || !gw.Is6() {
		return fmt.Errorf("SetAddressWithGateway(v6): gateway is not IPv6: %q", gateway)
	}
	gw, _ = netip.ParseAddr(strings.TrimSpace(gateway))
	if !pfx.Contains(gw) && !gw.IsLinkLocalUnicast() {
		return fmt.Errorf("gateway %s is not in interface subnet %s", gw, pfx)
	}
	return luid.AddRoute(netip.PrefixFrom(netip.IPv6Unspecified(), 0), gw, atLeast1(metric))
}

func (w *v6Wrapper) DeleteAddress(ifName, interfaceAddress string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	ip, ipErr := netip.ParseAddr(strings.TrimSpace(interfaceAddress))
	if ipErr != nil || !ip.Is6() {
		return fmt.Errorf("DeleteAddress(v6): not IPv6: %q", interfaceAddress)
	}
	row, err := luid.IPAddress(ip)
	if err != nil {
		return fmt.Errorf("DeleteAddress(v6): lookup failed: %w", err)
	}
	return row.Delete()
}

func (w *v6Wrapper) SetDNS(ifName string, dnsServers []string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	var addrs []netip.Addr
	for _, s := range dnsServers {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		a, aErr := netip.ParseAddr(s)
		if aErr != nil || !a.Is6() {
			return fmt.Errorf("SetDNS(v6): bad IPv6 DNS %q", s)
		}
		addrs = append(addrs, a)
	}
	if len(addrs) == 0 {
		if err := luid.SetDNS(wgwin.AddressFamily(windows.AF_INET6), nil, nil); err != nil {
			return err
		}
		_ = luid.FlushDNS(wgwin.AddressFamily(windows.AF_INET6))
		return nil
	}
	if err := luid.SetDNS(wgwin.AddressFamily(windows.AF_INET6), addrs, nil); err != nil {
		return err
	}
	_ = luid.FlushDNS(wgwin.AddressFamily(windows.AF_INET6))
	return nil
}

func (w *v6Wrapper) SetMTU(ifName string, mtu int) error {
	if mtu <= 0 {
		return fmt.Errorf("SetMTU(v6): invalid mtu %d", mtu)
	}
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	row, err := luid.IPInterface(wgwin.AddressFamily(windows.AF_INET6))
	if err != nil {
		return err
	}
	row.NLMTU = uint32(mtu)
	row.UseAutomaticMetric = false
	if row.Metric == 0 {
		row.Metric = 1
	}
	return row.Set()
}

func (w *v6Wrapper) AddRoutePrefix(prefix, ifName string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := parseIPv6Prefix(prefix)
	if err != nil {
		return err
	}
	// on-link: nexthop = ::
	return luid.AddRoute(pfx, netip.IPv6Unspecified(), atLeast1(metric))
}

func (w *v6Wrapper) DeleteRoutePrefix(prefix, ifName string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := parseIPv6Prefix(prefix)
	if err != nil {
		return err
	}
	return luid.DeleteRoute(pfx, netip.IPv6Unspecified())
}

func (w *v6Wrapper) DeleteDefaultRoute(ifName string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	tab, err := wgwin.GetIPForwardTable2(wgwin.AddressFamily(windows.AF_INET6))
	if err != nil {
		return err
	}
	var last error
	for i := range tab {
		r := &tab[i]
		if r.InterfaceLUID == luid && r.DestinationPrefix.PrefixLength == 0 {
			if err := r.Delete(); err != nil {
				last = err
			}
		}
	}
	return last
}

func (w *v6Wrapper) AddHostRouteViaGateway(hostIP, ifName, gateway string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	ip, ipErr := netip.ParseAddr(strings.TrimSpace(hostIP))
	if ipErr != nil || !ip.Is6() {
		return fmt.Errorf("AddHostRouteViaGateway(v6): not IPv6: %q", hostIP)
	}
	gw, gwErr := netip.ParseAddr(strings.TrimSpace(gateway))
	if gwErr != nil || !gw.Is6() {
		return fmt.Errorf("AddHostRouteViaGateway(v6): gateway not IPv6: %q", gateway)
	}
	return luid.AddRoute(netip.PrefixFrom(ip, 128), gw, atLeast1(metric))
}

func (w *v6Wrapper) AddHostRouteOnLink(hostIP, ifName string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	ip, ipErr := netip.ParseAddr(strings.TrimSpace(hostIP))
	if ipErr != nil || !ip.Is6() {
		return fmt.Errorf("AddHostRouteOnLink(v6): not IPv6: %q", hostIP)
	}
	return luid.AddRoute(netip.PrefixFrom(ip, 128), netip.IPv6Unspecified(), atLeast1(metric))
}

func (w *v6Wrapper) AddDefaultSplitRoutes(ifName string, metric int) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	for _, s := range []string{"::/1", "8000::/1"} {
		pfx, _ := netip.ParsePrefix(s)
		if err := luid.AddRoute(pfx, netip.IPv6Unspecified(), atLeast1(metric)); err != nil {
			return fmt.Errorf("AddDefaultSplitRoutes(v6 %s): %w", s, err)
		}
	}
	return nil
}

func (w *v6Wrapper) DeleteDefaultSplitRoutes(ifName string) error {
	luid, err := luidByName(ifName)
	if err != nil {
		return err
	}
	var last error
	for _, s := range []string{"::/1", "8000::/1"} {
		pfx, _ := netip.ParsePrefix(s)
		if err := luid.DeleteRoute(pfx, netip.IPv6Unspecified()); err != nil {
			last = fmt.Errorf("DeleteDefaultSplitRoutes(v6 %s): %w", s, err)
		}
	}
	return last
}

// -------------------- Helpers (IPv6) --------------------

func ipv6PrefixFromIPMask(ipStr, maskStr string) (netip.Prefix, error) {
	addr, addrErr := netip.ParseAddr(strings.TrimSpace(ipStr))
	if addrErr != nil || !addr.Is6() {
		return netip.Prefix{}, fmt.Errorf("ip is not IPv6: %q", ipStr)
	}
	maskStr = strings.TrimSpace(maskStr)
	if maskStr == "" {
		return netip.Prefix{}, fmt.Errorf("empty IPv6 mask")
	}
	// Case 1: numeric prefix length ("64")
	if n, err := strconv.Atoi(maskStr); err == nil {
		if n < 0 || n > 128 {
			return netip.Prefix{}, fmt.Errorf("bad IPv6 prefix len: %d", n)
		}
		return netip.PrefixFrom(addr, n), nil
	}
	// Case 2: IPv6 mask ("ffff:ffff:...") — rare but supported
	im := net.ParseIP(maskStr)
	if im == nil || im.To16() == nil {
		return netip.Prefix{}, fmt.Errorf("mask is not IPv6: %q", maskStr)
	}
	ones, bits := net.IPMask(im.To16()).Size()
	if bits != 128 || ones < 0 || ones > 128 {
		return netip.Prefix{}, fmt.Errorf("bad IPv6 mask: %q", maskStr)
	}
	return netip.PrefixFrom(addr, ones), nil
}

func parseIPv6Prefix(s string) (netip.Prefix, error) {
	pfx, pfxErr := netip.ParsePrefix(strings.TrimSpace(s))
	if pfxErr != nil || !pfx.Addr().Is6() {
		return netip.Prefix{}, fmt.Errorf("bad IPv6 prefix: %q", s)
	}
	return pfx, nil
}
