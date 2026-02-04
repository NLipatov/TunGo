package handlers

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

// --------- fakes & stubs (prefixed with ConfgenHandler...) ---------

// ConfgenHandlerMockMgr implements ServerConfigurationManager and lets us
// script configuration reads and increment errors.
type ConfgenHandlerMockMgr struct {
	cfg       *serverConfiguration.Configuration
	cfgErr    error
	incErr    error
	injectErr error
	incCalls  int
}

func (m *ConfgenHandlerMockMgr) Configuration() (*serverConfiguration.Configuration, error) {
	return m.cfg, m.cfgErr
}
func (m *ConfgenHandlerMockMgr) IncrementClientCounter() error {
	m.incCalls++
	if m.incErr != nil {
		return m.incErr
	}
	m.cfg.ClientCounter++
	return nil
}
func (m *ConfgenHandlerMockMgr) InjectX25519Keys(_, _ []byte) error {
	return m.injectErr
}

// ConfgenHandlerMockIP implements ip.Contract. We override only the two methods
// used by the handler, the rest are harmless no-ops.
type ConfgenHandlerMockIP struct {
	RouteDefaultFunc func() (string, error)
	AddrShowDevFunc  func(int, string) (string, error)
}

func (m ConfgenHandlerMockIP) TunTapAddDevTun(string) error    { return nil }
func (m ConfgenHandlerMockIP) LinkDelete(string) error         { return nil }
func (m ConfgenHandlerMockIP) LinkSetDevUp(string) error       { return nil }
func (m ConfgenHandlerMockIP) LinkSetDevMTU(string, int) error { return nil }
func (m ConfgenHandlerMockIP) AddrAddDev(string, string) error { return nil }
func (m ConfgenHandlerMockIP) AddrShowDev(ipV int, dev string) (string, error) {
	if m.AddrShowDevFunc != nil {
		return m.AddrShowDevFunc(ipV, dev)
	}
	return "", nil
}
func (m ConfgenHandlerMockIP) RouteDefault() (string, error) {
	if m.RouteDefaultFunc != nil {
		return m.RouteDefaultFunc()
	}
	return "", nil
}
func (m ConfgenHandlerMockIP) RouteAddDefaultDev(string) error             { return nil }
func (m ConfgenHandlerMockIP) RouteGet(string) (string, error)             { return "", nil }
func (m ConfgenHandlerMockIP) RouteAddDev(string, string) error            { return nil }
func (m ConfgenHandlerMockIP) RouteAddViaDev(string, string, string) error { return nil }
func (m ConfgenHandlerMockIP) RouteDel(string) error                       { return nil }

// ConfgenHandlerMockMarshaller lets us control MarshalIndent behavior.
type ConfgenHandlerMockMarshaller struct {
	data []byte
	err  error
}

func (m ConfgenHandlerMockMarshaller) MarshalIndent(_ any, _, _ string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.data != nil {
		return m.data, nil
	}
	return []byte(`{"ok":true}`), nil
}

// --------- helpers ---------

func validCfg() *serverConfiguration.Configuration {
	return &serverConfiguration.Configuration{
		FallbackServerAddress: "198.51.100.10",
		ClientCounter:         7,
		EnableUDP:             false,
		EnableTCP:             false,
		EnableWS:              true,
		X25519PublicKey:       []byte("PUB"),
		X25519PrivateKey:      []byte("PRIV"),
		TCPSettings: settings.Settings{
			InterfaceName:   "tun-tcp0",
			InterfaceIPCIDR: "10.0.0.1/24",
			Port:            "443",
			MTU:             1400,
			DialTimeoutMs:   1000,
			Protocol:        settings.TCP,
		},
		UDPSettings: settings.Settings{
			InterfaceName:   "tun-udp0",
			InterfaceIPCIDR: "10.1.0.1/24",
			Port:            "53",
			MTU:             1400,
			DialTimeoutMs:   1000,
			Protocol:        settings.UDP,
		},
		WSSettings: settings.Settings{
			InterfaceName:   "tun-ws0",
			InterfaceIPCIDR: "10.2.0.1/24",
			Port:            "8080",
			MTU:             1400,
			DialTimeoutMs:   1000,
			Protocol:        settings.WS,
		},
	}
}

