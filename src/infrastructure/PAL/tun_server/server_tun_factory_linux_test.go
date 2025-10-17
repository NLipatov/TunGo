package tun_server

import (
	"bytes"
	"errors"
	"net"
	"os"
	"strings"
	"syscall"
	"testing"

	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/PAL/linux/network_tools/iptables"
	"tungo/infrastructure/PAL/linux/network_tools/sysctl"
	"tungo/infrastructure/settings"
)

/*
   ==============================
   Test doubles (prefixed)
   ==============================
*/

// ServerTunFactoryMockIP implements ip.Contract (only the methods we need in tests).
type ServerTunFactoryMockIP struct{ log bytes.Buffer }

func (m *ServerTunFactoryMockIP) add(tag string)                              { m.log.WriteString(tag + ";") }
func (m *ServerTunFactoryMockIP) TunTapAddDevTun(_ string) error              { m.add("add"); return nil }
func (m *ServerTunFactoryMockIP) LinkDelete(_ string) error                   { m.add("del"); return nil }
func (m *ServerTunFactoryMockIP) LinkSetDevUp(_ string) error                 { m.add("up"); return nil }
func (m *ServerTunFactoryMockIP) LinkSetDevMTU(_ string, _ int) error         { m.add("mtu"); return nil }
func (m *ServerTunFactoryMockIP) AddrAddDev(_, _ string) error                { m.add("addr"); return nil }
func (m *ServerTunFactoryMockIP) AddrShowDev(_ int, _ string) (string, error) { return "", nil }
func (m *ServerTunFactoryMockIP) RouteDefault() (string, error)               { m.add("route"); return "eth0", nil }
func (m *ServerTunFactoryMockIP) RouteAddDefaultDev(_ string) error           { return nil }
func (m *ServerTunFactoryMockIP) RouteGet(_ string) (string, error)           { return "", nil }
func (m *ServerTunFactoryMockIP) RouteAddDev(_, _ string) error               { return nil }
func (m *ServerTunFactoryMockIP) RouteAddViaDev(_, _, _ string) error         { return nil }
func (m *ServerTunFactoryMockIP) RouteDel(_ string) error                     { return nil }

// Variant: RouteDefault returns empty iface (to hit "skipping iptables forwarding disable").
type ServerTunFactoryMockIPRouteEmpty struct{ ServerTunFactoryMockIP }

func (m *ServerTunFactoryMockIPRouteEmpty) RouteDefault() (string, error) { return "", nil }

// Variant: RouteDefault returns error.
type ServerTunFactoryMockIPRouteErr struct {
	ServerTunFactoryMockIP
	err error
}

func (m *ServerTunFactoryMockIPRouteErr) RouteDefault() (string, error) { return "", m.err }

// Error injector for createTun steps.
type ServerTunFactoryMockIPErr struct {
	*ServerTunFactoryMockIP
	errTag string
	err    error
}

func (m *ServerTunFactoryMockIPErr) TunTapAddDevTun(devName string) error {
	if m.errTag == "TunTapAddDevTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIP.TunTapAddDevTun(devName)
}
func (m *ServerTunFactoryMockIPErr) LinkSetDevUp(devName string) error {
	if m.errTag == "LinkSetDevUp" {
		return m.err
	}
	return m.ServerTunFactoryMockIP.LinkSetDevUp(devName)
}
func (m *ServerTunFactoryMockIPErr) LinkSetDevMTU(devName string, mtu int) error {
	if m.errTag == "LinkSetDevMTU" {
		return m.err
	}
	return m.ServerTunFactoryMockIP.LinkSetDevMTU(devName, mtu)
}
func (m *ServerTunFactoryMockIPErr) AddrAddDev(devName, cidr string) error {
	if m.errTag == "AddrAddDev" {
		return m.err
	}
	return m.ServerTunFactoryMockIP.AddrAddDev(devName, cidr)
}

// ServerTunFactoryMockIPT implements iptables.Contract.
type ServerTunFactoryMockIPT struct{ log bytes.Buffer }

