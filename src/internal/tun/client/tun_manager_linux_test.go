package client

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"os"
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
	log             bytes.Buffer
	routeReply      string
	failStep        string
	routeGetTargets []string
	routeAddTargets []string
	routeDelTargets []string
}

func (m *clienttunManagerIPMock) mark(s string) error {
	m.log.WriteString(s + ";")
	if m.failStep == s {
		return errors.New("boom")
	}
	return nil
}

func (m *clienttunManagerIPMock) TunTapAddDevTun(string) error    { return m.mark("add") }
func (m *clienttunManagerIPMock) LinkDelete(string) error         { m.log.WriteString("ldel;"); return nil }
func (m *clienttunManagerIPMock) LinkSetDevUp(string) error       { return m.mark("up") }
func (m *clienttunManagerIPMock) LinkSetDevMTU(string, int) error { return m.mark("mtu") }
func (m *clienttunManagerIPMock) AddrAddDev(string, string) error { return m.mark("addr") }
func (m *clienttunManagerIPMock) RouteDefault() (string, error)   { return "eth0", nil }
func (m *clienttunManagerIPMock) RouteGet(target string) (string, error) {
	m.routeGetTargets = append(m.routeGetTargets, target)
	return m.routeReply, nil
}
func (m *clienttunManagerIPMock) RouteAddDev(target, _ string) error {
	m.routeAddTargets = append(m.routeAddTargets, target)
	return m.mark("radd")
}
func (m *clienttunManagerIPMock) RouteAddViaDev(target, _, _ string) error {
	m.routeAddTargets = append(m.routeAddTargets, target)
	return m.mark("raddvia")
}
func (m *clienttunManagerIPMock) RouteAddSplitDefaultDev(string) error  { return m.mark("splitdef") }
func (m *clienttunManagerIPMock) Route6AddSplitDefaultDev(string) error { return m.mark("splitdef6") }
func (m *clienttunManagerIPMock) RouteDelSplitDefault(string) error {
	m.log.WriteString("splitdel;")
	return nil
}
func (m *clienttunManagerIPMock) Route6DelSplitDefault(string) error {
	m.log.WriteString("splitdel6;")
	return nil
}
func (m *clienttunManagerIPMock) RouteDel(target string) error {
	m.routeDelTargets = append(m.routeDelTargets, target)
	return m.mark("rdel")
}

// clienttunManagerIPGetErr forces RouteGet to return an error.
type clienttunManagerIPGetErr struct{ clienttunManagerIPMock }

func (m *clienttunManagerIPGetErr) RouteGet(string) (string, error) {
	return "", fmt.Errorf("failed to get route to server IP: %w", errors.New("geterr"))
}

// clienttunManagerIOCTLMock returns a pollable file or an injected error.
type clienttunManagerIOCTLMock struct {
	openErr error
	file    *os.File
}

