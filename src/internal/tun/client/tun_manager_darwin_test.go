//go:build darwin

package client

import (
	"errors"
	"net/netip"
	"reflect"
	"slices"
	"strings"
	"testing"

	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
)

type darwinIfconfigMock struct {
	addresses []netip.Prefix
	mtus      []int
	addrErr   error
	mtuErr    error
}

func (m *darwinIfconfigMock) LinkAddrAdd(_ string, prefix netip.Prefix) error {
	m.addresses = append(m.addresses, prefix)
	return m.addrErr
}

func (m *darwinIfconfigMock) SetMTU(_ string, mtu int) error {
	m.mtus = append(m.mtus, mtu)
	return m.mtuErr
}

type darwinRouteMock struct {
	added         []string
	addedSplit    []string
	addedPrefixes [][]string
	deleted       []string
	addErr        error
	splitErr      error
	delErr        error
}

type darwinDNSMock struct {
	setResolvers [][]string
	revertCalls  int
	setErr       error
	revertErr    error
}

func (m *darwinDNSMock) Set(resolvers []string) error {
	m.setResolvers = append(m.setResolvers, append([]string(nil), resolvers...))
	return m.setErr
}

func (m *darwinDNSMock) Revert() error {
	m.revertCalls++
	return m.revertErr
}

func (m *darwinRouteMock) Add(destination string) error {
	m.added = append(m.added, destination)
	return m.addErr
}

func (m *darwinRouteMock) AddSplit(ifName string, prefixes []string) error {
	m.addedSplit = append(m.addedSplit, ifName)
	m.addedPrefixes = append(m.addedPrefixes, slices.Clone(prefixes))
	return m.splitErr
}

func (m *darwinRouteMock) Del(destination string) error {
	m.deleted = append(m.deleted, destination)
	return m.delErr
}

type tunMock struct {
	name       string
	closeErr   error
	closeCalls int
}

func (*tunMock) Read([]byte) (int, error)    { return 0, nil }
func (*tunMock) Write(p []byte) (int, error) { return len(p), nil }
func (m *tunMock) Name() string              { return m.name }
func (m *tunMock) Close() error {
	m.closeCalls++
	return m.closeErr
}

func darwinSettings(v4, v6 bool) settings.Settings {
	active := settings.Settings{MTU: 1400}
	if v4 {
		active.IPv4Subnet = netip.MustParsePrefix("10.0.0.0/24")
		active.IPv4 = netip.MustParseAddr("10.0.0.2")
		active.DNSv4 = []string{"1.1.1.1", "8.8.8.8"}
	}
	if v6 {
		active.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
		active.IPv6 = netip.MustParseAddr("fd00::2")
		active.DNSv6 = []string{"2606:4700:4700::1111", "2001:4860:4860::8888"}
	}
	return active
}