// captureStdout redirects os.Stdout during fn and returns captured output.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	fn()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// --------- tests: GenerateNewClientConf ---------

func Test_GenerateNewClientConf_success_printsJSON(t *testing.T) {
	mgr := &ConfgenHandlerMockMgr{cfg: validCfg()}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())

	// Inject IP behavior: route ok, direct IP resolution ok.
	h.ip = ConfgenHandlerMockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "192.0.2.10", nil },
	}

	out := captureStdout(t, func() {
		if err := h.GenerateNewClientConf(); err != nil {
			t.Fatalf("unexpected: %v", err)
		}
	})
	if !strings.Contains(out, `"Protocol"`) {
		t.Fatalf("stdout must contain JSON, got: %s", out)
	}
}

func Test_GenerateNewClientConf_generate_error_wrapped(t *testing.T) {
	cfg := validCfg()
	cfg.FallbackServerAddress = "" // force failure when AddrShowDev errors
	mgr := &ConfgenHandlerMockMgr{cfg: cfg}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())

	h.ip = ConfgenHandlerMockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "", errors.New("no-ip") },
	}

	err := h.GenerateNewClientConf()
	if err == nil || !strings.Contains(err.Error(), "failed to generate client conf") {
		t.Fatalf("want wrapped generate error, got %v", err)
	}
}

func Test_GenerateNewClientConf_marshal_error_wrapped(t *testing.T) {
	mgr := &ConfgenHandlerMockMgr{cfg: validCfg()}
	h := NewConfgenHandler(mgr, ConfgenHandlerMockMarshaller{err: errors.New("marshal-fail")})

	h.ip = ConfgenHandlerMockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "192.0.2.10", nil },
	}
	err := h.GenerateNewClientConf()
	if err == nil || !strings.Contains(err.Error(), "failed to marshalize client conf") {
		t.Fatalf("want marshal wrapped error, got %v", err)
	}
}

// --------- tests: generate ---------

func Test_generate_config_error(t *testing.T) {
	mgr := &ConfgenHandlerMockMgr{cfgErr: errors.New("cfg-fail")}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())
	h.ip = ConfgenHandlerMockIP{}
	_, err := h.generate()
	if err == nil || !strings.Contains(err.Error(), "failed to read server configuration") {
		t.Fatalf("want config read error, got %v", err)
	}
}

func Test_generate_route_default_error(t *testing.T) {
	mgr := &ConfgenHandlerMockMgr{cfg: validCfg()}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())
	h.ip = ConfgenHandlerMockIP{
		RouteDefaultFunc: func() (string, error) { return "", errors.New("route-fail") },
	}
	_, err := h.generate()
	if err == nil || !strings.Contains(err.Error(), "route-fail") {
		t.Fatalf("want route-fail, got %v", err)
	}
}

func Test_generate_addr_error_no_fallback(t *testing.T) {
	cfg := validCfg()
	cfg.FallbackServerAddress = "" // no fallback → must error
	mgr := &ConfgenHandlerMockMgr{cfg: cfg}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())
	h.ip = ConfgenHandlerMockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "", errors.New("addr-fail") },
	}
	_, err := h.generate()
	if err == nil || !strings.Contains(err.Error(), "no fallback address") {
		t.Fatalf("want fallback error, got %v", err)
	}
}

func Test_generate_addr_error_with_fallback_and_success(t *testing.T) {
	mgr := &ConfgenHandlerMockMgr{cfg: validCfg()}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())
	// Force AddrShowDev error, expect fallback to be used.
	h.ip = ConfgenHandlerMockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "", errors.New("no-ip") },
	}
	conf, err := h.generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Fallback must be used as ConnectionIP for WS.
	if conf.WSSettings.ConnectionIP != mgr.cfg.FallbackServerAddress {
		t.Fatalf("expected fallback ConnectionIP, got %q", conf.WSSettings.ConnectionIP)
	}
	if mgr.incCalls != 1 {
		t.Fatalf("IncrementClientCounter not called")
	}
}

