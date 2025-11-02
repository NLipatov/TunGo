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

type v4Wrapper struct {
	commander PAL.Commander
}

func newV4Wrapper(c PAL.Commander) Contract { return &v4Wrapper{commander: c} }

func (w *v4Wrapper) Delete(dst string) error {
	dst = strings.TrimSpace(dst)
	if dst == "" {
		return fmt.Errorf("route delete: empty destination")
	}
	out, err := w.commander.CombinedOutput("route", "delete", dst)
	if err != nil {
		return fmt.Errorf("route delete %s: %v, output: %s", dst, err, out)
	}
	return nil
}

func (w *v4Wrapper) Print(t string) ([]byte, error) {
	args := []string{"print", "-4"}
	if s := strings.TrimSpace(t); s != "" {
		args = append(args, s)
	}
	out, err := w.commander.CombinedOutput("route", args...)
	if err != nil {
		return nil, fmt.Errorf("route %s: %v, output: %s", strings.Join(args, " "), err, out)
	}
	return out, nil
}

func (w *v4Wrapper) DefaultRoute() (gw, ifName string, metric int, err error) {
	rows, err := winipcfg.GetIPForwardTable2(winipcfg.AddressFamily(windows.AF_INET))
	if err != nil {
		return "", "", 0, fmt.Errorf("GetIPForwardTable2: %w", err)
	}
	var best *winipcfg.MibIPforwardRow2
	for i := range rows {
		pfx := rows[i].DestinationPrefix.Prefix()
		if !pfx.Addr().Is4() || pfx.Bits() != 0 { // 0.0.0.0/0
			continue
		}
		if best == nil || rows[i].Metric < best.Metric {
			best = &rows[i]
		}
	}
	if best == nil {
		return "", "", 0, fmt.Errorf("default v4 route not found")
	}
	if nh := best.NextHop.Addr(); nh.IsValid() && nh.Is4() && !nh.IsUnspecified() {
		gw = nh.String()
	}
	ifRow, _ := best.InterfaceLUID.Interface()
	if ifRow != nil {
		if a := ifRow.Alias(); a != "" {
			ifName = a
		}
	}
	return gw, ifName, int(best.Metric), nil
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

	alias := ""
	if ifRow, _ := best.InterfaceLUID.Interface(); ifRow != nil {
		if a := ifRow.Alias(); a != "" {
			alias = a
		}
	}

	return gw, alias, int(best.InterfaceIndex), int(best.Metric), nil
}
