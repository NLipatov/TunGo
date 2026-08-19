//go:build windows

package manager

import (
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"tungo/internal/config/settings"
)

type dualStackTunMock struct {
	closeErr error
}

func (d *dualStackTunMock) Read([]byte) (int, error)    { return 0, nil }
func (d *dualStackTunMock) Write(p []byte) (int, error) { return len(p), nil }
func (d *dualStackTunMock) Close() error {
	return d.closeErr
}

type dualStackNetCfgMock struct {
	bestRouteIf  string
	bestRouteIdx int
	emptyRouteIf bool
	bestRouteErr error
	setDNSErr    error
	flushDNSErr  error
	delSplitErr  error
	delRouteErr  error

	setDNSCalls   int
	flushDNSCalls int
	setDNSValues  [][]string

	deletedRoutes []string
}

func (m *dualStackNetCfgMock) FlushDNS() error {
	m.flushDNSCalls++
	return m.flushDNSErr
}
func (m *dualStackNetCfgMock) SetAddressStatic(_ string, _ netip.Prefix) error { return nil }
func (m *dualStackNetCfgMock) SetDNS(_ string, dnsServers []string) error {
	m.setDNSCalls++
	m.setDNSValues = append(m.setDNSValues, append([]string(nil), dnsServers...))
	return m.setDNSErr
}
func (m *dualStackNetCfgMock) SetMTU(_ string, _ int) error { return nil }
func (m *dualStackNetCfgMock) AddHostRouteViaGateway(_ netip.Addr, _ string, _ netip.Addr, _ int) error {
	return nil
}
func (m *dualStackNetCfgMock) AddHostRouteOnLink(_ netip.Addr, _ string, _ int) error {
	return nil
}
func (m *dualStackNetCfgMock) AddDefaultSplitRoutes(_ string, _ int) error { return nil }
func (m *dualStackNetCfgMock) DeleteDefaultSplitRoutes(_ string) error     { return m.delSplitErr }
func (m *dualStackNetCfgMock) DeleteRoute(destination netip.Addr) error {
	m.deletedRoutes = append(m.deletedRoutes, destination.String())
	return m.delRouteErr
}
func (m *dualStackNetCfgMock) DeleteRouteOnInterface(destination netip.Addr, ifName string) error {
	m.deletedRoutes = append(m.deletedRoutes, destination.String()+"@"+ifName)
	return m.delRouteErr
}
func (m *dualStackNetCfgMock) BestRoute(netip.Addr) (netip.Addr, string, int, int, error) {
	if m.bestRouteErr != nil {
		return netip.Addr{}, "", 0, 0, m.bestRouteErr
	}
	iface := m.bestRouteIf
	if iface == "" && !m.emptyRouteIf {
		iface = "Ethernet0"
	}
	idx := m.bestRouteIdx
	if idx <= 0 {
		idx = 1
	}
	return netip.Addr{}, iface, idx, 1, nil
}

func TestDualStackManager_AddStaticRouteToServer_UsesIPv6ServerAddr(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{}
	cfg6 := &dualStackNetCfgMock{bestRouteIf: "eth0"}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "2001:db8::1"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)

	if err := m.addStaticRouteToServer(netip.MustParseAddr("2001:db8::1")); err != nil {
		t.Fatalf("expected IPv6 host route, got %v", err)
	}
	if m.resolvedRouteIP4.IsValid() || m.resolvedRouteIP6 != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("unexpected cached routes: v4=%q v6=%q", m.resolvedRouteIP4, m.resolvedRouteIP6)
	}
}

func TestDualStackManager_AddStaticRouteToServer_UsesIPv4ServerAddr(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	cfg6 := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "198.51.100.10"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)

	if err := m.addStaticRouteToServer(netip.MustParseAddr("198.51.100.10")); err != nil {
		t.Fatalf("expected IPv4 host route, got %v", err)
	}
	if m.resolvedRouteIP4 != netip.MustParseAddr("198.51.100.10") || m.resolvedRouteIP6.IsValid() {
		t.Fatalf("unexpected cached routes: v4=%q v6=%q", m.resolvedRouteIP4, m.resolvedRouteIP6)
	}
}

func TestDualStackManager_AddStaticRouteToServer_PropagatesIPv4RouteError(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{bestRouteErr: errors.New("boom4")}
	cfg6 := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "198.51.100.10"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)

	err := m.addStaticRouteToServer(netip.MustParseAddr("198.51.100.10"))
	if err == nil || !strings.Contains(err.Error(), "boom4") {
		t.Fatalf("expected wrapped ipv4 route error, got %v", err)
	}
}

