//go:build windows

package ipcfg

import (
	"bytes"
	"fmt"
	"math"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"tungo/infrastructure/PAL/windows/ipcfg/network_interface/resolver"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const (
	// v6SplitOne covers addresses between :: (0000:0000:0000:0000:0000:0000:0000:0000) and 7fff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	v6SplitOne = "::/1"
	// v6SplitTwo covers addresses between 8000:: (8000:0000:0000:0000:0000:0000:0000:0000) and ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff
	v6SplitTwo = "8000::/1"
)

type v6 struct {
	resolver resolver.Contract
}

func newV6(resolver resolver.Contract) Contract {
	return &v6{
		resolver: resolver,
	}
}

func (v *v6) FlushDNS() error {
	dnsApi := windows.NewLazySystemDLL("dnsapi.dll")
	proc := dnsApi.NewProc("DnsFlushResolverCache")
	if err := dnsApi.Load(); err != nil {
		return fmt.Errorf("failed to load dnsapi.dll: %w", err)
	}
	r, _, callErr := proc.Call()
	if r == 0 {
		return fmt.Errorf("DnsFlushResolverCache failed: %v", callErr)
	}
	return nil
}

func (v *v6) SetAddressStatic(ifName, ip, mask string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := v.ipv6PrefixFromIPMask(ip, mask)
	if err != nil {
		return fmt.Errorf("SetAddressStatic(v6): %w", err)
	}
	return luid.SetIPAddressesForFamily(winipcfg.AddressFamily(windows.AF_INET6), []netip.Prefix{pfx})
}

func (v *v6) SetAddressWithGateway(ifName, ip, mask, gateway string, metric int) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := v.ipv6PrefixFromIPMask(ip, mask)
	if err != nil {
		return fmt.Errorf("SetAddressWithGateway(v6): %w", err)
	}
	if err = luid.SetIPAddressesForFamily(winipcfg.AddressFamily(windows.AF_INET6), []netip.Prefix{pfx}); err != nil {
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
	return luid.AddRoute(netip.PrefixFrom(netip.IPv6Unspecified(), 0), gw, uint32(min(1, metric)))
}

func (v *v6) DeleteAddress(ifName, interfaceAddress string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
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

func (v *v6) SetDNS(ifName string, dnsServers []string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
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
		if err := luid.SetDNS(winipcfg.AddressFamily(windows.AF_INET6), nil, nil); err != nil {
			return err
		}
		_ = luid.FlushDNS(winipcfg.AddressFamily(windows.AF_INET6))
		return nil
	}
	if err = luid.SetDNS(winipcfg.AddressFamily(windows.AF_INET6), addrs, nil); err != nil {
		return err
	}
	_ = luid.FlushDNS(winipcfg.AddressFamily(windows.AF_INET6))
	return nil
}

func (v *v6) SetMTU(ifName string, mtu int) error {
	if mtu <= 0 {
		return fmt.Errorf("SetMTU(v6): invalid mtu %d", mtu)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	row, err := luid.IPInterface(winipcfg.AddressFamily(windows.AF_INET6))
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

func (v *v6) AddRoutePrefix(prefix, ifName string, metric int) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := v.parseIPv6Prefix(prefix)
	if err != nil {
		return err
	}
	// on-link: nexthop = ::
	return luid.AddRoute(pfx, netip.IPv6Unspecified(), uint32(min(1, metric)))
}

func (v *v6) DeleteRoutePrefix(prefix, ifName string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	pfx, err := v.parseIPv6Prefix(prefix)
	if err != nil {
		return err
	}
	return luid.DeleteRoute(pfx, netip.IPv6Unspecified())
}

func (v *v6) DeleteDefaultRoute(ifName string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	tab, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET6))
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

func (v *v6) AddHostRouteViaGateway(hostIP, ifName, gateway string, metric int) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
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
	return luid.AddRoute(netip.PrefixFrom(ip, 128), gw, uint32(min(1, metric)))
}

func (v *v6) AddHostRouteOnLink(hostIP, ifName string, metric int) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	ip, ipErr := netip.ParseAddr(strings.TrimSpace(hostIP))
	if ipErr != nil || !ip.Is6() {
		return fmt.Errorf("AddHostRouteOnLink(v6): not IPv6: %q", hostIP)
	}
	return luid.AddRoute(netip.PrefixFrom(ip, 128), netip.IPv6Unspecified(), uint32(min(1, metric)))
}

func (v *v6) AddDefaultSplitRoutes(ifName string, metric int) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	for _, s := range []string{v6SplitOne, v6SplitTwo} {
		pfx, _ := netip.ParsePrefix(s)
		if err = luid.AddRoute(pfx, netip.IPv6Unspecified(), uint32(min(1, metric))); err != nil {
			return fmt.Errorf("AddDefaultSplitRoutes(v6 %s): %w", s, err)
		}
	}
	return nil
}

func (v *v6) DeleteDefaultSplitRoutes(ifName string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	var last error
	for _, s := range []string{v6SplitOne, v6SplitTwo} {
		pfx, _ := netip.ParsePrefix(s)
		if err := luid.DeleteRoute(pfx, netip.IPv6Unspecified()); err != nil {
			last = fmt.Errorf("DeleteDefaultSplitRoutes(v6 %s): %w", s, err)
		}
	}
	return last
}

