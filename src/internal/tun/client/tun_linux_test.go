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

// clientTUNIPMock simulates `ip` contract and records call sequence.
// `failStep` makes the corresponding step return an error.
type clientTUNIPMock struct {
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

func (m *clientTUNIPMock) mark(s string) error {
	m.log.WriteString(s + ";")
	if m.failStep == s {
		return errors.New("boom")
	}
	return nil
}

func (m *clientTUNIPMock) TunTapAddDevTun(string) error { return m.mark("add") }
func (m *clientTUNIPMock) LinkDelete(devName string) error {
	m.deletedInterfaces = append(m.deletedInterfaces, devName)
	m.log.WriteString("ldel;")
	return m.linkDeleteErr
}
func (m *clientTUNIPMock) LinkSetDevUp(string) error       { return m.mark("up") }
func (m *clientTUNIPMock) LinkSetDevMTU(string, int) error { return m.mark("mtu") }
func (m *clientTUNIPMock) AddrAddDev(string, string) error { return m.mark("addr") }
func (m *clientTUNIPMock) RouteDefault() (string, error)   { return "eth0", nil }
func (m *clientTUNIPMock) RouteGet(target netip.Addr) (string, error) {
	m.routeGetTargets = append(m.routeGetTargets, target)
	return m.routeReply, nil
}
func (m *clientTUNIPMock) RouteReplaceDev(target netip.Addr, _ string) error {
	m.routeReplaceTargets = append(m.routeReplaceTargets, target)
	return m.mark("rreplace")
}
func (m *clientTUNIPMock) RouteReplaceViaDev(target netip.Addr, _ string, _ netip.Addr) error {
	m.routeReplaceTargets = append(m.routeReplaceTargets, target)
	return m.mark("rreplacevia")
}
func (m *clientTUNIPMock) RouteAddSplitDev(_ string, prefixes []string) error {
	m.addedSplits4 = append(m.addedSplits4, slices.Clone(prefixes))
	return m.mark("split")
}
func (m *clientTUNIPMock) Route6AddSplitDev(_ string, prefixes []string) error {
	m.addedSplits6 = append(m.addedSplits6, slices.Clone(prefixes))
	return m.mark("split6")
}
func (m *clientTUNIPMock) RouteDelSplitDefault(_ string, prefixes []string) error {
	m.deletedSplits4 = append(m.deletedSplits4, slices.Clone(prefixes))
	m.log.WriteString("splitdel;")
	return nil
}
func (m *clientTUNIPMock) Route6DelSplitDefault(_ string, prefixes []string) error {
	m.deletedSplits6 = append(m.deletedSplits6, slices.Clone(prefixes))
	m.log.WriteString("splitdel6;")
	return nil
}
func (m *clientTUNIPMock) RouteDel(target netip.Addr) error {
	m.routeDelTargets = append(m.routeDelTargets, target)
	return m.mark("rdel")
}

// clientTUNIPGetErr forces RouteGet to return an error.
type clientTUNIPGetErr struct{ clientTUNIPMock }

func (m *clientTUNIPGetErr) RouteGet(netip.Addr) (string, error) {
	return "", fmt.Errorf("failed to get route to server IP: %w", errors.New("geterr"))
}

// clientTUNIOCTLMock returns a pollable file or an injected error.
type clientTUNIOCTLMock struct {
	openErr     error
	file        *os.File
	createCalls *int
}

// clientTUNMSSMock simulates mssclamp.Contract.
type clientTUNMSSMock struct {
	installErr        error
	removeErr         error
	installedFamilies *[]mssclamp.Families
	removedTunNames   *[]string
}

type clientTUNDNSMock struct {
	setInterfaces []string
	setResolvers4 [][]string
	setResolvers6 [][]string
	revertCalls   int
	setErr        error
	revertErr     error
}

func (m *clientTUNDNSMock) Set(ifName string, ipv4Resolvers, ipv6Resolvers []string) error {
	m.setInterfaces = append(m.setInterfaces, ifName)
	m.setResolvers4 = append(m.setResolvers4, append([]string(nil), ipv4Resolvers...))
	m.setResolvers6 = append(m.setResolvers6, append([]string(nil), ipv6Resolvers...))
	return m.setErr
}

func (m *clientTUNDNSMock) Revert() error {
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

func (m clientTUNMSSMock) Install(_ string, families mssclamp.Families) error {
	if m.installedFamilies != nil {
		*m.installedFamilies = append(*m.installedFamilies, families)
	}
	return m.installErr
}
func (m clientTUNMSSMock) Remove(tunName string) error {
	if m.removedTunNames != nil {
		*m.removedTunNames = append(*m.removedTunNames, tunName)
	}
	return m.removeErr
}

func (clientTUNIOCTLMock) DetectTunNameFromFd(*os.File) (string, error) { return "tun0", nil }
func (m clientTUNIOCTLMock) CreateTunInterface(string) (*os.File, error) {
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

func newTestTUN(
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
) *TUN {
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
	return &TUN{
		configuration: conf,
		settings:      profiles[proto],
		dns:           &clientTUNDNSMock{},
		ip:            ipMock,
		ioctl:         ioctlMock,
		mss:           mssMock,
	}
}

func assertOpenRolledBack(t *testing.T, m *TUN, ipMock *clientTUNIPMock) {
	t.Helper()
	if m.pinnedServerAddr.IsValid() {
		t.Fatalf("pinnedServerAddr = %s, want cleared after failed Open", m.pinnedServerAddr)
	}
	if len(ipMock.routeDelTargets) != 1 || ipMock.routeDelTargets[0] != testServerAddrV4 {
		t.Fatalf("RouteDel() targets = %v, want [%s]", ipMock.routeDelTargets, testServerAddrV4)
	}
	if m.tun != nil {
		t.Fatal("failed Open retained TUN")
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

func setLinuxActiveSettings(m *TUN, active settings.Settings) {
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

func TestLinuxTUNAppliesTunnelRoutes(t *testing.T) {
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
			tunnel, err := New(&client.Configuration{
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
			ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
			tunnel.ip = ipMock
			tunnel.dns = &clientTUNDNSMock{}
			tunnel.mss = clientTUNMSSMock{}
			tun := &clientTunMock{}
			tunnel.tun = tun
			if err := tunnel.configureTunnel(testServerAddrV4); err != nil {
				t.Fatalf("configureTunnel() error = %v", err)
			}
			if err := tunnel.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}
			if tun.closeCalls != 1 || tunnel.tun != nil {
				t.Fatalf("close state: calls=%d tun=%v", tun.closeCalls, tunnel.tun)
			}

			if !slices.Equal(tunnel.configuration.TunnelRoutesV4, originalV4) ||
				!slices.Equal(tunnel.configuration.TunnelRoutesV6, originalV6) {
				t.Fatalf("tunnel changed configured TunnelRoutes: IPv4=%v IPv6=%v", tunnel.configuration.TunnelRoutesV4, tunnel.configuration.TunnelRoutesV6)
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

func TestLinuxTUNDeletesInterfacesWithDifferentSubnets(t *testing.T) {
	tunnel, err := New(&client.Configuration{
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
	ipMock := &clientTUNIPMock{}
	tunnel.ip = ipMock
	tunnel.dns = &clientTUNDNSMock{}
	tunnel.mss = clientTUNMSSMock{}
	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	if want := []string{"tun1", "tun0"}; !slices.Equal(ipMock.deletedInterfaces, want) {
		t.Errorf("deleted interfaces = %v, want %v", ipMock.deletedInterfaces, want)
	}
	if len(ipMock.deletedSplits4) != 0 || len(ipMock.deletedSplits6) != 0 {
		t.Fatalf("Close() explicitly deleted split routes: IPv4=%v IPv6=%v", ipMock.deletedSplits4, ipMock.deletedSplits6)
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

	tunnel, err := New(configuration)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if tunnel.configuration != configuration || tunnel.settings.Protocol != settings.UDP ||
		tunnel.dns == nil || tunnel.ip == nil || tunnel.ioctl == nil || tunnel.mss == nil {
		t.Fatalf("New() returned incomplete tunnel: %+v", tunnel)
	}

	configuration.Protocol = settings.UNKNOWN
	if tunnel, err := New(configuration); err == nil || tunnel != nil {
		t.Fatalf("New(invalid configuration) = %v, %v; want nil and error", tunnel, err)
	}
}

func TestOpen_UDP_WithGateway(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
	var installedFamilies []mssclamp.Families
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{
		installedFamilies: &installedFamilies,
	})

	dev, err := m.Open(testServerAddrV4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev == nil {
		t.Fatal("nil device returned")
	}
	defer func() { _ = m.Close() }()

	want := "up;addr;rreplacevia;split;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call sequence mismatch\nwant %s\ngot  %s", want, got)
	}
	if len(installedFamilies) != 1 || installedFamilies[0] != (mssclamp.Families{IPv4: true}) {
		t.Fatalf("MSS families = %v, want IPv4 only", installedFamilies)
	}
}

func TestOpenExcludesOnlyCurrentServerRouteOnReconnect(t *testing.T) {
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
	tunnel, err := New(configuration)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ipMock := &clientTUNIPMock{}
	tunnel.ip = ipMock
	tunnel.ioctl = clientTUNIOCTLMock{}
	tunnel.dns = &clientTUNDNSMock{}
	tunnel.mss = clientTUNMSSMock{}

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
			if _, err := tunnel.Open(serverAddr); err != nil {
				t.Fatalf("Open() error = %v", err)
			}
			t.Cleanup(func() {
				if err := tunnel.Close(); err != nil {
					t.Errorf("Close() error = %v", err)
				}
			})
			if !reflect.DeepEqual(ipMock.addedSplits4, [][]string{test.wantV4}) ||
				!reflect.DeepEqual(ipMock.addedSplits6, [][]string{test.wantV6}) {
				t.Errorf("installed routes: IPv4=%v IPv6=%v, want IPv4=%v IPv6=%v",
					ipMock.addedSplits4, ipMock.addedSplits6, test.wantV4, test.wantV6)
			}
			if tunnel.pinnedServerAddr != serverAddr.Unmap() {
				t.Errorf("pinned server = %s, want %s", tunnel.pinnedServerAddr, serverAddr.Unmap())
			}
			if !slices.Equal(tunnel.splitsv4, routesV4) || !slices.Equal(tunnel.splitsv6, routesV6) {
				t.Errorf("tunnel changed stored routes: IPv4=%v IPv6=%v", tunnel.splitsv4, tunnel.splitsv6)
			}
			if !slices.Equal(configuration.TunnelRoutesV4, routesV4) || !slices.Equal(configuration.TunnelRoutesV6, routesV6) {
				t.Errorf("tunnel changed configured routes: IPv4=%v IPv6=%v", configuration.TunnelRoutesV4, configuration.TunnelRoutesV6)
			}
		})
	}
}

func TestOpenConfiguresAndRestoresDNS(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 dev eth0"}
	dnsMock := &clientTUNDNSMock{}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})
	m.dns = dnsMock

	if _, err := m.Open(testServerAddrV4); err != nil {
		t.Fatalf("Open() error = %v", err)
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

	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if dnsMock.revertCalls != 1 {
		t.Fatalf("DNS Revert() calls = %d, want 1", dnsMock.revertCalls)
	}
}

func TestOpenContinuesWhenDNSSetupFails(t *testing.T) {
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 dev eth0"}
	dnsMock := &clientTUNDNSMock{setErr: errors.New("DNS failed")}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})
	m.dns = dnsMock

	dev, err := m.Open(testServerAddrV4)
	if err != nil || dev == nil {
		t.Fatalf("Open() = %v, %v; want working degraded tunnel", dev, err)
	}
	if dnsMock.revertCalls != 0 || m.tun == nil {
		t.Fatalf("degraded state: Revert calls=%d TUN=%v", dnsMock.revertCalls, m.tun)
	}
	if !strings.Contains(logs.String(), "failed to configure DNS") ||
		!strings.Contains(logs.String(), "DNS failed") {
		t.Fatalf("DNS degradation log = %q", logs.String())
	}

	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if dnsMock.revertCalls != 1 {
		t.Fatalf("DNS Revert() calls = %d, want 1", dnsMock.revertCalls)
	}
}

func TestOpenRejectsInvalidServerAddr(t *testing.T) {
	ipMock := &clientTUNIPMock{}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	if _, err := m.Open(netip.Addr{}); err == nil || !strings.Contains(err.Error(), "invalid server address") {
		t.Fatalf("Open() error = %v, want invalid server address", err)
	}
	if ipMock.log.Len() != 0 {
		t.Fatalf("Open() configured TUN before validation: %q", ipMock.log.String())
	}
}

func TestOpenNormalizesIPv4MappedServerAddr(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	if _, err := m.Open(mustAddr("::ffff:198.51.100.1")); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if got := ipMock.routeGetTargets; len(got) != 1 || got[0] != testServerAddrV4 {
		t.Fatalf("RouteGet() targets = %v, want [%s]", got, testServerAddrV4)
	}
	if m.pinnedServerAddr != testServerAddrV4 {
		t.Fatalf("pinnedServerAddr = %s, want %s", m.pinnedServerAddr, testServerAddrV4)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := ipMock.routeDelTargets; len(got) != 1 || got[0] != testServerAddrV4 {
		t.Fatalf("RouteDel() targets = %v, want [%s]", got, testServerAddrV4)
	}
}

func TestOpen_TCP_NoGateway(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "203.0.113.1 dev eth0"} // no "via"
	m := newTestTUN(settings.TCP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	dev, err := m.Open(testServerAddrV4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev == nil {
		t.Fatal("nil device returned")
	}
	defer func() { _ = m.Close() }()

	want := "up;addr;rreplace;split;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call sequence mismatch\nwant %s\ngot  %s", want, got)
	}
}

func TestOpen_WS_Path(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "203.0.113.2 dev eth0"}
	m := newTestTUN(settings.WS, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	dev, err := m.Open(testServerAddrV4)
	if err != nil {
		t.Fatalf("WS path failed: %v", err)
	}
	if dev == nil {
		t.Fatal("nil device returned")
	}
	defer func() { _ = m.Close() }()
}

func TestOpen_ParseRouteError_NoDev(t *testing.T) {
	// Missing "dev" -> parse must fail.
	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 via 192.0.2.1"}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	if _, err := m.Open(testServerAddrV4); err == nil {
		t.Fatal("expected parse error (no dev)")
	} else if !strings.Contains(err.Error(), "failed to parse route to server IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpen_ParseRouteError_InvalidGateway(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 via invalid dev eth0"}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	if _, err := m.Open(testServerAddrV4); err == nil {
		t.Fatal("expected invalid gateway error")
	} else if !strings.Contains(err.Error(), "failed to parse route gateway") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpen_RouteGetError(t *testing.T) {
	ipMock := &clientTUNIPGetErr{}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	if _, err := m.Open(testServerAddrV4); err == nil {
		t.Fatal("expected RouteGet error")
	} else if !strings.Contains(err.Error(), "failed to get route to server IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpen_OpenTunError(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{openErr: errors.New("open fail")}, clientTUNMSSMock{})

	if _, err := m.Open(testServerAddrV4); err == nil {
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

func TestOpenReturnsInterfaceCleanupError(t *testing.T) {
	openErr := errors.New("open TUN failed")
	deleteErr := errors.New("delete interface failed")
	mssErr := errors.New("remove MSS clamping failed")
	m := newTestTUN(
		settings.UDP,
		&clientTUNIPMock{linkDeleteErr: deleteErr},
		clientTUNIOCTLMock{openErr: openErr},
		clientTUNMSSMock{removeErr: mssErr},
	)

	_, err := m.Open(testServerAddrV4)
	for _, wantErr := range []error{openErr, deleteErr, mssErr} {
		if !errors.Is(err, wantErr) {
			t.Errorf("Open() error = %v, want %v", err, wantErr)
		}
	}
}

func TestOpen_EpollErrorClosesTunFile(t *testing.T) {
	tunFile, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("open test TUN file: %v", err)
	}
	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newTestTUN(
		settings.UDP,
		ipMock,
		clientTUNIOCTLMock{file: tunFile},
		clientTUNMSSMock{},
	)

	if _, err := m.Open(testServerAddrV4); err == nil || !strings.Contains(err.Error(), "failed to initialize TUN I/O") {
		t.Fatalf("Open() error = %v, want epoll initialization error", err)
	}
	if _, err := tunFile.Stat(); err == nil {
		t.Fatal("Open() left TUN file open after epoll initialization error")
	}
	if strings.Contains(ipMock.log.String(), "up;") || len(ipMock.routeGetTargets) != 0 {
		t.Fatalf("TUN configuration started before initializing I/O: log=%q routes=%v", ipMock.log.String(), ipMock.routeGetTargets)
	}
	if len(ipMock.routeDelTargets) != 0 || m.pinnedServerAddr.IsValid() {
		t.Fatalf("epoll failure changed pinned route state: pinned=%s deleted=%v", m.pinnedServerAddr, ipMock.routeDelTargets)
	}
}

func TestOpenCreatesTunBeforeConfiguringLink(t *testing.T) {
	ipMock := &clientTUNIPMock{failStep: "up"}
	createCalls := 0
	m := newTestTUN(
		settings.UDP,
		ipMock,
		clientTUNIOCTLMock{createCalls: &createCalls},
		clientTUNMSSMock{},
	)

	if _, err := m.Open(testServerAddrV4); err == nil {
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
					ipMock := &clientTUNIPMock{routeReply: path.routeReply, failStep: step}
					m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})
					if _, err := m.Open(testServerAddrV4); err == nil {
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

func TestCloseCleansEveryConfiguredProfile(t *testing.T) {
	ipMock := &clientTUNIPMock{}
	var removedTunNames []string
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{
		removedTunNames: &removedTunNames,
	})

	if err := m.Close(); err != nil {
		t.Fatalf("Close error: %v", err)
	}
	if got, want := strings.Join(removedTunNames, ","), "tun1,tun0,tun2"; got != want {
		t.Fatalf("MSS cleanup interfaces = %q, want %q", got, want)
	}
	if got := strings.Count(ipMock.log.String(), "ldel;"); got != 3 {
		t.Fatalf("LinkDelete() calls = %d, want 3", got)
	}
}

func TestConfigureTUN_MSSInstallError(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 dev eth0"}
	var removedTunNames []string
	mssMock := clientTUNMSSMock{
		installErr:      errors.New("iptables fail"),
		removedTunNames: &removedTunNames,
	}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, mssMock)

	_, err := m.Open(testServerAddrV4)
	if err == nil {
		t.Fatal("expected MSS install error")
	}
	if !strings.Contains(err.Error(), "failed to install MSS clamping") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertOpenRolledBack(t, m, ipMock)
	if got, want := strings.Join(removedTunNames, ","), "tun1,tun0,tun2"; got != want {
		t.Fatalf("MSS rollback interfaces = %q, want %q", got, want)
	}
}

func TestOpen_IPv6_FullPath(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "2001:db8::1 via fe80::1 dev eth0"}
	var installedFamilies []mssclamp.Families
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{
		installedFamilies: &installedFamilies,
	})

	// Enable IPv6 on the active protocol's settings.
	tunnel.settings.IPv6 = mustAddr("fd00::2")
	tunnel.settings.IPv6Subnet = mustPrefix("fd00::/64")
	tunnel.settings.DNSv6 = []string{"2606:4700:4700::1111", "2001:4860:4860::8888"}
	tunnel.configuration.UDPSettings = tunnel.settings

	_, err := tunnel.Open(testServerAddrV6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = tunnel.Close() }()

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

func TestOpen_IPv6Only_FullPath(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "2001:db8::1 via fe80::1 dev eth0"}
	var installedFamilies []mssclamp.Families
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{
		installedFamilies: &installedFamilies,
	})
	active := tunnel.settings
	active.IPv4 = netip.Addr{}
	active.IPv4Subnet = netip.Prefix{}
	active.IPv6 = mustAddr("fd00::2")
	active.IPv6Subnet = mustPrefix("fd00::/64")
	setLinuxActiveSettings(tunnel, active)

	if _, err := tunnel.Open(testServerAddrV6); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = tunnel.Close() }()

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

func TestOpenSingleStackSkipsUncoveredServerFamily(t *testing.T) {
	ipMock := &clientTUNIPMock{}
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	if _, err := tunnel.Open(testServerAddrV6); err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = tunnel.Close() }()

	if len(ipMock.routeGetTargets) != 0 || len(ipMock.routeReplaceTargets) != 0 || tunnel.pinnedServerAddr.IsValid() {
		t.Fatalf("unexpected pinned route: get=%v replace=%v cached=%s", ipMock.routeGetTargets, ipMock.routeReplaceTargets, tunnel.pinnedServerAddr)
	}
}

func TestOpen_IPv6_AddrAddError(t *testing.T) {
	// When IPv6 AddrAddDev fails, creation should fail.
	calls := 0
	ipMock := &clientTUNIPMockFailNthAddr{
		clientTUNIPMock: clientTUNIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"},
		failOnCall:      2,
		callCount:       &calls,
	}
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	active := tunnel.settings
	active.IPv6 = mustAddr("fd00::2")
	active.IPv6Subnet = mustPrefix("fd00::/64")
	setLinuxActiveSettings(tunnel, active)

	_, err := tunnel.Open(testServerAddrV4)
	if err == nil {
		t.Fatal("expected error on IPv6 addr add failure")
	}
	if tunnel.pinnedServerAddr.IsValid() || len(ipMock.routeDelTargets) != 0 {
		t.Fatalf("failed address assignment changed pinned route state: pinned=%s deleted=%v", tunnel.pinnedServerAddr, ipMock.routeDelTargets)
	}
}

func TestOpen_IPv6_Route6SplitError(t *testing.T) {
	ipMock := &clientTUNIPMock{
		routeReply: "198.51.100.1 dev eth0",
		failStep:   "split6",
	}
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})
	active := tunnel.settings
	active.IPv6 = mustAddr("fd00::2")
	active.IPv6Subnet = mustPrefix("fd00::/64")
	setLinuxActiveSettings(tunnel, active)

	_, err := tunnel.Open(testServerAddrV4)
	if err == nil {
		t.Fatal("expected error on Route6AddSplitDev failure")
	}
	assertOpenRolledBack(t, tunnel, ipMock)
}

