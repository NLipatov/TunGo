//go:build windows

package ipcfg

import (
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

type V6 struct {
	resolver *resolver.Resolver
}

func NewV6() *V6 {
	return &V6{
		resolver: resolver.NewResolver(),
	}
}

func (v *V6) FlushDNS() error {
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

func (v *V6) SetAddressStatic(ifName string, prefix netip.Prefix) error {
	addr := prefix.Addr()
	if !prefix.IsValid() || !addr.Is6() || addr.Is4In6() {
		return fmt.Errorf("SetAddressStatic(v6): invalid IPv6 prefix %q", prefix)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	return luid.SetIPAddressesForFamily(winipcfg.AddressFamily(windows.AF_INET6), []netip.Prefix{prefix})
}

func (v *V6) SetDNS(ifName string, dnsServers []string) error {
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

func (v *V6) SetMTU(ifName string, mtu int) error {
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
		row.Metric = ipcfgMetric
	}
	return row.Set()
}

func (v *V6) AddHostRouteViaGateway(hostIP netip.Addr, ifName string, gateway netip.Addr) error {
	if !hostIP.Is6() || hostIP.Is4In6() {
		return fmt.Errorf("AddHostRouteViaGateway(v6): not IPv6: %q", hostIP)
	}
	if !gateway.Is6() || gateway.Is4In6() {
		return fmt.Errorf("AddHostRouteViaGateway(v6): gateway not IPv6: %q", gateway)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	return luid.AddRoute(netip.PrefixFrom(hostIP, 128), gateway, ipcfgMetric)
}

func (v *V6) AddHostRouteOnLink(hostIP netip.Addr, ifName string) error {
	if !hostIP.Is6() || hostIP.Is4In6() {
		return fmt.Errorf("AddHostRouteOnLink(v6): not IPv6: %q", hostIP)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	return luid.AddRoute(netip.PrefixFrom(hostIP, 128), netip.IPv6Unspecified(), ipcfgMetric)
}

func (v *V6) AddDefaultSplitRoutes(ifName string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	for _, s := range []string{v6SplitOne, v6SplitTwo} {
		pfx, _ := netip.ParsePrefix(s)
		if err = luid.AddRoute(pfx, netip.IPv6Unspecified(), ipcfgMetric); err != nil {
			return fmt.Errorf("AddDefaultSplitRoutes(v6 %s): %w", s, err)
		}
	}
	return nil
}

func (v *V6) DeleteDefaultSplitRoutes(ifName string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	var errs []error
	for _, s := range []string{v6SplitOne, v6SplitTwo} {
		pfx, _ := netip.ParsePrefix(s)
		if err := luid.DeleteRoute(pfx, netip.IPv6Unspecified()); err != nil {
			errs = append(errs, fmt.Errorf("DeleteDefaultSplitRoutes(v6 %s): %w", s, err))
		}
	}
	return errors.Join(errs...)
}

// DeleteRoute removes all IPv6 routes that exactly match dst (host "::1" → /128, or CIDR).
func (v *V6) DeleteRoute(destination netip.Addr) error {
	if !destination.Is6() || destination.Is4In6() {
		return fmt.Errorf("DeleteRoute(v6): not an IPv6 address: %q", destination)
	}
	pfx := netip.PrefixFrom(destination, 128)
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

func (v *V6) DeleteRouteOnInterface(destination netip.Addr, ifName string) error {
	if !destination.Is6() || destination.Is4In6() {
		return fmt.Errorf("DeleteRouteOnInterface(v6): not an IPv6 address: %q", destination)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	pfx := netip.PrefixFrom(destination, 128)
	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET6))
	if err != nil {
		return fmt.Errorf("GetIPForwardTable2(v6): %w", err)
	}
	var errs []error
	for i := range rows {
		r := &rows[i]
		dp := r.DestinationPrefix.Prefix()
		if !dp.Addr().Is6() || dp != pfx || r.InterfaceLUID != luid {
			continue
		}
		if delErr := r.Delete(); delErr != nil {
			errs = append(errs, delErr)
			continue
		}
	}
	return errors.Join(errs...)
}

// BestRoute returns (gateway, interfaceAlias, interfaceIndex, routeMetric) for IPv6.
// Picks the route with longest prefix match, then lowest effective metric (route+interface).
func (v *V6) BestRoute(dest netip.Addr) (netip.Addr, string, int, int, error) {
	dest = dest.WithZone("")
	if !dest.Is6() || dest.Is4In6() {
		return netip.Addr{}, "", 0, 0, fmt.Errorf("BestRoute(v6): not an IPv6 address: %q", dest)
	}

	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET6))
	if err != nil {
		return netip.Addr{}, "", 0, 0, fmt.Errorf("GetIPForwardTable2(v6): %w", err)
	}

	var (
		best       *winipcfg.MibIPforwardRow2
		bestPL     = -1
		bestMetric = uint32(math.MaxUint32)
	)
	for i := range rows {
		pfx := rows[i].DestinationPrefix.Prefix()
		if !pfx.Addr().Is6() || !pfx.Contains(dest) {
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
		return netip.Addr{}, "", 0, 0, fmt.Errorf("BestRoute(v6): no matching route for %s", dest)
	}

	var gw netip.Addr
	if nh := best.NextHop.Addr(); nh.IsValid() && nh.Is6() && !nh.IsUnspecified() {
		gw = nh
	}

	alias := v.resolver.NetworkInterfaceName(best.InterfaceLUID)
	if strings.TrimSpace(alias) == "" && best.InterfaceIndex != 0 {
		alias = strconv.Itoa(int(best.InterfaceIndex))
	}
	return gw, alias, int(best.InterfaceIndex), int(best.Metric), nil
}