func newDarwinTestManager(t *testing.T, active settings.Settings) (*Manager, *darwinIfconfigMock, *darwinIfconfigMock, *darwinRouteMock, *darwinRouteMock) {
	t.Helper()
	ifconfig4 := &darwinIfconfigMock{}
	ifconfig6 := &darwinIfconfigMock{}
	route4 := &darwinRouteMock{}
	route6 := &darwinRouteMock{}
	manager, err := New(&clientconfig.Configuration{
		ClientID:    1,
		Protocol:    settings.UDP,
		UDPSettings: active,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	manager.dns = &darwinDNSMock{}
	manager.ifconfig4, manager.ifconfig6 = ifconfig4, ifconfig6
	manager.route4, manager.route6 = route4, route6
	return manager, ifconfig4, ifconfig6, route4, route6
}

func TestDarwinManagerConfiguresDNSForEnabledFamilies(t *testing.T) {
	for _, test := range []struct {
		name string
		v4   bool
		v6   bool
		want []string
	}{
		{name: "IPv4", v4: true, want: []string{"1.1.1.1", "8.8.8.8"}},
		{name: "IPv6", v6: true, want: []string{"2606:4700:4700::1111", "2001:4860:4860::8888"}},
		{
			name: "dual stack",
			v4:   true,
			v6:   true,
			want: []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111", "2001:4860:4860::8888"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager, _, _, _, _ := newDarwinTestManager(t, darwinSettings(test.v4, test.v6))
			dnsMock := &darwinDNSMock{}
			manager.dns = dnsMock
			manager.tun = &tunMock{name: "utun42"}

			if err := manager.setDNS(); err != nil {
				t.Fatalf("setDNS() error = %v", err)
			}
			if !reflect.DeepEqual(dnsMock.setResolvers, [][]string{test.want}) {
				t.Fatalf("DNS resolvers = %v, want %v", dnsMock.setResolvers, test.want)
			}
		})
	}
}

func TestDarwinManagerReturnsDNSConfigurationError(t *testing.T) {
	failure := errors.New("DNS configuration failed")
	manager, _, _, _, _ := newDarwinTestManager(t, darwinSettings(true, false))
	dnsMock := &darwinDNSMock{setErr: failure}
	manager.dns = dnsMock

	err := manager.setDNS()
	if !errors.Is(err, failure) {
		t.Fatalf("setDNS() error = %v, want %v", err, failure)
	}
	if !strings.Contains(err.Error(), "set DNS") {
		t.Fatalf("setDNS() error = %q, want operation context", err)
	}
	if !reflect.DeepEqual(dnsMock.setResolvers, [][]string{{"1.1.1.1", "8.8.8.8"}}) {
		t.Fatalf("DNS resolvers = %v, want configured IPv4 resolvers", dnsMock.setResolvers)
	}
}

func TestDarwinManagerAppliesTunnelRoutes(t *testing.T) {
	for _, test := range []struct {
		name   string
		v4     []string
		v6     []string
		wantV4 []string
		wantV6 []string
	}{
		{
			name:   "custom prefixes",
			v4:     []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"},
			v6:     []string{"2001:db8:1::/64"},
			wantV4: []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "10.0.0.0/24"},
			wantV6: []string{"2001:db8:1::/64"},
		},
		{name: "empty IPv4", v4: []string{}, v6: []string{"2001:db8:1::/64"}, wantV4: []string{"10.0.0.0/24"}, wantV6: []string{"2001:db8:1::/64"}},
		{name: "empty IPv6", v4: []string{"192.0.2.0/24"}, v6: []string{}, wantV4: []string{"192.0.2.0/24", "10.0.0.0/24"}},
		{name: "both empty", v4: []string{}, v6: []string{}, wantV4: []string{"10.0.0.0/24"}},
		{
			name:   "full tunnel",
			v4:     []string{"0.0.0.0/1", "128.0.0.0/1"},
			v6:     []string{"::/1", "8000::/1"},
			wantV4: []string{"0.0.0.0/1", "128.0.0.0/1", "10.0.0.0/24"},
			wantV6: []string{"::/1", "8000::/1"},
		},
		{
			name:   "TUN subnets preserve broader and narrower routes",
			v4:     []string{"10.0.0.0/16", "10.0.0.0/24", "10.0.0.0/25", "192.0.2.0/24"},
			v6:     []string{"fd00::/48", "fd00::/64", "fd00::/80", "2001:db8::/64"},
			wantV4: []string{"10.0.0.0/16", "10.0.0.0/24", "10.0.0.0/25", "192.0.2.0/24"},
			wantV6: []string{"fd00::/48", "fd00::/80", "2001:db8::/64"},
		},
		{name: "only TUN subnets", v4: []string{"10.0.0.0/24"}, v6: []string{"fd00::/64"}, wantV4: []string{"10.0.0.0/24"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalV4, originalV6 := slices.Clone(test.v4), slices.Clone(test.v6)
			active := darwinSettings(true, true)
			active.IPv4Subnet = netip.MustParsePrefix("10.0.0.17/24")
			active.IPv6Subnet = netip.MustParsePrefix("fd00::17/64")
			configuration := &clientconfig.Configuration{
				ClientID:       1,
				Protocol:       settings.UDP,
				UDPSettings:    active,
				TunnelRoutesV4: test.v4,
				TunnelRoutesV6: test.v6,
			}
			manager, err := New(configuration)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			route4, route6 := &darwinRouteMock{}, &darwinRouteMock{}
			manager.route4, manager.route6 = route4, route6
			manager.ifconfig4, manager.ifconfig6 = &darwinIfconfigMock{}, &darwinIfconfigMock{}
			manager.dns = &darwinDNSMock{}
			tun := &tunMock{name: "utun42"}
			manager.tun = tun
			if err := manager.assignAddresses(); err != nil {
				t.Fatalf("assignAddresses() error = %v", err)
			}
			if err := manager.addSplitRoutes(netip.MustParseAddr("198.51.100.1")); err != nil {
				t.Fatalf("addSplitRoutes() error = %v", err)
			}
			if err := manager.CloseTunnel(); err != nil {
				t.Fatalf("CloseTunnel() error = %v", err)
			}
			if tun.closeCalls != 1 || manager.tun != nil {
				t.Fatalf("close state: calls=%d tun=%v", tun.closeCalls, manager.tun)
			}

			if !slices.Equal(configuration.TunnelRoutesV4, originalV4) ||
				!slices.Equal(configuration.TunnelRoutesV6, originalV6) {
				t.Fatalf("manager changed configured TunnelRoutes: IPv4=%v IPv6=%v", configuration.TunnelRoutesV4, configuration.TunnelRoutesV6)
			}

			for _, check := range []struct {
				name  string
				calls [][]string
				want  [][]string
			}{
				{name: "add IPv4", calls: route4.addedPrefixes, want: [][]string{test.wantV4}},
				{name: "add IPv6", calls: route6.addedPrefixes, want: [][]string{test.wantV6}},
			} {
				if len(check.calls) != len(check.want) {
					t.Fatalf("%s calls = %v, want %v", check.name, check.calls, check.want)
				}
				for i, prefixes := range check.calls {
					if !slices.Equal(prefixes, check.want[i]) {
						t.Errorf("%s call %d prefixes = %v, want %v", check.name, i, prefixes, check.want[i])
					}
				}
			}
		})
	}
}