func TestCloseWithoutOpenedServerSkipsHostRouteCleanup(t *testing.T) {
	ipMock := &clientTUNIPMock{}
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})

	if err := tunnel.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ipMock.routeDelTargets) != 0 {
		t.Fatalf("unexpected host route deletions: %v", ipMock.routeDelTargets)
	}
}

func TestCloseCancelsDefaultRouteWatcher(t *testing.T) {
	tunnel := newTestTUN(
		settings.UDP,
		&clientTUNIPMock{},
		clientTUNIOCTLMock{},
		clientTUNMSSMock{},
	)
	cancelled := false
	tunnel.defaultRouteWatcherCancel = func() { cancelled = true }

	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if !cancelled {
		t.Fatal("Close() did not cancel the default route watcher")
	}
}

func TestCloseRemovesOpenedServerRoute(t *testing.T) {
	ipMock := &clientTUNIPMock{routeReply: "198.51.100.1 dev eth0"}
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})
	_, err := tunnel.Open(testServerAddrV4)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if len(ipMock.routeDelTargets) != 1 || ipMock.routeDelTargets[0] != testServerAddrV4 {
		t.Fatalf("deleted host routes = %v, want [%s]", ipMock.routeDelTargets, testServerAddrV4)
	}
}

func TestCloseRetriesServerRouteDeletion(t *testing.T) {
	ipMock := &clientTUNIPMock{
		routeReply: "198.51.100.1 dev eth0",
		failStep:   "rdel",
	}
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})
	dev, err := tunnel.Open(testServerAddrV4)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := tunnel.Close(); err == nil {
		t.Fatal("Close() error = nil, want route deletion error")
	}
	if _, err := dev.Read(make([]byte, 1)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("TUN read after Close() error = %v, want %v", err, io.ErrClosedPipe)
	}
	if tunnel.tun != nil {
		t.Fatal("Close() retained a closed TUN")
	}
	if tunnel.pinnedServerAddr != testServerAddrV4 {
		t.Fatalf("pinnedServerAddr = %s, want retained %s", tunnel.pinnedServerAddr, testServerAddrV4)
	}

	ipMock.failStep = ""
	if err := tunnel.Close(); err != nil {
		t.Fatalf("retry Close() error = %v", err)
	}
	if tunnel.pinnedServerAddr.IsValid() {
		t.Fatalf("pinnedServerAddr = %s, want cleared address", tunnel.pinnedServerAddr)
	}
	if len(ipMock.routeDelTargets) != 2 {
		t.Fatalf("route deletion attempts = %d, want 2", len(ipMock.routeDelTargets))
	}
}

