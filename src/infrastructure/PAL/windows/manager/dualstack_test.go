//go:build windows

package manager

import (
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"tungo/application/network/routing/tun"
	"tungo/infrastructure/settings"
)

type dualStackTunMock struct {
	closed   bool
	closeErr error
}

func (d *dualStackTunMock) Read([]byte) (int, error)    { return 0, nil }
func (d *dualStackTunMock) Write(p []byte) (int, error) { return len(p), nil }
func (d *dualStackTunMock) Close() error {
	d.closed = true
	return d.closeErr
}

type dualStackNetCfgMock struct {
	bestRouteGW  string
	bestRouteIf  string
	bestRouteIdx int
	emptyRouteIf bool
	bestRouteErr error
	addSplitErr  error
	setAddrErr   error
	setMTUErr    error
	setDNSErr    error
	flushDNSErr  error
	delSplitErr  error
	delRouteErr  error

	deleteDefaultSplitCalls int
	addDefaultSplitCalls    int
	setMTUCalls             int
	setDNSCalls             int
	flushDNSCalls           int
	setDNSValues            [][]string

	deletedRoutes []string
}

func (m *dualStackNetCfgMock) FlushDNS() error {
	m.flushDNSCalls++
	return m.flushDNSErr
}
func (m *dualStackNetCfgMock) SetAddressStatic(_, _, _ string) error { return m.setAddrErr }
func (m *dualStackNetCfgMock) SetAddressWithGateway(_, _, _, _ string, _ int) error {
	return nil
}
func (m *dualStackNetCfgMock) DeleteAddress(_, _ string) error { return nil }
func (m *dualStackNetCfgMock) SetDNS(_ string, dnsServers []string) error {
	m.setDNSCalls++
	m.setDNSValues = append(m.setDNSValues, append([]string(nil), dnsServers...))
	return m.setDNSErr
}
func (m *dualStackNetCfgMock) SetMTU(_ string, _ int) error {
	m.setMTUCalls++
	return m.setMTUErr
}
func (m *dualStackNetCfgMock) AddRoutePrefix(_, _ string, _ int) error { return nil }
func (m *dualStackNetCfgMock) DeleteRoutePrefix(_, _ string) error     { return nil }
func (m *dualStackNetCfgMock) DeleteDefaultRoute(_ string) error       { return nil }
func (m *dualStackNetCfgMock) AddHostRouteViaGateway(_, _, _ string, _ int) error {
	return nil
}
func (m *dualStackNetCfgMock) AddHostRouteOnLink(_, _ string, _ int) error { return nil }
func (m *dualStackNetCfgMock) AddDefaultSplitRoutes(_ string, _ int) error {
	m.addDefaultSplitCalls++
	return m.addSplitErr
}
func (m *dualStackNetCfgMock) DeleteDefaultSplitRoutes(_ string) error {
	m.deleteDefaultSplitCalls++
	return m.delSplitErr
}
func (m *dualStackNetCfgMock) DeleteRoute(destination string) error {
	m.deletedRoutes = append(m.deletedRoutes, destination)
	return m.delRouteErr
}
func (m *dualStackNetCfgMock) DeleteRouteOnInterface(destination, ifName string) error {
	m.deletedRoutes = append(m.deletedRoutes, destination+"@"+ifName)
	return m.delRouteErr
}
func (m *dualStackNetCfgMock) Print(string) ([]byte, error) { return nil, nil }
func (m *dualStackNetCfgMock) BestRoute(string) (string, string, int, int, error) {
	if m.bestRouteErr != nil {
		return "", "", 0, 0, m.bestRouteErr
	}
	iface := m.bestRouteIf
	if iface == "" && !m.emptyRouteIf {
		iface = "Ethernet0"
	}
	idx := m.bestRouteIdx
	if idx <= 0 {
		idx = 1
	}
	return m.bestRouteGW, iface, idx, 1, nil
}

func TestDualStackManager_AddStaticRouteToServer4_SkipsWhenServerHasNoIPv4(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{}
	cfg6 := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "2001:db8::1"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)
	m.resolveRouteIPv4Fn = func() (string, error) {
		return "", errors.New("host \"2001:db8::1\" is IPv6, expected IPv4")
	}

	if err := m.addStaticRouteToServer4(); err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
}

func TestDualStackManager_AddStaticRouteToServer6_SkipsWhenServerHasNoIPv6(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{}
	cfg6 := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "198.51.100.10"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)
	m.resolveRouteIPv6Fn = func() (string, error) {
		return "", errors.New("host \"198.51.100.10\" is IPv4, expected IPv6")
	}

	if err := m.addStaticRouteToServer6(); err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
}

