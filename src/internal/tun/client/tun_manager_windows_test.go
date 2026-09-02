//go:build windows

package client

import (
	"errors"
	"fmt"
	"net/netip"
	"reflect"
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
	deleteSplitErr   error
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
	deletedSplits []string
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

func (m *windowsNetConfigMock) AddDefaultSplitRoutes(ifName string) error {
	m.addedSplits = append(m.addedSplits, ifName)
	return m.addSplitErr
}

func (m *windowsNetConfigMock) DeleteDefaultSplitRoutes(ifName string) error {
	m.deletedSplits = append(m.deletedSplits, ifName)
	return m.deleteSplitErr
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

func newWindowsTestManager(t *testing.T, active settings.Settings) (*Manager, *windowsNetConfigMock, *windowsNetConfigMock) {
	t.Helper()
	configuration := &clientconfig.Configuration{
		Protocol:    settings.UDP,
		UDPSettings: active,
	}
	netConfig4 := &windowsNetConfigMock{}
	netConfig6 := &windowsNetConfigMock{}
	manager := &Manager{
		configuration: configuration,
		settings:      active,
		netConfig4:    netConfig4,
		netConfig6:    netConfig6,
	}
	return manager, netConfig4, netConfig6
}

func TestWindowsManagerConfiguresEveryAddressMode(t *testing.T) {
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
			manager, netConfig4, netConfig6 := newWindowsTestManager(t, windowsSettings(test.v4, test.v6))
			if err := manager.assignAddresses(); err != nil {
				t.Fatalf("assignAddresses() error = %v", err)
			}
			if err := manager.addSplitRoutes(); err != nil {
				t.Fatalf("addSplitRoutes() error = %v", err)
			}
			if err := manager.setMTU(); err != nil {
				t.Fatalf("setMTU() error = %v", err)
			}
			if err := manager.setDNS(); err != nil {
				t.Fatalf("setDNS() error = %v", err)
			}

			var wantAddresses4, wantAddresses6 []netip.Prefix
			var wantSplits4, wantSplits6 []string
			var wantMTUs4, wantMTUs6 []int
			var wantDNSNames4, wantDNSNames6 []string
			var wantDNS4, wantDNS6 [][]string
			if test.v4 {
				wantAddresses4 = []netip.Prefix{netip.MustParsePrefix("10.0.0.2/24")}
				wantSplits4 = []string{"tun0"}
				wantMTUs4 = []int{settings.DefaultMTU}
				wantDNSNames4 = []string{"tun0"}
				wantDNS4 = [][]string{{"9.9.9.9"}}
			}
			if test.v6 {
				wantAddresses6 = []netip.Prefix{netip.MustParsePrefix("fd00::2/64")}
				wantSplits6 = []string{"tun0"}
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

func TestWindowsManagerReturnsConfigurationErrors(t *testing.T) {
	failure := errors.New("configuration failed")
	tests := []struct {
		name string
		fail func(*windowsNetConfigMock, *windowsNetConfigMock)
		run  func(*Manager) error
	}{
		{
			name: "IPv4 address",
			fail: func(v4, _ *windowsNetConfigMock) { v4.setAddressErr = failure },
			run:  (*Manager).assignAddresses,
		},
		{
			name: "IPv6 address",
			fail: func(_, v6 *windowsNetConfigMock) { v6.setAddressErr = failure },
			run:  (*Manager).assignAddresses,
		},
		{
			name: "IPv4 split routes",
			fail: func(v4, _ *windowsNetConfigMock) { v4.addSplitErr = failure },
			run:  (*Manager).addSplitRoutes,
		},
		{
			name: "IPv6 split routes",
			fail: func(_, v6 *windowsNetConfigMock) { v6.addSplitErr = failure },
			run:  (*Manager).addSplitRoutes,
		},
		{
			name: "IPv4 MTU",
			fail: func(v4, _ *windowsNetConfigMock) { v4.setMTUErr = failure },
			run:  (*Manager).setMTU,
		},
		{
			name: "IPv6 MTU",
			fail: func(_, v6 *windowsNetConfigMock) { v6.setMTUErr = failure },
			run:  (*Manager).setMTU,
		},
		{
			name: "IPv4 DNS",
			fail: func(v4, _ *windowsNetConfigMock) { v4.setDNSErr = failure },
			run:  (*Manager).setDNS,
		},
		{
			name: "IPv6 DNS",
			fail: func(_, v6 *windowsNetConfigMock) { v6.setDNSErr = failure },
			run:  (*Manager).setDNS,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, netConfig4, netConfig6 := newWindowsTestManager(t, windowsSettings(true, true))
			test.fail(netConfig4, netConfig6)

			if err := test.run(manager); !errors.Is(err, failure) {
				t.Fatalf("configuration error = %v, want %v", err, failure)
			}
		})
	}
}

func TestWindowsManagerDNSFlushIsBestEffortDuringSetup(t *testing.T) {
	manager, netConfig4, netConfig6 := newWindowsTestManager(t, windowsSettings(true, true))
	netConfig4.flushDNSErr = errors.New("flush IPv4 failed")
	netConfig6.flushDNSErr = errors.New("flush IPv6 failed")

	if err := manager.setDNS(); err != nil {
		t.Fatalf("setDNS() error = %v", err)
	}
	if netConfig4.flushDNSCalls != 1 || netConfig6.flushDNSCalls != 1 {
		t.Fatalf("FlushDNS() calls = IPv4:%d IPv6:%d", netConfig4.flushDNSCalls, netConfig6.flushDNSCalls)
	}
}

func TestWindowsManagerStopsDNSSetupAfterIPv4Failure(t *testing.T) {
	failure := errors.New("IPv4 DNS failed")
	manager, netConfig4, netConfig6 := newWindowsTestManager(t, windowsSettings(true, true))
	netConfig4.setDNSErr = failure

	err := manager.setDNS()
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

func TestWindowsManagerRollsBackPartialDNSSetup(t *testing.T) {
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
			manager, netConfig4, netConfig6 := newWindowsTestManager(t, windowsSettings(true, true))
			netConfig4.setDNSErr = test.cleanupErr
			netConfig4.setDNSErrAt = 2
			netConfig6.setDNSErr = setupErr

			err := manager.setDNS()
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

func TestWindowsManagerRejectsInvalidServerAddress(t *testing.T) {
	manager, _, _ := newWindowsTestManager(t, windowsSettings(true, true))
	if _, err := manager.OpenTunnel(netip.Addr{}); err == nil {
		t.Fatal("OpenTunnel() error = nil")
	}
}

func TestWindowsManagerPinsServerRouteWithMatchingConfigurator(t *testing.T) {
	manager, netConfig4, netConfig6 := newWindowsTestManager(t, windowsSettings(true, true))
	netConfig6.bestRouteIf = "Ethernet6"
	serverAddr := netip.MustParseAddr("2001:db8::1")

	if err := manager.pinServerRoute(serverAddr); err != nil {
		t.Fatalf("pinServerRoute() error = %v", err)
	}
	if len(netConfig4.addedRoutes) != 0 {
		t.Fatalf("IPv4 routes = %v", netConfig4.addedRoutes)
	}
	if want := []string{"2001:db8::1 dev Ethernet6"}; !reflect.DeepEqual(netConfig6.addedRoutes, want) {
		t.Fatalf("IPv6 routes = %v, want %v", netConfig6.addedRoutes, want)
	}
	if manager.pinnedServerAddr != serverAddr || manager.pinnedServerIf != "Ethernet6" {
		t.Fatalf("pinned route = %s@%s", manager.pinnedServerAddr, manager.pinnedServerIf)
	}
}

func TestWindowsManagerPinsServerRouteViaGateway(t *testing.T) {
	manager, netConfig4, _ := newWindowsTestManager(t, windowsSettings(true, false))
	netConfig4.bestRouteGateway = netip.MustParseAddr("192.0.2.1")
	netConfig4.bestRouteIf = "Ethernet0"
	serverAddr := netip.MustParseAddr("198.51.100.1")

	if err := manager.pinServerRoute(serverAddr); err != nil {
		t.Fatalf("pinServerRoute() error = %v", err)
	}
	want := []string{"198.51.100.1 via 192.0.2.1 dev Ethernet0"}
	if !reflect.DeepEqual(netConfig4.addedRoutes, want) {
		t.Fatalf("routes = %v, want %v", netConfig4.addedRoutes, want)
	}
}

func TestWindowsManagerReturnsBestRouteError(t *testing.T) {
	failure := errors.New("route lookup failed")
	manager, netConfig4, _ := newWindowsTestManager(t, windowsSettings(true, false))
	netConfig4.bestRouteErr = failure

	if err := manager.pinServerRoute(netip.MustParseAddr("198.51.100.1")); !errors.Is(err, failure) {
		t.Fatalf("pinServerRoute() error = %v, want %v", err, failure)
	}
	if len(netConfig4.addedRoutes) != 0 || manager.pinnedServerAddr.IsValid() || manager.pinnedServerIf != "" {
		t.Fatal("failed route lookup changed pinned route state")
	}
}

func TestWindowsManagerSkipsUncoveredServerFamily(t *testing.T) {
	manager, netConfig4, netConfig6 := newWindowsTestManager(t, windowsSettings(true, false))
	if err := manager.pinServerRoute(netip.MustParseAddr("2001:db8::1")); err != nil {
		t.Fatalf("pinServerRoute() error = %v", err)
	}
	if len(netConfig4.addedRoutes) != 0 || len(netConfig6.addedRoutes) != 0 || manager.pinnedServerAddr.IsValid() {
		t.Fatal("unexpected pinned route")
	}
}

func TestWindowsManagerDoesNotCacheFailedServerRoute(t *testing.T) {
	manager, netConfig4, _ := newWindowsTestManager(t, windowsSettings(true, false))
	netConfig4.addRouteErr = errors.New("route failed")
	if err := manager.pinServerRoute(netip.MustParseAddr("198.51.100.1")); err == nil {
		t.Fatal("pinServerRoute() error = nil")
	}
	if manager.pinnedServerAddr.IsValid() || manager.pinnedServerIf != "" {
		t.Fatalf("cached route = %s@%s", manager.pinnedServerAddr, manager.pinnedServerIf)
	}
}

func TestWindowsManagerUsesInterfaceIndexWhenAliasIsEmpty(t *testing.T) {
	manager, netConfig4, _ := newWindowsTestManager(t, windowsSettings(true, false))
	netConfig4.bestRouteIndex = 12
	if err := manager.pinServerRoute(netip.MustParseAddr("198.51.100.1")); err != nil {
		t.Fatalf("pinServerRoute() error = %v", err)
	}
	if manager.pinnedServerIf != "12" {
		t.Fatalf("pinnedServerIf = %q", manager.pinnedServerIf)
	}
}

func TestWindowsManagerCloseTunnelRetriesServerRouteCleanup(t *testing.T) {
	deleteErr := errors.New("route cleanup failed")
	manager, netConfig4, _ := newWindowsTestManager(t, windowsSettings(true, false))
	manager.pinnedServerAddr = netip.MustParseAddr("198.51.100.1")
	manager.pinnedServerIf = "Ethernet0"
	netConfig4.deleteRouteErr = deleteErr

	if err := manager.CloseTunnel(); !errors.Is(err, deleteErr) {
		t.Fatalf("CloseTunnel() error = %v, want %v", err, deleteErr)
	}
	if !manager.pinnedServerAddr.IsValid() || manager.pinnedServerIf == "" {
		t.Fatal("failed route cleanup cleared retry state")
	}

	netConfig4.deleteRouteErr = nil
	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("retry CloseTunnel() error = %v", err)
	}
	if manager.pinnedServerAddr.IsValid() || manager.pinnedServerIf != "" {
		t.Fatal("successful route cleanup retained state")
	}
}

func TestWindowsManagerCloseTunnelReturnsAllCleanupErrors(t *testing.T) {
	manager, netConfig4, netConfig6 := newWindowsTestManager(t, windowsSettings(true, true))
	netConfig4.deleteSplitErr = errors.New("split4 failed")
	netConfig4.setDNSErr = errors.New("dns4 failed")
	netConfig4.flushDNSErr = errors.New("flush4 failed")
	netConfig6.deleteSplitErr = errors.New("split6 failed")
	netConfig6.setDNSErr = errors.New("dns6 failed")
	netConfig6.flushDNSErr = errors.New("flush6 failed")
	manager.pinnedServerAddr = netip.MustParseAddr("2001:db8::1")
	manager.pinnedServerIf = "Ethernet6"
	netConfig6.deleteRouteErr = errors.New("route6 failed")
	manager.tun = &windowsTunMock{closeErr: errors.New("TUN close failed")}

	err := manager.CloseTunnel()
	if err == nil {
		t.Fatal("CloseTunnel() error = nil")
	}
	for _, want := range []string{"split4 failed", "dns4 failed", "split6 failed", "dns6 failed", "route6 failed", "TUN close failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("CloseTunnel() error = %v, want %q", err, want)
		}
	}
	for _, ignored := range []string{"flush4 failed", "flush6 failed"} {
		if strings.Contains(err.Error(), ignored) {
			t.Fatalf("CloseTunnel() error = %v, want cache flush failure ignored", err)
		}
	}
	if netConfig4.flushDNSCalls != 1 || netConfig6.flushDNSCalls != 1 {
		t.Fatalf("FlushDNS() calls = IPv4:%d IPv6:%d, want 1 each", netConfig4.flushDNSCalls, netConfig6.flushDNSCalls)
	}
	if manager.tun != nil {
		t.Fatal("CloseTunnel() retained a closed TUN")
	}
}

func TestWindowsManagerCloseTunnelReturnsStaleCleanupError(t *testing.T) {
	active := windowsSettings(true, false)
	manager, _, netConfig6 := newWindowsTestManager(t, active)
	stale := windowsSettings(false, true)
	stale.TunName = "stale6"
	manager.configuration.TCPSettings = stale
	staleErr := errors.New("stale cleanup failed")
	netConfig6.deleteSplitErr = staleErr

	if err := manager.CloseTunnel(); !errors.Is(err, staleErr) {
		t.Fatalf("CloseTunnel() error = %v, want %v", err, staleErr)
	}
}

func TestWindowsManagerCloseTunnelIgnoresMissingStaleInterface(t *testing.T) {
	active := windowsSettings(true, false)
	manager, _, netConfig6 := newWindowsTestManager(t, active)
	stale := windowsSettings(false, true)
	stale.TunName = "stale6"
	manager.configuration.TCPSettings = stale
	missing := fmt.Errorf("%w: %q", ipcfg.ErrInterfaceNotFound, stale.TunName)
	netConfig6.deleteSplitErr = missing
	netConfig6.setDNSErr = missing

	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
}

func TestWindowsManagerCloseTunnelIgnoresMissingActiveInterface(t *testing.T) {
	active := windowsSettings(true, false)
	manager, netConfig4, _ := newWindowsTestManager(t, active)
	missing := fmt.Errorf("%w: %q", ipcfg.ErrInterfaceNotFound, active.TunName)
	netConfig4.deleteSplitErr = missing
	netConfig4.setDNSErr = missing

	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
}

func TestWindowsManagerCloseTunnelIgnoresIncompleteStaleSettings(t *testing.T) {
	manager, _, _ := newWindowsTestManager(t, windowsSettings(true, false))
	manager.configuration.TCPSettings.TunName = "stale"

	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
}

func TestWindowsManagerCloseTunnelCleansStaleSettings(t *testing.T) {
	active := windowsSettings(true, false)
	manager, _, netConfig6 := newWindowsTestManager(t, active)
	stale := windowsSettings(false, true)
	stale.TunName = "stale6"
	manager.configuration.TCPSettings = stale

	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if !reflect.DeepEqual(netConfig6.deletedSplits, []string{"stale6"}) {
		t.Fatalf("stale split cleanup = %v", netConfig6.deletedSplits)
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
