package tun_server

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
	"tungo/application"
	"tungo/infrastructure/PAL/linux/network_tools/ioctl"
	"tungo/infrastructure/PAL/linux/network_tools/ip"
	"tungo/infrastructure/PAL/linux/network_tools/sysctl"
	"tungo/infrastructure/settings"
)

/* ============================ Mocks ============================ */

// mockIP implements ip.Contract (base “success” version).
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
func (m *mockIP) LinkExists(_ string) (bool, error)           { m.add("exists"); return true, nil }

// variants for branches
type mockIPExistsFalse struct{ *mockIP }

func (m *mockIPExistsFalse) LinkExists(string) (bool, error) { m.add("exists"); return false, nil }

type mockIPExistsErr struct{ *mockIP }

func (m *mockIPExistsErr) LinkExists(string) (bool, error) {
	m.add("exists")
	return false, errors.New("probe_err")
}

type mockIPDelNotFound struct{ *mockIP }

func (m *mockIPDelNotFound) LinkDelete(string) error {
	m.add("del")
	return errors.New("device does not exist")
}

type mockIPDelOtherErr struct{ *mockIP }

func (m *mockIPDelOtherErr) LinkDelete(string) error { m.add("del"); return errors.New("boom") }

// targeted errors inside createTun steps
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

// RouteDefault error (configure path)
type mockIPRouteErr struct {
	*mockIP
	err error
}

func (m *mockIPRouteErr) RouteDefault() (string, error) {
	m.add("route")
	return "", m.err
}

// mockIPT implements application.Netfilter (success version).
type mockIPT struct{ log bytes.Buffer }

func (m *mockIPT) add(tag string)                                  { m.log.WriteString(tag + ";") }
func (m *mockIPT) EnableDevMasquerade(_ string) error              { m.add("masq_on"); return nil }
func (m *mockIPT) DisableDevMasquerade(_ string) error             { m.add("masq_off"); return nil }
func (m *mockIPT) EnableForwardingFromTunToDev(_, _ string) error  { m.add("fwd_td"); return nil }
func (m *mockIPT) DisableForwardingFromTunToDev(_, _ string) error { m.add("fwd_td_off"); return nil }
func (m *mockIPT) EnableForwardingFromDevToTun(_, _ string) error  { m.add("fwd_dt"); return nil }
func (m *mockIPT) DisableForwardingFromDevToTun(_, _ string) error { m.add("fwd_dt_off"); return nil }
func (m *mockIPT) ConfigureMssClamping(_ string) error             { m.add("clamp"); return nil }

// errors in Netfilter
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
func (m *mockIPTErr) EnableForwardingFromTunToDev(tun, dev string) error {
	if m.errTag == "EnableForwardingFromTunToDev" {
		return m.err
	}
	return m.mockIPT.EnableForwardingFromTunToDev(tun, dev)
}
func (m *mockIPTErr) DisableForwardingFromTunToDev(tun, dev string) error {
	if m.errTag == "DisableForwardingFromTunToDev" {
		return m.err
	}
	return m.mockIPT.DisableForwardingFromTunToDev(tun, dev)
}
func (m *mockIPTErr) DisableForwardingFromDevToTun(tun, dev string) error {
	if m.errTag == "DisableForwardingFromDevToTun" {
		return m.err
	}
	return m.mockIPT.DisableForwardingFromDevToTun(tun, dev)
}
func (m *mockIPTErr) ConfigureMssClamping(dev string) error {
	if m.errTag == "ConfigureMssClamping" {
		return m.err
	}
	return m.mockIPT.ConfigureMssClamping(dev)
}

// mockIOCTL implements ioctl.Contract.
type mockIOCTL struct {
	name                 string
	createErr, detectErr error
}

func (m *mockIOCTL) CreateTunInterface(name string) (*os.File, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.name = name
	return os.Open(os.DevNull)
}
func (m *mockIOCTL) DetectTunNameFromFd(_ *os.File) (string, error) {
	if m.detectErr != nil {
		return "", m.detectErr
	}
	return m.name, nil
}

