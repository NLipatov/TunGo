package tun_server

import (
	"bytes"
	"errors"
	"net"
	"net/netip"
	"os"
	"strings"
	"syscall"
	"testing"
	application "tungo/application/network/routing/tun"

	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/PAL/linux/network_tools/iptables"
	"tungo/infrastructure/PAL/linux/network_tools/mssclamp"
	"tungo/infrastructure/PAL/linux/network_tools/sysctl"
	"tungo/infrastructure/settings"
)

/*
   ==============================
   Test doubles (prefixed)
   ==============================
*/

// plain test wrapper that does NOT use epoll and just passes through *os.File.
type testPlainWrapper struct{}

type testPlainDev struct{ f *os.File }

// Implement tun.Device on top of *os.File.
func (d *testPlainDev) Read(p []byte) (int, error)  { return d.f.Read(p) }
func (d *testPlainDev) Write(p []byte) (int, error) { return d.f.Write(p) }
func (d *testPlainDev) Close() error                { return d.f.Close() }
func (d *testPlainDev) Fd() uintptr                 { return d.f.Fd() }

// testPlainWrapper implements tun.Wrapper.
func (testPlainWrapper) Wrap(f *os.File) (application.Device, error) {
	return &testPlainDev{f: f}, nil
}

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
func (m *ServerTunFactoryMockIP) RouteAddDefaultDev(_ string) error            { return nil }
func (m *ServerTunFactoryMockIP) Route6AddDefaultDev(_ string) error           { return nil }
func (m *ServerTunFactoryMockIP) RouteAddSplitDefaultDev(_ string) error       { return nil }
func (m *ServerTunFactoryMockIP) Route6AddSplitDefaultDev(_ string) error      { return nil }
func (m *ServerTunFactoryMockIP) RouteDelSplitDefault(_ string) error          { return nil }
func (m *ServerTunFactoryMockIP) Route6DelSplitDefault(_ string) error         { return nil }
func (m *ServerTunFactoryMockIP) RouteGet(_ string) (string, error)            { return "", nil }
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
	if m.errTag == "SetMTU" {
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
func (m *ServerTunFactoryMockIPT) EnableForwardingTunToTun(_ string) error {
	m.add("fwd_tt")
	return nil
}
func (m *ServerTunFactoryMockIPT) DisableForwardingTunToTun(_ string) error {
	m.add("fwd_tt_off")
	return nil
}
func (m *ServerTunFactoryMockIPT) Enable6DevMasquerade(_ string) error               { return nil }
func (m *ServerTunFactoryMockIPT) Disable6DevMasquerade(_ string) error              { return nil }
func (m *ServerTunFactoryMockIPT) Enable6ForwardingFromTunToDev(_, _ string) error    { return nil }
func (m *ServerTunFactoryMockIPT) Disable6ForwardingFromTunToDev(_, _ string) error   { return nil }
func (m *ServerTunFactoryMockIPT) Enable6ForwardingFromDevToTun(_, _ string) error    { return nil }
func (m *ServerTunFactoryMockIPT) Disable6ForwardingFromDevToTun(_, _ string) error   { return nil }
func (m *ServerTunFactoryMockIPT) Enable6ForwardingTunToTun(_ string) error           { return nil }
func (m *ServerTunFactoryMockIPT) Disable6ForwardingTunToTun(_ string) error          { return nil }

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
func (m *ServerTunFactoryMockIPTErr) EnableForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "EnableForwardingFromDevToTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.EnableForwardingFromDevToTun(tunName, devName)
}
func (m *ServerTunFactoryMockIPTErr) DisableForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "DisableForwardingFromDevToTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.DisableForwardingFromDevToTun(tunName, devName)
}
func (m *ServerTunFactoryMockIPTErr) EnableForwardingTunToTun(tunName string) error {
	if m.errTag == "EnableForwardingTunToTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.EnableForwardingTunToTun(tunName)
}
func (m *ServerTunFactoryMockIPTErr) DisableForwardingTunToTun(tunName string) error {
	if m.errTag == "DisableForwardingTunToTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.DisableForwardingTunToTun(tunName)
}
func (m *ServerTunFactoryMockIPTErr) Enable6DevMasquerade(devName string) error {
	if m.errTag == "Enable6DevMasquerade" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.Enable6DevMasquerade(devName)
}
func (m *ServerTunFactoryMockIPTErr) Enable6ForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "Enable6ForwardingFromTunToDev" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.Enable6ForwardingFromTunToDev(tunName, devName)
}
func (m *ServerTunFactoryMockIPTErr) Enable6ForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "Enable6ForwardingFromDevToTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.Enable6ForwardingFromDevToTun(tunName, devName)
}
func (m *ServerTunFactoryMockIPTErr) Enable6ForwardingTunToTun(tunName string) error {
	if m.errTag == "Enable6ForwardingTunToTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.Enable6ForwardingTunToTun(tunName)
}
func (m *ServerTunFactoryMockIPTErr) Disable6ForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "Disable6ForwardingFromTunToDev" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.Disable6ForwardingFromTunToDev(tunName, devName)
}
func (m *ServerTunFactoryMockIPTErr) Disable6ForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "Disable6ForwardingFromDevToTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.Disable6ForwardingFromDevToTun(tunName, devName)
}
func (m *ServerTunFactoryMockIPTErr) Disable6ForwardingTunToTun(tunName string) error {
	if m.errTag == "Disable6ForwardingTunToTun" {
		return m.err
	}
	return m.ServerTunFactoryMockIPT.Disable6ForwardingTunToTun(tunName)
}