func TestDarwinManagerFiltersRoutesForCurrentServer(t *testing.T) {
	routesV4 := []string{"128.0.0.0/1", "198.51.100.1/32"}
	routesV6 := []string{"::/1", "fd00::/64", "2001:db8::1/128"}
	manager, _, _, route4, route6 := newDarwinTestManager(t, darwinSettings(true, true))
	manager.splitsv4, manager.splitsv6 = slices.Clone(routesV4), slices.Clone(routesV6)
	manager.tun = &tunMock{name: "utun42"}

	for _, test := range []struct {
		server string
		wantV4 []string
		wantV6 []string
	}{
		{
			server: "198.51.100.1",
			wantV4: []string{"128.0.0.0/1", "10.0.0.0/24"},
			wantV6: []string{"::/1", "2001:db8::1/128"},
		},
		{
			server: "2001:db8::1",
			wantV4: []string{"128.0.0.0/1", "198.51.100.1/32", "10.0.0.0/24"},
			wantV6: []string{"::/1"},
		},
	} {
		t.Run(test.server, func(t *testing.T) {
			route4.addedPrefixes, route6.addedPrefixes = nil, nil
			if err := manager.addSplitRoutes(netip.MustParseAddr(test.server)); err != nil {
				t.Fatalf("addSplitRoutes() error = %v", err)
			}
			if !reflect.DeepEqual(route4.addedPrefixes, [][]string{test.wantV4}) ||
				!reflect.DeepEqual(route6.addedPrefixes, [][]string{test.wantV6}) {
				t.Errorf("installed routes: IPv4=%v IPv6=%v, want IPv4=%v IPv6=%v",
					route4.addedPrefixes, route6.addedPrefixes, test.wantV4, test.wantV6)
			}
			if !slices.Equal(manager.splitsv4, routesV4) || !slices.Equal(manager.splitsv6, routesV6) {
				t.Errorf("manager changed stored routes: IPv4=%v IPv6=%v", manager.splitsv4, manager.splitsv6)
			}
		})
	}
}

