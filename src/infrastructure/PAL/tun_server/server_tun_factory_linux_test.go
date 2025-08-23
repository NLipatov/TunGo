package tun_server

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"tungo/application"
	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/PAL/linux/network_tools/sysctl"
	"tungo/infrastructure/settings"
)

// --- Mocks ---

// mockIP implements ip.Contract
type mockIP struct{ log bytes.Buffer }

func (m *mockIP) add(tag string)                              { m.log.WriteString(tag + ";") }
func (m *mockIP) TunTapAddDevTun(_ string) error              { m.add("add"); return nil }
func (m *mockIP) LinkDelete(_ string) error                   { m.add("del"); return nil }
func (m *mockIP) LinkSetDevUp(_ string) error                 { m.add("up"); return nil }
func (m *mockIP) LinkSetDevMTU(_ string, _ int) error         { m.add("mtu"); return nil }
func (m *mockIP) AddrAddDev(_, _ string) error                { m.add("addr"); return nil }
func (m *mockIP) AddrShowDev(_ int, _ string) (string, error) { return "", nil }
func (m *mockIP) RouteDefault() (string, error)               { m.add("route"); return "eth0", nil }
func (m *mockIP) RouteAddDefaultDev(_ string) error           { return nil }
func (m *mockIP) RouteGet(_ string) (string, error)           { return "", nil }
func (m *mockIP) RouteAddDev(_, _ string) error               { return nil }
func (m *mockIP) RouteAddViaDev(_, _, _ string) error         { return nil }
func (m *mockIP) RouteDel(_ string) error                     { return nil }

// mockIPT implements iptables.Contract
type mockIPT struct{ log bytes.Buffer }

func (m *mockIPT) add(tag string)                      { m.log.WriteString(tag + ";") }
func (m *mockIPT) EnableDevMasquerade(_ string) error  { m.add("masq_on"); return nil }
func (m *mockIPT) DisableDevMasquerade(_ string) error { m.add("masq_off"); return nil }
func (m *mockIPT) EnableForwardingFromTunToDev(_, _ string) error {
	m.add("fwd_td")
	return nil
}
func (m *mockIPT) DisableForwardingFromTunToDev(_, _ string) error {
	m.add("fwd_td_off")
	return nil
}
func (m *mockIPT) EnableForwardingFromDevToTun(_, _ string) error {
	m.add("fwd_dt")
	return nil
}
func (m *mockIPT) DisableForwardingFromDevToTun(_, _ string) error {
	m.add("fwd_dt_off")
	return nil
}
func (m *mockIPT) ConfigureMssClamping() error { m.add("clamp"); return nil }

// mockIOCTL implements ioctl.Contract
type mockIOCTL struct {
	name                 string
	createErr, detectErr error
}

func (m *mockIOCTL) CreateTunInterface(name string) (*os.File, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	// record but can be overridden in tests
	m.name = name
	return os.Open(os.DevNull)
}
func (m *mockIOCTL) DetectTunNameFromFd(_ *os.File) (string, error) {
	if m.detectErr != nil {
		return "", m.detectErr
	}
	return m.name, nil
}

// mockSys implements sysctl.Contract
type mockSys struct {
	netErr    bool
	wErr      bool
	netOutput []byte
}

func (m *mockSys) NetIpv4IpForward() ([]byte, error) {
	if m.netErr {
		return nil, errors.New("net_err")
	}
	if m.netOutput != nil {
		return m.netOutput, nil
	}
	return []byte("net.ipv4.ip_forward = 1\n"), nil
}
func (m *mockSys) WNetIpv4IpForward() ([]byte, error) {
	if m.wErr {
		return nil, errors.New("w_err")
	}
	return []byte("net.ipv4.ip_forward = 1\n"), nil
}

// mockIPErr wraps mockIP to inject errors in createTun steps
type mockIPErr struct {
	*mockIP
	errTag string
	err    error
}

func (m *mockIPErr) TunTapAddDevTun(devName string) error {
	if m.errTag == "TunTapAddDevTun" {
		return m.err
	}
	return m.mockIP.TunTapAddDevTun(devName)
}
func (m *mockIPErr) LinkSetDevUp(devName string) error {
	if m.errTag == "LinkSetDevUp" {
		return m.err
	}
	return m.mockIP.LinkSetDevUp(devName)
}
func (m *mockIPErr) LinkSetDevMTU(devName string, mtu int) error {
	if m.errTag == "LinkSetDevMTU" {
		return m.err
	}
	return m.mockIP.LinkSetDevMTU(devName, mtu)
}
func (m *mockIPErr) AddrAddDev(devName, cidr string) error {
	if m.errTag == "AddrAddDev" {
		return m.err
	}
	return m.mockIP.AddrAddDev(devName, cidr)
}

// mockIPRouteErr injects error into RouteDefault()
type mockIPRouteErr struct {
	*mockIP
	err error
}