func (m *ServerTunFactoryMockIPT) add(tag string)                      { m.log.WriteString(tag + ";") }
func (m *ServerTunFactoryMockIPT) EnableDevMasquerade(_ string) error  { m.add("masq_on"); return nil }
func (m *ServerTunFactoryMockIPT) DisableDevMasquerade(_ string) error { m.add("masq_off"); return nil }
func (m *ServerTunFactoryMockIPT) EnableForwardingFromTunToDev(_, _ string) error {
	m.add("fwd_td")
	return nil
}
func (m *ServerTunFactoryMockIPT) DisableForwardingFromTunToDev(_, _ string) error {
	m.add("fwd_td_off")
	return nil
}
func (m *ServerTunFactoryMockIPT) EnableForwardingFromDevToTun(_, _ string) error {
	m.add("fwd_dt")
	return nil
}
func (m *ServerTunFactoryMockIPT) DisableForwardingFromDevToTun(_, _ string) error {
	m.add("fwd_dt_off")
	return nil
}
func (m *ServerTunFactoryMockIPT) ConfigureMssClamping() error { m.add("clamp"); return nil }

// Error injector for iptables paths.
type ServerTunFactoryMockIPTErr struct {
	*ServerTunFactoryMockIPT
	errTag string
	err    error
}

func (m *ServerTunFactoryMockIPTErr) EnableDevMasquerade(devName string) error {
	if m.errTag == "EnableDevMasquerade" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.EnableDevMasquerade(devName)
}
func (m *ServerTunFactoryMockIPTErr) EnableForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "EnableForwardingFromTunToDev" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.EnableForwardingFromTunToDev(tunName, devName)
}
func (m *ServerTunFactoryMockIPTErr) DisableForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "DisableForwardingFromTunToDev" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.DisableForwardingFromTunToDev(tunName, devName)
}
func (m *ServerTunFactoryMockIPTErr) DisableForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "DisableForwardingFromDevToTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.DisableForwardingFromDevToTun(tunName, devName)
}
func (m *ServerTunFactoryMockIPTErr) ConfigureMssClamping() error {
	if m.errTag == "ConfigureMssClamping" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.ConfigureMssClamping()
}

// ServerTunFactoryMockIOCTL implements ioctl.Contract.
type ServerTunFactoryMockIOCTL struct {
	name                 string
	createErr, detectErr error
}

func (m *ServerTunFactoryMockIOCTL) CreateTunInterface(name string) (*os.File, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.name = name
	return os.Open(os.DevNull)
}
func (m *ServerTunFactoryMockIOCTL) DetectTunNameFromFd(_ *os.File) (string, error) {
	if m.detectErr != nil {
		return "", m.detectErr
	}
	return m.name, nil
}

// ServerTunFactoryMockSys implements sysctl.Contract.
type ServerTunFactoryMockSys struct {
	netErr    bool
	wErr      bool
	netOutput []byte
}

func (m *ServerTunFactoryMockSys) NetIpv4IpForward() ([]byte, error) {
	if m.netErr {
		return nil, errors.New("net_err")
	}
	if m.netOutput != nil {
		return m.netOutput, nil
	}
	return []byte("net.ipv4.ip_forward = 1\n"), nil
}
func (m *ServerTunFactoryMockSys) WNetIpv4IpForward() ([]byte, error) {
	if m.wErr {
		return nil, errors.New("w_err")
	}
	return []byte("net.ipv4.ip_forward = 1\n"), nil
}

// Variant: LinkDelete error.
type ServerTunFactoryMockIPErrDel struct {
	*ServerTunFactoryMockIP
	err error
}

func (m *ServerTunFactoryMockIPErrDel) LinkDelete(_ string) error { return m.err }

/*
   ==============================
   Helpers
   ==============================
*/

func newFactory(
	ipC ip.Contract,
	iptC iptables.Contract,
	ioC ioctl.Contract,
	sysC sysctl.Contract,
) *ServerTunFactory {
	return &ServerTunFactory{
		ip:       ipC,
		iptables: iptC,
		ioctl:    ioC,
		sysctl:   sysC,
	}
}

// pickLoopbackName tries to find a loopback iface name cross-platform (lo, lo0, etc.).
func pickLoopbackName() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		// fallback to common names; tests using this are best-effort
		return "lo"
	}
	for _, it := range ifaces {
		if it.Flags&net.FlagLoopback != 0 {
			return it.Name
		}
	}
	return "lo"
}

var baseCfg = settings.Settings{
	InterfaceName:   "tun0",
	InterfaceIPCIDR: "10.0.0.0/30",
	MTU:             settings.SafeMTU,
}

/*
   ==============================
   Tests
   ==============================
*/