// clienttunManagerMSSMock simulates mssclamp.Contract.
type clienttunManagerMSSMock struct {
	installErr        error
	removeErr         error
	installedFamilies *[]mssclamp.Families
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
func (m clienttunManagerMSSMock) Remove(string) error { return m.removeErr }

func (clienttunManagerIOCTLMock) DetectTunNameFromFd(*os.File) (string, error) { return "tun0", nil }
func (m clienttunManagerIOCTLMock) CreateTunInterface(string) (*os.File, error) {
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
		RouteAddSplitDefaultDev(string) error
		Route6AddSplitDefaultDev(string) error
		RouteDelSplitDefault(string) error
		Route6DelSplitDefault(string) error
		RouteGet(string) (string, error)
		RouteAddDev(string, string) error
		RouteAddViaDev(string, string, string) error
		RouteDel(string) error
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
			Addressing: settings.Addressing{
				TunName:    "tun0",
				IPv4Subnet: mustPrefix("10.0.0.0/30"),
				IPv4:       mustAddr("10.0.0.2"),
				Server:     mustHost("198.51.100.1"),
			},
			MTU:      1400,
			Protocol: settings.UDP,
		},
		settings.TCP: {
			Addressing: settings.Addressing{
				TunName:    "tun1",
				IPv4Subnet: mustPrefix("10.0.0.4/30"),
				IPv4:       mustAddr("10.0.0.6"),
				Server:     mustHost("203.0.113.1"),
			},
			MTU:      1400,
			Protocol: settings.TCP,
		},
		settings.WS: {
			Addressing: settings.Addressing{
				TunName:    "tun2",
				IPv4Subnet: mustPrefix("10.0.0.8/30"),
				IPv4:       mustAddr("10.0.0.10"),
				Server:     mustHost("203.0.113.2"),
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
	if len(ipMock.routeDelTargets) != 1 || ipMock.routeDelTargets[0] != testServerAddrV4.String() {
		t.Fatalf("RouteDel() targets = %v, want [%s]", ipMock.routeDelTargets, testServerAddrV4)
	}
	cleanupSteps := []string{"ldel;", "rdel;"}
	if hasIPv4(m.settings) {
		cleanupSteps = append(cleanupSteps, "splitdel;")
	}
	if hasIPv6(m.settings) {
		cleanupSteps = append(cleanupSteps, "splitdel6;")
	}
	for _, cleanupStep := range cleanupSteps {
		if !strings.Contains(ipMock.log.String(), cleanupStep) {
			t.Errorf("cleanup log = %q, want %q", ipMock.log.String(), cleanupStep)
		}
	}
}

func setLinuxActiveSettings(m *Manager, active settings.Settings) {
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

	want := "add;up;addr;raddvia;splitdef;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call sequence mismatch\nwant %s\ngot  %s", want, got)
	}
	if len(installedFamilies) != 1 || installedFamilies[0] != (mssclamp.Families{IPv4: true}) {
		t.Fatalf("MSS families = %v, want IPv4 only", installedFamilies)
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

	want := "add;up;addr;radd;splitdef;mtu;"
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
	assertOpenTunnelRolledBack(t, m, ipMock)
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
	assertOpenTunnelRolledBack(t, m, ipMock)
}

func TestConfigureTUN_ErrorPropagation_NoGatewayPath(t *testing.T) {
	steps := []string{"add", "up", "addr", "radd", "splitdef", "mtu"}
	for _, step := range steps {
		ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 dev eth0", failStep: step}
		m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
		if _, err := m.OpenTunnel(testServerAddrV4); err == nil {
			t.Fatalf("expected error on step %s", step)
		}
	}
}

func TestConfigureTUN_ErrorPropagation_WithGatewayPath(t *testing.T) {
	steps := []string{"add", "up", "addr", "raddvia", "splitdef", "mtu"}
	for _, step := range steps {
		ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0", failStep: step}
		m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
		if _, err := m.OpenTunnel(testServerAddrV4); err == nil {
			t.Fatalf("expected error on step %s", step)
		}
	}
}

func TestCloseTunnel_NoErrors(t *testing.T) {
	ipMock := &clienttunManagerIPMock{}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	if err := m.CloseTunnel(); err != nil {
		t.Fatalf("CloseTunnel error: %v", err)
	}
}

func TestConfigureTUN_MSSInstallError(t *testing.T) {
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	mssMock := clienttunManagerMSSMock{installErr: errors.New("iptables fail")}
	m := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, mssMock)

	_, err := m.OpenTunnel(testServerAddrV4)
	if err == nil {
		t.Fatal("expected MSS install error")
	}
	if !strings.Contains(err.Error(), "failed to install MSS clamping") {
		t.Fatalf("unexpected error: %v", err)
	}
	assertOpenTunnelRolledBack(t, m, ipMock)
}

