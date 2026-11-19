//go:build windows

package defaultroute

import (
	"net/netip"
	"testing"

	"golang.org/x/sys/windows"
	"golang.zx2c4.com/wireguard/windows/tunnel/winipcfg"
)

func TestChangesDefaultRoute(t *testing.T) {
	if changesDefaultRoute(nil, windows.AF_INET) {
		t.Fatal("got true for nil route, want false")
	}

	if changesDefaultRoute(&winipcfg.MibIPforwardRow2{}, windows.AF_INET) {
		t.Fatal("got true for invalid prefix, want false")
	}

	tests := []struct {
		name   string
		prefix netip.Prefix
		family winipcfg.AddressFamily
		want   bool
	}{
		{name: "IPv4 default route", prefix: netip.MustParsePrefix("0.0.0.0/0"), family: windows.AF_INET, want: true},
		{name: "IPv6 default route", prefix: netip.MustParsePrefix("::/0"), family: windows.AF_INET6, want: true},
		{name: "IPv6 default route while watching IPv4", prefix: netip.MustParsePrefix("::/0"), family: windows.AF_INET},
		{name: "IPv4 default route while watching IPv6", prefix: netip.MustParsePrefix("0.0.0.0/0"), family: windows.AF_INET6},
		{name: "IPv4 split route", prefix: netip.MustParsePrefix("0.0.0.0/1"), family: windows.AF_INET},
		{name: "IPv6 split route", prefix: netip.MustParsePrefix("::/1"), family: windows.AF_INET6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var row winipcfg.MibIPforwardRow2
			if err := row.DestinationPrefix.SetPrefix(tt.prefix); err != nil {
				t.Fatal(err)
			}
			if got := changesDefaultRoute(&row, tt.family); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestChangesInterface(t *testing.T) {
	tests := []struct {
		name  string
		iface *winipcfg.MibIPInterfaceRow
		want  bool
	}{
		{name: "nil interface"},
		{name: "same family", iface: &winipcfg.MibIPInterfaceRow{Family: windows.AF_INET}, want: true},
		{name: "other family", iface: &winipcfg.MibIPInterfaceRow{Family: windows.AF_INET6}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := changesInterface(tt.iface, windows.AF_INET); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBestDefaultRoute(t *testing.T) {
	const (
		ethernet winipcfg.LUID = 1
		wifi     winipcfg.LUID = 2
		vpn      winipcfg.LUID = 3
	)
	ethernetGateway := netip.MustParseAddr("10.0.0.3")
	wifiGateway := netip.MustParseAddr("192.168.1.1")

	tests := []struct {
		name       string
		routes     []winipcfg.MibIPforwardRow2
		interfaces []winipcfg.MibIPInterfaceRow
		want       route
		wantFound  bool
	}{
		{
			name:       "no default route",
			routes:     []winipcfg.MibIPforwardRow2{routeRow(t, "10.0.0.0/24", ethernetGateway, ethernet, 7, 0)},
			interfaces: []winipcfg.MibIPInterfaceRow{interfaceRow(ethernet, 25, true)},
		},
		{
			name:       "single default route",
			routes:     []winipcfg.MibIPforwardRow2{routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 0)},
			interfaces: []winipcfg.MibIPInterfaceRow{interfaceRow(ethernet, 25, true)},
			want:       route{nextHop: ethernetGateway, ifIndex: 7, metric: 25},
			wantFound:  true,
		},
		{
			name: "interface metric decides",
			routes: []winipcfg.MibIPforwardRow2{
				routeRow(t, "0.0.0.0/0", wifiGateway, wifi, 9, 0),
				routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 0),
			},
			interfaces: []winipcfg.MibIPInterfaceRow{interfaceRow(wifi, 50, true), interfaceRow(ethernet, 25, true)},
			want:       route{nextHop: ethernetGateway, ifIndex: 7, metric: 25},
			wantFound:  true,
		},
		{
			name: "route metric is added to interface metric",
			routes: []winipcfg.MibIPforwardRow2{
				routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 100),
				routeRow(t, "0.0.0.0/0", wifiGateway, wifi, 9, 0),
			},
			interfaces: []winipcfg.MibIPInterfaceRow{interfaceRow(ethernet, 25, true), interfaceRow(wifi, 50, true)},
			want:       route{nextHop: wifiGateway, ifIndex: 9, metric: 50},
			wantFound:  true,
		},
		{
			name: "same route with penalized metric",
			routes: []winipcfg.MibIPforwardRow2{
				routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 20000),
				routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 0),
			},
			interfaces: []winipcfg.MibIPInterfaceRow{interfaceRow(ethernet, 25, true)},
			want:       route{nextHop: ethernetGateway, ifIndex: 7, metric: 25},
			wantFound:  true,
		},
		{
			name: "disconnected and unknown interfaces are skipped",
			routes: []winipcfg.MibIPforwardRow2{
				routeRow(t, "0.0.0.0/0", wifiGateway, wifi, 9, 0),
				routeRow(t, "0.0.0.0/0", netip.IPv4Unspecified(), vpn, 11, 0),
				routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 0),
			},
			interfaces: []winipcfg.MibIPInterfaceRow{interfaceRow(wifi, 1, false), interfaceRow(ethernet, 25, true)},
			want:       route{nextHop: ethernetGateway, ifIndex: 7, metric: 25},
			wantFound:  true,
		},
		{
			name:       "other address family is skipped",
			routes:     []winipcfg.MibIPforwardRow2{routeRow(t, "::/0", netip.MustParseAddr("fe80::1"), ethernet, 7, 0)},
			interfaces: []winipcfg.MibIPInterfaceRow{interfaceRow(ethernet, 25, true)},
		},
		{
			name: "equal effective metrics with different paths are ambiguous",
			routes: []winipcfg.MibIPforwardRow2{
				routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 0),
				routeRow(t, "0.0.0.0/0", wifiGateway, wifi, 9, 0),
			},
			interfaces: []winipcfg.MibIPInterfaceRow{interfaceRow(ethernet, 25, true), interfaceRow(wifi, 25, true)},
		},
		{
			name: "equal effective metrics with the same path are not ambiguous",
			routes: []winipcfg.MibIPforwardRow2{
				routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 0),
				routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 0),
			},
			interfaces: []winipcfg.MibIPInterfaceRow{interfaceRow(ethernet, 25, true)},
			want:       route{nextHop: ethernetGateway, ifIndex: 7, metric: 25},
			wantFound:  true,
		},
		{
			name: "lower effective metric resolves earlier ambiguity",
			routes: []winipcfg.MibIPforwardRow2{
				routeRow(t, "0.0.0.0/0", wifiGateway, wifi, 9, 575),
				routeRow(t, "0.0.0.0/0", netip.IPv4Unspecified(), vpn, 11, 575),
				routeRow(t, "0.0.0.0/0", ethernetGateway, ethernet, 7, 75),
			},
			interfaces: []winipcfg.MibIPInterfaceRow{
				interfaceRow(wifi, 25, true), interfaceRow(vpn, 25, true), interfaceRow(ethernet, 25, true),
			},
			want:      route{nextHop: ethernetGateway, ifIndex: 7, metric: 100},
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := bestDefaultRoute(tt.routes, tt.interfaces, windows.AF_INET)
			if found != tt.wantFound || got != tt.want {
				t.Fatalf("got %+v, %v, want %+v, %v", got, found, tt.want, tt.wantFound)
			}
		})
	}
}