func (m *mockIPRouteErr) RouteDefault() (string, error) { return "", m.err }

// mockIPTErr wraps mockIPT to inject errors in configure & clear methods
type mockIPTErr struct {
	*mockIPT
	errTag string
	err    error
}

func (m *mockIPTErr) EnableDevMasquerade(devName string) error {
	if m.errTag == "EnableDevMasquerade" {
		return m.err
	}
	return m.mockIPT.EnableDevMasquerade(devName)
}
func (m *mockIPTErr) EnableForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "EnableForwardingFromTunToDev" {
		return m.err
	}
	return m.mockIPT.EnableForwardingFromTunToDev(tunName, devName)
}
func (m *mockIPTErr) ConfigureMssClamping() error {
	if m.errTag == "ConfigureMssClamping" {
		return m.err
	}
	return m.mockIPT.ConfigureMssClamping()
}
func (m *mockIPTErr) DisableForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "DisableForwardingFromTunToDev" {
		return m.err
	}
	return m.mockIPT.DisableForwardingFromTunToDev(tunName, devName)
}
func (m *mockIPTErr) DisableForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "DisableForwardingFromDevToTun" {
		return m.err
	}
	return m.mockIPT.DisableForwardingFromDevToTun(tunName, devName)
}

// mockIPErrDel injects error into LinkDelete()
type mockIPErrDel struct {
	*mockIP
	err error
}

func (m *mockIPErrDel) LinkDelete(_ string) error { return m.err }

// newFactory helper
func newFactory(
	ipC ip.Contract,
	iptC application.Netfilter,
	ioC ioctl.Contract,
	sysC sysctl.Contract,
) *ServerTunFactory {
	return &ServerTunFactory{
		ip:        ipC,
		netfilter: iptC,
		ioctl:     ioC,
		sysctl:    sysC,
	}
}

var cfg = settings.Settings{
	InterfaceName:   "tun0",
	InterfaceIPCIDR: "10.0.0.0/30",
	MTU:             settings.MTU,
}

// --- Tests ---

func TestCreateAndDispose(t *testing.T) {
	f := newFactory(&mockIP{}, &mockIPT{}, &mockIOCTL{}, &mockSys{})
	tun, err := f.CreateTunDevice(cfg)
	if err != nil {
		t.Fatalf("CreateTunDevice: %v", err)
	}
	if tun == nil {
		t.Fatal("expected non-nil tun file")
	}
	if err := f.DisposeTunDevices(cfg); err != nil {
		t.Fatalf("DisposeTunDevices: %v", err)
	}
}

func TestEnableForwarding_FirstCallError(t *testing.T) {
	f := newFactory(&mockIP{}, &mockIPT{}, &mockIOCTL{}, &mockSys{netErr: true})
	_, err := f.CreateTunDevice(cfg)
	if err == nil || !strings.Contains(err.Error(), "failed to enable IPv4 packet forwarding") {
		t.Errorf("expected forwarding error, got %v", err)
	}
}

func TestEnableForwarding_SecondCallError(t *testing.T) {
	f := newFactory(&mockIP{}, &mockIPT{}, &mockIOCTL{}, &mockSys{
		netOutput: []byte("net.ipv4.ip_forward = 0\n"), wErr: true,
	})
	_, err := f.CreateTunDevice(cfg)
	if err == nil || !strings.Contains(err.Error(), "failed to enable IPv4 packet forwarding") {
		t.Errorf("expected second-call forwarding error, got %v", err)
	}
}

func TestCreateTunDevice_CreateTunStepErrors(t *testing.T) {
	cases := []struct{ tag, want string }{
		{"TunTapAddDevTun", "could not create tuntap dev"},
		{"LinkSetDevUp", "could not set tuntap dev up"},
		{"LinkSetDevMTU", "could not set mtu on tuntap dev"},
		{"AddrAddDev", "failed to convert server ip to CIDR format"},
		{"CreateTunInterface", "failed to open TUN interface"},
	}
	for _, c := range cases {
		var ipMock ip.Contract
		ioMock := &mockIOCTL{}
		if c.tag == "CreateTunInterface" {
			ioMock.createErr = errors.New("io_err")
			ipMock = &mockIP{}
		} else {
			ipMock = &mockIPErr{
				mockIP: &mockIP{},
				errTag: c.tag,
				err:    errors.New("ip_err"),
			}
		}
		f := newFactory(ipMock, &mockIPT{}, ioMock, &mockSys{})
		_, err := f.CreateTunDevice(cfg)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("case %s: expected error containing %q, got %v", c.tag, c.want, err)
		}
	}
}

