package client

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/netip"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"tungo/internal/config/client"
	"tungo/internal/config/settings"
	"tungo/internal/tun/internal/linux/mssclamp"

	"golang.org/x/sys/unix"
)

// clienttunManagerIPMock simulates `ip` contract and records call sequence.
// `failStep` makes the corresponding step return an error.
type clienttunManagerIPMock struct {
	log                 bytes.Buffer
	routeReply          string
	failStep            string
	routeGetTargets     []netip.Addr
	routeReplaceTargets []netip.Addr
	routeDelTargets     []netip.Addr
	addedSplits4        [][]string
	addedSplits6        [][]string
	deletedSplits4      [][]string
	deletedSplits6      [][]string
	deletedInterfaces   []string
	linkDeleteErr       error
}

func (m *clienttunManagerIPMock) mark(s string) error {
	m.log.WriteString(s + ";")
	if m.failStep == s {
		return errors.New("boom")
	}
	return nil
}

func (m *clienttunManagerIPMock) TunTapAddDevTun(string) error { return m.mark("add") }
func (m *clienttunManagerIPMock) LinkDelete(devName string) error {
	m.deletedInterfaces = append(m.deletedInterfaces, devName)
	m.log.WriteString("ldel;")
	return m.linkDeleteErr
}
func (m *clienttunManagerIPMock) LinkSetDevUp(string) error       { return m.mark("up") }
func (m *clienttunManagerIPMock) LinkSetDevMTU(string, int) error { return m.mark("mtu") }
func (m *clienttunManagerIPMock) AddrAddDev(string, string) error { return m.mark("addr") }
func (m *clienttunManagerIPMock) RouteDefault() (string, error)   { return "eth0", nil }
func (m *clienttunManagerIPMock) RouteGet(target netip.Addr) (string, error) {
	m.routeGetTargets = append(m.routeGetTargets, target)
	return m.routeReply, nil
}
func (m *clienttunManagerIPMock) RouteReplaceDev(target netip.Addr, _ string) error {
	m.routeReplaceTargets = append(m.routeReplaceTargets, target)
	return m.mark("rreplace")
}
func (m *clienttunManagerIPMock) RouteReplaceViaDev(target netip.Addr, _ string, _ netip.Addr) error {
	m.routeReplaceTargets = append(m.routeReplaceTargets, target)
	return m.mark("rreplacevia")
}
func (m *clienttunManagerIPMock) RouteAddSplitDev(_ string, prefixes []string) error {
	m.addedSplits4 = append(m.addedSplits4, slices.Clone(prefixes))
	return m.mark("split")
}
func (m *clienttunManagerIPMock) Route6AddSplitDev(_ string, prefixes []string) error {
	m.addedSplits6 = append(m.addedSplits6, slices.Clone(prefixes))
	return m.mark("split6")
}
func (m *clienttunManagerIPMock) RouteDelSplitDefault(_ string, prefixes []string) error {
	m.deletedSplits4 = append(m.deletedSplits4, slices.Clone(prefixes))
	m.log.WriteString("splitdel;")
	return nil
}
func (m *clienttunManagerIPMock) Route6DelSplitDefault(_ string, prefixes []string) error {
	m.deletedSplits6 = append(m.deletedSplits6, slices.Clone(prefixes))
	m.log.WriteString("splitdel6;")
	return nil
}
func (m *clienttunManagerIPMock) RouteDel(target netip.Addr) error {
	m.routeDelTargets = append(m.routeDelTargets, target)
	return m.mark("rdel")
}

// clienttunManagerIPGetErr forces RouteGet to return an error.
type clienttunManagerIPGetErr struct{ clienttunManagerIPMock }

func (m *clienttunManagerIPGetErr) RouteGet(netip.Addr) (string, error) {
	return "", fmt.Errorf("failed to get route to server IP: %w", errors.New("geterr"))
}

// clienttunManagerIOCTLMock returns a pollable file or an injected error.
type clienttunManagerIOCTLMock struct {
	openErr     error
	file        *os.File
	createCalls *int
}

// clienttunManagerMSSMock simulates mssclamp.Contract.
type clienttunManagerMSSMock struct {
	installErr        error
	removeErr         error
	installedFamilies *[]mssclamp.Families
	removedTunNames   *[]string
}

type clienttunManagerDNSMock struct {
	setInterfaces []string
	setResolvers4 [][]string
	setResolvers6 [][]string
	revertCalls   int
	setErr        error
	revertErr     error
}

func (m *clienttunManagerDNSMock) Set(ifName string, ipv4Resolvers, ipv6Resolvers []string) error {
	m.setInterfaces = append(m.setInterfaces, ifName)
	m.setResolvers4 = append(m.setResolvers4, append([]string(nil), ipv4Resolvers...))
	m.setResolvers6 = append(m.setResolvers6, append([]string(nil), ipv6Resolvers...))
	return m.setErr
}

func (m *clienttunManagerDNSMock) Revert() error {
	m.revertCalls++
	return m.revertErr
}

type clientTunMock struct {
	closeErr   error
	closeCalls int
}

func (*clientTunMock) Read([]byte) (int, error)    { return 0, io.EOF }
func (*clientTunMock) Write(p []byte) (int, error) { return len(p), nil }
func (t *clientTunMock) Close() error {
	t.closeCalls++
	return t.closeErr
}

func mustHost(raw string) settings.Host {
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return settings.Host{Domain: raw}
	}
	if ip.Unmap().Is4() {
		return settings.Host{IPv4: ip.Unmap().String()}
	}
	return settings.Host{IPv6: ip.String()}
}

func mustPrefix(raw string) netip.Prefix {
	return netip.MustParsePrefix(raw)
}

func mustAddr(raw string) netip.Addr {
	return netip.MustParseAddr(raw)
}

var (
	testServerAddrV4 = mustAddr("198.51.100.1")
	testServerAddrV6 = mustAddr("2001:db8::1")
)

