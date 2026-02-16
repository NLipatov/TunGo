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

func TestV4Manager_CreateDevice_RollbackOnSplitRouteError(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0", addSplitErr: errors.New("split4 failed")}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
		},
		MTU: settings.SafeMTU,
	}

	m := newV4Manager(s, cfg)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }

	_, err := m.CreateDevice()
	if err == nil || !strings.Contains(err.Error(), "split4 failed") {
		t.Fatalf("expected split route error, got %v", err)
	}
	if !tunDev.closed {
		t.Fatal("expected tun device to be closed on rollback")
	}
	if cfg.deleteDefaultSplitCalls == 0 {
		t.Fatal("expected split routes cleanup")
	}
	if len(cfg.deletedRoutes) == 0 {
		t.Fatal("expected host route cleanup")
	}
	if !containsRouteDelete(cfg.deletedRoutes, "198.51.100.10@eth0") {
		t.Fatalf("expected route cleanup on best-route interface, got %v", cfg.deletedRoutes)
	}
	if !containsRouteDelete(cfg.deletedRoutes, "198.51.100.10") {
		t.Fatalf("expected stale route cleanup before add, got %v", cfg.deletedRoutes)
	}
}

func TestV6Manager_CreateDevice_RollbackOnSplitRouteError(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0", addSplitErr: errors.New("split6 failed")}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
		},
		MTU: settings.SafeMTU,
	}

	m := newV6Manager(s, cfg)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	_, err := m.CreateDevice()
	if err == nil || !strings.Contains(err.Error(), "split6 failed") {
		t.Fatalf("expected split route error, got %v", err)
	}
	if !tunDev.closed {
		t.Fatal("expected tun device to be closed on rollback")
	}
	if cfg.deleteDefaultSplitCalls == 0 {
		t.Fatal("expected split routes cleanup")
	}
	if len(cfg.deletedRoutes) == 0 {
		t.Fatal("expected host route cleanup")
	}
	if !containsRouteDelete(cfg.deletedRoutes, "2001:db8::1@eth0") {
		t.Fatalf("expected route cleanup on best-route interface, got %v", cfg.deletedRoutes)
	}
	if !containsRouteDelete(cfg.deletedRoutes, "2001:db8::1") {
		t.Fatalf("expected stale route cleanup before add, got %v", cfg.deletedRoutes)
	}
}

func TestV4Manager_AddStaticRouteToServer_UsesRouteEndpoint(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
		},
	}, cfg)
	m.SetRouteEndpoint(netip.MustParseAddrPort("203.0.113.11:443"))
	m.resolveRouteIPv4Fn = func() (string, error) { return "", errors.New("should not be called") }

	if err := m.addStaticRouteToServer(); err != nil {
		t.Fatalf("expected success using route endpoint, got %v", err)
	}
	if m.resolvedRouteIP != "203.0.113.11" {
		t.Fatalf("unexpected resolved route ip: %s", m.resolvedRouteIP)
	}
}

func TestV6Manager_AddStaticRouteToServer_UsesRouteEndpoint(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
		},
	}, cfg)
	m.SetRouteEndpoint(netip.MustParseAddrPort("[2001:db8::5]:443"))
	m.resolveRouteIPv6Fn = func() (string, error) { return "", errors.New("should not be called") }

	if err := m.addStaticRouteToServer(); err != nil {
		t.Fatalf("expected success using route endpoint, got %v", err)
	}
	if m.resolvedRouteIP != "2001:db8::5" {
		t.Fatalf("unexpected resolved route ip: %s", m.resolvedRouteIP)
	}
}

func TestV4Manager_AddStaticRouteToServer_UsesInterfaceIndexWhenAliasEmpty(t *testing.T) {
	cfg := &dualStackNetCfgMock{emptyRouteIf: true, bestRouteIdx: 12}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
		},
	}, cfg)
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }

	if err := m.addStaticRouteToServer(); err != nil {
		t.Fatalf("expected success with interface index fallback, got %v", err)
	}
	if m.resolvedRouteIf != "12" {
		t.Fatalf("unexpected resolved route interface: %s", m.resolvedRouteIf)
	}
	if !containsRouteDelete(cfg.deletedRoutes, "198.51.100.10") {
		t.Fatalf("expected stale global route cleanup, got %v", cfg.deletedRoutes)
	}
	if !containsRouteDelete(cfg.deletedRoutes, "198.51.100.10@12") {
		t.Fatalf("expected interface-scoped route cleanup, got %v", cfg.deletedRoutes)
	}
}

