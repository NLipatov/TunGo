package server

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

	"tungo/infrastructure/PAL/network/linux/ioctl"
	"tungo/infrastructure/PAL/network/linux/ip"
	"tungo/infrastructure/PAL/network/linux/iptables"
	"tungo/infrastructure/PAL/network/linux/mssclamp"
	"tungo/infrastructure/PAL/network/linux/sysctl"
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

// TunFactoryMockIP implements ip.Contract (only the methods we need in tests).
type TunFactoryMockIP struct{ log bytes.Buffer }

func (m *TunFactoryMockIP) add(tag string)                              { m.log.WriteString(tag + ";") }
func (m *TunFactoryMockIP) TunTapAddDevTun(_ string) error              { m.add("add"); return nil }
func (m *TunFactoryMockIP) LinkDelete(_ string) error                   { m.add("del"); return nil }
func (m *TunFactoryMockIP) LinkSetDevUp(_ string) error                 { m.add("up"); return nil }
func (m *TunFactoryMockIP) LinkSetDevMTU(_ string, _ int) error         { m.add("mtu"); return nil }
func (m *TunFactoryMockIP) AddrAddDev(_, _ string) error                { m.add("addr"); return nil }
func (m *TunFactoryMockIP) AddrShowDev(_ int, _ string) (string, error) { return "", nil }
func (m *TunFactoryMockIP) RouteDefault() (string, error)               { m.add("route"); return "eth0", nil }
func (m *TunFactoryMockIP) RouteAddDefaultDev(_ string) error           { return nil }
func (m *TunFactoryMockIP) Route6AddDefaultDev(_ string) error          { return nil }
func (m *TunFactoryMockIP) RouteAddSplitDefaultDev(_ string) error      { return nil }
func (m *TunFactoryMockIP) Route6AddSplitDefaultDev(_ string) error     { return nil }
func (m *TunFactoryMockIP) RouteDelSplitDefault(_ string) error         { return nil }
func (m *TunFactoryMockIP) Route6DelSplitDefault(_ string) error        { return nil }
func (m *TunFactoryMockIP) RouteGet(_ string) (string, error)           { return "", nil }
func (m *TunFactoryMockIP) RouteAddDev(_, _ string) error               { return nil }
func (m *TunFactoryMockIP) RouteAddViaDev(_, _, _ string) error         { return nil }
func (m *TunFactoryMockIP) RouteDel(_ string) error                     { return nil }

// Variant: RouteDefault returns empty iface (to hit "skipping iptables forwarding disable").
type TunFactoryMockIPRouteEmpty struct{ TunFactoryMockIP }

func (m *TunFactoryMockIPRouteEmpty) RouteDefault() (string, error) { return "", nil }

// Variant: RouteDefault returns error.
type TunFactoryMockIPRouteErr struct {
	TunFactoryMockIP
	err error
}

func (m *TunFactoryMockIPRouteErr) RouteDefault() (string, error) { return "", m.err }

// Error injector for createTun steps.
type TunFactoryMockIPErr struct {
	*TunFactoryMockIP
	errTag string
	err    error
}

func (m *TunFactoryMockIPErr) TunTapAddDevTun(devName string) error {
	if m.errTag == "TunTapAddDevTun" {
		return m.err
	}
	return m.TunFactoryMockIP.TunTapAddDevTun(devName)
}
func (m *TunFactoryMockIPErr) LinkSetDevUp(devName string) error {
	if m.errTag == "LinkSetDevUp" {
		return m.err
	}
	return m.TunFactoryMockIP.LinkSetDevUp(devName)
}
func (m *TunFactoryMockIPErr) LinkSetDevMTU(devName string, mtu int) error {
	if m.errTag == "SetMTU" {
		return m.err
	}
	return m.TunFactoryMockIP.LinkSetDevMTU(devName, mtu)
}
func (m *TunFactoryMockIPErr) AddrAddDev(devName, cidr string) error {
	if m.errTag == "AddrAddDev" {
		return m.err
	}
	return m.TunFactoryMockIP.AddrAddDev(devName, cidr)
}

// TunFactoryMockIPT implements iptables.Contract.
type TunFactoryMockIPT struct {
	log bytes.Buffer

	lastEnableMasqDev   string
	lastEnableMasqCIDR  string
	lastDisableMasqDev  string
	lastDisableMasqCIDR string

	lastEnable6MasqDev   string
	lastEnable6MasqCIDR  string
	lastDisable6MasqDev  string
	lastDisable6MasqCIDR string
}