func Test_generate_allocate_error_propagates(t *testing.T) {
	// Break TCP allocation by providing invalid CIDR.
	cfg := validCfg()
	cfg.TCPSettings.InterfaceIPCIDR = "bad"
	mgr := &ConfgenHandlerMockMgr{cfg: cfg}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())
	h.ip = ConfgenHandlerMockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "192.0.2.10", nil },
	}
	_, err := h.generate()
	if err == nil || !strings.Contains(err.Error(), "TCP interface address allocation fail") {
		t.Fatalf("want TCP allocation error, got %v", err)
	}
}

// --------- tests: allocateNewClientIP ---------

func Test_allocateNewClientIP_success(t *testing.T) {
	mgr := &ConfgenHandlerMockMgr{cfg: validCfg()}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())

	tcp, udp, ws, err := h.allocateNewClientIP(mgr.cfg)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if tcp == "" || udp == "" || ws == "" {
		t.Fatalf("empty addresses returned")
	}
	if mgr.incCalls != 1 {
		t.Fatalf("IncrementClientCounter must be called exactly once")
	}
}

func Test_allocateNewClientIP_tcp_error(t *testing.T) {
	cfg := validCfg()
	cfg.TCPSettings.InterfaceIPCIDR = "bad"
	mgr := &ConfgenHandlerMockMgr{cfg: cfg}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())

	_, _, _, err := h.allocateNewClientIP(cfg)
	if err == nil || !strings.Contains(err.Error(), "TCP interface address allocation fail") {
		t.Fatalf("want TCP alloc error, got %v", err)
	}
}

func Test_allocateNewClientIP_udp_error(t *testing.T) {
	cfg := validCfg()
	cfg.UDPSettings.InterfaceIPCIDR = "bad"
	mgr := &ConfgenHandlerMockMgr{cfg: cfg}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())

	_, _, _, err := h.allocateNewClientIP(cfg)
	if err == nil || !strings.Contains(err.Error(), "UDP interface address allocation fail") {
		t.Fatalf("want UDP alloc error, got %v", err)
	}
}

func Test_allocateNewClientIP_ws_error(t *testing.T) {
	cfg := validCfg()
	cfg.WSSettings.InterfaceIPCIDR = "bad"
	mgr := &ConfgenHandlerMockMgr{cfg: cfg}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())

	_, _, _, err := h.allocateNewClientIP(cfg)
	if err == nil || !strings.Contains(err.Error(), "WS interface address allocation fail") {
		t.Fatalf("want WS alloc error, got %v", err)
	}
}

func Test_allocateNewClientIP_increment_error(t *testing.T) {
	cfg := validCfg()
	mgr := &ConfgenHandlerMockMgr{cfg: cfg, incErr: errors.New("inc-fail")}
	h := NewConfgenHandler(mgr, NewJsonMarshaller())

	_, _, _, err := h.allocateNewClientIP(cfg)
	if err == nil || !strings.Contains(err.Error(), "inc-fail") {
		t.Fatalf("want increment error, got %v", err)
	}
}

// --------- tests: getDefaultProtocol ---------

func Test_getDefaultProtocol_priority(t *testing.T) {
	h := NewConfgenHandler(&ConfgenHandlerMockMgr{cfg: validCfg()}, NewJsonMarshaller())
	cfg := validCfg()

	// 1) UDP has priority
	cfg.EnableUDP, cfg.EnableTCP, cfg.EnableWS = true, true, true
	if got := h.getDefaultProtocol(cfg); got != settings.UDP {
		t.Fatalf("want UDP, got %v", got)
	}

	// 2) then TCP
	cfg.EnableUDP, cfg.EnableTCP, cfg.EnableWS = false, true, true
	if got := h.getDefaultProtocol(cfg); got != settings.TCP {
		t.Fatalf("want TCP, got %v", got)
	}

	// 3) else WS
	cfg.EnableUDP, cfg.EnableTCP, cfg.EnableWS = false, false, true
	if got := h.getDefaultProtocol(cfg); got != settings.WS {
		t.Fatalf("want WS, got %v", got)
	}
}
