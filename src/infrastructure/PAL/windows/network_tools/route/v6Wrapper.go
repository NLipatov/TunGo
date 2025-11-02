//go:build windows

package route

import (
	"fmt"
	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
	"math"
	"net"
	"net/netip"
	"strings"
	"tungo/infrastructure/PAL"
)

type v6Wrapper struct {
	commander PAL.Commander
}

func newV6Wrapper(c PAL.Commander) Contract { return &v6Wrapper{commander: c} }

func (w *v6Wrapper) Delete(dst string) error {
	dst = strings.TrimSpace(dropZone(dst))
	if dst == "" {
		return fmt.Errorf("route -6 delete: empty destination")
	}
	out, err := w.commander.CombinedOutput("route", "-6", "delete", dst)
	if err != nil {
		return fmt.Errorf("route delete %s: %v, output: %s", dst, err, out)
	}
	return nil
}

func (w *v6Wrapper) Print(t string) ([]byte, error) {
	args := []string{"print", "-6"}
	if s := strings.TrimSpace(t); s != "" {
		args = append(args, dropZone(s))
	}
	out, err := w.commander.CombinedOutput("route", args...)
	if err != nil {
		return nil, fmt.Errorf("route %s: %v, output: %s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

func dropZone(s string) string {
	if i := strings.IndexByte(s, '%'); i >= 0 {
		return s[:i]
	}
	return s
}

// DefaultRoute returns (::/0) route info for IPv6.
// Picks the ::/0 entry with the lowest metric.
// Returns gateway (empty means on-link), interface alias (friendly name), and metric.
func (w *v6Wrapper) DefaultRoute() (gw, ifName string, metric int, err error) {
	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET6))
	if err != nil {
		return "", "", 0, fmt.Errorf("GetIPForwardTable2(v6): %w", err)
	}

	var (
		best       *winipcfg.MibIPforwardRow2
		bestMetric uint32 = math.MaxUint32
	)

	for i := range rows {
		pfx := rows[i].DestinationPrefix.Prefix()
		// Only ::/0 (default) routes.
		if !pfx.Addr().Is6() || pfx.Bits() != 0 {
			continue
		}
		if best == nil || rows[i].Metric < bestMetric {
			best = &rows[i]
			bestMetric = rows[i].Metric
		}
	}

	if best == nil {
		return "", "", 0, fmt.Errorf("default v6 route not found")
	}

	// Next hop: empty/unspecified => on-link.
	if nh := best.NextHop.Addr(); nh.IsValid() && nh.Is6() && !nh.IsUnspecified() {
		gw = nh.String()
	}

	metric = int(best.Metric)

	// Resolve interface alias (friendly name).
	if ifRow, _ := best.InterfaceLUID.Interface(); ifRow != nil {
		if a := ifRow.Alias(); a != "" {
			ifName = a
		}
	}

	return
}

// BestRoute returns (gateway, interfaceAlias, interfaceIndex, routeMetric) for IPv6.
// Uses GetIPForwardTable2(AF_INET6) and picks the best entry by:
// 1) longest prefix match, 2) lowest metric. No external processes.
func (w *v6Wrapper) BestRoute(dest string) (string, string, int, int, error) {
	raw := strings.TrimSpace(dest)
	ipStr := dropZone(raw) // strip "%zone" if present, e.g., "fe80::1%12" -> "fe80::1"
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
		if pl > bestPL || (pl == bestPL && m < bestMetric) {
			best, bestPL, bestMetric = &rows[i], pl, m
		}
	}
	if best == nil {
		return "", "", 0, 0, fmt.Errorf("BestRoute(v6): no matching route for %s", dest)
	}

	// Gateway: empty/unspecified => on-link.
	var gw string
	if nh := best.NextHop.Addr(); nh.IsValid() && nh.Is6() && !nh.IsUnspecified() {
		gw = nh.String()
		// Note: for link-local gateways (fe80::/64) specify IF index when adding routes.
	}

	alias := ""
	if ifRow, _ := best.InterfaceLUID.Interface(); ifRow != nil {
		if a := ifRow.Alias(); a != "" {
			alias = a
		}
	}

	return gw, alias, int(best.InterfaceIndex), int(best.Metric), nil
}