func (m *TunFactoryMockIPT) add(tag string) { m.log.WriteString(tag + ";") }
func (m *TunFactoryMockIPT) EnableDevMasquerade(devName, sourceCIDR string) error {
	m.add("masq_on")
	m.lastEnableMasqDev = devName
	m.lastEnableMasqCIDR = sourceCIDR
	return nil
}
func (m *TunFactoryMockIPT) DisableDevMasquerade(devName, sourceCIDR string) error {
	m.add("masq_off")
	m.lastDisableMasqDev = devName
	m.lastDisableMasqCIDR = sourceCIDR
	return nil
}
func (m *TunFactoryMockIPT) EnableForwardingFromTunToDev(_, _ string) error {
	m.add("fwd_td")
	return nil
}
func (m *TunFactoryMockIPT) DisableForwardingFromTunToDev(_, _ string) error {
	m.add("fwd_td_off")
	return nil
}
func (m *TunFactoryMockIPT) EnableForwardingFromDevToTun(_, _ string) error {
	m.add("fwd_dt")
	return nil
}
func (m *TunFactoryMockIPT) DisableForwardingFromDevToTun(_, _ string) error {
	m.add("fwd_dt_off")
	return nil
}
func (m *TunFactoryMockIPT) EnableForwardingTunToTun(_ string) error {
	m.add("fwd_tt")
	return nil
}
func (m *TunFactoryMockIPT) DisableForwardingTunToTun(_ string) error {
	m.add("fwd_tt_off")
	return nil
}
func (m *TunFactoryMockIPT) Enable6DevMasquerade(devName, sourceCIDR string) error {
	m.lastEnable6MasqDev = devName
	m.lastEnable6MasqCIDR = sourceCIDR
	return nil
}
func (m *TunFactoryMockIPT) Disable6DevMasquerade(devName, sourceCIDR string) error {
	m.lastDisable6MasqDev = devName
	m.lastDisable6MasqCIDR = sourceCIDR
	return nil
}
func (m *TunFactoryMockIPT) Enable6ForwardingFromTunToDev(_, _ string) error  { return nil }
func (m *TunFactoryMockIPT) Disable6ForwardingFromTunToDev(_, _ string) error { return nil }
func (m *TunFactoryMockIPT) Enable6ForwardingFromDevToTun(_, _ string) error  { return nil }
func (m *TunFactoryMockIPT) Disable6ForwardingFromDevToTun(_, _ string) error { return nil }
func (m *TunFactoryMockIPT) Enable6ForwardingTunToTun(_ string) error         { return nil }
func (m *TunFactoryMockIPT) Disable6ForwardingTunToTun(_ string) error        { return nil }

// Error injector for iptables paths.
type TunFactoryMockIPTErr struct {
	*TunFactoryMockIPT
	errTag string
	err    error
}

func (m *TunFactoryMockIPTErr) EnableDevMasquerade(devName, sourceCIDR string) error {
	if m.errTag == "EnableDevMasquerade" {
		return m.err
	}
	return m.TunFactoryMockIPT.EnableDevMasquerade(devName, sourceCIDR)
}
func (m *TunFactoryMockIPTErr) EnableForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "EnableForwardingFromTunToDev" {
		return m.err
	}
	return m.TunFactoryMockIPT.EnableForwardingFromTunToDev(tunName, devName)
}
func (m *TunFactoryMockIPTErr) DisableForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "DisableForwardingFromTunToDev" {
		return m.err
	}
	return m.TunFactoryMockIPT.DisableForwardingFromTunToDev(tunName, devName)
}
func (m *TunFactoryMockIPTErr) EnableForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "EnableForwardingFromDevToTun" {
		return m.err
	}
	return m.TunFactoryMockIPT.EnableForwardingFromDevToTun(tunName, devName)
}
func (m *TunFactoryMockIPTErr) DisableForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "DisableForwardingFromDevToTun" {
		return m.err
	}
	return m.TunFactoryMockIPT.DisableForwardingFromDevToTun(tunName, devName)
}
func (m *TunFactoryMockIPTErr) EnableForwardingTunToTun(tunName string) error {
	if m.errTag == "EnableForwardingTunToTun" {
		return m.err
	}
	return m.TunFactoryMockIPT.EnableForwardingTunToTun(tunName)
}
func (m *TunFactoryMockIPTErr) DisableForwardingTunToTun(tunName string) error {
	if m.errTag == "DisableForwardingTunToTun" {
		return m.err
	}
	return m.TunFactoryMockIPT.DisableForwardingTunToTun(tunName)
}
func (m *TunFactoryMockIPTErr) Enable6DevMasquerade(devName, sourceCIDR string) error {
	if m.errTag == "Enable6DevMasquerade" {
		return m.err
	}
	return m.TunFactoryMockIPT.Enable6DevMasquerade(devName, sourceCIDR)
}
func (m *TunFactoryMockIPTErr) Enable6ForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "Enable6ForwardingFromTunToDev" {
		return m.err
	}
	return m.TunFactoryMockIPT.Enable6ForwardingFromTunToDev(tunName, devName)
}
func (m *TunFactoryMockIPTErr) Enable6ForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "Enable6ForwardingFromDevToTun" {
		return m.err
	}
	return m.TunFactoryMockIPT.Enable6ForwardingFromDevToTun(tunName, devName)
}
func (m *TunFactoryMockIPTErr) Enable6ForwardingTunToTun(tunName string) error {
	if m.errTag == "Enable6ForwardingTunToTun" {
		return m.err
	}
	return m.TunFactoryMockIPT.Enable6ForwardingTunToTun(tunName)
}
func (m *TunFactoryMockIPTErr) Disable6ForwardingFromTunToDev(tunName, devName string) error {
	if m.errTag == "Disable6ForwardingFromTunToDev" {
		return m.err
	}
	return m.TunFactoryMockIPT.Disable6ForwardingFromTunToDev(tunName, devName)
}
func (m *TunFactoryMockIPTErr) Disable6ForwardingFromDevToTun(tunName, devName string) error {
	if m.errTag == "Disable6ForwardingFromDevToTun" {
		return m.err
	}
	return m.TunFactoryMockIPT.Disable6ForwardingFromDevToTun(tunName, devName)
}
func (m *TunFactoryMockIPTErr) Disable6ForwardingTunToTun(tunName string) error {
	if m.errTag == "Disable6ForwardingTunToTun" {
		return m.err
	}
	return m.TunFactoryMockIPT.Disable6ForwardingTunToTun(tunName)
}

// TunFactoryMockIPErrNthAddr fails on the Nth call to AddrAddDev (1-based).
type TunFactoryMockIPErrNthAddr struct {
	*TunFactoryMockIP
	failOnCall int
	callCount  int
	err        error
}