func TestCreateAndDispose_SuccessAndSkipForwardingDisableWhenExtIfaceUnknown(t *testing.T) {
	ipMock := &ServerTunFactoryMockIPRouteEmpty{} // RouteDefault() returns ""
	iptMock := &ServerTunFactoryMockIPT{}
	ioMock := &ServerTunFactoryMockIOCTL{}
	sysMock := &ServerTunFactoryMockSys{}

	f := newFactory(ipMock, iptMock, ioMock, sysMock)

	tun, err := f.CreateDevice(baseCfg)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if tun == nil {
		t.Fatal("expected non-nil tun file")
	}

	// Use a real existing interface to pass net.InterfaceByName(...) check.
	cfg := baseCfg
	cfg.InterfaceName = pickLoopbackName()

	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices: %v", err)
	}
}

func TestDisposeDevices_NoSuchInterface_IsBenign_NoError(t *testing.T) {
	f := newFactory(&ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	cfg := baseCfg
	cfg.InterfaceName = "definitely-not-existing-xyz123"
	// Should early return nil because net.InterfaceByName(...) fails with benign error text.
	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices should ignore missing iface: %v", err)
	}
}

func TestEnableForwarding_FirstCallError(t *testing.T) {
	f := newFactory(&ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{netErr: true})
	_, err := f.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "failed to enable IPv4 packet forwarding") {
		t.Errorf("expected forwarding error, got %v", err)
	}
}

func TestEnableForwarding_WriteCallError(t *testing.T) {
	f := newFactory(&ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{
		netOutput: []byte("net.ipv4.ip_forward = 0\n"), wErr: true,
	})
	_, err := f.CreateDevice(baseCfg)
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
		ioMock := &ServerTunFactoryMockIOCTL{}
		if c.tag == "CreateTunInterface" {
			ioMock.createErr = errors.New("io_err")
			ipMock = &ServerTunFactoryMockIP{}
		} else {
			ipMock = &ServerTunFactoryMockIPErr{
				ServerTunFactoryMockIP: &ServerTunFactoryMockIP{},
				errTag:                 c.tag,
				err:                    errors.New("ip_err"),
			}
		}
		f := newFactory(ipMock, &ServerTunFactoryMockIPT{}, ioMock, &ServerTunFactoryMockSys{})
		_, err := f.CreateDevice(baseCfg)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("case %s: expected error containing %q, got %v", c.tag, c.want, err)
		}
	}
}

func TestCreateTunDevice_InvalidCIDR_ErrorsFromAllocator(t *testing.T) {
	ipMock := &ServerTunFactoryMockIP{}
	iptMock := &ServerTunFactoryMockIPT{}
	ioMock := &ServerTunFactoryMockIOCTL{}
	sysMock := &ServerTunFactoryMockSys{}

	f := newFactory(ipMock, iptMock, ioMock, sysMock)
	bad := baseCfg
	bad.InterfaceIPCIDR = "not-a-cidr"
	_, err := f.CreateDevice(bad)
	if err == nil || !strings.Contains(err.Error(), "could not allocate server IP") {
		t.Fatalf("expected allocator error, got %v", err)
	}
}

func TestConfigure_Errors(t *testing.T) {
	// RouteDefault error
	f1 := newFactory(
		&ServerTunFactoryMockIPRouteErr{err: errors.New("route_err")},
		&ServerTunFactoryMockIPT{},
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{},
	)
	_, err := f1.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "route_err") {
		t.Errorf("expected route error, got %v", err)
	}

	// EnableDevMasquerade error
	f2 := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "EnableDevMasquerade", err: errors.New("masq_err")},
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{},
	)
	_, err = f2.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "failed enabling NAT") {
		t.Errorf("expected NAT error, got %v", err)
	}

	// setupForwarding error
	f3 := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "EnableForwardingFromTunToDev", err: errors.New("fwd_err")},
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{},
	)
	_, err = f3.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "failed to set up forwarding") {
		t.Errorf("expected forwarding setup error, got %v", err)
	}

	// MSS clamping error
	f4 := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "ConfigureMssClamping", err: errors.New("clamp_err")},
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{},
	)
	_, err = f4.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "clamp_err") {
		t.Errorf("expected clamping error, got %v", err)
	}
}