// ServerTunFactoryMockIPErrNthAddr fails on the Nth call to AddrAddDev (1-based).
type ServerTunFactoryMockIPErrNthAddr struct {
	*ServerTunFactoryMockIP
	failOnCall int
	callCount  int
	err        error
}

func (m *ServerTunFactoryMockIPErrNthAddr) AddrAddDev(devName, cidr string) error {
	m.callCount++
	if m.callCount == m.failOnCall {
		return m.err
	}
	return m.ServerTunFactoryMockIP.AddrAddDev(devName, cidr)
}

// ServerTunFactoryMockMSS implements mssclamp.Contract.
type ServerTunFactoryMockMSS struct{ log bytes.Buffer }

func (m *ServerTunFactoryMockMSS) add(tag string)         { m.log.WriteString(tag + ";") }
func (m *ServerTunFactoryMockMSS) Install(_ string) error { m.add("mss_on"); return nil }
func (m *ServerTunFactoryMockMSS) Remove(_ string) error  { m.add("mss_off"); return nil }

// Error injector for MSS clamping paths.
type ServerTunFactoryMockMSSErr struct {
	*ServerTunFactoryMockMSS
	errTag string
	err    error
}

func (m *ServerTunFactoryMockMSSErr) Install(tunName string) error {
	if m.errTag == "Install" {
		return m.err
	}
	return m.ServerTunFactoryMockMSS.Install(tunName)
}

