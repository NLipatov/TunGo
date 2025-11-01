package tun_client

import (
	"bytes"
	"errors"
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
func (m *platformTunManagerIPMock) RouteGet(string) (string, error)         { return m.routeReply, nil }
func (m *platformTunManagerIPMock) RouteAddDev(string, string) error        { return m.mark("radd") }
func (m *platformTunManagerIPMock) RouteAddViaDev(string, string, string) error {
	return m.mark("raddvia")
}
func (m *platformTunManagerIPMock) RouteDel(string) error { m.log.WriteString("rdel;"); return nil }

// platformTunManagerIPGetErr forces RouteGet to error (code ignores err, falls to parse error).
type platformTunManagerIPGetErr struct{ platformTunManagerIPMock }

func (m *platformTunManagerIPGetErr) RouteGet(string) (string, error) {
	return "", errors.New("geterr")
}

// platformTunManagerIOCTLMock returns /dev/null or injected error.
type platformTunManagerIOCTLMock struct {
	openErr error
}

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
		RouteGet(string) (string, error)
		RouteAddDev(string, string) error
		RouteAddViaDev(string, string, string) error
		RouteDel(string) error
	},
	ioctlMock interface {
		DetectTunNameFromFd(*os.File) (string, error)
		CreateTunInterface(string) (*os.File, error)
	},
	wrap tun.Wrapper,
) *PlatformTunManager {
	cfg := client.Configuration{
		Protocol: proto,
		UDPSettings: settings.Settings{
			InterfaceName:    "tun0",
			InterfaceAddress: "10.0.0.2/30",
			ConnectionIP:     "198.51.100.1",
			MTU:              1400,
		},
		TCPSettings: settings.Settings{
			InterfaceName:    "tun1",
			InterfaceAddress: "10.0.0.6/30",
			ConnectionIP:     "203.0.113.1",
			MTU:              1400,
		},
		WSSettings: settings.Settings{
			InterfaceName:    "tun2",
			InterfaceAddress: "10.0.0.10/30",
			ConnectionIP:     "203.0.113.2",
			MTU:              1250,
		},
	}
	return &PlatformTunManager{
		configuration: cfg,
		ip:            ipMock,
		ioctl:         ioctlMock,
		wrapper:       wrap,
	}
}

//
// ============================ Tests ===========================
//

func TestCreateDevice_UDP_WithGateway(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{})

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
	m := newMgr(settings.TCP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{})

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
	m := newMgr(settings.WS, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{})

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
	m := newMgr(settings.Protocol(255), ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected unsupported protocol error")
	} else if !strings.Contains(err.Error(), "unsupported protocol") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDevice_ParseRouteError_NoDev(t *testing.T) {
	// Missing "dev" -> parse must fail.
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1"}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected parse error (no dev)")
	} else if !strings.Contains(err.Error(), "failed to parse route to server IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDevice_RouteGetError_LeadsToParseError(t *testing.T) {
	ipMock := &platformTunManagerIPGetErr{}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected parse error after RouteGet error")
	} else if !strings.Contains(err.Error(), "failed to parse route to server IP") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDevice_OpenTunError(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{openErr: errors.New("open fail")}, platformTunManagerPlainWrapper{})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected open TUN error")
	} else if !strings.Contains(err.Error(), "failed to open TUN interface") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateDevice_WrapError(t *testing.T) {
	ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 dev eth0"}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{err: errors.New("wrap fail")})

	if _, err := m.CreateDevice(); err == nil {
		t.Fatal("expected wrapper.Wrap error")
	}
}

func TestConfigureTUN_ErrorPropagation_NoGatewayPath(t *testing.T) {
	steps := []string{"add", "up", "addr", "radd", "def", "mtu"}
	for _, step := range steps {
		ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 dev eth0", failStep: step}
		m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{})
		if _, err := m.CreateDevice(); err == nil {
			t.Fatalf("expected error on step %s", step)
		}
	}
}

func TestConfigureTUN_ErrorPropagation_WithGatewayPath(t *testing.T) {
	steps := []string{"add", "up", "addr", "raddvia", "def", "mtu"}
	for _, step := range steps {
		ipMock := &platformTunManagerIPMock{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0", failStep: step}
		m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{})
		if _, err := m.CreateDevice(); err == nil {
			t.Fatalf("expected error on step %s", step)
		}
	}
}

func TestDisposeDevices(t *testing.T) {
	ipMock := &platformTunManagerIPMock{}
	m := newMgr(settings.UDP, ipMock, platformTunManagerIOCTLMock{}, platformTunManagerPlainWrapper{})

	if err := m.DisposeDevices(); err != nil {
		t.Fatalf("DisposeDevices error: %v", err)
	}
}