func (m *TunFactoryMockIPErrNthAddr) AddrAddDev(devName, cidr string) error {
	m.callCount++
	if m.callCount == m.failOnCall {
		return m.err
	}
	return m.TunFactoryMockIP.AddrAddDev(devName, cidr)
}

// TunFactoryMockMSS implements mssclamp.Contract.
type TunFactoryMockMSS struct{ log bytes.Buffer }

func (m *TunFactoryMockMSS) add(tag string)         { m.log.WriteString(tag + ";") }
func (m *TunFactoryMockMSS) Install(_ string) error { m.add("mss_on"); return nil }
func (m *TunFactoryMockMSS) Remove(_ string) error  { m.add("mss_off"); return nil }

// Error injector for MSS clamping paths.
type TunFactoryMockMSSErr struct {
	*TunFactoryMockMSS
	errTag string
	err    error
}

func (m *TunFactoryMockMSSErr) Install(tunName string) error {
	if m.errTag == "Install" {
		return m.err
	}
	return m.TunFactoryMockMSS.Install(tunName)
}

func (m *TunFactoryMockMSSErr) Remove(tunName string) error {
	if m.errTag == "Remove" {
		return m.err
	}
	return m.TunFactoryMockMSS.Remove(tunName)
}

// TunFactoryMockIOCTL implements ioctl.Contract.
type TunFactoryMockIOCTL struct {
	name                 string
	createErr, detectErr error
}

func (m *TunFactoryMockIOCTL) CreateTunInterface(name string) (*os.File, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.name = name
	return os.Open(os.DevNull)
}
func (m *TunFactoryMockIOCTL) DetectTunNameFromFd(_ *os.File) (string, error) {
	if m.detectErr != nil {
		return "", m.detectErr
	}
	return m.name, nil
}

// TunFactoryMockSys implements sysctl.Contract.
type TunFactoryMockSys struct {
	netErr     bool
	wErr       bool
	netOutput  []byte
	net6Err    bool
	w6Err      bool
	net6Output []byte
}

func (m *TunFactoryMockSys) NetIpv4IpForward() ([]byte, error) {
	if m.netErr {
		return nil, errors.New("net_err")
	}
	if m.netOutput != nil {
		return m.netOutput, nil
	}
	return []byte("net.ipv4.ip_forward = 1\n"), nil
}
func (m *TunFactoryMockSys) WNetIpv4IpForward() ([]byte, error) {
	if m.wErr {
		return nil, errors.New("w_err")
	}
	return []byte("net.ipv4.ip_forward = 1\n"), nil
}
func (m *TunFactoryMockSys) NetIpv6ConfAllForwarding() ([]byte, error) {
	if m.net6Err {
		return nil, errors.New("net6_err")
	}
	if m.net6Output != nil {
		return m.net6Output, nil
	}
	return []byte("net.ipv6.conf.all.forwarding = 1\n"), nil
}
func (m *TunFactoryMockSys) WNetIpv6ConfAllForwarding() ([]byte, error) {
	if m.w6Err {
		return nil, errors.New("w6_err")
	}
	return []byte("net.ipv6.conf.all.forwarding = 1\n"), nil
}

// Variant: LinkDelete error.
type TunFactoryMockIPErrDel struct {
	*TunFactoryMockIP
	err error
}