func (m *ServerTunFactoryMockMSSErr) Remove(tunName string) error {
	if m.errTag == "Remove" {
		return m.err
	}
	return m.ServerTunFactoryMockMSS.Remove(tunName)
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
	netErr     bool
	wErr       bool
	netOutput  []byte
	net6Err    bool
	w6Err      bool
	net6Output []byte
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
func (m *ServerTunFactoryMockSys) NetIpv6ConfAllForwarding() ([]byte, error) {
	if m.net6Err {
		return nil, errors.New("net6_err")
	}
	if m.net6Output != nil {
		return m.net6Output, nil
	}
	return []byte("net.ipv6.conf.all.forwarding = 1\n"), nil
}
func (m *ServerTunFactoryMockSys) WNetIpv6ConfAllForwarding() ([]byte, error) {
	if m.w6Err {
		return nil, errors.New("w6_err")
	}
	return []byte("net.ipv6.conf.all.forwarding = 1\n"), nil
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
	mssC mssclamp.Contract,
	ioC ioctl.Contract,
	sysC sysctl.Contract,
) *ServerTunFactory {
	if ipC == nil {
		ipC = &ServerTunFactoryMockIP{}
	}
	if iptC == nil {
		iptC = &ServerTunFactoryMockIPT{}
	}
	if mssC == nil {
		mssC = &ServerTunFactoryMockMSS{}
	}
	if ioC == nil {
		ioC = &ServerTunFactoryMockIOCTL{}
	}
	if sysC == nil {
		sysC = &ServerTunFactoryMockSys{}
	}
	return &ServerTunFactory{
		ip:       ipC,
		iptables: iptC,
		mss:      mssC,
		ioctl:    ioC,
		sysctl:   sysC,
		wrapper:  testPlainWrapper{},
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
	InterfaceSubnet: netip.MustParsePrefix("10.0.0.0/30"),
	MTU:             settings.SafeMTU,
}

var baseCfgIPv6 = settings.Settings{
	InterfaceName:   "tun0",
	InterfaceSubnet: netip.MustParsePrefix("10.0.0.0/30"),
	IPv6Subnet:      netip.MustParsePrefix("fd00::/64"),
	MTU:             settings.SafeMTU,
}

// ServerTunFactoryMockIPTBenign simulates benign iptables errors that must be ignored.
type ServerTunFactoryMockIPTBenign struct{ log bytes.Buffer }

func (m *ServerTunFactoryMockIPTBenign) EnableDevMasquerade(_ string) error { return nil }
func (m *ServerTunFactoryMockIPTBenign) DisableDevMasquerade(_ string) error {
	return errors.New("rule does not exist")
} // benign
func (m *ServerTunFactoryMockIPTBenign) EnableForwardingFromTunToDev(_, _ string) error {
	return nil
}
func (m *ServerTunFactoryMockIPTBenign) DisableForwardingFromTunToDev(_, _ string) error {
	return errors.New("no chain/target/match") // benign
}
func (m *ServerTunFactoryMockIPTBenign) EnableForwardingFromDevToTun(_, _ string) error { return nil }
func (m *ServerTunFactoryMockIPTBenign) DisableForwardingFromDevToTun(_, _ string) error {
	return errors.New("rule does not exist") // benign
}
func (m *ServerTunFactoryMockIPTBenign) EnableForwardingTunToTun(_ string) error  { return nil }
func (m *ServerTunFactoryMockIPTBenign) DisableForwardingTunToTun(_ string) error { return nil }
func (m *ServerTunFactoryMockIPTBenign) Enable6DevMasquerade(_ string) error               { return nil }
func (m *ServerTunFactoryMockIPTBenign) Disable6DevMasquerade(_ string) error              { return nil }
func (m *ServerTunFactoryMockIPTBenign) Enable6ForwardingFromTunToDev(_, _ string) error    { return nil }
func (m *ServerTunFactoryMockIPTBenign) Disable6ForwardingFromTunToDev(_, _ string) error   { return nil }
func (m *ServerTunFactoryMockIPTBenign) Enable6ForwardingFromDevToTun(_, _ string) error    { return nil }
func (m *ServerTunFactoryMockIPTBenign) Disable6ForwardingFromDevToTun(_, _ string) error   { return nil }
func (m *ServerTunFactoryMockIPTBenign) Enable6ForwardingTunToTun(_ string) error           { return nil }
func (m *ServerTunFactoryMockIPTBenign) Disable6ForwardingTunToTun(_ string) error          { return nil }

// ServerTunFactoryMockIPTAlwaysErr simulates non-benign iptables errors that are logged but not fatal.
type ServerTunFactoryMockIPTAlwaysErr struct{}

func (m *ServerTunFactoryMockIPTAlwaysErr) EnableDevMasquerade(_ string) error { return nil }
func (m *ServerTunFactoryMockIPTAlwaysErr) DisableDevMasquerade(_ string) error {
	return errors.New("permission denied")
}
func (m *ServerTunFactoryMockIPTAlwaysErr) EnableForwardingFromTunToDev(_, _ string) error {
	return nil
}
func (m *ServerTunFactoryMockIPTAlwaysErr) DisableForwardingFromTunToDev(_, _ string) error {
	return errors.New("permission denied")
}
func (m *ServerTunFactoryMockIPTAlwaysErr) EnableForwardingFromDevToTun(_, _ string) error {
	return nil
}
func (m *ServerTunFactoryMockIPTAlwaysErr) DisableForwardingFromDevToTun(_, _ string) error {
	return errors.New("permission denied")
}
func (m *ServerTunFactoryMockIPTAlwaysErr) EnableForwardingTunToTun(_ string) error { return nil }
func (m *ServerTunFactoryMockIPTAlwaysErr) DisableForwardingTunToTun(_ string) error {
	return errors.New("permission denied")
}
func (m *ServerTunFactoryMockIPTAlwaysErr) Enable6DevMasquerade(_ string) error { return nil }
func (m *ServerTunFactoryMockIPTAlwaysErr) Disable6DevMasquerade(_ string) error {
	return errors.New("permission denied")
}
func (m *ServerTunFactoryMockIPTAlwaysErr) Enable6ForwardingFromTunToDev(_, _ string) error {
	return nil
}
func (m *ServerTunFactoryMockIPTAlwaysErr) Disable6ForwardingFromTunToDev(_, _ string) error {
	return errors.New("permission denied")
}
func (m *ServerTunFactoryMockIPTAlwaysErr) Enable6ForwardingFromDevToTun(_, _ string) error {
	return nil
}
func (m *ServerTunFactoryMockIPTAlwaysErr) Disable6ForwardingFromDevToTun(_, _ string) error {
	return errors.New("permission denied")
}
func (m *ServerTunFactoryMockIPTAlwaysErr) Enable6ForwardingTunToTun(_ string) error { return nil }
func (m *ServerTunFactoryMockIPTAlwaysErr) Disable6ForwardingTunToTun(_ string) error {
	return errors.New("permission denied")
}

/*
   ==============================
   Tests
   ==============================
*/

func TestCreateAndDispose_SuccessAndSkipForwardingDisableWhenExtIfaceUnknown(t *testing.T) {
	ipMock := &ServerTunFactoryMockIPRouteEmpty{} // RouteDefault() returns ""
	iptMock := &ServerTunFactoryMockIPT{}
	mssMock := &ServerTunFactoryMockMSS{}
	ioMock := &ServerTunFactoryMockIOCTL{}
	sysMock := &ServerTunFactoryMockSys{}

	f := newFactory(ipMock, iptMock, mssMock, ioMock, sysMock)

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
	f := newFactory(&ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	cfg := baseCfg
	cfg.InterfaceName = "definitely-not-existing-xyz123"
	// Should early return nil because net.InterfaceByName(...) fails with benign error text.
	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices should ignore missing iface: %v", err)
	}
}

func TestEnableForwarding_FirstCallError(t *testing.T) {
	f := newFactory(&ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{netErr: true})
	_, err := f.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "failed to enable IPv4 packet forwarding") {
		t.Errorf("expected forwarding error, got %v", err)
	}
}

func TestEnableForwarding_WriteCallError(t *testing.T) {
	f := newFactory(&ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{
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
		{"SetMTU", "could not set mtu on tuntap dev"},
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
		f := newFactory(ipMock, &ServerTunFactoryMockIPT{}, nil, ioMock, &ServerTunFactoryMockSys{})
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

	f := newFactory(ipMock, iptMock, nil, ioMock, sysMock)
	bad := baseCfg
	bad.InterfaceSubnet = netip.Prefix{}
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
		nil,
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
		nil,
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
		nil,
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
		&ServerTunFactoryMockIPT{},
		&ServerTunFactoryMockMSSErr{ServerTunFactoryMockMSS: &ServerTunFactoryMockMSS{}, errTag: "Install", err: errors.New("clamp_err")},
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{},
	)
	_, err = f4.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "clamp_err") {
		t.Errorf("expected clamping error, got %v", err)
	}
}

func TestSetupAndClearForwarding_Errors(t *testing.T) {
	defaultIP, defaultIPT := &ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}

	f1 := newFactory(defaultIP, defaultIPT, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	if err := f1.setupForwarding("", "eth0", true, true); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("expected empty name error, got %v", err)
	}

	// setup: iptables error
	iptErr := &ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "EnableForwardingFromTunToDev", err: errors.New("f_err")}
	f2 := newFactory(defaultIP, iptErr, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	if err := f2.setupForwarding("tunZ", "eth0", true, true); err == nil ||
		!strings.Contains(err.Error(), "failed to setup forwarding rule") {
		t.Errorf("expected forwarding rule error, got %v", err)
	}

	// clear: empty name
	f3 := newFactory(defaultIP, defaultIPT, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	if err := f3.clearForwarding("", "eth0", true, true); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("expected empty name error, got %v", err)
	}

	// clear: DisableForwardingFromTunToDev error
	iptErr2 := &ServerTunFactoryMockIPTErr{ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{}, errTag: "DisableForwardingFromTunToDev", err: errors.New("dtd_err")}
	f4 := newFactory(defaultIP, iptErr2, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	if err := f4.clearForwarding("tunC", "eth0", true, true); err == nil ||
		!strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected clearForwarding error, got %v", err)
	}
}