// mockSys implements sysctl.Contract.
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

// helper factory
func newFactory(ipC ip.Contract, iptC application.Netfilter, ioC ioctl.Contract, sysC sysctl.Contract) *ServerTunFactory {
	return &ServerTunFactory{ip: ipC, netfilter: iptC, ioctl: ioC, sysctl: sysC}
}

var cfg = settings.Settings{
	InterfaceName:   "tun0",
	InterfaceIPCIDR: "10.0.0.0/30",
	MTU:             settings.MTU,
}

/* ============================ Tests ============================ */

func TestCreateAndDispose_Success(t *testing.T) {
	f := newFactory(&mockIP{}, &mockIPT{}, &mockIOCTL{}, &mockSys{})
	tun, err := f.CreateTunDevice(cfg)
	if err != nil {
		t.Fatalf("CreateTunDevice error: %v", err)
	}
	if tun == nil {
		t.Fatal("expected non-nil tun file")
	}
	if err := f.DisposeTunDevices(cfg); err != nil {
		t.Fatalf("DisposeTunDevices error: %v", err)
	}
}

func TestEnableForwarding_FirstCallError(t *testing.T) {
	f := newFactory(&mockIP{}, &mockIPT{}, &mockIOCTL{}, &mockSys{netErr: true})
	_, err := f.CreateTunDevice(cfg)
	if err == nil || !strings.Contains(err.Error(), "failed to enable IPv4 packet forwarding") {
		t.Errorf("want forwarding error, got %v", err)
	}
}

func TestEnableForwarding_SecondCallError(t *testing.T) {
	f := newFactory(&mockIP{}, &mockIPT{}, &mockIOCTL{}, &mockSys{
		netOutput: []byte("net.ipv4.ip_forward = 0\n"), wErr: true,
	})
	_, err := f.CreateTunDevice(cfg)
	if err == nil || !strings.Contains(err.Error(), "failed to enable IPv4 packet forwarding") {
		t.Errorf("want second-call forwarding error, got %v", err)
	}
}

func TestCreateTunDevice_CreateTunStepErrors(t *testing.T) {
	cases := []struct {
		tag  string
		want string
	}{
		{"TunTapAddDevTun", "could not create tuntap dev"},
		{"LinkSetDevUp", "could not set tuntap dev up"},
		{"LinkSetDevMTU", "could not set mtu on tuntap dev"},
		{"AddrAddDev", "failed to convert server ip to CIDR format"},
		{"CreateTunInterface", "failed to open TUN interface"},
	}
	for _, c := range cases {
		var ipMock ip.Contract = &mockIP{}
		ioMock := &mockIOCTL{}
		if c.tag == "CreateTunInterface" {
			ioMock.createErr = errors.New("io_err")
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
			t.Errorf("case %s: want error containing %q, got %v", c.tag, c.want, err)
		}
	}
}