func TestV6Manager_AddStaticRouteToServer_UsesInterfaceIndexWhenAliasEmpty(t *testing.T) {
	cfg := &dualStackNetCfgMock{emptyRouteIf: true, bestRouteIdx: 34}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
		},
	}, cfg)
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	if err := m.addStaticRouteToServer(); err != nil {
		t.Fatalf("expected success with interface index fallback, got %v", err)
	}
	if m.resolvedRouteIf != "34" {
		t.Fatalf("unexpected resolved route interface: %s", m.resolvedRouteIf)
	}
	if !containsRouteDelete(cfg.deletedRoutes, "2001:db8::1") {
		t.Fatalf("expected stale global route cleanup, got %v", cfg.deletedRoutes)
	}
	if !containsRouteDelete(cfg.deletedRoutes, "2001:db8::1@34") {
		t.Fatalf("expected interface-scoped route cleanup, got %v", cfg.deletedRoutes)
	}
}

func TestV4Manager_AddStaticRouteToServer_SkipsWhenRouteEndpointIsIPv6(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
		},
	}, cfg)
	m.SetRouteEndpoint(netip.MustParseAddrPort("[2001:db8::5]:443"))

	if err := m.addStaticRouteToServer(); err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
}

func TestV6Manager_AddStaticRouteToServer_SkipsWhenRouteEndpointIsIPv4(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
		},
	}, cfg)
	m.SetRouteEndpoint(netip.MustParseAddrPort("203.0.113.11:443"))

	if err := m.addStaticRouteToServer(); err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
}

func TestV4Manager_AddStaticRouteToServer_UsesResolverAfterRouteEndpointCleared(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
		},
	}, cfg)

	m.SetRouteEndpoint(netip.MustParseAddrPort("203.0.113.11:443"))
	m.SetRouteEndpoint(netip.AddrPort{})
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }

	if err := m.addStaticRouteToServer(); err != nil {
		t.Fatalf("expected resolver path after route endpoint clear, got %v", err)
	}
	if m.resolvedRouteIP != "198.51.100.10" {
		t.Fatalf("unexpected resolved route ip: %s", m.resolvedRouteIP)
	}
}

func TestV6Manager_AddStaticRouteToServer_UsesResolverAfterRouteEndpointCleared(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
		},
	}, cfg)

	m.SetRouteEndpoint(netip.MustParseAddrPort("[2001:db8::5]:443"))
	m.SetRouteEndpoint(netip.AddrPort{})
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	if err := m.addStaticRouteToServer(); err != nil {
		t.Fatalf("expected resolver path after route endpoint clear, got %v", err)
	}
	if m.resolvedRouteIP != "2001:db8::1" {
		t.Fatalf("unexpected resolved route ip: %s", m.resolvedRouteIP)
	}
}

func TestV4Manager_DisposeDevices_CleansDNS(t *testing.T) {
	cfg := &dualStackNetCfgMock{}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
		},
	}, cfg)

	if err := m.DisposeDevices(); err != nil {
		t.Fatalf("DisposeDevices unexpected error: %v", err)
	}
	if cfg.setDNSCalls == 0 {
		t.Fatal("expected DNS cleanup call on dispose")
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush on dispose")
	}
}

func TestV6Manager_DisposeDevices_CleansDNS(t *testing.T) {
	cfg := &dualStackNetCfgMock{}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
		},
	}, cfg)

	if err := m.DisposeDevices(); err != nil {
		t.Fatalf("DisposeDevices unexpected error: %v", err)
	}
	if cfg.setDNSCalls == 0 {
		t.Fatal("expected DNS cleanup call on dispose")
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush on dispose")
	}
}

