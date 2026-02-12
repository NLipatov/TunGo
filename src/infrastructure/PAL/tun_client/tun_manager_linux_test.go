package tun_client

import (
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"strings"
	"testing"

	"tungo/application/network/routing/tun"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
)

// platformTunManagerPlainDev is a minimal tun.Device over *os.File.
type platformTunManagerPlainDev struct{ f *os.File }

func (d *platformTunManagerPlainDev) Read(p []byte) (int, error)  { return d.f.Read(p) }
func (d *platformTunManagerPlainDev) Write(p []byte) (int, error) { return d.f.Write(p) }
func (d *platformTunManagerPlainDev) Close() error                { return d.f.Close() }

// platformTunManagerPlainWrapper implements tun.Wrapper and can inject an error.
type platformTunManagerPlainWrapper struct {
	err error
}

func (w platformTunManagerPlainWrapper) Wrap(f *os.File) (tun.Device, error) {
	if w.err != nil {
		return nil, w.err
	}
	return &platformTunManagerPlainDev{f: f}, nil
}

// platformTunManagerIPMock simulates `ip` contract and records call sequence.
// `failStep` makes the corresponding step return an error.
type platformTunManagerIPMock struct {
	log        bytes.Buffer
	routeReply string
	failStep   string
}

func (m *platformTunManagerIPMock) mark(s string) error {
	m.log.WriteString(s + ";")
	if m.failStep == s {
		return errors.New("boom")
	}
	return nil
}

func (m *platformTunManagerIPMock) TunTapAddDevTun(string) error            { return m.mark("add") }
func (m *platformTunManagerIPMock) LinkDelete(string) error                 { m.log.WriteString("ldel;"); return nil }
func (m *platformTunManagerIPMock) LinkSetDevUp(string) error               { return m.mark("up") }
func (m *platformTunManagerIPMock) LinkSetDevMTU(string, int) error         { return m.mark("mtu") }
func (m *platformTunManagerIPMock) AddrAddDev(string, string) error         { return m.mark("addr") }
func (m *platformTunManagerIPMock) AddrShowDev(int, string) (string, error) { return "", nil }
func (m *platformTunManagerIPMock) RouteDefault() (string, error)           { return "eth0", nil }
func (m *platformTunManagerIPMock) RouteAddDefaultDev(string) error         { return m.mark("def") }
func (m *platformTunManagerIPMock) Route6AddDefaultDev(string) error        { return m.mark("def6") }
func (m *platformTunManagerIPMock) RouteGet(string) (string, error)         { return m.routeReply, nil }
func (m *platformTunManagerIPMock) RouteAddDev(string, string) error        { return m.mark("radd") }
func (m *platformTunManagerIPMock) RouteAddViaDev(string, string, string) error {
	return m.mark("raddvia")
}
func (m *platformTunManagerIPMock) RouteDel(string) error { m.log.WriteString("rdel;"); return nil }

// platformTunManagerIPGetErr forces RouteGet to error (code ignores err, falls to parse error).
type platformTunManagerIPGetErr struct{ platformTunManagerIPMock }

func (m *platformTunManagerIPGetErr) RouteGet(string) (string, error) {
	return "", fmt.Errorf("failed to get route to server IP: %w", errors.New("geterr"))
}

// platformTunManagerIOCTLMock returns /dev/null or injected error.
type platformTunManagerIOCTLMock struct {
	openErr error
}

// platformTunManagerMSSMock simulates mssclamp.Contract.
type platformTunManagerMSSMock struct {
	installErr error
	removeErr  error
}

func mustHost(raw string) settings.Host {
	h, err := settings.NewHost(raw)
	if err != nil {
		panic(err)
	}
	return h
}

func mustPrefix(raw string) netip.Prefix {
	return netip.MustParsePrefix(raw)
}

func mustAddr(raw string) netip.Addr {
	return netip.MustParseAddr(raw)
}

func (m platformTunManagerMSSMock) Install(string) error { return m.installErr }
func (m platformTunManagerMSSMock) Remove(string) error  { return m.removeErr }

func (platformTunManagerIOCTLMock) DetectTunNameFromFd(*os.File) (string, error) { return "tun0", nil }
func (m platformTunManagerIOCTLMock) CreateTunInterface(string) (*os.File, error) {
	if m.openErr != nil {
		return nil, m.openErr
	}
	f, _ := os.Open(os.DevNull)
	return f, nil
}