func TestOpenTunnel_IPv6_FullPath(t *testing.T) {
	// IPv6 configured: should assign IPv6 address, set IPv6 default route,
	// and add route to IPv6 server.
	ipMock := &clienttunManagerIPMock{routeReply: "2001:db8::1 via fe80::1 dev eth0"}
	var installedFamilies []mssclamp.Families
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{
		installedFamilies: &installedFamilies,
	})

	// Enable IPv6 on the active protocol's settings.
	mgr.settings.IPv6 = mustAddr("fd00::2")
	mgr.settings.IPv6Subnet = mustPrefix("fd00::/64")
	mgr.configuration.UDPSettings = mgr.settings

	_, err := mgr.OpenTunnel(testServerAddrV6)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() { _ = mgr.CloseTunnel() }()

	// Should include: addr (IPv4), addr (IPv6), splitdef6, and the ipv6 route steps.
	got := ipMock.log.String()
	if !strings.Contains(got, "splitdef6;") {
		t.Fatalf("expected IPv6 default route step, got: %s", got)
	}
	// Two "addr;" calls: one for IPv4, one for IPv6
	if strings.Count(got, "addr;") != 2 {
		t.Fatalf("expected 2 addr calls (IPv4 + IPv6), got: %s", got)
	}
	if len(ipMock.routeGetTargets) != 1 || ipMock.routeGetTargets[0] != testServerAddrV6.String() {
		t.Fatalf("route lookup targets = %v, want [%s]", ipMock.routeGetTargets, testServerAddrV6)
	}
	if len(ipMock.routeAddTargets) != 1 || ipMock.routeAddTargets[0] != testServerAddrV6.String() {
		t.Fatalf("route add targets = %v, want [%s]", ipMock.routeAddTargets, testServerAddrV6)
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

	want := "add;up;addr;raddvia;splitdef6;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call order = %q, want %q", got, want)
	}
	if got := ipMock.routeGetTargets; len(got) != 1 || got[0] != testServerAddrV6.String() {
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

	if len(ipMock.routeGetTargets) != 0 || len(ipMock.routeAddTargets) != 0 || mgr.pinnedServerAddr.IsValid() {
		t.Fatalf("unexpected pinned route: get=%v add=%v cached=%s", ipMock.routeGetTargets, ipMock.routeAddTargets, mgr.pinnedServerAddr)
	}
}

func TestOpenTunnel_IPv6_AddrAddError(t *testing.T) {
	// When IPv6 AddrAddDev fails, creation should fail.
	calls := 0
	ipMock := &clienttunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
	// Override AddrAddDev to fail on the second call (IPv6).
	origMark := ipMock.mark
	_ = origMark
	mgr := newMgr(settings.UDP, &clienttunManagerIPMockFailNthAddr{
		clienttunManagerIPMock: clienttunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"},
		failOnCall:             2,
		callCount:              &calls,
	}, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})

	mgr.settings.IPv6 = mustAddr("fd00::2")
	mgr.settings.IPv6Subnet = mustPrefix("fd00::/64")

	_, err := mgr.OpenTunnel(testServerAddrV4)
	if err == nil {
		t.Fatal("expected error on IPv6 addr add failure")
	}
}

func TestOpenTunnel_IPv6_Route6DefaultError(t *testing.T) {
	ipMock := &clienttunManagerIPMock{
		routeReply: "198.51.100.1 dev eth0",
		failStep:   "splitdef6",
	}
	mgr := newMgr(settings.UDP, ipMock, clienttunManagerIOCTLMock{}, clienttunManagerMSSMock{})
	mgr.settings.IPv6 = mustAddr("fd00::2")
	mgr.settings.IPv6Subnet = mustPrefix("fd00::/64")

	_, err := mgr.OpenTunnel(testServerAddrV4)
	if err == nil {
		t.Fatal("expected error on Route6AddSplitDefaultDev failure")
	}
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
	if len(ipMock.routeDelTargets) != 1 || ipMock.routeDelTargets[0] != testServerAddrV4.String() {
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