func TestDualStackManager_AddStaticRouteToServer4_ErrorsWhenServerHasIPv4(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{}
	cfg6 := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "198.51.100.10"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)
	m.resolveRouteIPv4Fn = func() (string, error) { return "", errors.New("boom4") }

	err := m.addStaticRouteToServer4()
	if err == nil || !strings.Contains(err.Error(), "boom4") {
		t.Fatalf("expected wrapped ipv4 route error, got %v", err)
	}
}

func TestDualStackManager_AddStaticRouteToServer6_ErrorsWhenServerHasIPv6(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{}
	cfg6 := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "2001:db8::1"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)
	m.resolveRouteIPv6Fn = func() (string, error) { return "", errors.New("boom6") }

	err := m.addStaticRouteToServer6()
	if err == nil || !strings.Contains(err.Error(), "boom6") {
		t.Fatalf("expected wrapped ipv6 route error, got %v", err)
	}
}

func TestDualStackManager_AddStaticRoute_DomainResolveErrorsAreNotSuppressed(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{}
	cfg6 := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "vpn.example.com"),
		},
	}
	m := newDualStackManager(s, cfg4, cfg6)
	m.resolveRouteIPv4Fn = func() (string, error) {
		return "", errors.New("failed to resolve host \"vpn.example.com\": timeout")
	}
	m.resolveRouteIPv6Fn = func() (string, error) {
		return "", errors.New("failed to resolve host \"vpn.example.com\": timeout")
	}

	if err := m.addStaticRouteToServer4(); err == nil || !strings.Contains(err.Error(), "failed to resolve host") {
		t.Fatalf("expected IPv4 resolve failure to be returned, got %v", err)
	}
	if err := m.addStaticRouteToServer6(); err == nil || !strings.Contains(err.Error(), "failed to resolve host") {
		t.Fatalf("expected IPv6 resolve failure to be returned, got %v", err)
	}
}

func TestDualStackManager_AddStaticRoute_UsesRouteEndpoint(t *testing.T) {
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
	m.SetRouteEndpoint(netip.MustParseAddrPort("198.51.100.77:443"))
	m.resolveRouteIPv4Fn = func() (string, error) { return "", errors.New("should not be called") }
	m.resolveRouteIPv6Fn = func() (string, error) { return "", errors.New("should not be called") }

	if err := m.addStaticRouteToServer4(); err != nil {
		t.Fatalf("expected IPv4 route from endpoint, got %v", err)
	}
	if m.resolvedRouteIP4 != "198.51.100.77" {
		t.Fatalf("unexpected IPv4 route target: %s", m.resolvedRouteIP4)
	}
	if m.resolvedRouteIf4 != "eth0" {
		t.Fatalf("unexpected IPv4 route interface: %s", m.resolvedRouteIf4)
	}
	if err := m.addStaticRouteToServer6(); err != nil {
		t.Fatalf("expected IPv6 route to be skipped for IPv4 endpoint, got %v", err)
	}
}

func TestDualStackManager_AddStaticRoute_UsesResolverAfterRouteEndpointCleared(t *testing.T) {
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
	m.SetRouteEndpoint(netip.MustParseAddrPort("198.51.100.77:443"))
	m.SetRouteEndpoint(netip.AddrPort{})
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	if err := m.addStaticRouteToServer4(); err != nil {
		t.Fatalf("expected IPv4 resolver route after endpoint clear, got %v", err)
	}
	if err := m.addStaticRouteToServer6(); err != nil {
		t.Fatalf("expected IPv6 resolver route after endpoint clear, got %v", err)
	}
	if m.resolvedRouteIP4 != "198.51.100.10" || m.resolvedRouteIP6 != "2001:db8::1" {
		t.Fatalf("unexpected resolved routes: v4=%s v6=%s", m.resolvedRouteIP4, m.resolvedRouteIP6)
	}
	if m.resolvedRouteIf4 != "eth0" || m.resolvedRouteIf6 != "eth0" {
		t.Fatalf("unexpected resolved route interfaces: v4=%s v6=%s", m.resolvedRouteIf4, m.resolvedRouteIf6)
	}
}

