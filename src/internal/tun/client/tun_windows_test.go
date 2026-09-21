//go:build windows

package client

import (
	"errors"
	"fmt"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"testing"

	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/windows/ipcfg"
)

type windowsTunMock struct {
	closeErr   error
	closeCalls int
}

func (*windowsTunMock) Read([]byte) (int, error)    { return 0, nil }
func (*windowsTunMock) Write(p []byte) (int, error) { return len(p), nil }
func (m *windowsTunMock) Close() error {
	m.closeCalls++
	return m.closeErr
}

type windowsNetConfigMock struct {
	bestRouteGateway netip.Addr
	bestRouteIf      string
	bestRouteIndex   int
	bestRouteErr     error
	addRouteErr      error
	deleteRouteErr   error
	setAddressErr    error
	addSplitErr      error
	setMTUErr        error
	setDNSErr        error
	setDNSErrAt      int
	flushDNSErr      error

	addresses     []netip.Prefix
	mtus          []int
	dnsNames      []string
	dnsValues     [][]string
	addedRoutes   []string
	deletedRoutes []string
	addedSplits   []string
	addedPrefixes [][]string
	flushDNSCalls int
}

func (m *windowsNetConfigMock) FlushDNS() error {
	m.flushDNSCalls++
	return m.flushDNSErr
}

func (m *windowsNetConfigMock) SetAddressStatic(_ string, prefix netip.Prefix) error {
	m.addresses = append(m.addresses, prefix)
	return m.setAddressErr
}

func (m *windowsNetConfigMock) SetDNS(ifName string, resolvers []string) error {
	m.dnsNames = append(m.dnsNames, ifName)
	m.dnsValues = append(m.dnsValues, append([]string(nil), resolvers...))
	if m.setDNSErrAt > 0 && len(m.dnsNames) != m.setDNSErrAt {
		return nil
	}
	return m.setDNSErr
}

func (m *windowsNetConfigMock) SetMTU(_ string, mtu int) error {
	m.mtus = append(m.mtus, mtu)
	return m.setMTUErr
}

func (m *windowsNetConfigMock) AddHostRouteViaGateway(host netip.Addr, ifName string, gateway netip.Addr) error {
	m.addedRoutes = append(m.addedRoutes, host.String()+" via "+gateway.String()+" dev "+ifName)
	return m.addRouteErr
}

func (m *windowsNetConfigMock) AddHostRouteOnLink(host netip.Addr, ifName string) error {
	m.addedRoutes = append(m.addedRoutes, host.String()+" dev "+ifName)
	return m.addRouteErr
}

func (m *windowsNetConfigMock) AddSplitRoutes(ifName string, prefixes []string) error {
	m.addedSplits = append(m.addedSplits, ifName)
	m.addedPrefixes = append(m.addedPrefixes, slices.Clone(prefixes))
	return m.addSplitErr
}

func (m *windowsNetConfigMock) DeleteRoute(destination netip.Addr) error {
	m.deletedRoutes = append(m.deletedRoutes, destination.String())
	return m.deleteRouteErr
}

func (m *windowsNetConfigMock) DeleteRouteOnInterface(destination netip.Addr, ifName string) error {
	m.deletedRoutes = append(m.deletedRoutes, destination.String()+"@"+ifName)
	return m.deleteRouteErr
}

func (m *windowsNetConfigMock) BestRoute(netip.Addr) (netip.Addr, string, int, int, error) {
	if m.bestRouteErr != nil {
		return netip.Addr{}, "", 0, 0, m.bestRouteErr
	}
	ifName := m.bestRouteIf
	if ifName == "" && m.bestRouteIndex == 0 {
		ifName = "Ethernet0"
	}
	ifIndex := m.bestRouteIndex
	if ifIndex == 0 {
		ifIndex = 1
	}
	return m.bestRouteGateway, ifName, ifIndex, 1, nil
}

func windowsSettings(v4, v6 bool) settings.Settings {
	active := settings.Settings{
		Network: settings.Network{TunName: "tun0"},
		MTU:     settings.DefaultMTU,
	}
	if v4 {
		active.IPv4Subnet = netip.MustParsePrefix("10.0.0.0/24")
		active.IPv4 = netip.MustParseAddr("10.0.0.2")
		active.DNSv4 = []string{"9.9.9.9"}
	}
	if v6 {
		active.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
		active.IPv6 = netip.MustParseAddr("fd00::2")
		active.DNSv6 = []string{"2620:fe::9"}
	}
	return active
}