func (m clienttunManagerMSSMock) Install(_ string, families mssclamp.Families) error {
	if m.installedFamilies != nil {
		*m.installedFamilies = append(*m.installedFamilies, families)
	}
	return m.installErr
}
func (m clienttunManagerMSSMock) Remove(tunName string) error {
	if m.removedTunNames != nil {
		*m.removedTunNames = append(*m.removedTunNames, tunName)
	}
	return m.removeErr
}

func (clienttunManagerIOCTLMock) DetectTunNameFromFd(*os.File) (string, error) { return "tun0", nil }
func (m clienttunManagerIOCTLMock) CreateTunInterface(string) (*os.File, error) {
	if m.createCalls != nil {
		(*m.createCalls)++
	}
	if m.openErr != nil {
		return nil, m.openErr
	}
	if m.file != nil {
		return m.file, nil
	}
	fds, err := unix.Socketpair(unix.AF_UNIX, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}
	_ = unix.Close(fds[1])
	return os.NewFile(uintptr(fds[0]), "test-tun"), nil
}

func newMgr(
	proto settings.Protocol,
	ipMock interface { // minimal duck typing to avoid importing ip package
		TunTapAddDevTun(string) error
		LinkDelete(string) error
		LinkSetDevUp(string) error
		LinkSetDevMTU(string, int) error
		AddrAddDev(string, string) error
		RouteDefault() (string, error)
		RouteAddSplitDev(string, []string) error
		Route6AddSplitDev(string, []string) error
		RouteDelSplitDefault(string, []string) error
		Route6DelSplitDefault(string, []string) error
		RouteGet(netip.Addr) (string, error)
		RouteReplaceDev(netip.Addr, string) error
		RouteReplaceViaDev(netip.Addr, string, netip.Addr) error
		RouteDel(netip.Addr) error
	},
	ioctlMock interface {
		DetectTunNameFromFd(*os.File) (string, error)
		CreateTunInterface(string) (*os.File, error)
	},
	mssMock interface {
		Install(string, mssclamp.Families) error
		Remove(string) error
	},
) *Manager {
	profiles := map[settings.Protocol]settings.Settings{
		settings.UDP: {
			Network: settings.Network{
				TunName:    "tun0",
				IPv4Subnet: mustPrefix("10.0.0.0/30"),
				IPv4:       mustAddr("10.0.0.2"),
				Server:     mustHost("198.51.100.1"),
				DNSv4:      []string{"1.1.1.1", "8.8.8.8"},
			},
			MTU:      1400,
			Protocol: settings.UDP,
		},
		settings.TCP: {
			Network: settings.Network{
				TunName:    "tun1",
				IPv4Subnet: mustPrefix("10.0.0.4/30"),
				IPv4:       mustAddr("10.0.0.6"),
				Server:     mustHost("203.0.113.1"),
				DNSv4:      []string{"1.1.1.1", "8.8.8.8"},
			},
			MTU:      1400,
			Protocol: settings.TCP,
		},
		settings.WS: {
			Network: settings.Network{
				TunName:    "tun2",
				IPv4Subnet: mustPrefix("10.0.0.8/30"),
				IPv4:       mustAddr("10.0.0.10"),
				Server:     mustHost("203.0.113.2"),
				DNSv4:      []string{"1.1.1.1", "8.8.8.8"},
			},
			MTU:      1250,
			Protocol: settings.WS,
		},
	}
	conf := &client.Configuration{
		Protocol:    proto,
		TCPSettings: profiles[settings.TCP],
		UDPSettings: profiles[settings.UDP],
		WSSettings:  profiles[settings.WS],
	}
	return &Manager{
		configuration: conf,
		settings:      profiles[proto],
		dns:           &clienttunManagerDNSMock{},
		ip:            ipMock,
		ioctl:         ioctlMock,
		mss:           mssMock,
	}
}

func assertOpenTunnelRolledBack(t *testing.T, m *Manager, ipMock *clienttunManagerIPMock) {
	t.Helper()
	if m.pinnedServerAddr.IsValid() {
		t.Fatalf("pinnedServerAddr = %s, want cleared after failed OpenTunnel", m.pinnedServerAddr)
	}
	if len(ipMock.routeDelTargets) != 1 || ipMock.routeDelTargets[0] != testServerAddrV4 {
		t.Fatalf("RouteDel() targets = %v, want [%s]", ipMock.routeDelTargets, testServerAddrV4)
	}
	if m.tun != nil {
		t.Fatal("failed OpenTunnel retained TUN")
	}
	if len(ipMock.deletedSplits4) != 0 || len(ipMock.deletedSplits6) != 0 {
		t.Fatalf("rollback explicitly deleted split routes: IPv4=%v IPv6=%v", ipMock.deletedSplits4, ipMock.deletedSplits6)
	}
	for _, cleanupStep := range []string{"ldel;", "rdel;"} {
		if !strings.Contains(ipMock.log.String(), cleanupStep) {
			t.Errorf("cleanup log = %q, want %q", ipMock.log.String(), cleanupStep)
		}
	}
}

func setLinuxActiveSettings(m *Manager, active settings.Settings) {
	if active.IPv4Subnet.IsValid() && len(active.DNSv4) == 0 {
		active.DNSv4 = []string{"1.1.1.1", "8.8.8.8"}
	}
	if active.IPv6Subnet.IsValid() && len(active.DNSv6) == 0 {
		active.DNSv6 = []string{"2606:4700:4700::1111", "2001:4860:4860::8888"}
	}
	m.settings = active
	switch active.Protocol {
	case settings.TCP:
		m.configuration.TCPSettings = active
	case settings.UDP:
		m.configuration.UDPSettings = active
	case settings.WS, settings.WSS:
		m.configuration.WSSettings = active
	}
}

//
// ============================ Tests ===========================
//