func TestCreateTunDevice_ConfigureStepErrors(t *testing.T) {
	cases := []struct {
		setup func() (*ServerTunFactory, *mockIP)
		want  string
	}{
		{
			setup: func() (*ServerTunFactory, *mockIP) {
				ipb := &mockIP{}
				return newFactory(
					&mockIPRouteErr{ipb, errors.New("route_err")},
					&mockIPT{}, &mockIOCTL{}, &mockSys{},
				), ipb
			},
			want: "route_err",
		},
		{
			setup: func() (*ServerTunFactory, *mockIP) {
				ipb := &mockIP{}
				return newFactory(
					ipb,
					&mockIPTErr{&mockIPT{}, "EnableDevMasquerade", errors.New("masq_err")},
					&mockIOCTL{}, &mockSys{},
				), ipb
			},
			want: "failed enabling NAT",
		},
		{
			setup: func() (*ServerTunFactory, *mockIP) {
				ipb := &mockIP{}
				return newFactory(
					ipb,
					&mockIPTErr{&mockIPT{}, "EnableForwardingFromTunToDev", errors.New("fwd_err")},
					&mockIOCTL{}, &mockSys{},
				), ipb
			},
			want: "failed to set up forwarding",
		},
	}
	for _, c := range cases {
		f, ipb := c.setup()
		_, err := f.CreateTunDevice(cfg)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Fatalf("want configure error %q, got %v", c.want, err)
		}
		// verify cleanup executed: RouteDefault in unconfigure + attemptToRemove (exists+del)
		logs := ipb.log.String()
		if !strings.Contains(logs, "route;") || !strings.Contains(logs, "exists;") || !strings.Contains(logs, "del;") {
			t.Fatalf("expected cleanup route+exists+del, got logs: %s", logs)
		}
	}
}

func TestSetupForwarding_ErrorPaths(t *testing.T) {
	ipOK, iptOK := &mockIP{}, &mockIPT{}

	// detect name error
	ioDetErr := &mockIOCTL{detectErr: errors.New("det_err")}
	tun1, _ := ioDetErr.CreateTunInterface("tunX")
	f1 := newFactory(ipOK, iptOK, ioDetErr, &mockSys{})
	if err := f1.setupForwarding(tun1, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to determing tunnel ifName") {
		t.Errorf("want detect error, got %v", err)
	}

	// empty name
	ioEmpty := &mockIOCTL{}
	tun2, _ := ioEmpty.CreateTunInterface("tunY")
	ioEmpty.name = ""
	f2 := newFactory(ipOK, iptOK, ioEmpty, &mockSys{})
	if err := f2.setupForwarding(tun2, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("want empty name error, got %v", err)
	}

	// iptables error on first rule
	iptErr := &mockIPTErr{&mockIPT{}, "EnableForwardingFromTunToDev", errors.New("f_err")}
	ioOK := &mockIOCTL{}
	tun3, _ := ioOK.CreateTunInterface("tunZ")
	f3 := newFactory(ipOK, iptErr, ioOK, &mockSys{})
	if err := f3.setupForwarding(tun3, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to setup forwarding rule") {
		t.Errorf("want forwarding rule error, got %v", err)
	}
}

func TestClearForwarding_ErrorPaths(t *testing.T) {
	ipOK, iptOK := &mockIP{}, &mockIPT{}

	// detect name error
	ioDetErr := &mockIOCTL{detectErr: errors.New("det_err")}
	tun1, _ := ioDetErr.CreateTunInterface("tunA")
	f1 := newFactory(ipOK, iptOK, ioDetErr, &mockSys{})
	if err := f1.clearForwarding(tun1, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to determing tunnel ifName") {
		t.Errorf("want detect error, got %v", err)
	}

	// empty name
	ioEmpty := &mockIOCTL{}
	tun2, _ := ioEmpty.CreateTunInterface("tunB")
	ioEmpty.name = ""
	f2 := newFactory(ipOK, iptOK, ioEmpty, &mockSys{})
	if err := f2.clearForwarding(tun2, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to get TUN interface name") {
		t.Errorf("want empty name error, got %v", err)
	}

	// disable tun->dev error
	iptErr := &mockIPTErr{&mockIPT{}, "DisableForwardingFromTunToDev", errors.New("dtd_err")}
	ioOK := &mockIOCTL{}
	tun3, _ := ioOK.CreateTunInterface("tunC")
	f3 := newFactory(ipOK, iptErr, ioOK, &mockSys{})
	if err := f3.clearForwarding(tun3, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("want tun->dev error, got %v", err)
	}

	// disable dev->tun error (first disable passes, second fails)
	iptErr2 := &mockIPTErr{&mockIPT{}, "DisableForwardingFromDevToTun", errors.New("ddt_err")}
	tun4, _ := ioOK.CreateTunInterface("tunD")
	f4 := newFactory(ipOK, iptErr2, ioOK, &mockSys{})
	if err := f4.clearForwarding(tun4, "eth0"); err == nil ||
		!strings.Contains(err.Error(), "failed to execute iptables command") {
		t.Errorf("want dev->tun error, got %v", err)
	}
}