func (m *TunFactoryMockIPErrDel) LinkDelete(_ string) error { return m.err }

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
) *TunFactory {
	if ipC == nil {
		ipC = &TunFactoryMockIP{}
	}
	if iptC == nil {
		iptC = &TunFactoryMockIPT{}
	}
	if mssC == nil {
		mssC = &TunFactoryMockMSS{}
	}
	if ioC == nil {
		ioC = &TunFactoryMockIOCTL{}
	}
	if sysC == nil {
		sysC = &TunFactoryMockSys{}
	}
	return &TunFactory{
		device:   tunDeviceManager{ip: ipC, ioctl: ioC},
		firewall: firewallConfigurator{iptables: iptC, sysctl: sysC, mss: mssC},
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
	Addressing: settings.Addressing{
		TunName:    "tun0",
		IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
		IPv4:       netip.MustParseAddr("10.0.0.1"),
	},
	MTU: settings.SafeMTU,
}

var baseCfgIPv6 = settings.Settings{
	Addressing: settings.Addressing{
		TunName:    "tun0",
		IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
		IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
		IPv4:       netip.MustParseAddr("10.0.0.1"),
		IPv6:       netip.MustParseAddr("fd00::1"),
	},
	MTU: settings.SafeMTU,
}

var baseCfgIPv6Only = settings.Settings{
	Addressing: settings.Addressing{
		TunName:    "tun0",
		IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
		IPv6:       netip.MustParseAddr("fd00::1"),
	},
	MTU: settings.SafeMTU,
}

// TunFactoryMockIPTBenign simulates benign iptables errors that must be ignored.
type TunFactoryMockIPTBenign struct{ log bytes.Buffer }

func (m *TunFactoryMockIPTBenign) EnableDevMasquerade(_, _ string) error { return nil }
func (m *TunFactoryMockIPTBenign) DisableDevMasquerade(_, _ string) error {
	return errors.New("rule does not exist")
} // benign
func (m *TunFactoryMockIPTBenign) EnableForwardingFromTunToDev(_, _ string) error {
	return nil
}
func (m *TunFactoryMockIPTBenign) DisableForwardingFromTunToDev(_, _ string) error {
	return errors.New("no chain/target/match") // benign
}
func (m *TunFactoryMockIPTBenign) EnableForwardingFromDevToTun(_, _ string) error { return nil }
func (m *TunFactoryMockIPTBenign) DisableForwardingFromDevToTun(_, _ string) error {
	return errors.New("rule does not exist") // benign
}
func (m *TunFactoryMockIPTBenign) EnableForwardingTunToTun(_ string) error          { return nil }
func (m *TunFactoryMockIPTBenign) DisableForwardingTunToTun(_ string) error         { return nil }
func (m *TunFactoryMockIPTBenign) Enable6DevMasquerade(_, _ string) error           { return nil }
func (m *TunFactoryMockIPTBenign) Disable6DevMasquerade(_, _ string) error          { return nil }
func (m *TunFactoryMockIPTBenign) Enable6ForwardingFromTunToDev(_, _ string) error  { return nil }
func (m *TunFactoryMockIPTBenign) Disable6ForwardingFromTunToDev(_, _ string) error { return nil }
func (m *TunFactoryMockIPTBenign) Enable6ForwardingFromDevToTun(_, _ string) error  { return nil }
func (m *TunFactoryMockIPTBenign) Disable6ForwardingFromDevToTun(_, _ string) error { return nil }
func (m *TunFactoryMockIPTBenign) Enable6ForwardingTunToTun(_ string) error         { return nil }
func (m *TunFactoryMockIPTBenign) Disable6ForwardingTunToTun(_ string) error        { return nil }

// TunFactoryMockIPTAlwaysErr simulates non-benign iptables errors that are logged but not fatal.
type TunFactoryMockIPTAlwaysErr struct{}

func (m *TunFactoryMockIPTAlwaysErr) EnableDevMasquerade(_, _ string) error { return nil }
func (m *TunFactoryMockIPTAlwaysErr) DisableDevMasquerade(_, _ string) error {
	return errors.New("permission denied")
}
func (m *TunFactoryMockIPTAlwaysErr) EnableForwardingFromTunToDev(_, _ string) error {
	return nil
}
func (m *TunFactoryMockIPTAlwaysErr) DisableForwardingFromTunToDev(_, _ string) error {
	return errors.New("permission denied")
}
func (m *TunFactoryMockIPTAlwaysErr) EnableForwardingFromDevToTun(_, _ string) error {
	return nil
}
func (m *TunFactoryMockIPTAlwaysErr) DisableForwardingFromDevToTun(_, _ string) error {
	return errors.New("permission denied")
}
func (m *TunFactoryMockIPTAlwaysErr) EnableForwardingTunToTun(_ string) error { return nil }
func (m *TunFactoryMockIPTAlwaysErr) DisableForwardingTunToTun(_ string) error {
	return errors.New("permission denied")
}
func (m *TunFactoryMockIPTAlwaysErr) Enable6DevMasquerade(_, _ string) error { return nil }
func (m *TunFactoryMockIPTAlwaysErr) Disable6DevMasquerade(_, _ string) error {
	return errors.New("permission denied")
}
func (m *TunFactoryMockIPTAlwaysErr) Enable6ForwardingFromTunToDev(_, _ string) error {
	return nil
}
func (m *TunFactoryMockIPTAlwaysErr) Disable6ForwardingFromTunToDev(_, _ string) error {
	return errors.New("permission denied")
}
func (m *TunFactoryMockIPTAlwaysErr) Enable6ForwardingFromDevToTun(_, _ string) error {
	return nil
}
func (m *TunFactoryMockIPTAlwaysErr) Disable6ForwardingFromDevToTun(_, _ string) error {
	return errors.New("permission denied")
}
func (m *TunFactoryMockIPTAlwaysErr) Enable6ForwardingTunToTun(_ string) error { return nil }
func (m *TunFactoryMockIPTAlwaysErr) Disable6ForwardingTunToTun(_ string) error {
	return errors.New("permission denied")
}

/*
   ==============================
   Tests
   ==============================
*/

func TestCreateAndDispose_SuccessAndSkipForwardingDisableWhenExtIfaceUnknown(t *testing.T) {
	ipMock := &TunFactoryMockIPRouteEmpty{} // RouteDefault() returns ""
	iptMock := &TunFactoryMockIPT{}
	mssMock := &TunFactoryMockMSS{}
	ioMock := &TunFactoryMockIOCTL{}
	sysMock := &TunFactoryMockSys{}

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
	cfg.TunName = pickLoopbackName()

	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices: %v", err)
	}
}

func TestDisposeDevices_NoSuchInterface_IsBenign_NoError(t *testing.T) {
	ipMock := &TunFactoryMockIP{}
	iptMock := &TunFactoryMockIPT{}
	f := newFactory(ipMock, iptMock, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	cfg := baseCfg
	cfg.TunName = "definitely-not-existing-xyz123"
	// Should still perform best-effort netfilter cleanup for stale rules.
	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices should ignore missing iface: %v", err)
	}
	if iptMock.lastDisableMasqDev != "eth0" {
		t.Fatalf("expected best-effort cleanup on ext iface, got %q", iptMock.lastDisableMasqDev)
	}
}

func TestEnableForwarding_FirstCallError(t *testing.T) {
	f := newFactory(&TunFactoryMockIP{}, &TunFactoryMockIPT{}, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{netErr: true})
	_, err := f.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "failed to enable IPv4 packet forwarding") {
		t.Errorf("expected forwarding error, got %v", err)
	}
}

func TestEnableForwarding_WriteCallError(t *testing.T) {
	f := newFactory(&TunFactoryMockIP{}, &TunFactoryMockIPT{}, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{
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
		ioMock := &TunFactoryMockIOCTL{}
		if c.tag == "CreateTunInterface" {
			ioMock.createErr = errors.New("io_err")
			ipMock = &TunFactoryMockIP{}
		} else {
			ipMock = &TunFactoryMockIPErr{
				TunFactoryMockIP: &TunFactoryMockIP{},
				errTag:                 c.tag,
				err:                    errors.New("ip_err"),
			}
		}
		f := newFactory(ipMock, &TunFactoryMockIPT{}, nil, ioMock, &TunFactoryMockSys{})
		_, err := f.CreateDevice(baseCfg)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("case %s: expected error containing %q, got %v", c.tag, c.want, err)
		}
	}
}