func TestLinuxManagerAppliesTunnelRoutes(t *testing.T) {
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
			wantV4: []string{"192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"},
			wantV6: []string{"2001:db8:1::/64"},
		},
		{name: "empty IPv4", v4: []string{}, v6: []string{"2001:db8:1::/64"}, wantV4: []string{}, wantV6: []string{"2001:db8:1::/64"}},
		{name: "empty IPv6", v4: []string{"192.0.2.0/24"}, v6: []string{}, wantV4: []string{"192.0.2.0/24"}, wantV6: []string{}},
		{name: "both empty", v4: []string{}, v6: []string{}, wantV4: []string{}, wantV6: []string{}},
		{
			name:   "connected subnets preserve broader and narrower routes",
			v4:     []string{"10.0.0.0/16", "10.0.0.0/24", "10.0.0.0/25", "192.0.2.0/24"},
			v6:     []string{"fd00::/48", "fd00::/64", "fd00::/80", "2001:db8::/64"},
			wantV4: []string{"10.0.0.0/16", "10.0.0.0/25", "192.0.2.0/24"},
			wantV6: []string{"fd00::/48", "fd00::/80", "2001:db8::/64"},
		},
		{
			name:   "only connected subnets",
			v4:     []string{"10.0.0.0/24"},
			v6:     []string{"fd00::/64"},
			wantV4: []string{},
			wantV6: []string{},
		},
		{
			name:   "TUN subnets and server host route",
			v4:     []string{"10.0.0.0/24", "198.51.100.1/32", "198.51.100.0/24"},
			v6:     []string{"fd00::/64", "2001:db8::1/128"},
			wantV4: []string{"198.51.100.0/24"},
			wantV6: []string{"2001:db8::1/128"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			originalV4, originalV6 := slices.Clone(test.v4), slices.Clone(test.v6)
			manager, err := New(&client.Configuration{
				ClientID: 1,
				Protocol: settings.UDP,
				UDPSettings: settings.Settings{
					Network: settings.Network{
						TunName:    "tun0",
						IPv4Subnet: mustPrefix("10.0.0.17/24"),
						IPv6Subnet: mustPrefix("fd00::17/64"),
					},
					MTU: settings.DefaultMTU,
				},
				TunnelRoutesV4: test.v4,
				TunnelRoutesV6: test.v6,
			})
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
			manager.ip = ipMock
			manager.dns = &clienttunManagerDNSMock{}
			manager.mss = clienttunManagerMSSMock{}
			tun := &clientTunMock{}
			manager.tun = tun
			if err := manager.configureTunnel(testServerAddrV4); err != nil {
				t.Fatalf("configureTunnel() error = %v", err)
			}
			if err := manager.CloseTunnel(); err != nil {
				t.Fatalf("CloseTunnel() error = %v", err)
			}
			if tun.closeCalls != 1 || manager.tun != nil {
				t.Fatalf("close state: calls=%d tun=%v", tun.closeCalls, manager.tun)
			}

			if !slices.Equal(manager.configuration.TunnelRoutesV4, originalV4) ||
				!slices.Equal(manager.configuration.TunnelRoutesV6, originalV6) {
				t.Fatalf("manager changed configured TunnelRoutes: IPv4=%v IPv6=%v", manager.configuration.TunnelRoutesV4, manager.configuration.TunnelRoutesV6)
			}

			for _, check := range []struct {
				name  string
				calls [][]string
				want  []string
				count int
			}{
				{name: "add IPv4", calls: ipMock.addedSplits4, want: test.wantV4, count: 1},
				{name: "add IPv6", calls: ipMock.addedSplits6, want: test.wantV6, count: 1},
				{name: "delete IPv4", calls: ipMock.deletedSplits4, count: 0},
				{name: "delete IPv6", calls: ipMock.deletedSplits6, count: 0},
			} {
				if len(check.calls) != check.count {
					t.Fatalf("%s calls = %v, want %d calls", check.name, check.calls, check.count)
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

func TestLinuxManagerDeletesInterfacesWithDifferentSubnets(t *testing.T) {
	manager, err := New(&client.Configuration{
		ClientID: 1,
		Protocol: settings.UDP,
		TCPSettings: settings.Settings{
			Network: settings.Network{
				TunName:    "tun1",
				IPv4Subnet: mustPrefix("10.1.0.17/24"),
				IPv6Subnet: mustPrefix("fd01::17/64"),
			},
		},
		UDPSettings: settings.Settings{
			Network: settings.Network{
				TunName:    "tun0",
				IPv4Subnet: mustPrefix("10.0.0.17/24"),
				IPv6Subnet: mustPrefix("fd00::17/64"),
			},
			MTU: settings.DefaultMTU,
		},
		TunnelRoutesV4: []string{"10.0.0.0/24", "10.1.0.0/24", "192.0.2.0/24"},
		TunnelRoutesV6: []string{"fd00::/64", "fd01::/64", "2001:db8::/64"},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ipMock := &clienttunManagerIPMock{}
	manager.ip = ipMock
	manager.dns = &clienttunManagerDNSMock{}
	manager.mss = clienttunManagerMSSMock{}
	if err := manager.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}

	if want := []string{"tun1", "tun0"}; !slices.Equal(ipMock.deletedInterfaces, want) {
		t.Errorf("deleted interfaces = %v, want %v", ipMock.deletedInterfaces, want)
	}
	if len(ipMock.deletedSplits4) != 0 || len(ipMock.deletedSplits6) != 0 {
		t.Fatalf("CloseTunnel() explicitly deleted split routes: IPv4=%v IPv6=%v", ipMock.deletedSplits4, ipMock.deletedSplits6)
	}
}

func TestNewLinuxManager(t *testing.T) {
	configuration := &client.Configuration{
		ClientID: 1,
		Protocol: settings.UDP,
		UDPSettings: settings.Settings{
			Network: settings.Network{
				TunName:    "tun0",
				IPv4Subnet: mustPrefix("10.0.0.0/24"),
			},
			MTU: settings.DefaultMTU,
		},
	}

	manager, err := New(configuration)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if manager.configuration != configuration || manager.settings.Protocol != settings.UDP ||
		manager.dns == nil || manager.ip == nil || manager.ioctl == nil || manager.mss == nil {
		t.Fatalf("New() returned incomplete manager: %+v", manager)
	}

	configuration.Protocol = settings.UNKNOWN
	if manager, err := New(configuration); err == nil || manager != nil {
		t.Fatalf("New(invalid configuration) = %v, %v; want nil and error", manager, err)
	}
}

func TestOpenTunnel_UDP_WithGateway(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
	var installedFamilies []mssclamp.Families
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{
		installedFamilies: &installedFamilies,
	})

	dev, err := m.OpenTunnel(testServerAddrV4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev == nil {
		t.Fatal("nil device returned")
	}
	defer func() { _ = m.CloseTunnel() }()

	want := "up;addr;rreplacevia;split;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call sequence mismatch\nwant %s\ngot  %s", want, got)
	}
	if len(installedFamilies) != 1 || installedFamilies[0] != (mssclamp.Families{IPv4: true}) {
		t.Fatalf("MSS families = %v, want IPv4 only", installedFamilies)
	}
}

func TestOpenTunnelExcludesOnlyCurrentServerRouteOnReconnect(t *testing.T) {
	routesV4 := []string{"128.0.0.0/1", "198.51.100.0/24", "198.51.100.1/32", "198.51.100.2/32"}
	routesV6 := []string{"::/1", "2001:db8::/64", "2001:db8::1/128", "2001:db8::2/128"}
	configuration := &client.Configuration{
		ClientID: 1,
		Protocol: settings.UDP,
		UDPSettings: settings.Settings{
			Network: settings.Network{
				TunName:    "tun0",
				IPv4Subnet: mustPrefix("10.0.0.0/24"),
				IPv6Subnet: mustPrefix("fd00::/64"),
			},
			MTU: settings.DefaultMTU,
		},
		TunnelRoutesV4: slices.Clone(routesV4),
		TunnelRoutesV6: slices.Clone(routesV6),
	}
	manager, err := New(configuration)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ipMock := &clienttunManagerIPMock{}
	manager.ip = ipMock
	manager.ioctl = clienttunManagerIOCTLMock{}
	manager.dns = &clienttunManagerDNSMock{}
	manager.mss = clienttunManagerMSSMock{}

	for _, test := range []struct {
		server string
		wantV4 []string
		wantV6 []string
	}{
		{
			server: "198.51.100.1",
			wantV4: []string{"128.0.0.0/1", "198.51.100.0/24", "198.51.100.2/32"},
			wantV6: routesV6,
		},
		{
			server: "198.51.100.2",
			wantV4: []string{"128.0.0.0/1", "198.51.100.0/24", "198.51.100.1/32"},
			wantV6: routesV6,
		},
		{
			server: "2001:db8::1",
			wantV4: routesV4,
			wantV6: []string{"::/1", "2001:db8::/64", "2001:db8::2/128"},
		},
		{
			server: "2001:db8::2",
			wantV4: routesV4,
			wantV6: []string{"::/1", "2001:db8::/64", "2001:db8::1/128"},
		},
		{
			server: "::ffff:198.51.100.1",
			wantV4: []string{"128.0.0.0/1", "198.51.100.0/24", "198.51.100.2/32"},
			wantV6: routesV6,
		},
		{
			server: "203.0.113.1",
			wantV4: routesV4,
			wantV6: routesV6,
		},
	} {
		t.Run(test.server, func(t *testing.T) {
			serverAddr := mustAddr(test.server)
			ipMock.routeReply = serverAddr.Unmap().String() + " dev eth0"
			ipMock.addedSplits4, ipMock.addedSplits6 = nil, nil
			if _, err := manager.OpenTunnel(serverAddr); err != nil {
				t.Fatalf("OpenTunnel() error = %v", err)
			}
			t.Cleanup(func() {
				if err := manager.CloseTunnel(); err != nil {
					t.Errorf("CloseTunnel() error = %v", err)
				}
			})
			if !reflect.DeepEqual(ipMock.addedSplits4, [][]string{test.wantV4}) ||
				!reflect.DeepEqual(ipMock.addedSplits6, [][]string{test.wantV6}) {
				t.Errorf("installed routes: IPv4=%v IPv6=%v, want IPv4=%v IPv6=%v",
					ipMock.addedSplits4, ipMock.addedSplits6, test.wantV4, test.wantV6)
			}
			if manager.pinnedServerAddr != serverAddr.Unmap() {
				t.Errorf("pinned server = %s, want %s", manager.pinnedServerAddr, serverAddr.Unmap())
			}
			if !slices.Equal(manager.splitsv4, routesV4) || !slices.Equal(manager.splitsv6, routesV6) {
				t.Errorf("manager changed stored routes: IPv4=%v IPv6=%v", manager.splitsv4, manager.splitsv6)
			}
			if !slices.Equal(configuration.TunnelRoutesV4, routesV4) || !slices.Equal(configuration.TunnelRoutesV6, routesV6) {
				t.Errorf("manager changed configured routes: IPv4=%v IPv6=%v", configuration.TunnelRoutesV4, configuration.TunnelRoutesV6)
			}
		})
	}
}

func TestOpenTunnelConfiguresAndRestoresDNS(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	dnsMock := &clienttunManagerDNSMock{}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
	m.dns = dnsMock

	if _, err := m.OpenTunnel(testServerAddrV4); err != nil {
		t.Fatalf("OpenTunnel() error = %v", err)
	}
	if !reflect.DeepEqual(dnsMock.setInterfaces, []string{"tun0"}) ||
		!reflect.DeepEqual(dnsMock.setResolvers4, [][]string{{"1.1.1.1", "8.8.8.8"}}) ||
		!reflect.DeepEqual(dnsMock.setResolvers6, [][]string{nil}) {
		t.Fatalf(
			"DNS setup = interfaces %v IPv4 %v IPv6 %v",
			dnsMock.setInterfaces,
			dnsMock.setResolvers4,
			dnsMock.setResolvers6,
		)
	}

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if dnsMock.revertCalls != 1 {
		t.Fatalf("DNS Revert() calls = %d, want 1", dnsMock.revertCalls)
	}
}

func TestOpenTunnelContinuesWhenDNSSetupFails(t *testing.T) {
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	dnsMock := &clienttunManagerDNSMock{setErr: errors.New("DNS failed")}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
	m.dns = dnsMock

	dev, err := m.OpenTunnel(testServerAddrV4)
	if err != nil || dev == nil {
		t.Fatalf("OpenTunnel() = %v, %v; want working degraded tunnel", dev, err)
	}
	if dnsMock.revertCalls != 0 || m.tun == nil {
		t.Fatalf("degraded state: Revert calls=%d TUN=%v", dnsMock.revertCalls, m.tun)
	}
	if !strings.Contains(logs.String(), "failed to configure DNS") ||
		!strings.Contains(logs.String(), "DNS failed") {
		t.Fatalf("DNS degradation log = %q", logs.String())
	}

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if dnsMock.revertCalls != 1 {
		t.Fatalf("DNS Revert() calls = %d, want 1", dnsMock.revertCalls)
	}
}

func TestOpenTunnelRejectsInvalidServerAddr(t *testing.T) {
	ipMock := &clienttunManagerIPMock{}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	if _, err := m.OpenTunnel(netip.Addr{}); err == nil || !strings.Contains(err.Error(), "invalid server address") {
		t.Fatalf("OpenTunnel() error = %v, want invalid server address", err)
	}
	if ipMock.log.Len() != 0 {
		t.Fatalf("OpenTunnel() configured TUN before validation: %q", ipMock.log.String())
	}
}

func TestOpenTunnelNormalizesIPv4MappedServerAddr(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	if _, err := m.OpenTunnel(mustAddr("::ffff:198.51.100.1")); err != nil {
		t.Fatalf("OpenTunnel() error = %v", err)
	}
	if got := ipMock.routeGetTargets; len(got) != 1 || got[0] != testServerAddrV4 {
		t.Fatalf("RouteGet() targets = %v, want [%s]", got, testServerAddrV4)
	}
	if m.pinnedServerAddr != testServerAddrV4 {
		t.Fatalf("pinnedServerAddr = %s, want %s", m.pinnedServerAddr, testServerAddrV4)
	}
	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if got := ipMock.routeDelTargets; len(got) != 1 || got[0] != testServerAddrV4 {
		t.Fatalf("RouteDel() targets = %v, want [%s]", got, testServerAddrV4)
	}
}

func TestOpenTunnel_TCP_NoGateway(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "203.0.113.1 dev eth0"} // no "via"
	m := newMgr(settings.TCP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	dev, err := m.OpenTunnel(testServerAddrV4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev == nil {
		t.Fatal("nil device returned")
	}
	defer func() { _ = m.CloseTunnel() }()

	want := "up;addr;rreplace;split;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call sequence mismatch\nwant %s\ngot  %s", want, got)
	}
}

func TestOpenTunnel_WS_Path(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "203.0.113.2 dev eth0"}
	m := newMgr(settings.WS, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	dev, err := m.OpenTunnel(testServerAddrV4)
	if err != nil {
		t.Fatalf("WS path failed: %v", err)
	}
	if dev == nil {
		t.Fatal("nil device returned")
	}
	defer func() { _ = m.CloseTunnel() }()
}

func TestOpenTunnel_ParseRouteError_NoDev(t *testing.T) {
	// Missing "dev" -> parse must fail.
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1"}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	if _, err := m.OpenTunnel(testServerAddrV4); err == nil {
		t.Fatal("expected parse error (no dev)")
	} else if !strings.Contains(err.Error(), "failed to parse route to server IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenTunnel_ParseRouteError_InvalidGateway(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 via invalid dev eth0"}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	if _, err := m.OpenTunnel(testServerAddrV4); err == nil {
		t.Fatal("expected invalid gateway error")
	} else if !strings.Contains(err.Error(), "failed to parse route gateway") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenTunnel_RouteGetError(t *testing.T) {
	ipMock := &clienttunManagerIPGetErr{}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	if _, err := m.OpenTunnel(testServerAddrV4); err == nil {
		t.Fatal("expected RouteGet error")
	} else if !strings.Contains(err.Error(), "failed to get route to server IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenTunnel_OpenTunError(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{openErr: errors.New("open fail")}, clienttunManagerMSSMock{})

	if _, err := m.OpenTunnel(testServerAddrV4); err == nil {
		t.Fatal("expected open TUN error")
	} else if !strings.Contains(err.Error(), "failed to open TUN interface") {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(ipMock.log.String(), "up;") || len(ipMock.routeGetTargets) != 0 {
		t.Fatalf("TUN configuration started before opening the device: log=%q routes=%v", ipMock.log.String(), ipMock.routeGetTargets)
	}
	if len(ipMock.routeDelTargets) != 0 || m.pinnedServerAddr.IsValid() {
		t.Fatalf("open failure changed pinned route state: pinned=%s deleted=%v", m.pinnedServerAddr, ipMock.routeDelTargets)
	}
}

func TestOpenTunnelReturnsInterfaceCleanupError(t *testing.T) {
	openErr := errors.New("open TUN failed")
	deleteErr := errors.New("delete interface failed")
	mssErr := errors.New("remove MSS clamping failed")
	m := newMgr(
		settings.UDP,
		&clienttunManagerIPMock{linkDeleteErr: deleteErr},
		clienttunManagerIOCTLMock{openErr: openErr},
		clienttunManagerMSSMock{removeErr: mssErr},
	)

	_, err := m.OpenTunnel(testServerAddrV4)
	for _, wantErr := range []error{openErr, deleteErr, mssErr} {
		if !errors.Is(err, wantErr) {
			t.Errorf("OpenTunnel() error = %v, want %v", err, wantErr)
		}
	}
}

func TestOpenTunnel_EpollErrorClosesTunFile(t *testing.T) {
	tunFile, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open test TUN file: %v", err)
	}
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newMgr(
		settings.UDP,
		ipMock,
		clienttunManagerIOCTLMock{file: tunFile},
		clienttunManagerMSSMock{},
	)

	if _, err := m.OpenTunnel(testServerAddrV4); err == nil || !strings.Contains(err.Error(), "failed to initialize TUN I/O") {
		t.Fatalf("OpenTunnel() error = %v, want epoll initialization error", err)
	}
	if _, err := tunFile.Stat(); err == nil {
		t.Fatal("OpenTunnel() left TUN file open after epoll initialization error")
	}
	if strings.Contains(ipMock.log.String(), "up;") || len(ipMock.routeGetTargets) != 0 {
		t.Fatalf("TUN configuration started before initializing I/O: log=%q routes=%v", ipMock.log.String(), ipMock.routeGetTargets)
	}
	if len(ipMock.routeDelTargets) != 0 || m.pinnedServerAddr.IsValid() {
		t.Fatalf("epoll failure changed pinned route state: pinned=%s deleted=%v", m.pinnedServerAddr, ipMock.routeDelTargets)
	}
}

func TestOpenTunnelCreatesTunBeforeConfiguringLink(t *testing.T) {
	ipMock := &clienttunManagerIPMock{failStep: "up"}
	createCalls := 0
	m := newMgr(
		settings.UDP,
		ipMock,
		clienttunManagerIOCTLMock{createCalls: &createCalls},
		clienttunManagerMSSMock{},
	)

	if _, err := m.OpenTunnel(testServerAddrV4); err == nil {
		t.Fatal("expected link configuration error")
	}
	if createCalls != 1 {
		t.Fatalf("CreateTunInterface() calls = %d, want 1 before link configuration", createCalls)
	}
}

func TestConfigureTUNErrorRollback(t *testing.T) {
	paths := []struct {
		name       string
		routeReply string
		steps      []string
	}{
		{
			name:       "on-link server",
			routeReply: "198.51.100.1 dev eth0",
			steps:      []string{"up", "addr", "rreplace", "split", "mtu"},
		},
		{
			name:       "server via gateway",
			routeReply: "198.51.100.1 via 192.0.2.1 dev eth0",
			steps:      []string{"up", "addr", "rreplacevia", "split", "mtu"},
		},
	}

	for _, path := range paths {
		t.Run(path.name, func(t *testing.T) {
			for _, step := range path.steps {
				t.Run(step, func(t *testing.T) {
					ipMock := &clienttunManagerIPMock{routeReply: path.routeReply, failStep: step}
					m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
					if _, err := m.OpenTunnel(testServerAddrV4); err == nil {
						t.Fatalf("expected error on step %s", step)
					}
					if m.pinnedServerAddr.IsValid() {
						t.Fatalf("failed step %s retained pinned route state", step)
					}
					wantRouteDeletes := 0
					if step == "split" || step == "mtu" {
						wantRouteDeletes = 1
					}
					if len(ipMock.routeDelTargets) != wantRouteDeletes {
						t.Fatalf("failed step %s route deletions = %v, want %d", step, ipMock.routeDelTargets, wantRouteDeletes)
					}
				})
			}
		})
	}
}

func TestCloseTunnelCleansEveryConfiguredProfile(t *testing.T) {
	ipMock := &clienttunManagerIPMock{}
	var removedTunNames []string
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{
		removedTunNames: &removedTunNames,
	})

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel error: %v", err)
	}
	if got, want := strings.Join(removedTunNames, ","), "tun1,tun0,tun2"; got != want {
		t.Fatalf("MSS cleanup interfaces = %q, want %q", got, want)
	}
	if got := strings.Count(ipMock.log.String(), "ldel;"); got != 3 {
		t.Fatalf("LinkDelete() calls = %d, want 3", got)
	}
}

func TestConfigureTUN_MSSInstallError(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	var removedTunNames []string
	mssMock := clienttunManagerMSSMock{
		installErr:      errors.New("iptables fail"),
		removedTunNames: &removedTunNames,
	}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, mssMock)

	_, err := m.OpenTunnel(testServerAddrV4)
	if err == nil {
		t.Fatal("expected MSS install error")
	}
	if !strings.Contains(err.Error(), "failed to install MSS clamping") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertOpenTunnelRolledBack(t, m, ipMock)
	if got, want := strings.Join(removedTunNames, ","), "tun1,tun0,tun2"; got != want {
		t.Fatalf("MSS rollback interfaces = %q, want %q", got, want)
	}
}