func TestDualStackManager_AddStaticRouteToServer_PropagatesIPv6RouteError(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{}
	cfg6 := &dualStackNetCfgMock{bestRouteErr: errors.New("boom6")}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "2001:db8::1"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)

	err := m.addStaticRouteToServer(netip.MustParseAddr("2001:db8::1"))
	if err == nil || !strings.Contains(err.Error(), "boom6") {
		t.Fatalf("expected wrapped ipv6 route error, got %v", err)
	}
}

func TestDualStackManager_AddStaticRoute_UsesPassedIPv4(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	cfg6 := &dualStackNetCfgMock{bestRouteIf: "eth0"}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "vpn.example.com"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)

	if err := m.addStaticRouteToServer(netip.MustParseAddr("198.51.100.77")); err != nil {
		t.Fatalf("expected IPv4 route from passed address, got %v", err)
	}
	if m.resolvedRouteIP4 != netip.MustParseAddr("198.51.100.77") {
		t.Fatalf("unexpected IPv4 route target: %s", m.resolvedRouteIP4)
	}
	if m.resolvedRouteIf4 != "eth0" {
		t.Fatalf("unexpected IPv4 route interface: %s", m.resolvedRouteIf4)
	}
	if m.resolvedRouteIP6.IsValid() {
		t.Fatalf("unexpected IPv6 route target: %s", m.resolvedRouteIP6)
	}
}

func TestDualStackManager_AddStaticRoute_UsesPassedIPv6(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	cfg6 := &dualStackNetCfgMock{bestRouteIf: "eth0"}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "vpn.example.com"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)

	if err := m.addStaticRouteToServer(netip.MustParseAddr("2001:db8::1")); err != nil {
		t.Fatalf("expected IPv6 route from passed address, got %v", err)
	}
	if m.resolvedRouteIP4.IsValid() || m.resolvedRouteIP6 != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("unexpected resolved routes: v4=%s v6=%s", m.resolvedRouteIP4, m.resolvedRouteIP6)
	}
	if m.resolvedRouteIf4 != "" || m.resolvedRouteIf6 != "eth0" {
		t.Fatalf("unexpected resolved route interfaces: v4=%s v6=%s", m.resolvedRouteIf4, m.resolvedRouteIf6)
	}
}

func TestDualStackManager_CloseTunnel_ClearsRouteStatePerFamily(t *testing.T) {
	err4 := errors.New("IPv4 route cleanup fail")
	err6 := errors.New("IPv6 route cleanup fail")
	tests := []struct {
		name    string
		delErr4 error
		delErr6 error
		wantIP4 netip.Addr
		wantIP6 netip.Addr
		wantIf4 string
		wantIf6 string
	}{
		{name: "both deleted"},
		{
			name:    "IPv4 delete fails",
			delErr4: err4,
			wantIP4: netip.MustParseAddr("198.51.100.10"),
			wantIf4: "eth0",
		},
		{
			name:    "IPv6 delete fails",
			delErr6: err6,
			wantIP6: netip.MustParseAddr("2001:db8::1"),
			wantIf6: "eth0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg4 := &dualStackNetCfgMock{delRouteErr: tt.delErr4}
			cfg6 := &dualStackNetCfgMock{delRouteErr: tt.delErr6}
			m := newDualStackManager(settings.Settings{
				Addressing: settings.Addressing{TunName: "tun0"},
			}, cfg4, cfg6)
			m.resolvedRouteIP4 = netip.MustParseAddr("198.51.100.10")
			m.resolvedRouteIf4 = "eth0"
			m.resolvedRouteIP6 = netip.MustParseAddr("2001:db8::1")
			m.resolvedRouteIf6 = "eth0"

			err := m.CloseTunnel()
			if tt.delErr4 != nil && !errors.Is(err, tt.delErr4) {
				t.Fatalf("CloseTunnel() error = %v, want IPv4 error %v", err, tt.delErr4)
			}
			if tt.delErr6 != nil && !errors.Is(err, tt.delErr6) {
				t.Fatalf("CloseTunnel() error = %v, want IPv6 error %v", err, tt.delErr6)
			}
			if tt.delErr4 == nil && tt.delErr6 == nil && err != nil {
				t.Fatalf("CloseTunnel() error = %v", err)
			}
			if m.resolvedRouteIP4 != tt.wantIP4 || m.resolvedRouteIf4 != tt.wantIf4 {
				t.Fatalf("unexpected IPv4 route state: ip=%q interface=%q", m.resolvedRouteIP4, m.resolvedRouteIf4)
			}
			if m.resolvedRouteIP6 != tt.wantIP6 || m.resolvedRouteIf6 != tt.wantIf6 {
				t.Fatalf("unexpected IPv6 route state: ip=%q interface=%q", m.resolvedRouteIP6, m.resolvedRouteIf6)
			}
		})
	}
}