func newMgr(
	proto settings.Protocol,
	ipMock interface { // minimal duck typing to avoid importing ip package
		TunTapAddDevTun(string) error
		LinkDelete(string) error
		LinkSetDevUp(string) error
		LinkSetDevMTU(string, int) error
		AddrAddDev(string, string) error
		AddrShowDev(int, string) (string, error)
		RouteDefault() (string, error)
		RouteAddDefaultDev(string) error
		Route6AddDefaultDev(string) error
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
		Install(string) error
		Remove(string) error
	},
	wrap tun.Wrapper,
) *PlatformTunManager {
	cfg := client.Configuration{
		Protocol: proto,
		UDPSettings: settings.Settings{
			InterfaceName:    "tun0",
			InterfaceSubnet:  mustPrefix("10.0.0.0/30"),
			InterfaceIP:      mustAddr("10.0.0.2"),
			Host:             mustHost("198.51.100.1"),
			MTU:              1400,
		},
		TCPSettings: settings.Settings{
			InterfaceName:    "tun1",
			InterfaceSubnet:  mustPrefix("10.0.0.4/30"),
			InterfaceIP:      mustAddr("10.0.0.6"),
			Host:             mustHost("203.0.113.1"),
			MTU:              1400,
		},
		WSSettings: settings.Settings{
			InterfaceName:    "tun2",
			InterfaceSubnet:  mustPrefix("10.0.0.8/30"),
			InterfaceIP:      mustAddr("10.0.0.10"),
			Host:             mustHost("203.0.113.2"),
			MTU:              1250,
		},
	}
	return &PlatformTunManager{
		configuration: cfg,
		ip:            ipMock,
		ioctl:         ioctlMock,
		mss:           mssMock,
		wrapper:       wrap,
	}
}

//
// ============================ Tests ===========================
//

func TestCreateDevice_UDP_WithGateway(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	dev, err := m.CreateDevice()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev == nil {
		t.Fatal("nil device returned")
	}
	_ = dev.Close()

	want := "add;up;addr;raddvia;def;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call sequence mismatch\nwant %s\ngot  %s", want, got)
	}
}

func TestCreateDevice_TCP_NoGateway(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "203.0.113.1 dev eth0"} // no "via"
	m := newMgr(settings.TCP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	dev, err := m.CreateDevice()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dev == nil {
		t.Fatal("nil device returned")
	}
	_ = dev.Close()

	want := "add;up;addr;radd;def;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call sequence mismatch\nwant %s\ngot  %s", want, got)
	}
}

func TestCreateDevice_WS_Path(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "203.0.113.2 dev eth0"}
	m := newMgr(settings.WS, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	dev, err := m.CreateDevice()
	if err != nil {
		t.Fatalf("WS path failed: %v", err)
	}
	if dev == nil {
		t.Fatal("nil device returned")
	}
	_ = dev.Close()
}

func TestCreateDevice_UnsupportedProtocol(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newMgr(settings.Protocol(255), ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected unsupported protocol error")
	} else if !strings.Contains(err.Error(), "unsupported protocol") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDevice_ParseRouteError_NoDev(t *testing.T) {
	// Missing "dev" -> parse must fail.
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1"}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected parse error (no dev)")
	} else if !strings.Contains(err.Error(), "failed to parse route to server IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDevice_RouteGetError_LeadsToParseError(t *testing.T) {
	ipMock := &platformTunManagerIPGetErr{}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected RouteGet error")
	} else if !strings.Contains(err.Error(), "failed to get route to server IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDevice_OpenTunError(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{openErr: errors.New("open fail")}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected open TUN error")
	} else if !strings.Contains(err.Error(), "failed to open TUN interface") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDevice_WrapError(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{err: errors.New("wrap fail")})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected wrapper.Wrap error")
	}
}

func TestConfigureTUN_ErrorPropagation_NoGatewayPath(t *testing.T) {
	steps := []string{"add", "up", "addr", "radd", "def", "mtu"}
	for _, step := range steps {
		ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 dev eth0", failStep: step}
		m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})
		if _, err := m.CreateDevice(); err == nil {
			t.Fatalf("expected error on step %s", step)
		}
	}
}

func TestConfigureTUN_ErrorPropagation_WithGatewayPath(t *testing.T) {
	steps := []string{"add", "up", "addr", "raddvia", "def", "mtu"}
	for _, step := range steps {
		ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0", failStep: step}
		m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})
		if _, err := m.CreateDevice(); err == nil {
			t.Fatalf("expected error on step %s", step)
		}
	}
}

func TestDisposeDevices_NoErrors(t *testing.T) {
	ipMock := &platformTunManagerIPMock{}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	if err := m.DisposeDevices(); err != nil {
		t.Fatalf("DisposeDevices error: %v", err)
	}
}