func TestDualStackManager_CreateDevice_RollbackOnIPv6SplitRouteError(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	cfg6 := &dualStackNetCfgMock{bestRouteIf: "eth0", addSplitErr: errors.New("split6 failed")}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "198.51.100.10").WithIPv6(netip.MustParseAddr("2001:db8::1")),
		},
		MTU: settings.SafeMTU,
	}

	m := newDualStackManager(s, cfg4, cfg6)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	_, err := m.CreateDevice()
	if err == nil || !strings.Contains(err.Error(), "split6 failed") {
		t.Fatalf("expected IPv6 split route error, got %v", err)
	}
	if !tunDev.closed {
		t.Fatal("expected tun device to be closed on rollback")
	}
	if cfg4.deleteDefaultSplitCalls == 0 || cfg6.deleteDefaultSplitCalls == 0 {
		t.Fatalf("expected split routes cleanup for both families, got v4=%d v6=%d", cfg4.deleteDefaultSplitCalls, cfg6.deleteDefaultSplitCalls)
	}
	if len(cfg4.deletedRoutes) == 0 || len(cfg6.deletedRoutes) == 0 {
		t.Fatalf("expected host route cleanup for both families, got v4=%v v6=%v", cfg4.deletedRoutes, cfg6.deletedRoutes)
	}
	if !containsRouteDelete(cfg4.deletedRoutes, "198.51.100.10@eth0") {
		t.Fatalf("expected IPv4 route cleanup on best-route interface, got %v", cfg4.deletedRoutes)
	}
	if !containsRouteDelete(cfg6.deletedRoutes, "2001:db8::1@eth0") {
		t.Fatalf("expected IPv6 route cleanup on best-route interface, got %v", cfg6.deletedRoutes)
	}
	if !containsRouteDelete(cfg4.deletedRoutes, "198.51.100.10") {
		t.Fatalf("expected stale IPv4 route cleanup before add, got %v", cfg4.deletedRoutes)
	}
	if !containsRouteDelete(cfg6.deletedRoutes, "2001:db8::1") {
		t.Fatalf("expected stale IPv6 route cleanup before add, got %v", cfg6.deletedRoutes)
	}
}

func TestDualStackManager_CreateDevice_RollbackClearsDNSOnIPv6DNSError(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	cfg6 := &dualStackNetCfgMock{bestRouteIf: "eth0", setDNSErr: errors.New("dns6 failed")}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "198.51.100.10").WithIPv6(netip.MustParseAddr("2001:db8::1")),
		},
		MTU: settings.SafeMTU,
	}

	m := newDualStackManager(s, cfg4, cfg6)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	_, err := m.CreateDevice()
	if err == nil || !strings.Contains(err.Error(), "dns6 failed") {
		t.Fatalf("expected IPv6 DNS error, got %v", err)
	}
	if cfg4.setDNSCalls < 2 {
		t.Fatalf("expected IPv4 DNS set+cleanup calls, got %d", cfg4.setDNSCalls)
	}
	if cfg6.setDNSCalls < 2 {
		t.Fatalf("expected IPv6 DNS set+cleanup calls, got %d", cfg6.setDNSCalls)
	}
	if cfg4.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush during rollback")
	}
	if cfg6.flushDNSCalls == 0 {
		t.Fatal("expected IPv6 DNS flush during rollback")
	}
}

func TestDualStackManager_DisposeDevices_ReturnsCleanupErrors(t *testing.T) {
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
	m.resolvedRouteIP4 = "198.51.100.10"
	m.resolvedRouteIP6 = "2001:db8::1"
	m.resolvedRouteIf4 = "eth0"
	m.resolvedRouteIf6 = "eth0"
	m.tun = &dualStackTunMock{closeErr: errors.New("tun close fail")}

	err := m.DisposeDevices()
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

func TestDualStackManager_CreateDevice_UsesConfiguredDNS(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	cfg6 := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "198.51.100.10").WithIPv6(netip.MustParseAddr("2001:db8::1")),
			DNSv4:      []string{"9.9.9.9", "1.0.0.1"},
			DNSv6:      []string{"2620:fe::9", "2001:4860:4860::8844"},
		},
		MTU: settings.SafeMTU,
	}

	m := newDualStackManager(s, cfg4, cfg6)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	if _, err := m.CreateDevice(); err != nil {
		t.Fatalf("CreateDevice unexpected error: %v", err)
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

func TestDualStackManager_CreateDevice_IgnoresDNSErrorOnIPv6FlushFailure(t *testing.T) {
	cfg4 := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	cfg6 := &dualStackNetCfgMock{bestRouteIf: "eth0", flushDNSErr: errors.New("flush6 fail")}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "198.51.100.10").WithIPv6(netip.MustParseAddr("2001:db8::1")),
		},
		MTU: settings.SafeMTU,
	}

	m := newDualStackManager(s, cfg4, cfg6)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	if _, err := m.CreateDevice(); err != nil {
		t.Fatalf("CreateDevice should ignore IPv6 DNS flush failure, got %v", err)
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
