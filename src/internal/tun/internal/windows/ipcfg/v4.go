//go:build windows

package ipcfg

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"
	"tungo/internal/tun/internal/windows/ipcfg/network_interface/resolver"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

const (
	// v4SplitOne covers half of IPv4 address space
	// (addresses between 0.0.0.0 and 127.255.255.255)
	v4SplitOne = "0.0.0.0/1"
	// v4SplitTwo v4SplitOne covers half of IPv4 address space
	// (addresses between 128.0.0.0 and 255.255.255.255)
	v4SplitTwo = "128.0.0.0/1"
)

type V4 struct {
	resolver *resolver.Resolver
}

func NewV4() *V4 {
	return &V4{
		resolver: resolver.NewResolver(),
	}
}

func (v *V4) FlushDNS() error {
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
func (v *V4) SetAddressStatic(ifName string, prefix netip.Prefix) error {
	if !prefix.IsValid() || !prefix.Addr().Is4() {
		return fmt.Errorf("SetAddressStatic: invalid IPv4 prefix %q", prefix)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	return luid.SetIPAddressesForFamily(winipcfg.AddressFamily(windows.AF_INET), []netip.Prefix{prefix})
}

func (v *V4) DeleteAddress(ifName, interfaceAddress string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
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

func (v *V4) SetDNS(ifName string, dnsServers []string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	if len(dnsServers) == 0 {
		// "DHCP-like": clear DNS list for IPv4
		setDNSErr := luid.SetDNS(winipcfg.AddressFamily(windows.AF_INET), nil, nil)
		if setDNSErr != nil {
			return setDNSErr
		}
		_ = luid.FlushDNS(winipcfg.AddressFamily(windows.AF_INET))
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
	if err := luid.SetDNS(winipcfg.AddressFamily(windows.AF_INET), addresses, nil); err != nil {
		return err
	}
	_ = luid.FlushDNS(winipcfg.AddressFamily(windows.AF_INET))
	return nil
}

func (v *V4) SetMTU(ifName string, mtu int) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	if mtu <= 0 {
		return fmt.Errorf("SetMTU: invalid mtu %d", mtu)
	}
	iFace, err := luid.IPInterface(winipcfg.AddressFamily(windows.AF_INET))
	if err != nil {
		return err
	}
	iFace.NLMTU = uint32(mtu)
	// Make metric explicit & low-ish if not set, to avoid auto-metric surprises.
	iFace.UseAutomaticMetric = false
	if iFace.Metric == 0 {
		iFace.Metric = 1
	}
	return iFace.Set()
}

func (v *V4) DeleteDefaultRoute(ifName string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	table, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET))
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

func (v *V4) AddHostRouteViaGateway(hostIP netip.Addr, ifName string, gateway netip.Addr, metric int) error {
	if !hostIP.Is4() {
		return fmt.Errorf("AddHostRouteViaGateway: not an IPv4: %q", hostIP)
	}
	if !gateway.Is4() {
		return fmt.Errorf("AddHostRouteViaGateway: gateway is not IPv4: %q", gateway)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	return luid.AddRoute(netip.PrefixFrom(hostIP, 32), gateway, uint32(min(1, metric)))
}

func (v *V4) AddHostRouteOnLink(hostIP netip.Addr, ifName string, metric int) error {
	if !hostIP.Is4() {
		return fmt.Errorf("AddHostRouteOnLink: not an IPv4: %q", hostIP)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	return luid.AddRoute(netip.PrefixFrom(hostIP, 32), netip.IPv4Unspecified(), uint32(min(1, metric)))
}

func (v *V4) AddDefaultSplitRoutes(ifName string, metric int) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	for _, s := range []string{v4SplitOne, v4SplitTwo} {
		pfx, _ := netip.ParsePrefix(s) // valid by const
		if roteErr := luid.AddRoute(
			pfx,
			netip.IPv4Unspecified(),
			uint32(min(1, metric)),
		); roteErr != nil {
			return fmt.Errorf("AddDefaultSplitRoutes(%s): %w", s, roteErr)
		}
	}
	return nil
}

func (v *V4) DeleteDefaultSplitRoutes(ifName string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	var last error
	for _, s := range []string{v4SplitOne, v4SplitTwo} {
		pfx, _ := netip.ParsePrefix(s)
		if err := luid.DeleteRoute(pfx, netip.IPv4Unspecified()); err != nil {
			last = fmt.Errorf("DeleteDefaultSplitRoutes(%s): %w", s, err)
		}
	}
	return last
}

// DeleteRoute removes all IPv4 routes that exactly match dst (host "a.b.c.d" → /32, or CIDR).
func (v *V4) DeleteRoute(destination netip.Addr) error {
	if !destination.Is4() {
		return fmt.Errorf("DeleteRoute: not an IPv4 address: %q", destination)
	}
	pfx := netip.PrefixFrom(destination, 32)
	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET))
	if err != nil {
		return fmt.Errorf("GetIPForwardTable2: %w", err)
	}
	var (
		found int
		last  error
	)
	for i := range rows {
		r := &rows[i]
		dp := r.DestinationPrefix.Prefix()
		if !dp.Addr().Is4() {
			continue
		}
		if dp == pfx {
			if delErr := r.Delete(); delErr != nil {
				last = delErr
				continue
			}
			found++
		}
	}
	if found == 0 {
		return nil
	}
	return last
}

func (v *V4) DeleteRouteOnInterface(destination netip.Addr, ifName string) error {
	if !destination.Is4() {
		return fmt.Errorf("DeleteRouteOnInterface: not an IPv4 address: %q", destination)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	pfx := netip.PrefixFrom(destination, 32)
	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET))
	if err != nil {
		return fmt.Errorf("GetIPForwardTable2: %w", err)
	}
	var errs []error
	for i := range rows {
		r := &rows[i]
		dp := r.DestinationPrefix.Prefix()
		if !dp.Addr().Is4() || dp != pfx || r.InterfaceLUID != luid {
			continue
		}
		if delErr := r.Delete(); delErr != nil {
			errs = append(errs, delErr)
			continue
		}
	}
	return errors.Join(errs...)
}