func TestDarwinManagerConfiguresEveryAddressMode(t *testing.T) {
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
			manager, ifconfig4, ifconfig6, route4, route6 := newDarwinTestManager(t, darwinSettings(test.v4, test.v6))
			manager.tun = &tunMock{name: "utun42"}

			if err := manager.setMTU(); err != nil {
				t.Fatalf("setMTU() error = %v", err)
			}
			if err := manager.assignAddresses(); err != nil {
				t.Fatalf("assignAddresses() error = %v", err)
			}
			manager.splitsv4 = []string{"192.0.2.0/24"}
			manager.splitsv6 = []string{"2001:db8::/64"}
			if err := manager.addSplitRoutes(netip.MustParseAddr("198.51.100.1")); err != nil {
				t.Fatalf("addSplitRoutes() error = %v", err)
			}

			var wantAddresses4, wantAddresses6 []netip.Prefix
			var wantSplits4, wantSplits6 []string
			var wantPrefixes4, wantPrefixes6 [][]string
			var wantMTUs4, wantMTUs6 []int
			if test.v4 {
				wantAddresses4 = []netip.Prefix{netip.MustParsePrefix("10.0.0.2/24")}
				wantSplits4 = []string{"utun42"}
				wantPrefixes4 = [][]string{{"192.0.2.0/24", "10.0.0.0/24"}}
				wantMTUs4 = []int{1400}
			}
			if test.v6 {
				wantAddresses6 = []netip.Prefix{netip.MustParsePrefix("fd00::2/64")}
				wantSplits6 = []string{"utun42"}
				wantPrefixes6 = [][]string{{"2001:db8::/64"}}
				if !test.v4 {
					wantMTUs6 = []int{1400}
				}
			}

			if !reflect.DeepEqual(ifconfig4.addresses, wantAddresses4) {
				t.Fatalf("IPv4 addresses = %v, want %v", ifconfig4.addresses, wantAddresses4)
			}
			if !reflect.DeepEqual(ifconfig6.addresses, wantAddresses6) {
				t.Fatalf("IPv6 addresses = %v, want %v", ifconfig6.addresses, wantAddresses6)
			}
			if !reflect.DeepEqual(route4.addedSplit, wantSplits4) {
				t.Fatalf("IPv4 split interfaces = %v, want %v", route4.addedSplit, wantSplits4)
			}
			if !reflect.DeepEqual(route6.addedSplit, wantSplits6) {
				t.Fatalf("IPv6 split interfaces = %v, want %v", route6.addedSplit, wantSplits6)
			}
			if !reflect.DeepEqual(route4.addedPrefixes, wantPrefixes4) {
				t.Fatalf("IPv4 split prefixes = %v, want %v", route4.addedPrefixes, wantPrefixes4)
			}
			if !reflect.DeepEqual(route6.addedPrefixes, wantPrefixes6) {
				t.Fatalf("IPv6 split prefixes = %v, want %v", route6.addedPrefixes, wantPrefixes6)
			}
			if !reflect.DeepEqual(ifconfig4.mtus, wantMTUs4) {
				t.Fatalf("IPv4 MTUs = %v, want %v", ifconfig4.mtus, wantMTUs4)
			}
			if !reflect.DeepEqual(ifconfig6.mtus, wantMTUs6) {
				t.Fatalf("IPv6 MTUs = %v, want %v", ifconfig6.mtus, wantMTUs6)
			}
		})
	}
}

func TestDarwinManagerReturnsConfigurationErrors(t *testing.T) {
	failure := errors.New("configuration failed")
	tests := []struct {
		name   string
		active settings.Settings
		fail   func(*darwinIfconfigMock, *darwinIfconfigMock, *darwinRouteMock, *darwinRouteMock)
		run    func(*Manager) error
	}{
		{
			name:   "IPv4 MTU",
			active: darwinSettings(true, false),
			fail:   func(v4, _ *darwinIfconfigMock, _, _ *darwinRouteMock) { v4.mtuErr = failure },
			run:    (*Manager).setMTU,
		},
		{
			name:   "IPv6 MTU",
			active: darwinSettings(false, true),
			fail:   func(_, v6 *darwinIfconfigMock, _, _ *darwinRouteMock) { v6.mtuErr = failure },
			run:    (*Manager).setMTU,
		},
		{
			name:   "IPv4 address",
			active: darwinSettings(true, true),
			fail:   func(v4, _ *darwinIfconfigMock, _, _ *darwinRouteMock) { v4.addrErr = failure },
			run:    (*Manager).assignAddresses,
		},
		{
			name:   "IPv6 address",
			active: darwinSettings(true, true),
			fail:   func(_, v6 *darwinIfconfigMock, _, _ *darwinRouteMock) { v6.addrErr = failure },
			run:    (*Manager).assignAddresses,
		},
		{
			name:   "IPv4 split routes",
			active: darwinSettings(true, true),
			fail:   func(_, _ *darwinIfconfigMock, v4, _ *darwinRouteMock) { v4.splitErr = failure },
			run: func(m *Manager) error {
				return m.addSplitRoutes(netip.MustParseAddr("198.51.100.1"))
			},
		},
		{
			name:   "IPv6 split routes",
			active: darwinSettings(true, true),
			fail:   func(_, _ *darwinIfconfigMock, _, v6 *darwinRouteMock) { v6.splitErr = failure },
			run: func(m *Manager) error {
				return m.addSplitRoutes(netip.MustParseAddr("198.51.100.1"))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager, ifconfig4, ifconfig6, route4, route6 := newDarwinTestManager(t, test.active)
			manager.tun = &tunMock{name: "utun42"}
			test.fail(ifconfig4, ifconfig6, route4, route6)

			if err := test.run(manager); !errors.Is(err, failure) {
				t.Fatalf("configuration error = %v, want %v", err, failure)
			}
		})
	}
}