func TestDisposeTunDevices_DeleteError(t *testing.T) {
	// Use existing interface name to get past net.InterfaceByName
	cfg := baseCfg
	cfg.InterfaceName = pickLoopbackName()
	f := newFactory(&ServerTunFactoryMockIPErrDel{ServerTunFactoryMockIP: &ServerTunFactoryMockIP{}, err: errors.New("del_err")}, &ServerTunFactoryMockIPT{}, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
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
		nil,
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
	f := newFactory(&ServerTunFactoryMockIP{}, iptErr, nil, ioMock, &ServerTunFactoryMockSys{})
	err := f.Unconfigure(tun)
	if err == nil || !strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Fatalf("expected clearForwarding error, got %v", err)
	}
}

func TestUnconfigure_Success(t *testing.T) {
	ioMock := &ServerTunFactoryMockIOCTL{}
	tun, _ := ioMock.CreateTunInterface("tunOK")
	f := newFactory(&ServerTunFactoryMockIP{}, &ServerTunFactoryMockIPT{}, nil, ioMock, &ServerTunFactoryMockSys{})
	if err := f.Unconfigure(tun); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsBenignNetfilterError_Table(t *testing.T) {
	f := newFactory(nil, nil, nil, nil, nil)
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
		if !f.isBenignNetfilterError(errors.New(s)) {
			t.Errorf("expected benign for %q", s)
		}
	}
	if f.isBenignNetfilterError(errors.New("permission denied")) {
		t.Errorf("unexpected benign for non-matching error")
	}
	if f.isBenignNetfilterError(nil) {
		t.Errorf("nil must not be benign")
	}
}