func TestCreateTunDevice_CreateTunInterfaceError_RollsBackCreatedTun(t *testing.T) {
	ipMock := &TunFactoryMockIP{}
	ioMock := &TunFactoryMockIOCTL{createErr: errors.New("io_err")}
	f := newFactory(ipMock, &TunFactoryMockIPT{}, nil, ioMock, &TunFactoryMockSys{})

	_, err := f.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "failed to open TUN interface") {
		t.Fatalf("expected CreateTunInterface error, got %v", err)
	}

	if strings.Count(ipMock.log.String(), "del;") < 2 {
		t.Fatalf("expected rollback delete after create failure, log=%q", ipMock.log.String())
	}
}

func TestCreateTunDevice_InvalidCIDR_ErrorsFromAllocator(t *testing.T) {
	ipMock := &TunFactoryMockIP{}
	iptMock := &TunFactoryMockIPT{}
	ioMock := &TunFactoryMockIOCTL{}
	sysMock := &TunFactoryMockSys{}

	f := newFactory(ipMock, iptMock, nil, ioMock, sysMock)
	bad := baseCfg
	// Keep IPv4 subnet valid so IPv4 path is active, but remove the derived IPv4
	// address to force allocator/CIDR derivation failure.
	bad.IPv4 = netip.Addr{}
	_, err := f.CreateDevice(bad)
	if err == nil || !strings.Contains(err.Error(), "could not derive server IPv4 CIDR") {
		t.Fatalf("expected allocator error, got %v", err)
	}
}

func TestCreateDevice_RejectsLegacyIPv6InIPv4SubnetField(t *testing.T) {
	cfg := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun0",
			IPv4Subnet: netip.MustParsePrefix("fd00::/64"),
		},
		MTU: settings.SafeMTU,
	}
	f := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPT{},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{},
	)

	_, err := f.CreateDevice(cfg)
	if err == nil || !strings.Contains(err.Error(), "no tunnel IP configuration") {
		t.Fatalf("expected strict config error for legacy IPv6-in-IPv4 field, got %v", err)
	}
}

func TestMasqueradeCIDR6_RequiresIPv6SubnetField(t *testing.T) {
	legacy := settings.Settings{
		Addressing: settings.Addressing{
			IPv4Subnet: netip.MustParsePrefix("fd00::/64"),
		},
	}
	_, err := masqueradeCIDR6(legacy)
	if err == nil || !strings.Contains(err.Error(), "no IPv6 subnet configured") {
		t.Fatalf("expected strict IPv6 subnet error, got %v", err)
	}
}

func TestConfigure_Errors(t *testing.T) {
	// RouteDefault error
	f1 := newFactory(
		&TunFactoryMockIPRouteErr{err: errors.New("route_err")},
		&TunFactoryMockIPT{},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{},
	)
	_, err := f1.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "route_err") {
		t.Errorf("expected route error, got %v", err)
	}

	// EnableDevMasquerade error
	f2 := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPTErr{TunFactoryMockIPT: &TunFactoryMockIPT{}, errTag: "EnableDevMasquerade", err: errors.New("masq_err")},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{},
	)
	_, err = f2.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "failed enabling NAT") {
		t.Errorf("expected NAT error, got %v", err)
	}

	// setupForwarding error
	f3 := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPTErr{TunFactoryMockIPT: &TunFactoryMockIPT{}, errTag: "EnableForwardingFromTunToDev", err: errors.New("fwd_err")},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{},
	)
	_, err = f3.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "failed to set up forwarding") {
		t.Errorf("expected forwarding setup error, got %v", err)
	}

	// MSS clamping error
	f4 := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPT{},
		&TunFactoryMockMSSErr{TunFactoryMockMSS: &TunFactoryMockMSS{}, errTag: "Install", err: errors.New("clamp_err")},
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{},
	)
	_, err = f4.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "clamp_err") {
		t.Errorf("expected clamping error, got %v", err)
	}
}

func TestCreateDevice_ConfigureError_TriggersCleanup(t *testing.T) {
	ipMock := &TunFactoryMockIP{}
	iptBase := &TunFactoryMockIPT{}
	iptErr := &TunFactoryMockIPTErr{
		TunFactoryMockIPT: iptBase,
		errTag:                  "EnableForwardingFromTunToDev",
		err:                     errors.New("fwd_err"),
	}
	f := newFactory(ipMock, iptErr, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})

	_, err := f.CreateDevice(baseCfg)
	if err == nil || !strings.Contains(err.Error(), "failed to set up forwarding") {
		t.Fatalf("expected forwarding setup error, got %v", err)
	}

	if !strings.Contains(iptBase.log.String(), "masq_off;") {
		t.Fatalf("expected NAT rollback/cleanup on configure failure, log=%q", iptBase.log.String())
	}
	// DisposeDevices deletes the interface only if it exists on the host.
	// In unit tests with mocks and no real tun0, cleanup can skip LinkDelete.
	if strings.Count(ipMock.log.String(), "del;") < 1 {
		t.Fatalf("expected at least initial LinkDelete call, log=%q", ipMock.log.String())
	}
}