func TestDarwinManagerRejectsInvalidServerAddress(t *testing.T) {
	manager, _, _, _, _ := newDarwinTestManager(t, darwinSettings(true, true))
	if _, err := manager.OpenTunnel(netip.Addr{}); err == nil {
		t.Fatal("OpenTunnel() error = nil")
	}
}

func TestDarwinManagerPinsOnlyCoveredServerFamily(t *testing.T) {
	t.Run("IPv4 tunnel skips IPv6 server", func(t *testing.T) {
		manager, _, _, route4, route6 := newDarwinTestManager(t, darwinSettings(true, false))
		if err := manager.pinServerRoute(netip.MustParseAddr("2001:db8::1")); err != nil {
			t.Fatalf("pinServerRoute() error = %v", err)
		}
		if len(route4.added) != 0 || len(route6.added) != 0 || manager.pinnedServerAddr.IsValid() {
			t.Fatal("unexpected pinned route")
		}
	})

	t.Run("dual stack pins IPv6 server", func(t *testing.T) {
		manager, _, _, route4, route6 := newDarwinTestManager(t, darwinSettings(true, true))
		serverAddr := netip.MustParseAddr("2001:db8::1")
		if err := manager.pinServerRoute(serverAddr); err != nil {
			t.Fatalf("pinServerRoute() error = %v", err)
		}
		if len(route4.added) != 0 || len(route6.added) != 1 || manager.pinnedServerAddr != serverAddr {
			t.Fatalf("routes: IPv4=%v IPv6=%v pinned=%s", route4.added, route6.added, manager.pinnedServerAddr)
		}
	})
}

func TestDarwinManagerDoesNotCacheFailedServerRoute(t *testing.T) {
	manager, _, _, route4, _ := newDarwinTestManager(t, darwinSettings(true, false))
	route4.addErr = errors.New("route failed")
	if err := manager.pinServerRoute(netip.MustParseAddr("198.51.100.1")); err == nil {
		t.Fatal("pinServerRoute() error = nil")
	}
	if manager.pinnedServerAddr.IsValid() {
		t.Fatalf("pinnedServerAddr = %s", manager.pinnedServerAddr)
	}
}

func TestDarwinManagerCloseTunnelUsesInterfaceCleanup(t *testing.T) {
	manager, _, _, route4, route6 := newDarwinTestManager(t, darwinSettings(true, true))
	tun := &tunMock{name: "utun42"}
	manager.tun = tun
	manager.pinnedServerAddr = netip.MustParseAddr("2001:db8::1")

	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if len(route4.deleted) != 0 || len(route6.deleted) != 1 {
		t.Fatalf("route cleanup: IPv4=%v IPv6=%v", route4.deleted, route6.deleted)
	}
	if tun.closeCalls != 1 || manager.tun != nil || manager.pinnedServerAddr.IsValid() {
		t.Fatalf("close state: calls=%d tun=%v pinned=%s", tun.closeCalls, manager.tun, manager.pinnedServerAddr)
	}
	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("second CloseTunnel() error = %v", err)
	}
	if tun.closeCalls != 1 || len(route6.deleted) != 1 {
		t.Fatalf("repeated cleanup: close calls=%d deleted routes=%v", tun.closeCalls, route6.deleted)
	}
}

