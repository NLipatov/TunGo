package tun_server

import (
	"bytes"
	"os"
	"testing"
	"time"

	"tungo/settings"
)

type mockIP struct{ log bytes.Buffer }

func (m *mockIP) add(tag string) { m.log.WriteString(tag + ";") }

func (m *mockIP) TunTapAddDevTun(string) error                { m.add("add"); return nil }
func (m *mockIP) LinkDelete(string) error                     { m.add("del"); return nil }
func (m *mockIP) LinkSetDevUp(string) error                   { m.add("up"); return nil }
func (m *mockIP) LinkSetDevMTU(string, int) error             { m.add("mtu"); return nil }
func (m *mockIP) AddrAddDev(string, string) error             { m.add("addr"); return nil }
func (m *mockIP) AddrShowDev(int, string) (string, error)     { return "", nil }
func (m *mockIP) RouteDefault() (string, error)               { return "eth0", nil }
func (m *mockIP) RouteAddDefaultDev(string) error             { return nil }
func (m *mockIP) RouteGet(string) (string, error)             { return "", nil }
func (m *mockIP) RouteAddDev(string, string) error            { return nil }
func (m *mockIP) RouteAddViaDev(string, string, string) error { return nil }
func (m *mockIP) RouteDel(string) error                       { return nil }

type mockIPT struct{ log bytes.Buffer }

func (m *mockIPT) add(tag string) { m.log.WriteString(tag + ";") }

func (m *mockIPT) EnableDevMasquerade(string) error                  { m.add("masq_on"); return nil }
func (m *mockIPT) DisableDevMasquerade(string) error                 { m.add("masq_off"); return nil }
func (m *mockIPT) EnableForwardingFromTunToDev(string, string) error { m.add("fwd_td"); return nil }
func (m *mockIPT) DisableForwardingFromTunToDev(string, string) error {
	m.add("fwd_td_off")
	return nil
}
func (m *mockIPT) EnableForwardingFromDevToTun(string, string) error { m.add("fwd_dt"); return nil }
func (m *mockIPT) DisableForwardingFromDevToTun(string, string) error {
	m.add("fwd_dt_off")
	return nil
}
func (m *mockIPT) ConfigureMssClamping() error { m.add("clamp"); return nil }

type mockIOCTL struct{ name string }

func (m *mockIOCTL) DetectTunNameFromFd(*os.File) (string, error) { return m.name, nil }

func (m *mockIOCTL) CreateTunInterface(n string) (*os.File, error) {
	m.name = n
	// use /dev/null so Close succeeds
	f, _ := os.Open(os.DevNull)
	return f, nil
}

type mockSys struct{ on bool }

func (m *mockSys) NetIpv4IpForward() ([]byte, error) {
	if m.on {
		return []byte("net.ipv4.ip_forward = 1\n"), nil
	}
	return []byte("net.ipv4.ip_forward = 0\n"), nil
}
func (m *mockSys) WNetIpv4IpForward() ([]byte, error) {
	m.on = true
	return []byte("net.ipv4.ip_forward = 1\n"), nil
}

func newFactory() *ServerTunFactory {
	return &ServerTunFactory{
		ip:       &mockIP{},
		iptables: &mockIPT{},
		ioctl:    &mockIOCTL{},
		sysctl:   &mockSys{},
	}
}

var cfg = settings.ConnectionSettings{
	InterfaceName:   "tun0",
	InterfaceIPCIDR: "10.0.0.0/30",
	MTU:             1420,
}

func TestCreateAndDispose(t *testing.T) {
	f := newFactory()

	tun, err := f.CreateTunDevice(cfg)
	if err != nil {
		t.Fatalf("CreateTunDevice: %v", err)
	}
	if tun == nil {
		t.Fatal("nil *os.File returned")
	}
	if err := f.DisposeTunDevices(cfg); err != nil {
		t.Fatalf("DisposeTunDevices: %v", err)
	}
}

func TestEnableForwardingToggle(t *testing.T) {
	f := newFactory()
	if err := f.enableForwarding(); err != nil {
		t.Fatalf("first call: %v", err)
	}
	// second call should be no-op
	if err := f.enableForwarding(); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

func TestConfigureUnconfigure(t *testing.T) {
	f := newFactory()
	tun, _ := f.ioctl.CreateTunInterface("tunX")

	if err := f.configure(tun); err != nil {
		t.Fatalf("configure: %v", err)
	}
	f.Unconfigure(tun)
}

func TestForwardingRulesPath(t *testing.T) {
	f := newFactory()
	tun, _ := f.ioctl.CreateTunInterface("tunY")

	if err := f.setupForwarding(tun, "eth0"); err != nil {
		t.Fatalf("setupForwarding: %v", err)
	}
	if err := f.clearForwarding(tun, "eth0"); err != nil {
		t.Fatalf("clearForwarding: %v", err)
	}
}

func TestMain(m *testing.M) {
	time.Sleep(5 * time.Millisecond) // tiny pause for slow CI
	os.Exit(m.Run())
}