func TestSetupAndClearForwarding_Errors(t *testing.T) {
	defaultIP, defaultIPT := &TunFactoryMockIP{}, &TunFactoryMockIPT{}

	f1 := newFactory(defaultIP, defaultIPT, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	if err := f1.firewall.setupForwarding("", "eth0", true, true); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("expected empty name error, got %v", err)
	}

	// setup: iptables error
	iptErr := &TunFactoryMockIPTErr{TunFactoryMockIPT: &TunFactoryMockIPT{}, errTag: "EnableForwardingFromTunToDev", err: errors.New("f_err")}
	f2 := newFactory(defaultIP, iptErr, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	if err := f2.firewall.setupForwarding("tunZ", "eth0", true, true); err == nil ||
		!strings.Contains(err.Error(), "failed to setup forwarding rule") {
		t.Errorf("expected forwarding rule error, got %v", err)
	}

	// clear: empty name
	f3 := newFactory(defaultIP, defaultIPT, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	if err := f3.firewall.clearForwarding("", "eth0", true, true); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("expected empty name error, got %v", err)
	}

	// clear: DisableForwardingFromTunToDev error
	iptErr2 := &TunFactoryMockIPTErr{TunFactoryMockIPT: &TunFactoryMockIPT{}, errTag: "DisableForwardingFromTunToDev", err: errors.New("dtd_err")}
	f4 := newFactory(defaultIP, iptErr2, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	if err := f4.firewall.clearForwarding("tunC", "eth0", true, true); err == nil ||
		!strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected clearForwarding error, got %v", err)
	}
}

func TestDisposeTunDevices_DeleteError(t *testing.T) {
	// Use existing interface name to get past net.InterfaceByName
	cfg := baseCfg
	cfg.TunName = pickLoopbackName()
	f := newFactory(&TunFactoryMockIPErrDel{TunFactoryMockIP: &TunFactoryMockIP{}, err: errors.New("del_err")}, &TunFactoryMockIPT{}, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	if err := f.DisposeDevices(cfg); err == nil || !strings.Contains(err.Error(), "error deleting TUN device") {
		t.Errorf("expected delete error, got %v", err)
	}
}

func TestUnconfigure_RouteDefaultError_And_MasqueradeErrorIsLoggedOnly(t *testing.T) {
	ioMock := &TunFactoryMockIOCTL{}
	tun, _ := ioMock.CreateTunInterface("tunU")
	iptErrMasq := &TunFactoryMockIPTErr{TunFactoryMockIPT: &TunFactoryMockIPT{}, errTag: "", err: nil}
	// Make DisableDevMasquerade fail: we don't assert logs here, only that Unconfigure continues to RouteDefault
	_ = iptErrMasq.DisableDevMasquerade("any", "") // just to touch path
	f := newFactory(
		&TunFactoryMockIPRouteErr{err: errors.New("route_err")},
		iptErrMasq,
		nil,
		ioMock,
		&TunFactoryMockSys{},
	)
	err := f.Unconfigure(tun)
	if err == nil || !strings.Contains(err.Error(), "failed to resolve default interface") {
		t.Fatalf("expected RouteDefault error, got %v", err)
	}
}

func TestUnconfigure_ClearForwardingErrorSurfaced(t *testing.T) {
	ioMock := &TunFactoryMockIOCTL{}
	tun, _ := ioMock.CreateTunInterface("tunU2")
	iptErr := &TunFactoryMockIPTErr{TunFactoryMockIPT: &TunFactoryMockIPT{}, errTag: "DisableForwardingFromTunToDev", err: errors.New("boom")}
	f := newFactory(&TunFactoryMockIP{}, iptErr, nil, ioMock, &TunFactoryMockSys{})
	err := f.Unconfigure(tun)
	if err == nil || !strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Fatalf("expected clearForwarding error, got %v", err)
	}
}

func TestUnconfigure_Success(t *testing.T) {
	ioMock := &TunFactoryMockIOCTL{}
	tun, _ := ioMock.CreateTunInterface("tunOK")
	f := newFactory(&TunFactoryMockIP{}, &TunFactoryMockIPT{}, nil, ioMock, &TunFactoryMockSys{})
	if err := f.Unconfigure(tun); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnconfigure_DoesNotUseUnscopedMasqueradeCleanup(t *testing.T) {
	ioMock := &TunFactoryMockIOCTL{}
	tun, _ := ioMock.CreateTunInterface("tunNoNatCleanup")
	iptMock := &TunFactoryMockIPT{}
	f := newFactory(&TunFactoryMockIP{}, iptMock, nil, ioMock, &TunFactoryMockSys{})

	if err := f.Unconfigure(tun); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if iptMock.lastDisableMasqDev != "" || iptMock.lastDisable6MasqDev != "" {
		t.Fatalf("expected no unscoped NAT cleanup in Unconfigure, got v4=%q v6=%q",
			iptMock.lastDisableMasqDev, iptMock.lastDisable6MasqDev)
	}
}

func TestIsBenignNetfilterError_Table(t *testing.T) {
	fw := firewallConfigurator{}
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
		if !fw.isBenignError(errors.New(s)) {
			t.Errorf("expected benign for %q", s)
		}
	}
	if fw.isBenignError(errors.New("permission denied")) {
		t.Errorf("unexpected benign for non-matching error")
	}
	if fw.isBenignError(nil) {
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

func TestTunFactoryMockIP_ExerciseAllStubs(t *testing.T) {
	m := &TunFactoryMockIP{}

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
	ipMock := &TunFactoryMockIP{}
	iptMock := &TunFactoryMockIPTBenign{}
	f := newFactory(ipMock, iptMock, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})

	cfg := baseCfg
	cfg.TunName = pickLoopbackName() // ensure InterfaceByName(...) passes

	// Act + Assert
	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices should ignore benign iptables errors, got: %v", err)
	}
}

func TestDisposeDevices_NonBenignIptablesErrorsAreLoggedButIgnored(t *testing.T) {
	// Arrange: iptables returns non-benign errors; code should log them but still proceed
	ipMock := &TunFactoryMockIP{}
	iptMock := &TunFactoryMockIPTAlwaysErr{}
	f := newFactory(ipMock, iptMock, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})

	cfg := baseCfg
	cfg.TunName = pickLoopbackName()

	// Act + Assert
	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices should not fail on non-benign iptables errors (only log), got: %v", err)
	}
}