func TestCloseReturnsTunCloseErrorAndClearsTun(t *testing.T) {
	closeErr := errors.New("close failed")
	tun := &clientTunMock{closeErr: closeErr}
	tunnel := newTestTUN(
		settings.UDP,
		&clientTUNIPMock{},
		clientTUNIOCTLMock{},
		clientTUNMSSMock{},
	)
	tunnel.tun = tun

	if err := tunnel.Close(); !errors.Is(err, closeErr) {
		t.Fatalf("Close() error = %v, want %v", err, closeErr)
	}
	if tun.closeCalls != 1 {
		t.Fatalf("TUN Close() calls = %d, want 1", tun.closeCalls)
	}
	if tunnel.tun != nil {
		t.Fatal("Close() retained TUN after Close returned an error")
	}
}

func TestCloseReturnsLinkDeleteErrorAndContinuesCleanup(t *testing.T) {
	deleteErr := errors.New("delete interface failed")
	ipMock := &clientTUNIPMock{linkDeleteErr: deleteErr}
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})
	tun := &clientTunMock{}
	tunnel.tun = tun
	tunnel.pinnedServerAddr = testServerAddrV4

	if err := tunnel.Close(); !errors.Is(err, deleteErr) {
		t.Errorf("Close() error = %v, want %v", err, deleteErr)
	}
	if tun.closeCalls != 1 || tunnel.tun != nil {
		t.Errorf("TUN cleanup: close calls = %d, TUN = %v", tun.closeCalls, tunnel.tun)
	}
	if !slices.Equal(ipMock.deletedInterfaces, []string{"tun1", "tun0", "tun2"}) {
		t.Errorf("deleted interfaces = %v, want all configured interfaces", ipMock.deletedInterfaces)
	}
	if !slices.Equal(ipMock.routeDelTargets, []netip.Addr{testServerAddrV4}) || tunnel.pinnedServerAddr.IsValid() {
		t.Errorf("server route cleanup: deleted = %v, pinned = %v", ipMock.routeDelTargets, tunnel.pinnedServerAddr)
	}
}