func TestIsBenignInterfaceError(t *testing.T) {
	f := newFactory(nil, nil, nil, nil, nil)

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

func TestIsBenignInterfaceError_NilIsFalse(t *testing.T) {
	// nil must not be treated as benign
	f := newFactory(nil, nil, nil, nil, nil)
	if f.isBenignInterfaceError(nil) {
		t.Fatalf("nil must not be benign")
	}
}

func TestDisposeDevices_BenignIptablesErrorsAreIgnored(t *testing.T) {
	// Arrange: ext iface is non-empty, iptables returns benign errors ⇒ must be ignored
	ipMock := &ServerTunFactoryMockIP{}
	iptMock := &ServerTunFactoryMockIPTBenign{}
	f := newFactory(ipMock, iptMock, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})

	cfg := baseCfg
	cfg.InterfaceName = pickLoopbackName() // ensure InterfaceByName(...) passes

	// Act + Assert
	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices should ignore benign iptables errors, got: %v", err)
	}
}

func TestDisposeDevices_NonBenignIptablesErrorsAreLoggedButIgnored(t *testing.T) {
	// Arrange: iptables returns non-benign errors; code should log them but still proceed
	ipMock := &ServerTunFactoryMockIP{}
	iptMock := &ServerTunFactoryMockIPTAlwaysErr{}
	f := newFactory(ipMock, iptMock, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})

	cfg := baseCfg
	cfg.InterfaceName = pickLoopbackName()

	// Act + Assert
	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices should not fail on non-benign iptables errors (only log), got: %v", err)
	}
}