func TestConfigureTUN_MSSInstallError(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	mssMock := platformTunManagerMSSMock{installErr: errors.New("iptables fail")}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, mssMock, platformTunManagerPlainWrapper{})

	_, err := m.CreateDevice()
	if err == nil {
		t.Fatal("expected MSS install error")
	}
	if !strings.Contains(err.Error(), "failed to install MSS clamping") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDevice_IPv6_FullPath(t *testing.T) {
	// IPv6 configured: should assign IPv6 address, set IPv6 default route,
	// and add route to IPv6 server.
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
	mgr := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	// Enable IPv6 on the active protocol's settings.
	mgr.configuration.UDPSettings.IPv6IP = mustAddr("fd00::2")
	mgr.configuration.UDPSettings.IPv6Subnet = mustPrefix("fd00::/64")
	mgr.configuration.UDPSettings.IPv6Host = mustHost("2001:db8::1")

	dev, err := mgr.CreateDevice()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = dev.Close()

	// Should include: addr (IPv4), addr (IPv6), def6, and the ipv6 route steps.
	got := ipMock.log.String()
	if !strings.Contains(got, "def6;") {
		t.Fatalf("expected IPv6 default route step, got: %s", got)
	}
	// Two "addr;" calls: one for IPv4, one for IPv6
	if strings.Count(got, "addr;") != 2 {
		t.Fatalf("expected 2 addr calls (IPv4 + IPv6), got: %s", got)
	}
}

func TestCreateDevice_IPv6_AddrAddError(t *testing.T) {
	// When IPv6 AddrAddDev fails, creation should fail.
	calls := 0
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
	// Override AddrAddDev to fail on the second call (IPv6).
	origMark := ipMock.mark
	_ = origMark
	mgr := newMgr(settings.UDP, &platformTunManagerIPMockFailNthAddr{
		platformTunManagerIPMock: platformTunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"},
		failOnCall:              2,
		callCount:               &calls,
	}, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})

	mgr.configuration.UDPSettings.IPv6IP = mustAddr("fd00::2")
	mgr.configuration.UDPSettings.IPv6Subnet = mustPrefix("fd00::/64")

	_, err := mgr.CreateDevice()
	if err == nil {
		t.Fatal("expected error on IPv6 addr add failure")
	}
}

func TestCreateDevice_IPv6_Route6DefaultError(t *testing.T) {
	ipMock := &platformTunManagerIPMock{
		routeReply: "198.51.100.1 dev eth0",
		failStep:   "def6",
	}
	mgr := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})
	mgr.configuration.UDPSettings.IPv6IP = mustAddr("fd00::2")
	mgr.configuration.UDPSettings.IPv6Subnet = mustPrefix("fd00::/64")

	_, err := mgr.CreateDevice()
	if err == nil {
		t.Fatal("expected error on Route6AddDefaultDev failure")
	}
}

func TestDisposeDevices_IPv6HostRouteCleanup(t *testing.T) {
	ipMock := &platformTunManagerIPMock{}
	mgr := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerMSSMock{}, platformTunManagerPlainWrapper{})
	mgr.configuration.UDPSettings.IPv6Host = mustHost("2001:db8::1")
	mgr.configuration.TCPSettings.IPv6Host = mustHost("2001:db8::2")
	mgr.configuration.WSSettings.IPv6Host = mustHost("2001:db8::3")

	if err := mgr.DisposeDevices(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Each protocol should have rdel for IPv4 host + rdel for IPv6 host + ldel.
	got := ipMock.log.String()
	if strings.Count(got, "rdel;") != 6 {
		t.Fatalf("expected 6 route deletions (3 IPv4 + 3 IPv6), got: %s", got)
	}
}

// platformTunManagerIPMockFailNthAddr fails AddrAddDev on the N-th call.
type platformTunManagerIPMockFailNthAddr struct {
	platformTunManagerIPMock
	failOnCall int
	callCount  *int
}

func (m *platformTunManagerIPMockFailNthAddr) AddrAddDev(dev, cidr string) error {
	*m.callCount++
	if *m.callCount == m.failOnCall {
		return errors.New("addr add failed")
	}
	return nil
}

func TestDisposeDevices_MSSRemoveError_Logged(t *testing.T) {
	// MSS remove errors are logged but do NOT cause DisposeDevices to fail.
	ipMock := &platformTunManagerIPMock{}
	mssMock := platformTunManagerMSSMock{removeErr: errors.New("cleanup fail")}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, mssMock, platformTunManagerPlainWrapper{})

	// Should not return error because MSS remove errors are only logged.
	if err := m.DisposeDevices(); err != nil {
		t.Fatalf("expected no error (MSS remove only logged), got %v", err)
	}
}