func newWindowsTestTUN(t *testing.T, active settings.Settings) (*TUN, *windowsNetConfigMock, *windowsNetConfigMock) {
	t.Helper()
	configuration := &clientconfig.Configuration{
		Protocol:    settings.UDP,
		UDPSettings: active,
	}
	netConfig4 := &windowsNetConfigMock{}
	netConfig6 := &windowsNetConfigMock{}
	tunnel := &TUN{
		configuration: configuration,
		settings:      active,
		netConfig4:    netConfig4,
		netConfig6:    netConfig6,
	}
	return tunnel, netConfig4, netConfig6
}

func TestWindowsTUNAppliesTunnelRoutesAndClosesTun(t *testing.T) {
	for _, test := range []struct {
		name string
		v4   []string
		v6   []string
	}{
		{name: "custom prefixes", v4: []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"}, v6: []string{"2001:db8:1::/64"}},
		{name: "empty IPv4", v4: []string{}, v6: []string{"2001:db8:1::/64"}},
		{name: "empty IPv6", v4: []string{"192.0.2.0/24"}, v6: []string{}},
		{name: "both empty", v4: []string{}, v6: []string{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tunnel, err := New(&clientconfig.Configuration{
				ClientID:       1,
				Protocol:       settings.UDP,
				UDPSettings:    windowsSettings(true, true),
				TunnelRoutesV4: test.v4,
				TunnelRoutesV6: test.v6,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			netConfig4, netConfig6 := &windowsNetConfigMock{}, &windowsNetConfigMock{}
			tunnel.netConfig4, tunnel.netConfig6 = netConfig4, netConfig6
			tun := &windowsTunMock{}
			tunnel.tun = tun
			if err := tunnel.addSplitRoutes(netip.MustParseAddr("198.51.100.1")); err != nil {
				t.Fatalf("addSplitRoutes() error = %v", err)
			}
			if err := tunnel.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if err := tunnel.Close(); err != nil {
				t.Fatalf("repeated Close() error = %v", err)
			}
			if tun.closeCalls != 1 || tunnel.tun != nil {
				t.Fatalf("TUN cleanup: close calls = %d, retained = %v", tun.closeCalls, tunnel.tun != nil)
			}

			for _, check := range []struct {
				name  string
				calls [][]string
				want  []string
			}{
				{name: "add IPv4", calls: netConfig4.addedPrefixes, want: test.v4},
				{name: "add IPv6", calls: netConfig6.addedPrefixes, want: test.v6},
			} {
				if len(check.calls) != 1 {
					t.Fatalf("%s calls = %v, want one call", check.name, check.calls)
				}
				for _, prefixes := range check.calls {
					if !slices.Equal(prefixes, check.want) {
						t.Errorf("%s prefixes = %v, want %v", check.name, prefixes, check.want)
					}
				}
			}
		})
	}
}

func TestWindowsTUNFiltersTunSubnetAndCurrentServerRoutes(t *testing.T) {
	routesV4 := []string{"128.0.0.0/1", "198.51.100.1/32", "10.0.0.0/24", "10.0.0.0/16"}
	routesV6 := []string{"::/1", "2001:db8::1/128", "fd00::/64", "fd00::/48"}
	tunnel, netConfig4, netConfig6 := newWindowsTestTUN(t, windowsSettings(true, true))
	tunnel.splitsv4, tunnel.splitsv6 = slices.Clone(routesV4), slices.Clone(routesV6)

	for _, test := range []struct {
		server string
		wantV4 []string
		wantV6 []string
	}{
		{
			server: "198.51.100.1",
			wantV4: []string{"128.0.0.0/1", "10.0.0.0/16"},
			wantV6: []string{"::/1", "2001:db8::1/128", "fd00::/48"},
		},
		{
			server: "2001:db8::1",
			wantV4: []string{"128.0.0.0/1", "198.51.100.1/32", "10.0.0.0/16"},
			wantV6: []string{"::/1", "fd00::/48"},
		},
	} {
		t.Run(test.server, func(t *testing.T) {
			netConfig4.addedPrefixes, netConfig6.addedPrefixes = nil, nil
			tunnel.tun = &windowsTunMock{}
			if err := tunnel.addSplitRoutes(netip.MustParseAddr(test.server)); err != nil {
				t.Fatalf("addSplitRoutes() error = %v", err)
			}
			if !reflect.DeepEqual(netConfig4.addedPrefixes, [][]string{test.wantV4}) ||
				!reflect.DeepEqual(netConfig6.addedPrefixes, [][]string{test.wantV6}) {
				t.Errorf("installed routes: IPv4=%v IPv6=%v, want IPv4=%v IPv6=%v",
					netConfig4.addedPrefixes, netConfig6.addedPrefixes, test.wantV4, test.wantV6)
			}
			if err := tunnel.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if !slices.Equal(tunnel.splitsv4, routesV4) || !slices.Equal(tunnel.splitsv6, routesV6) {
				t.Errorf("tunnel changed stored routes: IPv4=%v IPv6=%v", tunnel.splitsv4, tunnel.splitsv6)
			}
		})
	}
}

func TestWindowsTUNConfiguresEveryAddressMode(t *testing.T) {
	for _, test := range []struct {
		name string
		v4   bool
		v6   bool
	}{
		{name: "IPv4", v4: true},
		{name: "IPv6", v6: true},
		{name: "dual stack", v4: true, v6: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			tunnel, netConfig4, netConfig6 := newWindowsTestTUN(t, windowsSettings(test.v4, test.v6))
			if err := tunnel.assignAddresses(); err != nil {
				t.Fatalf("assignAddresses() error = %v", err)
			}
			tunnel.splitsv4 = []string{"192.0.2.0/24"}
			tunnel.splitsv6 = []string{"2001:db8::/64"}
			if err := tunnel.addSplitRoutes(netip.MustParseAddr("198.51.100.1")); err != nil {
				t.Fatalf("addSplitRoutes() error = %v", err)
			}
			if err := tunnel.setMTU(); err != nil {
				t.Fatalf("setMTU() error = %v", err)
			}
			if err := tunnel.setDNS(); err != nil {
				t.Fatalf("setDNS() error = %v", err)
			}

			var wantAddresses4, wantAddresses6 []netip.Prefix
			var wantSplits4, wantSplits6 []string
			var wantPrefixes4, wantPrefixes6 [][]string
			var wantMTUs4, wantMTUs6 []int
			var wantDNSNames4, wantDNSNames6 []string
			var wantDNS4, wantDNS6 [][]string
			if test.v4 {
				wantAddresses4 = []netip.Prefix{netip.MustParsePrefix("10.0.0.2/24")}
				wantSplits4 = []string{"tun0"}
				wantPrefixes4 = [][]string{{"192.0.2.0/24"}}
				wantMTUs4 = []int{settings.DefaultMTU}
				wantDNSNames4 = []string{"tun0"}
				wantDNS4 = [][]string{{"9.9.9.9"}}
			}
			if test.v6 {
				wantAddresses6 = []netip.Prefix{netip.MustParsePrefix("fd00::2/64")}
				wantSplits6 = []string{"tun0"}
				wantPrefixes6 = [][]string{{"2001:db8::/64"}}
				wantMTUs6 = []int{settings.DefaultMTU}
				wantDNSNames6 = []string{"tun0"}
				wantDNS6 = [][]string{{"2620:fe::9"}}
			}

			if !reflect.DeepEqual(netConfig4.addresses, wantAddresses4) {
				t.Fatalf("IPv4 addresses = %v, want %v", netConfig4.addresses, wantAddresses4)
			}
			if !reflect.DeepEqual(netConfig6.addresses, wantAddresses6) {
				t.Fatalf("IPv6 addresses = %v, want %v", netConfig6.addresses, wantAddresses6)
			}
			if !reflect.DeepEqual(netConfig4.addedSplits, wantSplits4) {
				t.Fatalf("IPv4 split interfaces = %v, want %v", netConfig4.addedSplits, wantSplits4)
			}
			if !reflect.DeepEqual(netConfig6.addedSplits, wantSplits6) {
				t.Fatalf("IPv6 split interfaces = %v, want %v", netConfig6.addedSplits, wantSplits6)
			}
			if !reflect.DeepEqual(netConfig4.addedPrefixes, wantPrefixes4) {
				t.Fatalf("IPv4 split prefixes = %v, want %v", netConfig4.addedPrefixes, wantPrefixes4)
			}
			if !reflect.DeepEqual(netConfig6.addedPrefixes, wantPrefixes6) {
				t.Fatalf("IPv6 split prefixes = %v, want %v", netConfig6.addedPrefixes, wantPrefixes6)
			}
			if !reflect.DeepEqual(netConfig4.mtus, wantMTUs4) {
				t.Fatalf("IPv4 MTUs = %v, want %v", netConfig4.mtus, wantMTUs4)
			}
			if !reflect.DeepEqual(netConfig6.mtus, wantMTUs6) {
				t.Fatalf("IPv6 MTUs = %v, want %v", netConfig6.mtus, wantMTUs6)
			}
			if !reflect.DeepEqual(netConfig4.dnsNames, wantDNSNames4) {
				t.Fatalf("IPv4 DNS interfaces = %v, want %v", netConfig4.dnsNames, wantDNSNames4)
			}
			if !reflect.DeepEqual(netConfig6.dnsNames, wantDNSNames6) {
				t.Fatalf("IPv6 DNS interfaces = %v, want %v", netConfig6.dnsNames, wantDNSNames6)
			}
			if !reflect.DeepEqual(netConfig4.dnsValues, wantDNS4) {
				t.Fatalf("IPv4 DNS values = %v, want %v", netConfig4.dnsValues, wantDNS4)
			}
			if !reflect.DeepEqual(netConfig6.dnsValues, wantDNS6) {
				t.Fatalf("IPv6 DNS values = %v, want %v", netConfig6.dnsValues, wantDNS6)
			}
		})
	}
}

func TestWindowsTUNReturnsConfigurationErrors(t *testing.T) {
	failure := errors.New("configuration failed")
	tests := []struct {
		name string
		fail func(*windowsNetConfigMock, *windowsNetConfigMock)
		run  func(*TUN) error
	}{
		{
			name: "IPv4 address",
			fail: func(v4, _ *windowsNetConfigMock) { v4.setAddressErr = failure },
			run:  (*TUN).assignAddresses,
		},
		{
			name: "IPv6 address",
			fail: func(_, v6 *windowsNetConfigMock) { v6.setAddressErr = failure },
			run:  (*TUN).assignAddresses,
		},
		{
			name: "IPv4 split routes",
			fail: func(v4, _ *windowsNetConfigMock) { v4.addSplitErr = failure },
			run: func(m *TUN) error {
				return m.addSplitRoutes(netip.MustParseAddr("198.51.100.1"))
			},
		},
		{
			name: "IPv6 split routes",
			fail: func(_, v6 *windowsNetConfigMock) { v6.addSplitErr = failure },
			run: func(m *TUN) error {
				return m.addSplitRoutes(netip.MustParseAddr("198.51.100.1"))
			},
		},
		{
			name: "IPv4 MTU",
			fail: func(v4, _ *windowsNetConfigMock) { v4.setMTUErr = failure },
			run:  (*TUN).setMTU,
		},
		{
			name: "IPv6 MTU",
			fail: func(_, v6 *windowsNetConfigMock) { v6.setMTUErr = failure },
			run:  (*TUN).setMTU,
		},
		{
			name: "IPv4 DNS",
			fail: func(v4, _ *windowsNetConfigMock) { v4.setDNSErr = failure },
			run:  (*TUN).setDNS,
		},
		{
			name: "IPv6 DNS",
			fail: func(_, v6 *windowsNetConfigMock) { v6.setDNSErr = failure },
			run:  (*TUN).setDNS,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tunnel, netConfig4, netConfig6 := newWindowsTestTUN(t, windowsSettings(true, true))
			test.fail(netConfig4, netConfig6)

			if err := test.run(tunnel); !errors.Is(err, failure) {
				t.Fatalf("configuration error = %v, want %v", err, failure)
			}
		})
	}
}

func TestWindowsTUNDNSFlushIsBestEffortDuringSetup(t *testing.T) {
	tunnel, netConfig4, netConfig6 := newWindowsTestTUN(t, windowsSettings(true, true))
	netConfig4.flushDNSErr = errors.New("flush IPv4 failed")
	netConfig6.flushDNSErr = errors.New("flush IPv6 failed")

	if err := tunnel.setDNS(); err != nil {
		t.Fatalf("setDNS() error = %v", err)
	}
	if netConfig4.flushDNSCalls != 1 || netConfig6.flushDNSCalls != 1 {
		t.Fatalf("FlushDNS() calls = IPv4:%d IPv6:%d", netConfig4.flushDNSCalls, netConfig6.flushDNSCalls)
	}
}

func TestWindowsTUNStopsDNSSetupAfterIPv4Failure(t *testing.T) {
	failure := errors.New("IPv4 DNS failed")
	tunnel, netConfig4, netConfig6 := newWindowsTestTUN(t, windowsSettings(true, true))
	netConfig4.setDNSErr = failure

	err := tunnel.setDNS()
	if !errors.Is(err, failure) {
		t.Fatalf("setDNS() error = %v, want %v", err, failure)
	}
	if !strings.Contains(err.Error(), "set IPv4 DNS") {
		t.Fatalf("setDNS() error = %q, want operation context", err)
	}
	if !reflect.DeepEqual(netConfig4.dnsValues, [][]string{{"9.9.9.9"}}) {
		t.Fatalf("IPv4 DNS calls = %v, want one setup attempt", netConfig4.dnsValues)
	}
	if len(netConfig6.dnsValues) != 0 {
		t.Fatalf("IPv6 DNS calls = %v, want none", netConfig6.dnsValues)
	}
	if netConfig4.flushDNSCalls != 0 || netConfig6.flushDNSCalls != 0 {
		t.Fatalf("FlushDNS() calls = IPv4:%d IPv6:%d, want none", netConfig4.flushDNSCalls, netConfig6.flushDNSCalls)
	}
}

func TestWindowsTUNRollsBackPartialDNSSetup(t *testing.T) {
	setupErr := errors.New("IPv6 DNS failed")
	cleanupErr := errors.New("IPv4 DNS cleanup failed")
	for _, test := range []struct {
		name       string
		cleanupErr error
	}{
		{name: "rollback succeeds"},
		{name: "rollback fails", cleanupErr: cleanupErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			tunnel, netConfig4, netConfig6 := newWindowsTestTUN(t, windowsSettings(true, true))
			netConfig4.setDNSErr = test.cleanupErr
			netConfig4.setDNSErrAt = 2
			netConfig6.setDNSErr = setupErr

			err := tunnel.setDNS()
			if !errors.Is(err, setupErr) {
				t.Fatalf("setDNS() error = %v, want setup cause", err)
			}
			if test.cleanupErr != nil && !errors.Is(err, test.cleanupErr) {
				t.Fatalf("setDNS() error = %v, want cleanup cause", err)
			}
			if !reflect.DeepEqual(netConfig4.dnsValues, [][]string{{"9.9.9.9"}, nil}) {
				t.Fatalf("IPv4 DNS calls = %v, want setup then rollback", netConfig4.dnsValues)
			}
		})
	}
}

