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

func TestV4Manager_AddStaticRouteToServer_UsesPassedServerAddr(t *testing.T) {
	cfg := &dualStackNetCfgMock{
		bestRouteGateway: netip.MustParseAddr("192.0.2.1"),
		bestRouteIf:      "eth0",
	}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
		},
	}, cfg)

	if err := m.addStaticRouteToServer(netip.MustParseAddr("203.0.113.11")); err != nil {
		t.Fatalf("expected success using passed server address, got %v", err)
	}
	if m.resolvedRouteIP != netip.MustParseAddr("203.0.113.11") {
		t.Fatalf("unexpected resolved route ip: %s", m.resolvedRouteIP)
	}
	if want := []string{"203.0.113.11 via 192.0.2.1 dev eth0"}; !reflect.DeepEqual(cfg.addedRoutes, want) {
		t.Fatalf("added routes = %v, want %v", cfg.addedRoutes, want)
	}
}

func TestV6Manager_AddStaticRouteToServer_UsesPassedServerAddr(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
		},
	}, cfg)

	if err := m.addStaticRouteToServer(netip.MustParseAddr("2001:db8::5")); err != nil {
		t.Fatalf("expected success using passed server address, got %v", err)
	}
	if m.resolvedRouteIP != netip.MustParseAddr("2001:db8::5") {
		t.Fatalf("unexpected resolved route ip: %s", m.resolvedRouteIP)
	}
	if want := []string{"2001:db8::5 dev eth0"}; !reflect.DeepEqual(cfg.addedRoutes, want) {
		t.Fatalf("added routes = %v, want %v", cfg.addedRoutes, want)
	}
}

func TestSingleStackManager_AddStaticRouteToServer_DoesNotCacheFailedRoute(t *testing.T) {
	addErr := errors.New("add route")

	t.Run("IPv4", func(t *testing.T) {
		cfg := &dualStackNetCfgMock{bestRouteIf: "eth0", addRouteErr: addErr}
		m := newV4Manager(settings.Settings{}, cfg)

		err := m.addStaticRouteToServer(netip.MustParseAddr("198.51.100.10"))
		if !errors.Is(err, addErr) {
			t.Fatalf("addStaticRouteToServer() error = %v, want %v", err, addErr)
		}
		if m.resolvedRouteIP.IsValid() || m.resolvedRouteIf != "" {
			t.Fatalf("failed route was cached: ip=%q interface=%q", m.resolvedRouteIP, m.resolvedRouteIf)
		}
	})

	t.Run("IPv6", func(t *testing.T) {
		cfg := &dualStackNetCfgMock{bestRouteIf: "eth0", addRouteErr: addErr}
		m := newV6Manager(settings.Settings{}, cfg)

		err := m.addStaticRouteToServer(netip.MustParseAddr("2001:db8::1"))
		if !errors.Is(err, addErr) {
			t.Fatalf("addStaticRouteToServer() error = %v, want %v", err, addErr)
		}
		if m.resolvedRouteIP.IsValid() || m.resolvedRouteIf != "" {
			t.Fatalf("failed route was cached: ip=%q interface=%q", m.resolvedRouteIP, m.resolvedRouteIf)
		}
	})
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
	if err := m.addStaticRouteToServer(netip.MustParseAddr("198.51.100.10")); err != nil {
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
	if err := m.addStaticRouteToServer(netip.MustParseAddr("2001:db8::1")); err != nil {
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

func TestV4Manager_AddStaticRouteToServer_SkipsIPv6Server(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
		},
	}, cfg)

	if err := m.addStaticRouteToServer(netip.MustParseAddr("2001:db8::5")); err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
}

func TestV6Manager_AddStaticRouteToServer_SkipsIPv4Server(t *testing.T) {
	cfg := &dualStackNetCfgMock{bestRouteIf: "eth0"}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
		},
	}, cfg)

	if err := m.addStaticRouteToServer(netip.MustParseAddr("203.0.113.11")); err != nil {
		t.Fatalf("expected skip without error, got %v", err)
	}
}