func TestDualStackManager_CloseTunnel_ReturnsCleanupErrors(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{
		delSplitErr: errors.New("split4 cleanup fail"),
		delRouteErr: errors.New("route4 cleanup fail"),
		setDNSErr:   errors.New("dns4 cleanup fail"),
	}
	cfg6 := &dualStackNetCfgMock{
		delSplitErr: errors.New("split6 cleanup fail"),
		delRouteErr: errors.New("route6 cleanup fail"),
		setDNSErr:   errors.New("dns6 cleanup fail"),
	}
	m := newDualStackManager(settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
		},
	}, cfg4, cfg6)
	m.resolvedRouteIP4 = netip.MustParseAddr("198.51.100.10")
	m.resolvedRouteIP6 = netip.MustParseAddr("2001:db8::1")
	m.resolvedRouteIf4 = "eth0"
	m.resolvedRouteIf6 = "eth0"
	m.tun = &dualStackTunMock{closeErr: errors.New("tun close fail")}

	err := m.CloseTunnel()
	if err == nil {
		t.Fatal("expected aggregated cleanup error")
	}
	msg := err.Error()
	for _, want := range []string{
		"split4 cleanup fail",
		"split6 cleanup fail",
		"route4 cleanup fail",
		"route6 cleanup fail",
		"dns4 cleanup fail",
		"dns6 cleanup fail",
		"tun close fail",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected cleanup error to contain %q, got %v", want, err)
		}
	}
}

func TestDualStackManager_SetDNSToTunDevice_UsesConfiguredDNS(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{}
	cfg6 := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     settings.Host{IPv4: "198.51.100.10", IPv6: "2001:db8::1"},
			DNSv4:      []string{"9.9.9.9", "1.0.0.1"},
			DNSv6:      []string{"2620:fe::9", "2001:4860:4860::8844"},
		},
		MTU: settings.DefaultIPv6MTU,
	}

	m := newDualStackManager(s, cfg4, cfg6)

	if err := m.setDNSToTunDevice(); err != nil {
		t.Fatalf("setDNSToTunDevice() error = %v", err)
	}
	if len(cfg4.setDNSValues) == 0 || len(cfg6.setDNSValues) == 0 {
		t.Fatalf("expected both DNS set calls, got v4=%d v6=%d", len(cfg4.setDNSValues), len(cfg6.setDNSValues))
	}
	if !reflect.DeepEqual(cfg4.setDNSValues[0], s.DNSv4Resolvers()) {
		t.Fatalf("unexpected IPv4 DNS resolvers: got %v want %v", cfg4.setDNSValues[0], s.DNSv4Resolvers())
	}
	if !reflect.DeepEqual(cfg6.setDNSValues[0], s.DNSv6Resolvers()) {
		t.Fatalf("unexpected IPv6 DNS resolvers: got %v want %v", cfg6.setDNSValues[0], s.DNSv6Resolvers())
	}
	if cfg4.flushDNSCalls == 0 || cfg6.flushDNSCalls == 0 {
		t.Fatalf("expected DNS flush calls for both families, got v4=%d v6=%d", cfg4.flushDNSCalls, cfg6.flushDNSCalls)
	}
}

func TestDualStackManager_SetDNSToTunDevice_IgnoresIPv6FlushFailure(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{}
	cfg6 := &dualStackNetCfgMock{flushDNSErr: errors.New("flush6 fail")}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     settings.Host{IPv4: "198.51.100.10", IPv6: "2001:db8::1"},
		},
		MTU: settings.DefaultIPv6MTU,
	}

	m := newDualStackManager(s, cfg4, cfg6)

	if err := m.setDNSToTunDevice(); err != nil {
		t.Fatalf("setDNSToTunDevice() should ignore IPv6 flush failure, got %v", err)
	}
	if cfg4.flushDNSCalls == 0 || cfg6.flushDNSCalls == 0 {
		t.Fatalf("expected flush attempts for both families, got v4=%d v6=%d", cfg4.flushDNSCalls, cfg6.flushDNSCalls)
	}
}

func containsRouteDelete(routes []string, want string) bool {
	for _, route := range routes {
		if route == want {
			return true
		}
	}
	return false
}