func TestWindowsTUNRejectsInvalidServerAddress(t *testing.T) {
	tunnel, _, _ := newWindowsTestTUN(t, windowsSettings(true, true))
	if _, err := tunnel.Open(netip.Addr{}); err == nil {
		t.Fatal("Open() error = nil")
	}
}

func TestWindowsTUNPinsServerRouteWithMatchingConfigurator(t *testing.T) {
	tunnel, netConfig4, netConfig6 := newWindowsTestTUN(t, windowsSettings(true, true))
	netConfig6.bestRouteIf = "Ethernet6"
	serverAddr := netip.MustParseAddr("2001:db8::1")

	if err := tunnel.pinServerRoute(serverAddr); err != nil {
		t.Fatalf("pinServerRoute() error = %v", err)
	}
	if len(netConfig4.addedRoutes) != 0 {
		t.Fatalf("IPv4 routes = %v", netConfig4.addedRoutes)
	}
	if want := []string{"2001:db8::1 dev Ethernet6"}; !reflect.DeepEqual(netConfig6.addedRoutes, want) {
		t.Fatalf("IPv6 routes = %v, want %v", netConfig6.addedRoutes, want)
	}
	if tunnel.pinnedServerAddr != serverAddr || tunnel.pinnedServerIf != "Ethernet6" {
		t.Fatalf("pinned route = %s@%s", tunnel.pinnedServerAddr, tunnel.pinnedServerIf)
	}
}