func TestOpenTunnel_IPv6_FullPath(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "2001:db8::1 via fe80::1 dev eth0"}
	var installedFamilies []mssclamp.Families
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{
		installedFamilies: &installedFamilies,
	})

	// Enable IPv6 on the active protocol's settings.
	mgr.settings.IPv6 = mustAddr("fd00::2")
	mgr.settings.IPv6Subnet = mustPrefix("fd00::/64")
	mgr.settings.DNSv6 = []string{"2606:4700:4700::1111", "2001:4860:4860::8888"}
	mgr.configuration.UDPSettings = mgr.settings

	_, err := mgr.OpenTunnel(testServerAddrV6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = mgr.CloseTunnel() }()

	got := ipMock.log.String()
	if !strings.Contains(got, "split6;") {
		t.Fatalf("expected IPv6 split route step, got: %s", got)
	}
	// Two "addr;" calls: one for IPv4, one for IPv6
	if strings.Count(got, "addr;") != 2 {
		t.Fatalf("expected 2 addr calls (IPv4 + IPv6), got: %s", got)
	}
	if len(ipMock.routeGetTargets) != 1 || ipMock.routeGetTargets[0] != testServerAddrV6 {
		t.Fatalf("route lookup targets = %v, want [%s]", ipMock.routeGetTargets, testServerAddrV6)
	}
	if len(ipMock.routeReplaceTargets) != 1 || ipMock.routeReplaceTargets[0] != testServerAddrV6 {
		t.Fatalf("route replace targets = %v, want [%s]", ipMock.routeReplaceTargets, testServerAddrV6)
	}
	if len(installedFamilies) != 1 || installedFamilies[0] != (mssclamp.Families{IPv4: true, IPv6: true}) {
		t.Fatalf("MSS families = %v, want dual stack", installedFamilies)
	}
}