// DeleteRoute removes all IPv6 routes that exactly match dst (host "::1" → /128, or CIDR).
func (v *v6) DeleteRoute(dst string) error {
	pfx, err := v.parseDestPrefixV6(dst)
	if err != nil {
		return fmt.Errorf("DeleteRoute(v6): %w", err)
	}
	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET6))
	if err != nil {
		return fmt.Errorf("GetIPForwardTable2(v6): %w", err)
	}
	var (
		found int
		last  error
	)
	for i := range rows {
		r := &rows[i]
		dp := r.DestinationPrefix.Prefix()
		if !dp.Addr().Is6() {
			continue
		}
		if dp == pfx {
			if routeErr := r.Delete(); routeErr != nil {
				last = routeErr
				continue
			}
			found++
		}
	}
	// idempotent cleanup: silently ignore if not found
	if found == 0 {
		return nil
	}
	return last
}

// Print returns a human-readable dump of the IPv6 route table.
// If t is non-empty, only lines containing t are included (substring match).
func (v *v6) Print(t string) ([]byte, error) {
	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET6))
	if err != nil {
		return nil, fmt.Errorf("GetIPForwardTable2(v6): %w", err)
	}
	var b bytes.Buffer
	b.WriteString("DestPrefix\tNextHop\tIfAlias\tIfIndex\tMetric\n")
	for i := range rows {
		r := &rows[i]
		dp := r.DestinationPrefix.Prefix()
		if !dp.Addr().Is6() {
			continue
		}
		nh := r.NextHop.Addr()
		nextHop := "on-link"
		if nh.IsValid() && nh.Is6() && !nh.IsUnspecified() {
			nextHop = nh.String()
		}
		alias := v.resolver.NetworkInterfaceName(r.InterfaceLUID)
		line := fmt.Sprintf("%s\t%s\t%s\t%d\t%d\n",
			dp.String(), nextHop, alias, r.InterfaceIndex, r.Metric)
		if t == "" || strings.Contains(line, t) {
			b.WriteString(line)
		}
	}
	return b.Bytes(), nil
}

// BestRoute returns (gateway, interfaceAlias, interfaceIndex, routeMetric) for IPv6.
// Picks the route with longest prefix match, then lowest effective metric (route+interface).
func (v *v6) BestRoute(dest string) (string, string, int, int, error) {
	raw := strings.TrimSpace(dest)
	ipStr := v.dropZone(raw) // strip zone, e.g. fe80::1%12 → fe80::1
	ip := net.ParseIP(ipStr)
	if ip == nil || ip.To4() != nil || ip.To16() == nil {
		return "", "", 0, 0, fmt.Errorf("BestRoute(v6): not an IPv6 address: %q", dest)
	}

	var b16 [16]byte
	copy(b16[:], ip.To16())
	dst := netip.AddrFrom16(b16)

	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET6))
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("GetIPForwardTable2(v6): %w", err)
	}

	var (
		best       *winipcfg.MibIPforwardRow2
		bestPL     = -1
		bestMetric = uint32(math.MaxUint32)
	)
	for i := range rows {
		pfx := rows[i].DestinationPrefix.Prefix()
		if !pfx.Addr().Is6() || !pfx.Contains(dst) {
			continue
		}
		pl := pfx.Bits()
		m := rows[i].Metric
		if ifRow, _ := rows[i].InterfaceLUID.IPInterface(winipcfg.AddressFamily(windows.AF_INET6)); ifRow != nil {
			m += ifRow.Metric // effective metric
		}
		if pl > bestPL || (pl == bestPL && m < bestMetric) {
			best, bestPL, bestMetric = &rows[i], pl, m
		}
	}
	if best == nil {
		return "", "", 0, 0, fmt.Errorf("BestRoute(v6): no matching route for %s", dest)
	}

	var gw string
	if nh := best.NextHop.Addr(); nh.IsValid() && nh.Is6() && !nh.IsUnspecified() {
		gw = nh.String()
	}

	alias := v.resolver.NetworkInterfaceName(best.InterfaceLUID)
	return gw, alias, int(best.InterfaceIndex), int(best.Metric), nil
}

func (v *v6) ipv6PrefixFromIPMask(ipStr, maskStr string) (netip.Prefix, error) {
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

func (v *v6) parseIPv6Prefix(s string) (netip.Prefix, error) {
	pfx, pfxErr := netip.ParsePrefix(strings.TrimSpace(s))
	if pfxErr != nil || !pfx.Addr().Is6() {
		return netip.Prefix{}, fmt.Errorf("bad IPv6 prefix: %q", s)
	}
	return pfx, nil
}

// dropZone removes "%zone" from link-local IPv6 addresses, e.g. "fe80::1%12".
func (v *v6) dropZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}

// parseDestPrefixV6 parses IPv6 destination (CIDR or host -> /128).
func (v *v6) parseDestPrefixV6(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(v.dropZone(s))
	if s == "" {
		return netip.Prefix{}, fmt.Errorf("empty destination")
	}
	if strings.Contains(s, "/") {
		pfx, pfxErr := netip.ParsePrefix(s)
		if pfxErr != nil || !pfx.Addr().Is6() {
			return netip.Prefix{}, fmt.Errorf("bad IPv6 prefix: %q", s)
		}
		return pfx, nil
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() != nil || ip.To16() == nil {
		return netip.Prefix{}, fmt.Errorf("not an IPv6 address: %q", s)
	}
	var a16 [16]byte
	copy(a16[:], ip.To16())
	return netip.PrefixFrom(netip.AddrFrom16(a16), 128), nil
}