func TestWindowsTUNPinsServerRouteViaGateway(t *testing.T) {
	tunnel, netConfig4, _ := newWindowsTestTUN(t, windowsSettings(true, false))
	netConfig4.bestRouteGateway = netip.MustParseAddr("192.0.2.1")
	netConfig4.bestRouteIf = "Ethernet0"
	serverAddr := netip.MustParseAddr("198.51.100.1")

	if err := tunnel.pinServerRoute(serverAddr); err != nil {
		t.Fatalf("pinServerRoute() error = %v", err)
	}
	want := []string{"198.51.100.1 via 192.0.2.1 dev Ethernet0"}
	if !reflect.DeepEqual(netConfig4.addedRoutes, want) {
		t.Fatalf("routes = %v, want %v", netConfig4.addedRoutes, want)
	}
}

func TestWindowsTUNReturnsBestRouteError(t *testing.T) {
	failure := errors.New("route lookup failed")
	tunnel, netConfig4, _ := newWindowsTestTUN(t, windowsSettings(true, false))
	netConfig4.bestRouteErr = failure

	if err := tunnel.pinServerRoute(netip.MustParseAddr("198.51.100.1")); !errors.Is(err, failure) {
		t.Fatalf("pinServerRoute() error = %v, want %v", err, failure)
	}
	if len(netConfig4.addedRoutes) != 0 || tunnel.pinnedServerAddr.IsValid() || tunnel.pinnedServerIf != "" {
		t.Fatal("failed route lookup changed pinned route state")
	}
}