func TestOpenTunnel_IPv6Only_FullPath(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "2001:db8::1 via fe80::1 dev eth0"}
	var installedFamilies []mssclamp.Families
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{
		installedFamilies: &installedFamilies,
	})
	active := mgr.settings
	active.IPv4 = netip.Addr{}
	active.IPv4Subnet = netip.Prefix{}
	active.IPv6 = mustAddr("fd00::2")
	active.IPv6Subnet = mustPrefix("fd00::/64")
	setLinuxActiveSettings(mgr, active)

	if _, err := mgr.OpenTunnel(testServerAddrV6); err != nil {
		t.Fatalf("OpenTunnel() error = %v", err)
	}
	defer func() { _ = mgr.CloseTunnel() }()

	want := "up;addr;rreplacevia;split6;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	if got := ipMock.routeGetTargets; len(got) != 1 || got[0] != testServerAddrV6 {
		t.Fatalf("RouteGet() targets = %v, want [%s]", got, testServerAddrV6)
	}
	if len(installedFamilies) != 1 || installedFamilies[0] != (mssclamp.Families{IPv6: true}) {
		t.Fatalf("MSS families = %v, want IPv6 only", installedFamilies)
	}
}

func TestOpenTunnelSingleStackSkipsUncoveredServerFamily(t *testing.T) {
	ipMock := &clienttunManagerIPMock{}
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	if _, err := mgr.OpenTunnel(testServerAddrV6); err != nil {
		t.Fatalf("OpenTunnel() error = %v", err)
	}
	defer func() { _ = mgr.CloseTunnel() }()

	if len(ipMock.routeGetTargets) != 0 || len(ipMock.routeReplaceTargets) != 0 || mgr.pinnedServerAddr.IsValid() {
		t.Fatalf("unexpected pinned route: get=%v replace=%v cached=%s", ipMock.routeGetTargets, ipMock.routeReplaceTargets, mgr.pinnedServerAddr)
	}
}

