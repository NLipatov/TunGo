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

type v4Wrapper struct {
}

func newV4Wrapper() Contract { return &v4Wrapper{} }

// Delete removes all IPv4 routes that exactly match dst (host "a.b.c.d" → /32, or CIDR).
func (w *v4Wrapper) Delete(dst string) error {
	pfx, err := parseDestPrefixV4(dst)
	if err != nil {
		return fmt.Errorf("route delete: %w", err)
	}
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
			if err := r.Delete(); err != nil {
				last = err
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

// Print returns a human-readable dump of the IPv4 route table.
// If t is non-empty, only lines containing t are included (substring match).
func (w *v4Wrapper) Print(t string) ([]byte, error) {
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
		alias := displayNameFromLUID(r.InterfaceLUID, r.InterfaceIndex)
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
func (w *v4Wrapper) BestRoute(dest string) (string, string, int, int, error) {
	ip := net.ParseIP(strings.TrimSpace(dest)).To4()
	if ip == nil {
		return "", "", 0, 0, fmt.Errorf("BestRoute(v4): not an IPv4 address: %q", dest)
	}

	var b4 [4]byte
	copy(b4[:], ip)
	dst := netip.AddrFrom4(b4)

	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET))
	if err != nil {
		return "", "", 0, 0, fmt.Errorf("GetIPForwardTable2(v4): %w", err)
	}

	var (
		best       *winipcfg.MibIPforwardRow2
		bestPL     = -1
		bestMetric = uint32(math.MaxUint32)
	)
	for i := range rows {
		pfx := rows[i].DestinationPrefix.Prefix() // netip.Prefix
		if !pfx.Addr().Is4() || !pfx.Contains(dst) {
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
		return "", "", 0, 0, fmt.Errorf("BestRoute(v4): no matching route for %s", dest)
	}

	// Gateway: empty/unspecified => on-link.
	var gw string
	if nh := best.NextHop.Addr(); nh.IsValid() && nh.Is4() && !nh.IsUnspecified() {
		gw = nh.String()
	}

	alias := displayNameFromLUID(best.InterfaceLUID, best.InterfaceIndex)
	return gw, alias, int(best.InterfaceIndex), int(best.Metric), nil
}
func parseDestPrefixV4(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return netip.Prefix{}, fmt.Errorf("empty destination")
	}
	// CIDR?
	if strings.Contains(s, "/") {
		pfx, pfxErr := netip.ParsePrefix(s)
		if pfxErr != nil || !pfx.Addr().Is4() {
			return netip.Prefix{}, fmt.Errorf("bad IPv4 prefix: %q", s)
		}
		return pfx, nil
	}
	// Host → /32
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return netip.Prefix{}, fmt.Errorf("not an IPv4 address: %q", s)
	}
	var a4 [4]byte
	copy(a4[:], ip)
	return netip.PrefixFrom(netip.AddrFrom4(a4), 32), nil
}

func displayNameFromLUID(luid winipcfg.LUID, ifIndex uint32) string {
	if ifRow, _ := luid.Interface(); ifRow != nil {
		if s := strings.TrimSpace(ifRow.Alias()); s != "" {
			return s
		}
		if s := strings.TrimSpace(ifRow.Description()); s != "" {
			return s
		}
	}
	if addrs, err := winipcfg.GetAdaptersAddresses(winipcfg.AddressFamily(windows.AF_UNSPEC), 0); err == nil {
		for _, a := range addrs {
			if a.LUID == luid {
				if s := strings.TrimSpace(a.FriendlyName()); s != "" {
					return s
				}
				break
			}
		}
	}
	if ifIndex != 0 {
		return fmt.Sprintf("if#%d", ifIndex)
	}
	return ""
}