func TestDarwinManagerClosesTunAfterIPv6RouteFailure(t *testing.T) {
	active := darwinSettings(true, true)
	active.IPv4 = netip.MustParseAddr("10.8.0.2")
	active.IPv4Subnet = netip.MustParsePrefix("10.8.0.0/20")
	manager, ifconfig4, _, route4, route6 := newDarwinTestManager(t, active)
	manager.splitsv6 = []string{"2001:db8::/64"}
	failure := errors.New("IPv6 route failed")
	route6.splitErr = failure
	tun := &tunMock{name: "utun42"}
	manager.tun = tun

	if err := manager.assignAddresses(); err != nil {
		t.Fatalf("assignAddresses() error = %v", err)
	}
	if len(route4.addedPrefixes) != 0 || len(route6.addedPrefixes) != 0 {
		t.Fatal("address assignment installed split routes")
	}
	if err := manager.addSplitRoutes(netip.MustParseAddr("198.51.100.1")); !errors.Is(err, failure) {
		t.Fatalf("addSplitRoutes() error = %v, want %v", err, failure)
	}
	wantAddress := []netip.Prefix{netip.MustParsePrefix("10.8.0.2/20")}
	if !slices.Equal(ifconfig4.addresses, wantAddress) {
		t.Fatalf("IPv4 addresses = %v, want %v", ifconfig4.addresses, wantAddress)
	}
	if !reflect.DeepEqual(route4.addedPrefixes, [][]string{{"10.8.0.0/20"}}) {
		t.Fatalf("IPv4 routes = %v, want subnet route before IPv6 failure", route4.addedPrefixes)
	}

	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if tun.closeCalls != 1 || manager.tun != nil {
		t.Fatalf("close state: calls=%d tun=%v", tun.closeCalls, manager.tun)
	}
}

func TestDarwinManagerCloseTunnelReturnsAllCleanupErrors(t *testing.T) {
	manager, _, _, route4, _ := newDarwinTestManager(t, darwinSettings(true, true))
	dnsMock := &darwinDNSMock{revertErr: errors.New("DNS restore failed")}
	manager.dns = dnsMock
	route4.delErr = errors.New("route4 failed")
	manager.pinnedServerAddr = netip.MustParseAddr("198.51.100.1")
	manager.tun = &tunMock{name: "utun42", closeErr: errors.New("TUN close failed")}

	err := manager.CloseTunnel()
	if err == nil {
		t.Fatal("CloseTunnel() error = nil")
	}
	for _, want := range []string{"DNS restore failed", "route4 failed", "TUN close failed"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("CloseTunnel() error = %v, want %q", err, want)
		}
	}
	if manager.tun != nil {
		t.Fatal("CloseTunnel() retained a closed TUN")
	}
	if !manager.pinnedServerAddr.IsValid() {
		t.Fatal("failed route cleanup cleared retry state")
	}

	route4.delErr = nil
	dnsMock.revertErr = nil
	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("retry CloseTunnel() error = %v", err)
	}
	if manager.pinnedServerAddr.IsValid() {
		t.Fatal("successful route cleanup retained retry state")
	}
}

func TestDarwinManagerCloseTunnelRetriesDNSRestore(t *testing.T) {
	manager, _, _, _, _ := newDarwinTestManager(t, darwinSettings(true, false))
	dnsMock := &darwinDNSMock{revertErr: errors.New("DNS restore failed")}
	manager.dns = dnsMock
	tun := &tunMock{name: "utun42"}
	manager.tun = tun

	if err := manager.CloseTunnel(); err == nil || !strings.Contains(err.Error(), "DNS restore failed") {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if manager.tun != nil || tun.closeCalls != 1 || dnsMock.revertCalls != 1 {
		t.Fatalf("failed restore state: TUN=%v close calls=%d Revert calls=%d", manager.tun, tun.closeCalls, dnsMock.revertCalls)
	}

	dnsMock.revertErr = nil
	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("retry CloseTunnel() error = %v", err)
	}
	if dnsMock.revertCalls != 2 {
		t.Fatalf("DNS Revert() calls = %d, want 2", dnsMock.revertCalls)
	}
	if tun.closeCalls != 1 {
		t.Fatalf("TUN close calls = %d, want 1", tun.closeCalls)
	}
}