func TestOpenTunnel_IPv6_AddrAddError(t *testing.T) {
	// When IPv6 AddrAddDev fails, creation should fail.
	calls := 0
	ipMock := &clienttunManagerIPMockFailNthAddr{
		clienttunManagerIPMock: clienttunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"},
		failOnCall:             2,
		callCount:              &calls,
	}
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	active := mgr.settings
	active.IPv6 = mustAddr("fd00::2")
	active.IPv6Subnet = mustPrefix("fd00::/64")
	setLinuxActiveSettings(mgr, active)

	_, err := mgr.OpenTunnel(testServerAddrV4)
	if err == nil {
		t.Fatal("expected error on IPv6 addr add failure")
	}
	if mgr.pinnedServerAddr.IsValid() || len(ipMock.routeDelTargets) != 0 {
		t.Fatalf("failed address assignment changed pinned route state: pinned=%s deleted=%v", mgr.pinnedServerAddr, ipMock.routeDelTargets)
	}
}

func TestOpenTunnel_IPv6_Route6SplitError(t *testing.T) {
	ipMock := &clienttunManagerIPMock{
		routeReply: "198.51.100.1 dev eth0",
		failStep:   "split6",
	}
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
	active := mgr.settings
	active.IPv6 = mustAddr("fd00::2")
	active.IPv6Subnet = mustPrefix("fd00::/64")
	setLinuxActiveSettings(mgr, active)

	_, err := mgr.OpenTunnel(testServerAddrV4)
	if err == nil {
		t.Fatal("expected error on Route6AddSplitDev failure")
	}
	assertOpenTunnelRolledBack(t, mgr, ipMock)
}

