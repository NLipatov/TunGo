//go:build windows

package route

import (
	"bytes"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
	"math"
	"net"
	"net/netip"
	"strings"
)

// v6Wrapper implements route.Contract for IPv6 using native Win32 IP Helper API (winipcfg).
type v6Wrapper struct{}

func newV6Wrapper() Contract { return &v6Wrapper{} }

// Delete removes all IPv6 routes that exactly match dst (host "::1" → /128, or CIDR).
func (w *v6Wrapper) Delete(dst string) error {
	pfx, err := parseDestPrefixV6(dst)
	if err != nil {
		return fmt.Errorf("route delete(v6): %w", err)
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
			if err := r.Delete(); err != nil {
				last = err
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
func (w *v6Wrapper) Print(t string) ([]byte, error) {
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
		alias := displayNameFromLUID(r.InterfaceLUID, r.InterfaceIndex)
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
func (w *v6Wrapper) BestRoute(dest string) (string, string, int, int, error) {
	raw := strings.TrimSpace(dest)
	ipStr := dropZone(raw) // strip zone, e.g. fe80::1%12 → fe80::1
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

	alias := displayNameFromLUID(best.InterfaceLUID, best.InterfaceIndex)
	return gw, alias, int(best.InterfaceIndex), int(best.Metric), nil
}

// dropZone removes "%zone" from link-local IPv6 addresses, e.g. "fe80::1%12".
func dropZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}

// parseDestPrefixV6 parses IPv6 destination (CIDR or host -> /128).
func parseDestPrefixV6(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(dropZone(s))
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