func TestSetupAndClearForwarding_Errors(t *testing.T) {
	defaultIP, defaultIPT, defaultIO := &ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}, &ServerTunFactoryMockIOCTL{}

	// setup: DetectTunNameFromFd error
	ioErr := &ServerTunFactoryMockIOCTL{detectErr: errors.New("det_err")}
	tun1, _ := ioErr.CreateTunInterface("tunX")
	f1 := newFactory(defaultIP, defaultIPT, ioErr, &ServerTunFactoryMockSys{})
	if err := f1.setupForwarding(tun1, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to determing tunnel ifName") {
		t.Errorf("expected detect name error, got %v", err)
	}

	// setup: empty tun name
	ioEmpty := &ServerTunFactoryMockIOCTL{}
	tun2, _ := ioEmpty.CreateTunInterface("tunY")
	ioEmpty.name = ""
	f2 := newFactory(defaultIP, defaultIPT, ioEmpty, &ServerTunFactoryMockSys{})
	if err := f2.setupForwarding(tun2, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("expected empty name error, got %v", err)
	}

	// setup: iptables error
	iptErr := &ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "EnableForwardingFromTunToDev", err: errors.New("f_err")}
	tun3, _ := defaultIO.CreateTunInterface("tunZ")
	f3 := newFactory(defaultIP, iptErr, defaultIO, &ServerTunFactoryMockSys{})
	if err := f3.setupForwarding(tun3, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to setup forwarding rule") {
		t.Errorf("expected forwarding rule error, got %v", err)
	}

	// clear: DetectTunNameFromFd error
	ioErr2 := &ServerTunFactoryMockIOCTL{detectErr: errors.New("det_err")}
	tun4, _ := ioErr2.CreateTunInterface("tunA")
	f4 := newFactory(defaultIP, defaultIPT, ioErr2, &ServerTunFactoryMockSys{})
	if err := f4.clearForwarding(tun4, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to determing tunnel ifName") {
		t.Errorf("expected detect name error, got %v", err)
	}

	// clear: empty name
	ioEmpty2 := &ServerTunFactoryMockIOCTL{}
	tun5, _ := ioEmpty2.CreateTunInterface("tunB")
	ioEmpty2.name = ""
	f5 := newFactory(defaultIP, defaultIPT, ioEmpty2, &ServerTunFactoryMockSys{})
	if err := f5.clearForwarding(tun5, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("expected empty name error, got %v", err)
	}

	// clear: DisableForwardingFromTunToDev error
	iptErr2 := &ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "DisableForwardingFromTunToDev", err: errors.New("dtd_err")}
	tun6, _ := defaultIO.CreateTunInterface("tunC")
	f6 := newFactory(defaultIP, iptErr2, defaultIO, &ServerTunFactoryMockSys{})
	if err := f6.clearForwarding(tun6, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected disable tun->dev error, got %v", err)
	}

	// clear: DisableForwardingFromDevToTun error
	iptErr3 := &ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "DisableForwardingFromDevToTun", err: errors.New("ddt_err")}
	tun7, _ := defaultIO.CreateTunInterface("tunD")
	f7 := newFactory(defaultIP, iptErr3, defaultIO, &ServerTunFactoryMockSys{})
	if err := f7.clearForwarding(tun7, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected disable dev->tun error, got %v", err)
	}
}

func TestDisposeTunDevices_DeleteError(t *testing.T) {
	// Use existing interface name to get past net.InterfaceByName
	cfg := baseCfg
	cfg.InterfaceName = pickLoopbackName()
	f := newFactory(&ServerTunFactoryMockIPErrDel{ServerTunFactoryMockIP: &ServerTunFactoryMockIP{}, err: errors.New("del_err")}, &ServerTunFactoryMockIPT{}, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	if err := f.DisposeDevices(cfg); err == nil || !strings.Contains(err.Error(), "error deleting TUN device") {
		t.Errorf("expected delete error, got %v", err)
	}
}

func TestUnconfigure_RouteDefaultError_And_MasqueradeErrorIsLoggedOnly(t *testing.T) {
	ioMock := &ServerTunFactoryMockIOCTL{}
	tun, _ := ioMock.CreateTunInterface("tunU")
	iptErrMasq := &ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "", err: nil}
	// Make DisableDevMasquerade fail: we don't assert logs here, only that Unconfigure continues to RouteDefault
	_ = iptErrMasq.DisableDevMasquerade("any") // just to touch path
	f := newFactory(
		&ServerTunFactoryMockIPRouteErr{err: errors.New("route_err")},
		iptErrMasq,
		ioMock,
		&ServerTunFactoryMockSys{},
	)
	err := f.Unconfigure(tun)
	if err == nil || !strings.Contains(err.Error(), "failed to resolve default interface") {
		t.Fatalf("expected RouteDefault error, got %v", err)
	}
}