func TestCloseTunnelWithoutOpenedServerSkipsHostRouteCleanup(t *testing.T) {
	ipMock := &clienttunManagerIPMock{}
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	if err := mgr.CloseTunnel(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ipMock.routeDelTargets) != 0 {
		t.Fatalf("unexpected host route deletions: %v", ipMock.routeDelTargets)
	}
}

func TestCloseTunnelCancelsDefaultRouteWatcher(t *testing.T) {
	mgr := newMgr(
		settings.UDP,
		&clienttunManagerIPMock{},
		clienttunManagerIOCTLMock{},
		clienttunManagerMSSMock{},
	)
	cancelled := false
	mgr.defaultRouteWatcherCancel = func() { cancelled = true }

	if err := mgr.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if !cancelled {
		t.Fatal("CloseTunnel() did not cancel the default route watcher")
	}
}

func TestCloseTunnelRemovesOpenedServerRoute(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
	_, err := mgr.OpenTunnel(testServerAddrV4)
	if err != nil {
		t.Fatalf("OpenTunnel() error = %v", err)
	}

	if err := mgr.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if len(ipMock.routeDelTargets) != 1 || ipMock.routeDelTargets[0] != testServerAddrV4 {
		t.Fatalf("deleted host routes = %v, want [%s]", ipMock.routeDelTargets, testServerAddrV4)
	}
}