func TestWindowsTUNSkipsUncoveredServerFamily(t *testing.T) {
	tunnel, netConfig4, netConfig6 := newWindowsTestTUN(t, windowsSettings(true, false))
	if err := tunnel.pinServerRoute(netip.MustParseAddr("2001:db8::1")); err != nil {
		t.Fatalf("pinServerRoute() error = %v", err)
	}
	if len(netConfig4.addedRoutes) != 0 || len(netConfig6.addedRoutes) != 0 || tunnel.pinnedServerAddr.IsValid() {
		t.Fatal("unexpected pinned route")
	}
}

func TestWindowsTUNDoesNotCacheFailedServerRoute(t *testing.T) {
	tunnel, netConfig4, _ := newWindowsTestTUN(t, windowsSettings(true, false))
	netConfig4.addRouteErr = errors.New("route failed")
	if err := tunnel.pinServerRoute(netip.MustParseAddr("198.51.100.1")); err == nil {
		t.Fatal("pinServerRoute() error = nil")
	}
	if tunnel.pinnedServerAddr.IsValid() || tunnel.pinnedServerIf != "" {
		t.Fatalf("cached route = %s@%s", tunnel.pinnedServerAddr, tunnel.pinnedServerIf)
	}
}

func TestWindowsTUNUsesInterfaceIndexWhenAliasIsEmpty(t *testing.T) {
	tunnel, netConfig4, _ := newWindowsTestTUN(t, windowsSettings(true, false))
	netConfig4.bestRouteIndex = 12
	if err := tunnel.pinServerRoute(netip.MustParseAddr("198.51.100.1")); err != nil {
		t.Fatalf("pinServerRoute() error = %v", err)
	}
	if tunnel.pinnedServerIf != "12" {
		t.Fatalf("pinnedServerIf = %q", tunnel.pinnedServerIf)
	}
}