func TestV4Manager_CloseTunnel_CleansDNS(t *testing.T) {
	cfg := &dualStackNetCfgMock{}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
		},
	}, cfg)

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel unexpected error: %v", err)
	}
	if cfg.setDNSCalls == 0 {
		t.Fatal("expected DNS cleanup call on dispose")
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush on dispose")
	}
}

func TestV6Manager_CloseTunnel_CleansDNS(t *testing.T) {
	cfg := &dualStackNetCfgMock{}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
		},
	}, cfg)

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel unexpected error: %v", err)
	}
	if cfg.setDNSCalls == 0 {
		t.Fatal("expected DNS cleanup call on dispose")
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush on dispose")
	}
}

func TestV4Manager_CloseTunnel_ClearsRouteStateAfterSuccessfulDelete(t *testing.T) {
	cfg := &dualStackNetCfgMock{}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{TunName: "tun0"},
	}, cfg)
	m.resolvedRouteIP = netip.MustParseAddr("198.51.100.10")
	m.resolvedRouteIf = "eth0"

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if m.resolvedRouteIP.IsValid() || m.resolvedRouteIf != "" {
		t.Fatalf("route state was not cleared: ip=%q interface=%q", m.resolvedRouteIP, m.resolvedRouteIf)
	}
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("second CloseTunnel() error = %v", err)
	}
	if got := len(cfg.deletedRoutes); got != 1 {
		t.Fatalf("route deletion calls = %d, want 1", got)
	}
}

func TestV4Manager_CloseTunnel_KeepsRouteStateAfterDeleteError(t *testing.T) {
	deleteErr := errors.New("route cleanup fail")
	cfg := &dualStackNetCfgMock{delRouteErr: deleteErr}
	m := newV4Manager(settings.Settings{
		Addressing: settings.Addressing{TunName: "tun0"},
	}, cfg)
	m.resolvedRouteIP = netip.MustParseAddr("198.51.100.10")
	m.resolvedRouteIf = "eth0"

	if err := m.CloseTunnel(); !errors.Is(err, deleteErr) {
		t.Fatalf("CloseTunnel() error = %v, want %v", err, deleteErr)
	}
	if m.resolvedRouteIP != netip.MustParseAddr("198.51.100.10") || m.resolvedRouteIf != "eth0" {
		t.Fatalf("route state was not preserved: ip=%q interface=%q", m.resolvedRouteIP, m.resolvedRouteIf)
	}

	cfg.delRouteErr = nil
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("retry CloseTunnel() error = %v", err)
	}
	if m.resolvedRouteIP.IsValid() || m.resolvedRouteIf != "" {
		t.Fatalf("route state was not cleared after retry: ip=%q interface=%q", m.resolvedRouteIP, m.resolvedRouteIf)
	}
	if got := len(cfg.deletedRoutes); got != 2 {
		t.Fatalf("route deletion calls = %d, want 2", got)
	}
}

func TestV6Manager_CloseTunnel_ClearsRouteStateAfterSuccessfulDelete(t *testing.T) {
	cfg := &dualStackNetCfgMock{}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{TunName: "tun0"},
	}, cfg)
	m.resolvedRouteIP = netip.MustParseAddr("2001:db8::1")
	m.resolvedRouteIf = "eth0"

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if m.resolvedRouteIP.IsValid() || m.resolvedRouteIf != "" {
		t.Fatalf("route state was not cleared: ip=%q interface=%q", m.resolvedRouteIP, m.resolvedRouteIf)
	}
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("second CloseTunnel() error = %v", err)
	}
	if got := len(cfg.deletedRoutes); got != 1 {
		t.Fatalf("route deletion calls = %d, want 1", got)
	}
}