func TestDisposeTunDevices_Branches(t *testing.T) {
	// 1) interface not exists -> early return
	f1 := newFactory(&mockIPExistsFalse{&mockIP{}}, &mockIPT{}, &mockIOCTL{}, &mockSys{})
	if err := f1.DisposeTunDevices(cfg); err != nil {
		t.Fatalf("unexpected error for no-interface: %v", err)
	}

	// 2) LinkExists error -> best-effort (logged, but no error)
	buf := &bytes.Buffer{}
	old := log.Writer()
	log.SetOutput(buf)
	defer log.SetOutput(old)

	f2 := newFactory(&mockIPExistsErr{&mockIP{}}, &mockIPT{}, &mockIOCTL{}, &mockSys{})
	if err := f2.DisposeTunDevices(cfg); err != nil {
		t.Fatalf("unexpected error with LinkExists error path: %v", err)
	}
	if !strings.Contains(strings.ToLower(buf.String()), "link-exists check failed") {
		t.Errorf("expected log about link-exists failure, got: %s", buf.String())
	}

	// 3) delete “not found” -> ok
	f3 := newFactory(&mockIPDelNotFound{&mockIP{}}, &mockIPT{}, &mockIOCTL{}, &mockSys{})
	if err := f3.DisposeTunDevices(cfg); err != nil {
		t.Fatalf("unexpected error on not-found delete: %v", err)
	}

	// 4) delete “other error” -> attemptToRemoveTunDevByName returns error, Dispose logs it
	buf.Reset()
	f4 := newFactory(&mockIPDelOtherErr{&mockIP{}}, &mockIPT{}, &mockIOCTL{}, &mockSys{})
	if err := f4.DisposeTunDevices(cfg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "attemptToRemoveTunDevByName failed") {
		t.Errorf("expected log from DisposeTunDevices on delete error, got: %s", buf.String())
	}
}

func Test_attemptToRemoveTunDevByName_StandaloneErrors(t *testing.T) {
	// ensure function itself returns error on “other” delete error
	f := newFactory(&mockIPDelOtherErr{&mockIP{}}, &mockIPT{}, &mockIOCTL{}, &mockSys{})
	err := f.attemptToRemoveTunDevByName("tunX")
	if err == nil || !strings.Contains(err.Error(), `delete "tunX":`) {
		t.Fatalf("want delete error, got %v", err)
	}
}

func Test_unconfigureByTunDevName_LogsBranches(t *testing.T) {
	// 1) RouteDefault error -> early return
	buf := &bytes.Buffer{}
	old := log.Writer()
	log.SetOutput(buf)
	defer log.SetOutput(old)

	f1 := newFactory(&mockIPRouteErr{&mockIP{}, errors.New("rd_err")}, &mockIPT{}, &mockIOCTL{}, &mockSys{})
	f1.unconfigureByTunDevName("tun0")
	if !strings.Contains(buf.String(), "failed to detect default route iface") {
		t.Errorf("expected route error log, got: %s", buf.String())
	}

	// 2) other logged branches
	buf.Reset()
	f2 := newFactory(&mockIP{}, &mockIPTErr{&mockIPT{}, "DisableForwardingFromTunToDev", errors.New("x")}, &mockIOCTL{}, &mockSys{})
	f2.unconfigureByTunDevName("tun0")
	out := strings.ToLower(buf.String())
	if !strings.Contains(out, "failed to disable nat") && !strings.Contains(out, "failed to disable fwd tun->dev") {
		t.Errorf("expected logs from unconfigure, got: %s", out)
	}
}