func TestWindowsTUNCloseRetriesServerRouteCleanup(t *testing.T) {
	deleteErr := errors.New("route cleanup failed")
	tunnel, netConfig4, _ := newWindowsTestTUN(t, windowsSettings(true, false))
	tunnel.pinnedServerAddr = netip.MustParseAddr("198.51.100.1")
	tunnel.pinnedServerIf = "Ethernet0"
	netConfig4.deleteRouteErr = deleteErr

	if err := tunnel.Close(); !errors.Is(err, deleteErr) {
		t.Fatalf("Close() error = %v, want %v", err, deleteErr)
	}
	if !tunnel.pinnedServerAddr.IsValid() || tunnel.pinnedServerIf == "" {
		t.Fatal("failed route cleanup cleared retry state")
	}

	netConfig4.deleteRouteErr = nil
	if err := tunnel.Close(); err != nil {
		t.Fatalf("retry Close() error = %v", err)
	}
	if tunnel.pinnedServerAddr.IsValid() || tunnel.pinnedServerIf != "" {
		t.Fatal("successful route cleanup retained state")
	}
}

func TestWindowsTUNCloseReturnsAllCleanupErrors(t *testing.T) {
	tunnel, netConfig4, netConfig6 := newWindowsTestTUN(t, windowsSettings(true, true))
	netConfig4.setDNSErr = errors.New("dns4 failed")
	netConfig4.flushDNSErr = errors.New("flush4 failed")
	netConfig6.setDNSErr = errors.New("dns6 failed")
	netConfig6.flushDNSErr = errors.New("flush6 failed")
	tunnel.pinnedServerAddr = netip.MustParseAddr("2001:db8::1")
	tunnel.pinnedServerIf = "Ethernet6"
	netConfig6.deleteRouteErr = errors.New("route6 failed")
	tun := &windowsTunMock{closeErr: errors.New("TUN close failed")}
	tunnel.tun = tun

	err := tunnel.Close()
	if err == nil {
		t.Fatal("Close() error = nil")
	}
	for _, want := range []string{"dns4 failed", "dns6 failed", "route6 failed", "TUN close failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("Close() error = %v, want %q", err, want)
		}
	}
	for _, ignored := range []string{"flush4 failed", "flush6 failed"} {
		if strings.Contains(err.Error(), ignored) {
			t.Fatalf("Close() error = %v, want cache flush failure ignored", err)
		}
	}
	if netConfig4.flushDNSCalls != 1 || netConfig6.flushDNSCalls != 1 {
		t.Fatalf("FlushDNS() calls = IPv4:%d IPv6:%d, want 1 each", netConfig4.flushDNSCalls, netConfig6.flushDNSCalls)
	}
	if tun.closeCalls != 1 || tunnel.tun != nil {
		t.Fatalf("TUN cleanup: close calls = %d, retained = %v", tun.closeCalls, tunnel.tun != nil)
	}
}