func TestCloseTunnelRetriesServerRouteDeletion(t *testing.T) {
	ipMock := &clienttunManagerIPMock{
		routeReply: "198.51.100.1 dev eth0",
		failStep:   "rdel",
	}
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
	dev, err := mgr.OpenTunnel(testServerAddrV4)
	if err != nil {
		t.Fatalf("OpenTunnel() error = %v", err)
	}
	if err := mgr.CloseTunnel(); err == nil {
		t.Fatal("CloseTunnel() error = nil, want route deletion error")
	}
	if _, err := dev.Read(make([]byte, 1)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("TUN read after CloseTunnel() error = %v, want %v", err, io.ErrClosedPipe)
	}
	if mgr.tun != nil {
		t.Fatal("CloseTunnel() retained a closed TUN")
	}
	if mgr.pinnedServerAddr != testServerAddrV4 {
		t.Fatalf("pinnedServerAddr = %s, want retained %s", mgr.pinnedServerAddr, testServerAddrV4)
	}

	ipMock.failStep = ""
	if err := mgr.CloseTunnel(); err != nil {
		t.Fatalf("retry CloseTunnel() error = %v", err)
	}
	if mgr.pinnedServerAddr.IsValid() {
		t.Fatalf("pinnedServerAddr = %s, want cleared address", mgr.pinnedServerAddr)
	}
	if len(ipMock.routeDelTargets) != 2 {
		t.Fatalf("route deletion attempts = %d, want 2", len(ipMock.routeDelTargets))
	}
}

func TestCloseTunnelReturnsTunCloseErrorAndClearsTun(t *testing.T) {
	closeErr := errors.New("close failed")
	tun := &clientTunMock{closeErr: closeErr}
	mgr := newMgr(
		settings.UDP,
		&clienttunManagerIPMock{},
		clienttunManagerIOCTLMock{},
		clienttunManagerMSSMock{},
	)
	mgr.tun = tun

	if err := mgr.CloseTunnel(); !errors.Is(err, closeErr) {
		t.Fatalf("CloseTunnel() error = %v, want %v", err, closeErr)
	}
	if tun.closeCalls != 1 {
		t.Fatalf("TUN Close() calls = %d, want 1", tun.closeCalls)
	}
	if mgr.tun != nil {
		t.Fatal("CloseTunnel() retained TUN after Close returned an error")
	}
}

func TestCloseTunnelReturnsLinkDeleteErrorAndContinuesCleanup(t *testing.T) {
	deleteErr := errors.New("delete interface failed")
	ipMock := &clienttunManagerIPMock{linkDeleteErr: deleteErr}
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
	tun := &clientTunMock{}
	mgr.tun = tun
	mgr.pinnedServerAddr = testServerAddrV4

	if err := mgr.CloseTunnel(); !errors.Is(err, deleteErr) {
		t.Errorf("CloseTunnel() error = %v, want %v", err, deleteErr)
	}
	if tun.closeCalls != 1 || mgr.tun != nil {
		t.Errorf("TUN cleanup: close calls = %d, TUN = %v", tun.closeCalls, mgr.tun)
	}
	if !slices.Equal(ipMock.deletedInterfaces, []string{"tun1", "tun0", "tun2"}) {
		t.Errorf("deleted interfaces = %v, want all configured interfaces", ipMock.deletedInterfaces)
	}
	if !slices.Equal(ipMock.routeDelTargets, []netip.Addr{testServerAddrV4}) || mgr.pinnedServerAddr.IsValid() {
		t.Errorf("server route cleanup: deleted = %v, pinned = %v", ipMock.routeDelTargets, mgr.pinnedServerAddr)
	}
}

func TestCloseTunnelRetriesDNSRestore(t *testing.T) {
	dnsMock := &clienttunManagerDNSMock{revertErr: errors.New("restore failed")}
	mgr := newMgr(
		settings.UDP,
		&clienttunManagerIPMock{},
		clienttunManagerIOCTLMock{},
		clienttunManagerMSSMock{},
	)
	mgr.dns = dnsMock
	tun := &clientTunMock{}
	mgr.tun = tun

	if err := mgr.CloseTunnel(); err == nil || !strings.Contains(err.Error(), "restore failed") {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if mgr.tun != nil || tun.closeCalls != 1 || dnsMock.revertCalls != 1 {
		t.Fatalf("failed restore state: TUN=%v close calls=%d Revert calls=%d", mgr.tun, tun.closeCalls, dnsMock.revertCalls)
	}

	dnsMock.revertErr = nil
	if err := mgr.CloseTunnel(); err != nil {
		t.Fatalf("retry CloseTunnel() error = %v", err)
	}
	if dnsMock.revertCalls != 2 {
		t.Fatalf("DNS Revert() calls = %d, want 2", dnsMock.revertCalls)
	}
	if tun.closeCalls != 1 {
		t.Fatalf("TUN close calls = %d, want 1", tun.closeCalls)
	}
}

func TestCloseTunnelSkipsProfilesWithoutTunName(t *testing.T) {
	ipMock := &clienttunManagerIPMock{}
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
	mgr.configuration.TCPSettings.TunName = ""
	mgr.configuration.WSSettings.TunName = ""

	if err := mgr.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel() error = %v", err)
	}
	if got := strings.Count(ipMock.log.String(), "ldel;"); got != 1 {
		t.Fatalf("LinkDelete() calls = %d, want 1", got)
	}
}

// clienttunManagerIPMockFailNthAddr fails AddrAddDev on the N-th call.
type clienttunManagerIPMockFailNthAddr struct {
	clienttunManagerIPMock
	failOnCall int
	callCount  *int
}

func (m *clienttunManagerIPMockFailNthAddr) AddrAddDev(dev, cidr string) error {
	*m.callCount++
	if *m.callCount == m.failOnCall {
		return errors.New("addr add failed")
	}
	return nil
}

func TestCloseTunnelReturnsMSSRemoveError(t *testing.T) {
	cleanupErr := errors.New("cleanup fail")
	ipMock := &clienttunManagerIPMock{}
	mssMock := clienttunManagerMSSMock{removeErr: cleanupErr}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, mssMock)

	if err := m.CloseTunnel(); !errors.Is(err, cleanupErr) {
		t.Fatalf("CloseTunnel() error = %v, want %v", err, cleanupErr)
	}
}
