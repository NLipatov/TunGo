package tun_client

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
)

type mockIP struct {
	log        bytes.Buffer
	routeReply string
	failStep   string
}

func (m *mockIP) mark(s string) error {
	m.log.WriteString(s + ";")
	if m.failStep == s {
		return errors.New("boom")
	}
	return nil
}

func (m *mockIP) TunTapAddDevTun(string) error                { return m.mark("add") }
func (m *mockIP) LinkDelete(string) error                     { return nil }
func (m *mockIP) LinkSetDevUp(string) error                   { return m.mark("up") }
func (m *mockIP) LinkSetDevMTU(string, int) error             { return m.mark("mtu") }
func (m *mockIP) AddrAddDev(string, string) error             { return m.mark("addr") }
func (m *mockIP) AddrShowDev(int, string) (string, error)     { return "", nil }
func (m *mockIP) RouteDefault() (string, error)               { return "eth0", nil }
func (m *mockIP) RouteAddDefaultDev(string) error             { return m.mark("def") }
func (m *mockIP) RouteGet(string) (string, error)             { return m.routeReply, nil }
func (m *mockIP) RouteAddDev(string, string) error            { return m.mark("radd") }
func (m *mockIP) RouteAddViaDev(string, string, string) error { return m.mark("raddvia") }
func (m *mockIP) RouteDel(string) error                       { return nil }

type mockIPT struct{ called bool }

func (m *mockIPT) EnableDevMasquerade(string) error                  { return nil }
func (m *mockIPT) DisableDevMasquerade(string) error                 { return nil }
func (m *mockIPT) EnableForwardingFromTunToDev(string, string) error { return nil }
func (m *mockIPT) DisableForwardingFromTunToDev(string, string) error {
	return nil
}
func (m *mockIPT) EnableForwardingFromDevToTun(string, string) error { return nil }
func (m *mockIPT) DisableForwardingFromDevToTun(string, string) error {
	return nil
}
func (m *mockIPT) ConfigureMssClamping() error { m.called = true; return nil }

type mockIOCTL struct{}

func (mockIOCTL) DetectTunNameFromFd(*os.File) (string, error) { return "tun0", nil }
func (mockIOCTL) CreateTunInterface(string) (*os.File, error) {
	f, _ := os.Open(os.DevNull)
	return f, nil
}

func mgr(proto settings.Protocol, ipMock *mockIP) *PlatformTunManager {
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
	}
	return &PlatformTunManager{
		conf:     cfg,
		ip:       ipMock,
		iptables: &mockIPT{},
		ioctl:    &mockIOCTL{},
	}
}

func TestCreateTunDevice_UDP(t *testing.T) {
	ipMock := &mockIP{routeReply: "198.51.100.1 via 192.0.2.1 dev eth0"}
	m := mgr(settings.UDP, ipMock)

	f, err := m.CreateTunDevice()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f == nil {
		t.Fatal("nil *os.File returned")
	}
	want := "add;up;addr;raddvia;def;mtu;"
	if got := ipMock.log.String(); got != want {
		t.Fatalf("call sequence mismatch\nwant %s\ngot  %s", want, got)
	}
}

func TestCreateTunDevice_TCP(t *testing.T) {
	ipMock := &mockIP{routeReply: "203.0.113.1 dev eth0"} // no gateway
	m := mgr(settings.TCP, ipMock)

	if _, err := m.CreateTunDevice(); err != nil {
		t.Fatalf("TCP path failed: %v", err)
	}
	if !m.iptables.(*mockIPT).called {
		t.Fatal("ConfigureMssClamping not invoked")
	}
}

func TestCreateTunDevice_Unsupported(t *testing.T) {
	m := mgr(settings.UDP, &mockIP{}) // fake proto
	if _, err := m.CreateTunDevice(); err == nil {
		t.Fatal("expected unsupported protocol error")
	}
}

func TestConfigureTUN_ErrorsPropagate(t *testing.T) {
	steps := []string{"add", "up", "addr", "radd", "def", "mtu"}
	for _, step := range steps {
		ipMock := &mockIP{routeReply: "198.51.100.1 dev eth0", failStep: step}
		m := mgr(settings.UDP, ipMock)
		if _, err := m.CreateTunDevice(); err == nil {
			t.Fatalf("want error on step %s", step)
		}
	}
}

func TestDisposeTunDevices(t *testing.T) {
	ipMock := &mockIP{}
	m := mgr(settings.UDP, ipMock)
	if err := m.DisposeTunDevices(); err != nil {
		t.Fatalf("DisposeTunDevices error: %v", err)
	}
}