func TestUnconfigure_DetectTunNameError_ContinuesToRouteDefault(t *testing.T) {
	// If DetectTunNameFromFd fails, Unconfigure must continue and then surface RouteDefault error.
	ioMock := &ServerTunFactoryMockIOCTL{detectErr: errors.New("detect_failed")}
	tun, _ := ioMock.CreateTunInterface("tunX")
	f := newFactory(
		&ServerTunFactoryMockIPRouteErr{err: errors.New("route_err")},
		&ServerTunFactoryMockIPT{},
		nil,
		ioMock,
		&ServerTunFactoryMockSys{},
	)

	err := f.Unconfigure(tun)
	if err == nil || !strings.Contains(err.Error(), "failed to resolve default interface") {
		t.Fatalf("expected RouteDefault error after detect failure, got %v", err)
	}
}

func TestEnableForwarding_WritesWhenDisabled_Succeeds(t *testing.T) {
	// First sysctl returns 0 → we write 1 and proceed successfully.
	f := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPT{},
		nil,
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{netOutput: []byte("net.ipv4.ip_forward = 0\n")},
	)

	tun, err := f.CreateDevice(baseCfg)
	if err != nil {
		t.Fatalf("CreateDevice should succeed after enabling ip_forward, got: %v", err)
	}
	if tun == nil {
		t.Fatal("expected non-nil tun file")
	}
	_ = tun.Close()
}

func TestEnableForwarding_IPv6ReadError(t *testing.T) {
	f := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPT{},
		nil,
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{net6Err: true},
	)
	_, err := f.CreateDevice(baseCfgIPv6)
	if err == nil || !strings.Contains(err.Error(), "failed to read IPv6 forwarding state") {
		t.Errorf("expected IPv6 read error, got %v", err)
	}
}

func TestEnableForwarding_IPv6Skipped_WhenNoIPv6Subnet(t *testing.T) {
	// IPv6 sysctl fails, but baseCfg has no IPv6Subnet — must succeed.
	f := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPT{},
		nil,
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{net6Err: true},
	)
	tun, err := f.CreateDevice(baseCfg)
	if err != nil {
		t.Fatalf("CreateDevice should skip IPv6 forwarding when no IPv6 subnet, got: %v", err)
	}
	_ = tun.Close()
}

func TestEnableForwarding_IPv6WriteError(t *testing.T) {
	f := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPT{},
		nil,
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{
			net6Output: []byte("net.ipv6.conf.all.forwarding = 0\n"),
			w6Err:      true,
		},
	)
	_, err := f.CreateDevice(baseCfgIPv6)
	if err == nil || !strings.Contains(err.Error(), "failed to enable IPv6 packet forwarding") {
		t.Errorf("expected IPv6 write error, got %v", err)
	}
}

func TestEnableForwarding_IPv6WritesWhenDisabled_Succeeds(t *testing.T) {
	f := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPT{},
		nil,
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{net6Output: []byte("net.ipv6.conf.all.forwarding = 0\n")},
	)
	tun, err := f.CreateDevice(baseCfgIPv6)
	if err != nil {
		t.Fatalf("CreateDevice should succeed after enabling IPv6 forwarding, got: %v", err)
	}
	_ = tun.Close()
}

func TestCreateTunDevice_WithIPv6Subnet_Success(t *testing.T) {
	cfg := baseCfg
	cfg.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
	f := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPT{},
		nil,
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{},
	)
	tun, err := f.CreateDevice(cfg)
	if err != nil {
		t.Fatalf("CreateDevice with IPv6 subnet should succeed, got: %v", err)
	}
	_ = tun.Close()
}

