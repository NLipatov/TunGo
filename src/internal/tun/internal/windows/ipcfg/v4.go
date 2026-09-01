//go:build windows

package ipcfg

import (
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"
	"tungo/internal/tun/internal/splitroute"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

type V4 struct {
	resolver *interfaceResolver
}

func NewV4() *V4 {
	return &V4{
		resolver: newInterfaceResolver(),
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

func (v *V4) SetDNS(ifName string, dnsServers []string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	if len(dnsServers) == 0 {
		// "DHCP-like": clear DNS list for IPv4
		return luid.SetDNS(winipcfg.AddressFamily(windows.AF_INET), nil, nil)
	}
	addresses := make([]netip.Addr, 0, len(dnsServers))
	for _, s := range dnsServers {
		a, aErr := netip.ParseAddr(strings.TrimSpace(s))
		if aErr != nil || !a.Is4() {
			return fmt.Errorf("SetDNS: bad IPv4 DNS %q", s)
		}
		addresses = append(addresses, a)
	}
	return luid.SetDNS(winipcfg.AddressFamily(windows.AF_INET), addresses, nil)
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
		iFace.Metric = ipcfgMetric
	}
	return iFace.Set()
}

func (v *V4) AddHostRouteViaGateway(hostIP netip.Addr, ifName string, gateway netip.Addr) error {
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
	return luid.AddRoute(netip.PrefixFrom(hostIP, 32), gateway, ipcfgMetric)
}

func (v *V4) AddHostRouteOnLink(hostIP netip.Addr, ifName string) error {
	if !hostIP.Is4() {
		return fmt.Errorf("AddHostRouteOnLink: not an IPv4: %q", hostIP)
	}
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	return luid.AddRoute(netip.PrefixFrom(hostIP, 32), netip.IPv4Unspecified(), ipcfgMetric)
}

func (v *V4) AddDefaultSplitRoutes(ifName string) error {
	luid, err := v.resolver.NetworkInterfaceByName(ifName)
	if err != nil {
		return err
	}
	for _, s := range []string{splitroute.IPv4LowerHalf, splitroute.IPv4UpperHalf} {
		pfx, _ := netip.ParsePrefix(s) // valid by const
		if roteErr := luid.AddRoute(
			pfx,
			netip.IPv4Unspecified(),
			ipcfgMetric,
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
	for _, s := range []string{splitroute.IPv4LowerHalf, splitroute.IPv4UpperHalf} {
		pfx, _ := netip.ParsePrefix(s)
		if err := luid.DeleteRoute(pfx, netip.IPv4Unspecified()); err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) {
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
		if delErr := r.Delete(); delErr != nil && !errors.Is(delErr, windows.ERROR_NOT_FOUND) {
			errs = append(errs, delErr)
			continue
		}
	}
	return errors.Join(errs...)
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