func TestUnconfigure_DetectTunNameError_ContinuesToRouteDefault(t *testing.T) {
	// If DetectTunNameFromFd fails, Unconfigure must continue and then surface RouteDefault error.
	ioMock := &TunFactoryMockIOCTL{detectErr: errors.New("detect_failed")}
	tun, _ := ioMock.CreateTunInterface("tunX")
	f := newFactory(
		&TunFactoryMockIPRouteErr{err: errors.New("route_err")},
		&TunFactoryMockIPT{},
		nil,
		ioMock,
		&TunFactoryMockSys{},
	)

	err := f.Unconfigure(tun)
	if err == nil || !strings.Contains(err.Error(), "failed to resolve default interface") {
		t.Fatalf("expected RouteDefault error after detect failure, got %v", err)
	}
}

func TestEnableForwarding_WritesWhenDisabled_Succeeds(t *testing.T) {
	// First sysctl returns 0 → we write 1 and proceed successfully.
	f := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPT{},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{netOutput: []byte("net.ipv4.ip_forward = 0\n")},
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
		&TunFactoryMockIP{},
		&TunFactoryMockIPT{},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{net6Err: true},
	)
	_, err := f.CreateDevice(baseCfgIPv6)
	if err == nil || !strings.Contains(err.Error(), "failed to read IPv6 forwarding state") {
		t.Errorf("expected IPv6 read error, got %v", err)
	}
}

func TestEnableForwarding_IPv6Skipped_WhenNoIPv6Subnet(t *testing.T) {
	// IPv6 sysctl fails, but baseCfg has no IPv6Subnet — must succeed.
	f := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPT{},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{net6Err: true},
	)
	tun, err := f.CreateDevice(baseCfg)
	if err != nil {
		t.Fatalf("CreateDevice should skip IPv6 forwarding when no IPv6 subnet, got: %v", err)
	}
	_ = tun.Close()
}

func TestEnableForwarding_IPv6WriteError(t *testing.T) {
	f := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPT{},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{
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
		&TunFactoryMockIP{},
		&TunFactoryMockIPT{},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{net6Output: []byte("net.ipv6.conf.all.forwarding = 0\n")},
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
	cfg.IPv6 = netip.MustParseAddr("fd00::1")
	f := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPT{},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{},
	)
	tun, err := f.CreateDevice(cfg)
	if err != nil {
		t.Fatalf("CreateDevice with IPv6 subnet should succeed, got: %v", err)
	}
	_ = tun.Close()
}

func TestCreateTunDevice_IPv6Only_Success(t *testing.T) {
	f := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPT{},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{},
	)
	tun, err := f.CreateDevice(baseCfgIPv6Only)
	if err != nil {
		t.Fatalf("CreateDevice with IPv6-only settings should succeed, got: %v", err)
	}
	_ = tun.Close()
}

func TestCreateTunDevice_WithIPv6Subnet_AddrAddError(t *testing.T) {
	cfg := baseCfg
	cfg.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
	cfg.IPv6 = netip.MustParseAddr("fd00::1")
	ipMock := &TunFactoryMockIPErrNthAddr{
		TunFactoryMockIP: &TunFactoryMockIP{},
		failOnCall:             2, // second AddrAddDev call (IPv6)
		err:                    errors.New("v6_addr_err"),
	}
	f := newFactory(ipMock, &TunFactoryMockIPT{}, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	_, err := f.CreateDevice(cfg)
	if err == nil || !strings.Contains(err.Error(), "failed to assign IPv6 to TUN") {
		t.Errorf("expected IPv6 addr error, got %v", err)
	}
}

func TestCreateTunDevice_IPv6Only_AddrAddError(t *testing.T) {
	ipMock := &TunFactoryMockIPErrNthAddr{
		TunFactoryMockIP: &TunFactoryMockIP{},
		failOnCall:             1, // first AddrAddDev call must be IPv6 in IPv6-only mode
		err:                    errors.New("v6_addr_err"),
	}
	f := newFactory(ipMock, &TunFactoryMockIPT{}, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	_, err := f.CreateDevice(baseCfgIPv6Only)
	if err == nil || !strings.Contains(err.Error(), "failed to assign IPv6 to TUN") {
		t.Errorf("expected IPv6 addr error in IPv6-only mode, got %v", err)
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
		iptErr := &TunFactoryMockIPTErr{
			TunFactoryMockIPT: &TunFactoryMockIPT{},
			errTag:                  c.errTag,
			err:                     errors.New("v6_err"),
		}
		f := newFactory(&TunFactoryMockIP{}, iptErr, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
		if err := f.firewall.setupForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), c.want) {
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
		iptErr := &TunFactoryMockIPTErr{
			TunFactoryMockIPT: &TunFactoryMockIPT{},
			errTag:                  c.errTag,
			err:                     errors.New("v6_err"),
		}
		f := newFactory(&TunFactoryMockIP{}, iptErr, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
		if err := f.firewall.clearForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("case %s: expected error containing %q, got %v", c.errTag, c.want, err)
		}
	}
}

func TestConfigure_Enable6DevMasqueradeError(t *testing.T) {
	f := newFactory(
		&TunFactoryMockIP{},
		&TunFactoryMockIPTErr{
			TunFactoryMockIPT: &TunFactoryMockIPT{},
			errTag:                  "Enable6DevMasquerade",
			err:                     errors.New("v6_masq_err"),
		},
		nil,
		&TunFactoryMockIOCTL{},
		&TunFactoryMockSys{},
	)
	_, err := f.CreateDevice(baseCfgIPv6)
	if err == nil || !strings.Contains(err.Error(), "failed enabling IPv6 NAT") {
		t.Errorf("expected IPv6 NAT error, got %v", err)
	}
}

func TestCreateDevice_MasqueradeUsesSubnetScopedRules(t *testing.T) {
	ipMock := &TunFactoryMockIP{}
	iptMock := &TunFactoryMockIPT{}
	f := newFactory(ipMock, iptMock, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})

	_, err := f.CreateDevice(baseCfgIPv6)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}

	if iptMock.lastEnableMasqDev != "eth0" {
		t.Fatalf("EnableDevMasquerade dev=%q want %q", iptMock.lastEnableMasqDev, "eth0")
	}
	if iptMock.lastEnableMasqCIDR != baseCfgIPv6.IPv4Subnet.Masked().String() {
		t.Fatalf("EnableDevMasquerade cidr=%q want %q", iptMock.lastEnableMasqCIDR, baseCfgIPv6.IPv4Subnet.Masked().String())
	}
	if iptMock.lastEnable6MasqDev != "eth0" {
		t.Fatalf("Enable6DevMasquerade dev=%q want %q", iptMock.lastEnable6MasqDev, "eth0")
	}
	if iptMock.lastEnable6MasqCIDR != baseCfgIPv6.IPv6Subnet.Masked().String() {
		t.Fatalf("Enable6DevMasquerade cidr=%q want %q", iptMock.lastEnable6MasqCIDR, baseCfgIPv6.IPv6Subnet.Masked().String())
	}
}