func TestCloseRetriesDNSRestore(t *testing.T) {
	dnsMock := &clientTUNDNSMock{revertErr: errors.New("restore failed")}
	tunnel := newTestTUN(
		settings.UDP,
		&clientTUNIPMock{},
		clientTUNIOCTLMock{},
		clientTUNMSSMock{},
	)
	tunnel.dns = dnsMock
	tun := &clientTunMock{}
	tunnel.tun = tun

	if err := tunnel.Close(); err == nil || !strings.Contains(err.Error(), "restore failed") {
		t.Fatalf("Close() error = %v", err)
	}
	if tunnel.tun != nil || tun.closeCalls != 1 || dnsMock.revertCalls != 1 {
		t.Fatalf("failed restore state: TUN=%v close calls=%d Revert calls=%d", tunnel.tun, tun.closeCalls, dnsMock.revertCalls)
	}

	dnsMock.revertErr = nil
	if err := tunnel.Close(); err != nil {
		t.Fatalf("retry Close() error = %v", err)
	}
	if dnsMock.revertCalls != 2 {
		t.Fatalf("DNS Revert() calls = %d, want 2", dnsMock.revertCalls)
	}
	if tun.closeCalls != 1 {
		t.Fatalf("TUN close calls = %d, want 1", tun.closeCalls)
	}
}

