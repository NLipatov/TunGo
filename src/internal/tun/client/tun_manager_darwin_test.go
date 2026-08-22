//go:build darwin

package client

import (
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"

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
	added        []string
	addedSplit   []string
	deleted      []string
	deletedSplit []string
	addErr       error
	splitErr     error
	delErr       error
	delSplitErr  error
}

func (m *darwinRouteMock) Add(destination string) error {
	m.added = append(m.added, destination)
	return m.addErr
}

func (m *darwinRouteMock) AddSplit(ifName string) error {
	m.addedSplit = append(m.addedSplit, ifName)
	return m.splitErr
}

func (m *darwinRouteMock) DelSplit(ifName string) error {
	m.deletedSplit = append(m.deletedSplit, ifName)
	return m.delSplitErr
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
	}
	if v6 {
		active.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
		active.IPv6 = netip.MustParseAddr("fd00::2")
	}
	return active
}

func newDarwinTestManager(t *testing.T, active settings.Settings) (*Manager, *darwinIfconfigMock, *darwinIfconfigMock, *darwinRouteMock, *darwinRouteMock) {
	t.Helper()
	ifconfig4 := &darwinIfconfigMock{}
	ifconfig6 := &darwinIfconfigMock{}
	route4 := &darwinRouteMock{}
	route6 := &darwinRouteMock{}
	manager := &Manager{
		settings:  active,
		ifconfig4: ifconfig4,
		ifconfig6: ifconfig6,
		route4:    route4,
		route6:    route6,
	}
	return manager, ifconfig4, ifconfig6, route4, route6
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
			if err := manager.addSplitRoutes(); err != nil {
				t.Fatalf("addSplitRoutes() error = %v", err)
			}

			var wantAddresses4, wantAddresses6 []netip.Prefix
			var wantSplits4, wantSplits6 []string
			var wantMTUs4, wantMTUs6 []int
			if test.v4 {
				wantAddresses4 = []netip.Prefix{netip.MustParsePrefix("10.0.0.2/32")}
				wantSplits4 = []string{"utun42"}
				wantMTUs4 = []int{1400}
			}
			if test.v6 {
				wantAddresses6 = []netip.Prefix{netip.MustParsePrefix("fd00::2/64")}
				wantSplits6 = []string{"utun42"}
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
			if !reflect.DeepEqual(ifconfig4.mtus, wantMTUs4) {
				t.Fatalf("IPv4 MTUs = %v, want %v", ifconfig4.mtus, wantMTUs4)
			}
			if !reflect.DeepEqual(ifconfig6.mtus, wantMTUs6) {
				t.Fatalf("IPv6 MTUs = %v, want %v", ifconfig6.mtus, wantMTUs6)
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

func TestDarwinManagerCloseTunnelCleansEnabledFamilies(t *testing.T) {
	manager, _, _, route4, route6 := newDarwinTestManager(t, darwinSettings(true, true))
	tun := &tunMock{name: "utun42"}
	manager.tun = tun
	manager.pinnedServerAddr = netip.MustParseAddr("2001:db8::1")

	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if len(route4.deletedSplit) != 1 || len(route6.deletedSplit) != 1 {
		t.Fatalf("split cleanup: IPv4=%v IPv6=%v", route4.deletedSplit, route6.deletedSplit)
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
}

func TestDarwinManagerCloseTunnelReturnsAllCleanupErrors(t *testing.T) {
	manager, _, _, route4, route6 := newDarwinTestManager(t, darwinSettings(true, true))
	route4.delSplitErr = errors.New("split4 failed")
	route6.delSplitErr = errors.New("split6 failed")
	route6.delErr = errors.New("route6 failed")
	manager.pinnedServerAddr = netip.MustParseAddr("2001:db8::1")
	manager.tun = &tunMock{name: "utun42", closeErr: errors.New("TUN close failed")}

	err := manager.CloseTunnel()
	if err == nil {
		t.Fatal("CloseTunnel() error = nil")
	}
	for _, want := range []string{"split4 failed", "split6 failed", "route6 failed", "TUN close failed"} {
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

	route6.delErr = nil
	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("retry CloseTunnel() error = %v", err)
	}
	if manager.pinnedServerAddr.IsValid() {
		t.Fatal("successful route cleanup retained retry state")
	}
}