func TestV6Manager_CloseTunnel_KeepsRouteStateAfterDeleteError(t *testing.T) {
	deleteErr := errors.New("route cleanup fail")
	cfg := &dualStackNetCfgMock{delRouteErr: deleteErr}
	m := newV6Manager(settings.Settings{
		Addressing: settings.Addressing{TunName: "tun0"},
	}, cfg)
	m.resolvedRouteIP = netip.MustParseAddr("2001:db8::1")
	m.resolvedRouteIf = "eth0"

	if err := m.CloseTunnel(); !errors.Is(err, deleteErr) {
		t.Fatalf("CloseTunnel() error = %v, want %v", err, deleteErr)
	}
	if m.resolvedRouteIP != netip.MustParseAddr("2001:db8::1") || m.resolvedRouteIf != "eth0" {
		t.Fatalf("route state was not preserved: ip=%q interface=%q", m.resolvedRouteIP, m.resolvedRouteIf)
	}

	cfg.delRouteErr = nil
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("retry CloseTunnel() error = %v", err)
	}
	if m.resolvedRouteIP.IsValid() || m.resolvedRouteIf != "" {
		t.Fatalf("route state was not cleared after retry: ip=%q interface=%q", m.resolvedRouteIP, m.resolvedRouteIf)
	}
	if got := len(cfg.deletedRoutes); got != 2 {
		t.Fatalf("route deletion calls = %d, want 2", got)
	}
}

func TestV4Manager_CloseTunnel_ReturnsCleanupErrors(t *testing.T) {
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
	m.resolvedRouteIP = netip.MustParseAddr("198.51.100.10")
	m.resolvedRouteIf = "eth0"
	m.tun = &dualStackTunMock{closeErr: errors.New("tun close fail")}

	err := m.CloseTunnel()
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

func TestV6Manager_CloseTunnel_ReturnsCleanupErrors(t *testing.T) {
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
	m.resolvedRouteIP = netip.MustParseAddr("2001:db8::1")
	m.resolvedRouteIf = "eth0"
	m.tun = &dualStackTunMock{closeErr: errors.New("tun close fail")}

	err := m.CloseTunnel()
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

func TestV4Manager_SetDNSToTunDevice_UsesConfiguredDNS(t *testing.T) {
	cfg := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
			DNSv4:      []string{"9.9.9.9", "1.0.0.1"},
		},
		MTU: settings.DefaultMTU,
	}

	m := newV4Manager(s, cfg)

	if err := m.setDNSToTunDevice(); err != nil {
		t.Fatalf("setDNSToTunDevice() error = %v", err)
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

func TestV6Manager_SetDNSToTunDevice_UsesConfiguredDNS(t *testing.T) {
	cfg := &dualStackNetCfgMock{}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
			DNSv6:      []string{"2606:4700:4700::1111", "2620:fe::9"},
		},
		MTU: settings.MinimumIPv6MTU,
	}

	m := newV6Manager(s, cfg)

	if err := m.setDNSToTunDevice(); err != nil {
		t.Fatalf("setDNSToTunDevice() error = %v", err)
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

func TestV4Manager_SetDNSToTunDevice_IgnoresFlushFailure(t *testing.T) {
	cfg := &dualStackNetCfgMock{
		flushDNSErr: errors.New("flush fail"),
	}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
			IPv4:       netip.MustParseAddr("10.0.0.2"),
			Server:     mustHost(t, "198.51.100.10"),
		},
		MTU: settings.DefaultMTU,
	}

	m := newV4Manager(s, cfg)

	if err := m.setDNSToTunDevice(); err != nil {
		t.Fatalf("setDNSToTunDevice() should ignore flush failure, got %v", err)
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush attempt")
	}
}

func TestV6Manager_SetDNSToTunDevice_IgnoresFlushFailure(t *testing.T) {
	cfg := &dualStackNetCfgMock{
		flushDNSErr: errors.New("flush fail"),
	}

	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			IPv6:       netip.MustParseAddr("fd00::2"),
			Server:     mustHost(t, "2001:db8::1"),
		},
		MTU: settings.MinimumIPv6MTU,
	}

	m := newV6Manager(s, cfg)

	if err := m.setDNSToTunDevice(); err != nil {
		t.Fatalf("setDNSToTunDevice() should ignore flush failure, got %v", err)
	}
	if cfg.flushDNSCalls == 0 {
		t.Fatal("expected DNS flush attempt")
	}
}