func TestCreateTunDevice_ConfigureStepErrors(t *testing.T) {
	cases := []struct {
		setup func() *ServerTunFactory
		want  string
	}{
		{
			setup: func() *ServerTunFactory {
				return newFactory(
					&mockIPRouteErr{&mockIP{}, errors.New("route_err")},
					&mockIPT{}, &mockIOCTL{}, &mockSys{},
				)
			},
			want: "route_err",
		},
		{
			setup: func() *ServerTunFactory {
				return newFactory(
					&mockIP{},
					&mockIPTErr{&mockIPT{}, "EnableDevMasquerade", errors.New("masq_err")},
					&mockIOCTL{}, &mockSys{},
				)
			},
			want: "failed enabling NAT",
		},
		{
			setup: func() *ServerTunFactory {
				return newFactory(
					&mockIP{},
					&mockIPTErr{&mockIPT{}, "EnableForwardingFromTunToDev", errors.New("fwd_err")},
					&mockIOCTL{}, &mockSys{},
				)
			},
			want: "failed to set up forwarding",
		},
	}
	for _, c := range cases {
		f := c.setup()
		_, err := f.CreateTunDevice(cfg)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("expected configure error %q, got %v", c.want, err)
		}
	}
}

func TestDisposeTunDevices_ErrorPaths(t *testing.T) {
	// open tun error
	f1 := newFactory(&mockIP{}, &mockIPT{}, &mockIOCTL{createErr: errors.New("io_err")}, &mockSys{})
	if err := f1.DisposeTunDevices(cfg); err == nil ||
		!strings.Contains(err.Error(), "failed to open TUN interface") {
		t.Errorf("expected open tun error, got %v", err)
	}
	// delete link error
	f2 := newFactory(&mockIPErrDel{&mockIP{}, errors.New("del_err")}, &mockIPT{}, &mockIOCTL{}, &mockSys{})
	if err := f2.DisposeTunDevices(cfg); err == nil ||
		!strings.Contains(err.Error(), "error deleting TUN device") {
		t.Errorf("expected delete error, got %v", err)
	}
}

func TestSetupAndClearForwarding_Errors(t *testing.T) {
	defaultIP, defaultIPT, defaultIO := &mockIP{}, &mockIPT{}, &mockIOCTL{}

	// setup: detect error
	ioErr := &mockIOCTL{detectErr: errors.New("det_err")}
	tun1, _ := ioErr.CreateTunInterface("tunX")
	f1 := newFactory(defaultIP, defaultIPT, ioErr, &mockSys{})
	if err := f1.setupForwarding(tun1, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to determing tunnel ifName") {
		t.Errorf("expected detect name error, got %v", err)
	}

	// setup: empty name
	ioEmpty := &mockIOCTL{}
	tun2, _ := ioEmpty.CreateTunInterface("tunY")
	ioEmpty.name = "" // override after CreateTunInterface
	f2 := newFactory(defaultIP, defaultIPT, ioEmpty, &mockSys{})
	if err := f2.setupForwarding(tun2, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("expected empty name error, got %v", err)
	}

	// setup: iptables error
	iptErr := &mockIPTErr{&mockIPT{}, "EnableForwardingFromTunToDev", errors.New("f_err")}
	tun3, _ := defaultIO.CreateTunInterface("tunZ")
	f3 := newFactory(defaultIP, iptErr, defaultIO, &mockSys{})
	if err := f3.setupForwarding(tun3, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to setup forwarding rule") {
		t.Errorf("expected forwarding rule error, got %v", err)
	}

	// clear: detect error
	ioErr2 := &mockIOCTL{detectErr: errors.New("det_err")}
	tun4, _ := ioErr2.CreateTunInterface("tunA")
	f4 := newFactory(defaultIP, defaultIPT, ioErr2, &mockSys{})
	if err := f4.clearForwarding(tun4, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to determing tunnel ifName") {
		t.Errorf("expected detect name error, got %v", err)
	}

	// clear: empty name
	ioEmpty2 := &mockIOCTL{}
	tun5, _ := ioEmpty2.CreateTunInterface("tunB")
	ioEmpty2.name = "" // override after CreateTunInterface
	f5 := newFactory(defaultIP, defaultIPT, ioEmpty2, &mockSys{})
	if err := f5.clearForwarding(tun5, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("expected empty name error, got %v", err)
	}

	// clear: disable tun->dev error
	iptErr2 := &mockIPTErr{&mockIPT{}, "DisableForwardingFromTunToDev", errors.New("dtd_err")}
	tun6, _ := defaultIO.CreateTunInterface("tunC")
	f6 := newFactory(defaultIP, iptErr2, defaultIO, &mockSys{})
	if err := f6.clearForwarding(tun6, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected disable tun->dev error, got %v", err)
	}

	// clear: disable dev->tun error
	iptErr3 := &mockIPTErr{&mockIPT{}, "DisableForwardingFromDevToTun", errors.New("ddt_err")}
	tun7, _ := defaultIO.CreateTunInterface("tunD")
	f7 := newFactory(defaultIP, iptErr3, defaultIO, &mockSys{})
	if err := f7.clearForwarding(tun7, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected disable dev->tun error, got %v", err)
	}
}