func TestCreateTunDevice_WithIPv6Subnet_AddrAddError(t *testing.T) {
	cfg := baseCfg
	cfg.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
	ipMock := &ServerTunFactoryMockIPErrNthAddr{
		ServerTunFactoryMockIP: &ServerTunFactoryMockIP{},
		failOnCall:             2, // second AddrAddDev call (IPv6)
		err:                    errors.New("v6_addr_err"),
	}
	f := newFactory(ipMock, &ServerTunFactoryMockIPT{}, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	_, err := f.CreateDevice(cfg)
	if err == nil || !strings.Contains(err.Error(), "failed to assign IPv6 to TUN") {
		t.Errorf("expected IPv6 addr error, got %v", err)
	}
}

func TestSetupForwarding_IPv6Errors(t *testing.T) {
	cases := []struct {
		errTag string
		want   string
	}{
		{"Enable6ForwardingFromTunToDev", "failed to setup IPv6 forwarding rule"},
		{"Enable6ForwardingFromDevToTun", "failed to setup IPv6 forwarding rule"},
		{"Enable6ForwardingTunToTun", "failed to setup IPv6 client-to-client forwarding rule"},
	}
	for _, c := range cases {
		iptErr := &ServerTunFactoryMockIPTErr{
			ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{},
			errTag:                  c.errTag,
			err:                     errors.New("v6_err"),
		}
		f := newFactory(&ServerTunFactoryMockIP{}, iptErr, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
		if err := f.setupForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("case %s: expected error containing %q, got %v", c.errTag, c.want, err)
		}
	}
}

func TestClearForwarding_IPv6Errors(t *testing.T) {
	cases := []struct {
		errTag string
		want   string
	}{
		{"Disable6ForwardingFromTunToDev", "failed to execute ip6tables command"},
		{"Disable6ForwardingFromDevToTun", "failed to execute ip6tables command"},
		{"Disable6ForwardingTunToTun", "failed to execute ip6tables command"},
	}
	for _, c := range cases {
		iptErr := &ServerTunFactoryMockIPTErr{
			ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{},
			errTag:                  c.errTag,
			err:                     errors.New("v6_err"),
		}
		f := newFactory(&ServerTunFactoryMockIP{}, iptErr, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
		if err := f.clearForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("case %s: expected error containing %q, got %v", c.errTag, c.want, err)
		}
	}
}

func TestConfigure_Enable6DevMasqueradeError(t *testing.T) {
	f := newFactory(
		&ServerTunFactoryMockIP{},
		&ServerTunFactoryMockIPTErr{
			ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{},
			errTag:                  "Enable6DevMasquerade",
			err:                     errors.New("v6_masq_err"),
		},
		nil,
		&ServerTunFactoryMockIOCTL{},
		&ServerTunFactoryMockSys{},
	)
	_, err := f.CreateDevice(baseCfgIPv6)
	if err == nil || !strings.Contains(err.Error(), "failed enabling IPv6 NAT") {
		t.Errorf("expected IPv6 NAT error, got %v", err)
	}
}

func TestEnableForwarding_ForwardingFromDevToTun_Error(t *testing.T) {
	iptErr := &ServerTunFactoryMockIPTErr{
		ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{},
		errTag:                  "EnableForwardingFromDevToTun",
		err:                     errors.New("fwd_dt_err"),
	}
	f := newFactory(&ServerTunFactoryMockIP{}, iptErr, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	if err := f.setupForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), "failed to setup forwarding rule") {
		t.Errorf("expected forwarding rule error, got %v", err)
	}
}

func TestEnableForwarding_ForwardingTunToTun_Error(t *testing.T) {
	iptErr := &ServerTunFactoryMockIPTErr{
		ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{},
		errTag:                  "EnableForwardingTunToTun",
		err:                     errors.New("fwd_tt_err"),
	}
	f := newFactory(&ServerTunFactoryMockIP{}, iptErr, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	if err := f.setupForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), "failed to setup client-to-client forwarding rule") {
		t.Errorf("expected client-to-client forwarding error, got %v", err)
	}
}

func TestClearForwarding_DisableForwardingFromDevToTun_Error(t *testing.T) {
	iptErr := &ServerTunFactoryMockIPTErr{
		ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{},
		errTag:                  "DisableForwardingFromDevToTun",
		err:                     errors.New("dtd_err"),
	}
	f := newFactory(&ServerTunFactoryMockIP{}, iptErr, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	if err := f.clearForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected iptables error, got %v", err)
	}
}

func TestClearForwarding_DisableForwardingTunToTun_Error(t *testing.T) {
	iptErr := &ServerTunFactoryMockIPTErr{
		ServerTunFactoryMockIPT: &ServerTunFactoryMockIPT{},
		errTag:                  "DisableForwardingTunToTun",
		err:                     errors.New("dtt_err"),
	}
	f := newFactory(&ServerTunFactoryMockIP{}, iptErr, nil, &ServerTunFactoryMockIOCTL{}, &ServerTunFactoryMockSys{})
	if err := f.clearForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected iptables error, got %v", err)
	}
}