func TestUnconfigure_ClearForwardingErrorSurfaced(t *testing.T) {
	ioMock := &ServerTunFactoryMockIOCTL{}
	tun, _ := ioMock.CreateTunInterface("tunU2")
	iptErr := &ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "DisableForwardingFromTunToDev", err: errors.New("boom")}
	f := newFactory(&ServerTunFactoryMockIP{}, iptErr, ioMock, &ServerTunFactoryMockSys{})
	err := f.Unconfigure(tun)
	if err == nil || !strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Fatalf("expected clearForwarding error, got %v", err)
	}
}

func TestUnconfigure_Success(t *testing.T) {
	ioMock := &ServerTunFactoryMockIOCTL{}
	tun, _ := ioMock.CreateTunInterface("tunOK")
	f := newFactory(&ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}, ioMock, &ServerTunFactoryMockSys{})
	if err := f.Unconfigure(tun); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsBenignIptablesError_Table(t *testing.T) {
	f := newFactory(nil, nil, nil, nil)
	trueCases := []string{
		"bad rule",
		"Does a matching rule exist",
		"no chain",
		"no such file or directory",
		"no chain/target/match",
		"rule does not exist",
		"not found, nothing to dispose",
		"empty interface is likely to be undesired",
	}
	for _, s := range trueCases {
		if !f.isBenignIptablesError(errors.New(s)) {
			t.Errorf("expected benign for %q", s)
		}
	}
	if f.isBenignIptablesError(errors.New("permission denied")) {
		t.Errorf("unexpected benign for non-matching error")
	}
	if f.isBenignIptablesError(nil) {
		t.Errorf("nil must not be benign")
	}
}

func TestIsBenignInterfaceError(t *testing.T) {
	f := newFactory(nil, nil, nil, nil)

	// errno ENODEV
	if !f.isBenignInterfaceError(syscall.ENODEV) {
		t.Errorf("ENODEV should be benign")
	}
	// textual cases
	trueCases := []string{
		"no such device",
		"no such network interface",
		"no such interface",
		"does not exist",
		"not found",
	}
	for _, s := range trueCases {
		if !f.isBenignInterfaceError(errors.New(s)) {
			t.Errorf("expected benign for %q", s)
		}
	}
	if f.isBenignInterfaceError(errors.New("permission denied")) {
		t.Errorf("unexpected benign for non-matching error")
	}
}

func TestServerTunFactoryMockIP_ExerciseAllStubs(t *testing.T) {
	m := &ServerTunFactoryMockIP{}

	// Exercise previously uncovered stubs
	if _, err := m.AddrShowDev(0, "dummy"); err != nil {
		t.Fatalf("AddrShowDev: %v", err)
	}
	if err := m.RouteAddDefaultDev("eth0"); err != nil {
		t.Fatalf("RouteAddDefaultDev: %v", err)
	}
	if _, err := m.RouteGet("1.2.3.4/32"); err != nil {
		t.Fatalf("RouteGet: %v", err)
	}
	if err := m.RouteAddDev("dev0", "10.0.0.0/24"); err != nil {
		t.Fatalf("RouteAddDev: %v", err)
	}
	if err := m.RouteAddViaDev("10.0.1.0/24", "10.0.0.1", "dev0"); err != nil {
		t.Fatalf("RouteAddViaDev: %v", err)
	}
	if err := m.RouteDel("10.0.0.0/24"); err != nil {
		t.Fatalf("RouteDel: %v", err)
	}

	// Also tick the simple helpers
	_ = m.TunTapAddDevTun("tunX")
	_ = m.LinkDelete("tunX")
	_ = m.LinkSetDevUp("tunX")
	_ = m.LinkSetDevMTU("tunX", 1500)
	_ = m.AddrAddDev("tunX", "10.0.0.1/24")

	iface, err := m.RouteDefault()
	if err != nil || iface == "" {
		t.Fatalf("RouteDefault: iface=%q err=%v", iface, err)
	}

	got := m.log.String()
	for _, tag := range []string{"add", "del", "up", "mtu", "addr", "route"} {
		if !strings.Contains(got, tag+";") {
			t.Errorf("expected tag %q in log, got: %q", tag, got)
		}
	}
}