// Print returns a human-readable dump of the IPv4 route table.
// If t is non-empty, only lines containing t are included (substring match).
func (v *V4) Print(t string) ([]byte, error) {
	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET))
	if err != nil {
		return nil, fmt.Errorf("GetIPForwardTable2: %w", err)
	}
	var b bytes.Buffer
	b.WriteString("DestPrefix\tNextHop\tIfAlias\tIfIndex\tMetric\n")
	for i := range rows {
		r := &rows[i]
		dp := r.DestinationPrefix.Prefix()
		if !dp.Addr().Is4() {
			continue
		}
		nh := r.NextHop.Addr()
		nextHop := "on-link"
		if nh.IsValid() && nh.Is4() && !nh.IsUnspecified() {
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

// BestRoute returns (gateway, interfaceAlias, interfaceIndex, routeMetric) for IPv4.
// Uses GetIPForwardTable2(AF_INET) and picks the best entry by:
// 1) longest prefix match, 2) lowest metric. No external processes.
func (v *V4) BestRoute(dest netip.Addr) (netip.Addr, string, int, int, error) {
	if !dest.Is4() {
		return netip.Addr{}, "", 0, 0, fmt.Errorf("BestRoute(v4): not an IPv4 address: %q", dest)
	}

	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET))
	if err != nil {
		return netip.Addr{}, "", 0, 0, fmt.Errorf("GetIPForwardTable2(v4): %w", err)
	}

	var (
		best       *winipcfg.MibIPforwardRow2
		bestPL     = -1
		bestMetric = uint32(math.MaxUint32)
	)
	for i := range rows {
		pfx := rows[i].DestinationPrefix.Prefix() // netip.Prefix
		if !pfx.Addr().Is4() || !pfx.Contains(dest) {
			continue
		}
		pl := pfx.Bits()
		m := rows[i].Metric
		if ifRow, _ := rows[i].InterfaceLUID.IPInterface(winipcfg.AddressFamily(windows.AF_INET)); ifRow != nil {
			m += ifRow.Metric
		}
		if pl > bestPL || (pl == bestPL && m < bestMetric) {
			best, bestPL, bestMetric = &rows[i], pl, m
		}
	}
	if best == nil {
		return netip.Addr{}, "", 0, 0, fmt.Errorf("BestRoute(v4): no matching route for %s", dest)
	}

	// Gateway: empty/unspecified => on-link.
	var gw netip.Addr
	if nh := best.NextHop.Addr(); nh.IsValid() && nh.Is4() && !nh.IsUnspecified() {
		gw = nh
	}

	alias := v.resolver.NetworkInterfaceName(best.InterfaceLUID)
	if strings.TrimSpace(alias) == "" && best.InterfaceIndex != 0 {
		alias = strconv.Itoa(int(best.InterfaceIndex))
	}
	return gw, alias, int(best.InterfaceIndex), int(best.Metric), nil
}