func TestWindowsTUNCloseReturnsStaleCleanupError(t *testing.T) {
	active := windowsSettings(true, false)
	tunnel, _, netConfig6 := newWindowsTestTUN(t, active)
	stale := windowsSettings(false, true)
	stale.TunName = "stale6"
	tunnel.configuration.TCPSettings = stale
	staleErr := errors.New("stale cleanup failed")
	netConfig6.setDNSErr = staleErr

	if err := tunnel.Close(); !errors.Is(err, staleErr) {
		t.Fatalf("Close() error = %v, want %v", err, staleErr)
	}
}

func TestWindowsTUNCloseIgnoresMissingStaleInterface(t *testing.T) {
	active := windowsSettings(true, false)
	tunnel, _, netConfig6 := newWindowsTestTUN(t, active)
	stale := windowsSettings(false, true)
	stale.TunName = "stale6"
	tunnel.configuration.TCPSettings = stale
	missing := fmt.Errorf("%w: %q", ipcfg.ErrInterfaceNotFound, stale.TunName)
	netConfig6.setDNSErr = missing

	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestWindowsTUNCloseIgnoresMissingActiveInterface(t *testing.T) {
	active := windowsSettings(true, false)
	tunnel, netConfig4, _ := newWindowsTestTUN(t, active)
	missing := fmt.Errorf("%w: %q", ipcfg.ErrInterfaceNotFound, active.TunName)
	netConfig4.setDNSErr = missing

	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestWindowsTUNCloseIgnoresIncompleteStaleSettings(t *testing.T) {
	tunnel, _, _ := newWindowsTestTUN(t, windowsSettings(true, false))
	tunnel.configuration.TCPSettings.TunName = "stale"

	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestWindowsTUNCloseCleansStaleSettings(t *testing.T) {
	active := windowsSettings(true, false)
	tunnel, _, netConfig6 := newWindowsTestTUN(t, active)
	stale := windowsSettings(false, true)
	stale.TunName = "stale6"
	tunnel.configuration.TCPSettings = stale

	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !reflect.DeepEqual(netConfig6.dnsNames, []string{"stale6"}) {
		t.Fatalf("stale DNS cleanup = %v", netConfig6.dnsNames)
	}
}

func TestWindowsRouteInterfaceName(t *testing.T) {
	if got, err := routeInterfaceName(" Ethernet0 ", 15); err != nil || got != "Ethernet0" {
		t.Fatalf("routeInterfaceName(alias) = %q, %v", got, err)
	}
	if got, err := routeInterfaceName("", 12); err != nil || got != "12" {
		t.Fatalf("routeInterfaceName(index) = %q, %v", got, err)
	}
	if _, err := routeInterfaceName("", 0); err == nil {
		t.Fatal("routeInterfaceName() error = nil")
	}
}