func TestV4Manager_DisposeDevices_ReturnsCleanupErrors(t *testing.T) {
	cfg := &dualStackNetCfgMock{
		delSplitErr: errors.New("split cleanup fail"),
		delRouteErr: errors.New("route cleanup fail"),
		setDNSErr:   errors.New("dns cleanup fail"),
	}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
		},
	}, cfg)
	m.resolvedRouteIP = "198.51.100.10"
	m.resolvedRouteIf = "eth0"
	m.tun = &dualStackTunMock{closeErr: errors.New("tun close fail")}

	err := m.DisposeDevices()
	if err == nil {
		t.Fatal("expected aggregated cleanup error")
	}
	msg := err.Error()
	for _, want := range []string{
		"split cleanup fail",
		"route cleanup fail",
		"dns cleanup fail",
		"tun close fail",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected cleanup error to contain %q, got %v", want, err)
		}
	}
}

func TestV6Manager_DisposeDevices_ReturnsCleanupErrors(t *testing.T) {
	cfg := &dualStackNetCfgMock{
		delSplitErr: errors.New("split cleanup fail"),
		delRouteErr: errors.New("route cleanup fail"),
		setDNSErr:   errors.New("dns cleanup fail"),
	}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
		},
	}, cfg)
	m.resolvedRouteIP = "2001:db8::1"
	m.resolvedRouteIf = "eth0"
	m.tun = &dualStackTunMock{closeErr: errors.New("tun close fail")}

	err := m.DisposeDevices()
	if err == nil {
		t.Fatal("expected aggregated cleanup error")
	}
	msg := err.Error()
	for _, want := range []string{
		"split cleanup fail",
		"route cleanup fail",
		"dns cleanup fail",
		"tun close fail",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected cleanup error to contain %q, got %v", want, err)
		}
	}
}

func TestV4Manager_CreateDevice_UsesConfiguredDNS(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
			DNSv4:      []string{"9.9.9.9", "1.0.0.1"},
		},
		MTU: settings.SafeMTU,
	}

	m := newV4Manager(s, cfg)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }

	if _, err := m.CreateDevice(); err != nil {
		t.Fatalf("CreateDevice unexpected error: %v", err)
	}
	if len(cfg.setDNSValues) == 0 {
		t.Fatal("expected DNS set call")
	}
	if !reflect.DeepEqual(cfg.setDNSValues[0], s.DNSv4Resolvers()) {
		t.Fatalf("unexpected DNS resolvers: got %v want %v", cfg.setDNSValues[0], s.DNSv4Resolvers())
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush call")
	}
}

func TestV6Manager_CreateDevice_UsesConfiguredDNS(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
			DNSv6:      []string{"2606:4700:4700::1111", "2620:fe::9"},
		},
		MTU: settings.SafeMTU,
	}

	m := newV6Manager(s, cfg)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	if _, err := m.CreateDevice(); err != nil {
		t.Fatalf("CreateDevice unexpected error: %v", err)
	}
	if len(cfg.setDNSValues) == 0 {
		t.Fatal("expected DNS set call")
	}
	if !reflect.DeepEqual(cfg.setDNSValues[0], s.DNSv6Resolvers()) {
		t.Fatalf("unexpected DNS resolvers: got %v want %v", cfg.setDNSValues[0], s.DNSv6Resolvers())
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush call")
	}
}

func TestV4Manager_CreateDevice_IgnoresDNSErrorOnFlushFailure(t *testing.T) {
	cfg := &dualStackNetCfgMock{
		bestRouteIf: "eth0",
		flushDNSErr: errors.New("flush fail"),
	}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
		},
		MTU: settings.SafeMTU,
	}

	m := newV4Manager(s, cfg)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv4Fn = func() (string, error) { return "198.51.100.10", nil }

	if _, err := m.CreateDevice(); err != nil {
		t.Fatalf("CreateDevice should ignore DNS flush failure, got %v", err)
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush attempt")
	}
}

func TestV6Manager_CreateDevice_IgnoresDNSErrorOnFlushFailure(t *testing.T) {
	cfg := &dualStackNetCfgMock{
		bestRouteIf: "eth0",
		flushDNSErr: errors.New("flush fail"),
	}
	tunDev := &dualStackTunMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
		},
		MTU: settings.SafeMTU,
	}

	m := newV6Manager(s, cfg)
	m.createTunDeviceFn = func() (tun.Device, error) { return tunDev, nil }
	m.resolveRouteIPv6Fn = func() (string, error) { return "2001:db8::1", nil }

	if _, err := m.CreateDevice(); err != nil {
		t.Fatalf("CreateDevice should ignore DNS flush failure, got %v", err)
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush attempt")
	}
}