func TestCloseSkipsProfilesWithoutTunName(t *testing.T) {
	ipMock := &clientTUNIPMock{}
	tunnel := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, clientTUNMSSMock{})
	tunnel.configuration.TCPSettings.TunName = ""
	tunnel.configuration.WSSettings.TunName = ""

	if err := tunnel.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := strings.Count(ipMock.log.String(), "ldel;"); got != 1 {
		t.Fatalf("LinkDelete() calls = %d, want 1", got)
	}
}

// clientTUNIPMockFailNthAddr fails AddrAddDev on the N-th call.
type clientTUNIPMockFailNthAddr struct {
	clientTUNIPMock
	failOnCall int
	callCount  *int
}

func (m *clientTUNIPMockFailNthAddr) AddrAddDev(dev, cidr string) error {
	*m.callCount++
	if *m.callCount == m.failOnCall {
		return errors.New("addr add failed")
	}
	return nil
}

func TestCloseReturnsMSSRemoveError(t *testing.T) {
	cleanupErr := errors.New("cleanup fail")
	ipMock := &clientTUNIPMock{}
	mssMock := clientTUNMSSMock{removeErr: cleanupErr}
	m := newTestTUN(settings.UDP, ipMock, clientTUNIOCTLMock{}, mssMock)

	if err := m.Close(); !errors.Is(err, cleanupErr) {
		t.Fatalf("Close() error = %v, want %v", err, cleanupErr)
	}
}
