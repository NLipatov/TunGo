//go:build windows

package defaultroute

import (
	"context"
	"net/netip"
	"sync"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

type route struct {
	nextHop         netip.Addr
	ifIndex, metric uint32
}

func Watch(ctx context.Context, serverAddr netip.Addr) (<-chan error, error) {
	changed := make(chan struct{})
	errCh := make(chan error, 1)

	family := addrFamily(serverAddr)
	// The snapshot is taken before registering so the callback only reads it.
	// Without a snapshot every default route event counts as a change.
	prev, prevOk, _ := defaultRoute(family)

	var once sync.Once
	callback, err := winipcfg.RegisterRouteChangeCallback(
		func(_ winipcfg.MibNotificationType, row *winipcfg.MibIPforwardRow2) {
			if changesDefaultRoute(row, family) && !ignore(prev, prevOk, family) {
				once.Do(func() {
					close(changed)
				})
			}
		},
	)
	if err != nil {
		return nil, err
	}

	go func() {
		select {
		case <-ctx.Done():
			_ = callback.Unregister()
		case <-changed:
			errCh <- callback.Unregister()
		}
	}()

	return errCh, nil
}

func ignore(prev route, prevOk bool, family winipcfg.AddressFamily) bool {
	if !prevOk {
		return false
	}
	cur, curOk, err := defaultRoute(family)
	return err == nil && curOk &&
		cur.nextHop == prev.nextHop && cur.ifIndex == prev.ifIndex &&
		// A zero interface index leaves the route identity unknown,
		// so it cannot prove the route is unchanged.
		cur.ifIndex != 0
}

func addrFamily(addr netip.Addr) winipcfg.AddressFamily {
	if addr.Unmap().Is4() {
		return windows.AF_INET
	}
	return windows.AF_INET6
}

func changesDefaultRoute(
	row *winipcfg.MibIPforwardRow2,
	family winipcfg.AddressFamily,
) bool {
	if row == nil || row.DestinationPrefix.RawPrefix.Family != family {
		return false
	}
	prefix := row.DestinationPrefix.Prefix()
	return prefix.IsValid() && prefix.Bits() == 0
}

func defaultRoute(family winipcfg.AddressFamily) (route, bool, error) {
	routes, err := winipcfg.GetIPForwardTable2(family)
	if err != nil {
		return route{}, false, err
	}
	interfaces, err := winipcfg.GetIPInterfaceTable(family)
	if err != nil {
		return route{}, false, err
	}
	r, ok := bestDefaultRoute(routes, interfaces, family)
	return r, ok, nil
}

// bestDefaultRoute picks the default route Windows uses: the lowest sum of
// route and interface metrics among routes on connected interfaces.
func bestDefaultRoute(
	routes []winipcfg.MibIPforwardRow2,
	interfaces []winipcfg.MibIPInterfaceRow,
	family winipcfg.AddressFamily,
) (route, bool) {
	connected := make(map[winipcfg.LUID]uint32, len(interfaces))
	for _, iface := range interfaces {
		if iface.Connected {
			connected[iface.InterfaceLUID] = iface.Metric
		}
	}
	best, found := route{}, false
	for i := range routes {
		row := &routes[i]
		if !changesDefaultRoute(row, family) {
			continue
		}
		interfaceMetric, ok := connected[row.InterfaceLUID]
		if !ok {
			continue
		}
		r := route{
			nextHop: row.NextHop.Addr(),
			ifIndex: row.InterfaceIndex,
			metric:  row.Metric + interfaceMetric,
		}
		if !found || best.metric > r.metric {
			found = true
			best = r
		}
	}
	return best, found
}