func routeRow(
	t *testing.T,
	prefix string,
	nextHop netip.Addr,
	luid winipcfg.LUID,
	ifIndex, metric uint32,
) winipcfg.MibIPforwardRow2 {
	t.Helper()
	row := winipcfg.MibIPforwardRow2{InterfaceLUID: luid, InterfaceIndex: ifIndex, Metric: metric}
	if err := row.DestinationPrefix.SetPrefix(netip.MustParsePrefix(prefix)); err != nil {
		t.Fatal(err)
	}
	if err := row.NextHop.SetAddr(nextHop); err != nil {
		t.Fatal(err)
	}
	return row
}

func interfaceRow(luid winipcfg.LUID, metric uint32, connected bool) winipcfg.MibIPInterfaceRow {
	return winipcfg.MibIPInterfaceRow{InterfaceLUID: luid, Metric: metric, Connected: connected}
}

func TestAddrFamily(t *testing.T) {
	tests := []struct {
		addr netip.Addr
		want winipcfg.AddressFamily
	}{
		{addr: netip.MustParseAddr("192.0.2.1"), want: windows.AF_INET},
		{addr: netip.MustParseAddr("::ffff:192.0.2.1"), want: windows.AF_INET},
		{addr: netip.MustParseAddr("2001:db8::1"), want: windows.AF_INET6},
	}

	for _, tt := range tests {
		t.Run(tt.addr.String(), func(t *testing.T) {
			if got := addrFamily(tt.addr); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWatchStarts(t *testing.T) {
	errCh, err := Watch(t.Context(), netip.MustParseAddr("192.0.2.1"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if errCh == nil {
		t.Fatal("got nil channel")
	}
}

func TestIgnoreUnchangedRoutingTable(t *testing.T) {
	snapshot, ok, err := defaultRoute(windows.AF_INET)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok || snapshot.ifIndex == 0 {
		t.Skip("no IPv4 default route with a known interface")
	}
	if !ignore(snapshot, ok, windows.AF_INET) {
		t.Fatal("got change for an unchanged routing table, want ignore")
	}
}