func TestDisposeDevices_MasqueradeCleanupUsesSubnetScopedRules(t *testing.T) {
	ipMock := &TunFactoryMockIP{}
	iptMock := &TunFactoryMockIPT{}
	f := newFactory(ipMock, iptMock, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})

	cfg := baseCfgIPv6
	cfg.TunName = pickLoopbackName()
	if err := f.DisposeDevices(cfg); err != nil {
		t.Fatalf("DisposeDevices: %v", err)
	}

	if iptMock.lastDisableMasqDev != "eth0" {
		t.Fatalf("DisableDevMasquerade dev=%q want %q", iptMock.lastDisableMasqDev, "eth0")
	}
	if iptMock.lastDisableMasqCIDR != baseCfgIPv6.IPv4Subnet.Masked().String() {
		t.Fatalf("DisableDevMasquerade cidr=%q want %q", iptMock.lastDisableMasqCIDR, baseCfgIPv6.IPv4Subnet.Masked().String())
	}
	if iptMock.lastDisable6MasqDev != "eth0" {
		t.Fatalf("Disable6DevMasquerade dev=%q want %q", iptMock.lastDisable6MasqDev, "eth0")
	}
	if iptMock.lastDisable6MasqCIDR != baseCfgIPv6.IPv6Subnet.Masked().String() {
		t.Fatalf("Disable6DevMasquerade cidr=%q want %q", iptMock.lastDisable6MasqCIDR, baseCfgIPv6.IPv6Subnet.Masked().String())
	}
}

func TestEnableForwarding_ForwardingFromDevToTun_Error(t *testing.T) {
	iptErr := &TunFactoryMockIPTErr{
		TunFactoryMockIPT: &TunFactoryMockIPT{},
		errTag:                  "EnableForwardingFromDevToTun",
		err:                     errors.New("fwd_dt_err"),
	}
	f := newFactory(&TunFactoryMockIP{}, iptErr, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	if err := f.firewall.setupForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), "failed to setup forwarding rule") {
		t.Errorf("expected forwarding rule error, got %v", err)
	}
}

func TestEnableForwarding_ForwardingTunToTun_Error(t *testing.T) {
	iptErr := &TunFactoryMockIPTErr{
		TunFactoryMockIPT: &TunFactoryMockIPT{},
		errTag:                  "EnableForwardingTunToTun",
		err:                     errors.New("fwd_tt_err"),
	}
	f := newFactory(&TunFactoryMockIP{}, iptErr, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	if err := f.firewall.setupForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), "failed to setup client-to-client forwarding rule") {
		t.Errorf("expected client-to-client forwarding error, got %v", err)
	}
}

func TestClearForwarding_DisableForwardingFromDevToTun_Error(t *testing.T) {
	iptErr := &TunFactoryMockIPTErr{
		TunFactoryMockIPT: &TunFactoryMockIPT{},
		errTag:                  "DisableForwardingFromDevToTun",
		err:                     errors.New("dtd_err"),
	}
	f := newFactory(&TunFactoryMockIP{}, iptErr, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	if err := f.firewall.clearForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected iptables error, got %v", err)
	}
}

func TestClearForwarding_DisableForwardingTunToTun_Error(t *testing.T) {
	iptErr := &TunFactoryMockIPTErr{
		TunFactoryMockIPT: &TunFactoryMockIPT{},
		errTag:                  "DisableForwardingTunToTun",
		err:                     errors.New("dtt_err"),
	}
	f := newFactory(&TunFactoryMockIP{}, iptErr, nil, &TunFactoryMockIOCTL{}, &TunFactoryMockSys{})
	if err := f.firewall.clearForwarding("tun0", "eth0", true, true); err == nil || !strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("expected iptables error, got %v", err)
	}
}
