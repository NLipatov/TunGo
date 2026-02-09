package confgen

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"
	"tungo/infrastructure/settings"
)

// --------- fakes & stubs ---------

type mockMgr struct {
	cfg        *serverConfiguration.Configuration
	cfgErr     error
	incErr     error
	addPeerErr error
	incCalls   int
	addedPeers []serverConfiguration.AllowedPeer
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

func (m *mockMgr) Configuration() (*serverConfiguration.Configuration, error) {
	return m.cfg, m.cfgErr
}
func (m *mockMgr) IncrementClientCounter() error {
	m.incCalls++
	if m.incErr != nil {
		return m.incErr
	}
	m.cfg.ClientCounter++
	return nil
}
func (m *mockMgr) InjectX25519Keys(_, _ []byte) error { return nil }
func (m *mockMgr) AddAllowedPeer(peer serverConfiguration.AllowedPeer) error {
	if m.addPeerErr != nil {
		return m.addPeerErr
	}
	m.addedPeers = append(m.addedPeers, peer)
	return nil
}
func (m *mockMgr) InvalidateCache() {}

// mockIP implements ip.Contract.
type mockIP struct {
	RouteDefaultFunc func() (string, error)
	AddrShowDevFunc  func(int, string) (string, error)
}

func (m mockIP) TunTapAddDevTun(string) error                        { return nil }
func (m mockIP) LinkDelete(string) error                             { return nil }
func (m mockIP) LinkSetDevUp(string) error                           { return nil }
func (m mockIP) LinkSetDevMTU(string, int) error                     { return nil }
func (m mockIP) AddrAddDev(string, string) error                     { return nil }
func (m mockIP) RouteAddDefaultDev(string) error                     { return nil }
func (m mockIP) RouteGet(string) (string, error)                     { return "", nil }
func (m mockIP) RouteAddDev(string, string) error                    { return nil }
func (m mockIP) RouteAddViaDev(string, string, string) error         { return nil }
func (m mockIP) RouteDel(string) error                               { return nil }
func (m mockIP) AddrShowDev(ipV int, dev string) (string, error) {
	if m.AddrShowDevFunc != nil {
		return m.AddrShowDevFunc(ipV, dev)
	}
	return "", nil
}
func (m mockIP) RouteDefault() (string, error) {
	if m.RouteDefaultFunc != nil {
		return m.RouteDefaultFunc()
	}
	return "", nil
}

// --------- helpers ---------

func validCfg() *serverConfiguration.Configuration {
	return &serverConfiguration.Configuration{
		FallbackServerAddress: "198.51.100.10",
		ClientCounter:         7,
		EnableUDP:             false,
		EnableTCP:             false,
		EnableWS:              true,
		X25519PublicKey:        []byte("PUB"),
		X25519PrivateKey:      []byte("PRIV"),
		TCPSettings: settings.Settings{
			InterfaceName:   "tun-tcp0",
			InterfaceSubnet: mustPrefix("10.0.0.0/24"),
			Port:            443,
			MTU:             1400,
			DialTimeoutMs:   1000,
			Protocol:        settings.TCP,
		},
		UDPSettings: settings.Settings{
			InterfaceName:   "tun-udp0",
			InterfaceSubnet: mustPrefix("10.1.0.0/24"),
			Port:            53,
			MTU:             1400,
			DialTimeoutMs:   1000,
			Protocol:        settings.UDP,
		},
		WSSettings: settings.Settings{
			InterfaceName:   "tun-ws0",
			InterfaceSubnet: mustPrefix("10.2.0.0/24"),
			Port:            8080,
			MTU:             1400,
			DialTimeoutMs:   1000,
			Protocol:        settings.WS,
		},
	}
}

func generatorWithMocks(mgr *mockMgr, ip mockIP) *Generator {
	g := NewGenerator(mgr, &primitives.DefaultKeyDeriver{})
	g.ip = ip
	return g
}

// --------- tests: Generate ---------

func TestGenerate_success(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "192.0.2.10", nil },
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !conf.TCPSettings.InterfaceIP.IsValid() {
		t.Fatal("TCP InterfaceIP must be valid")
	}
}

func TestGenerate_config_error(t *testing.T) {
	mgr := &mockMgr{cfgErr: errors.New("cfg-fail")}
	g := generatorWithMocks(mgr, mockIP{})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "failed to read server configuration") {
		t.Fatalf("want config read error, got %v", err)
	}
}

func TestGenerate_route_default_error(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "", errors.New("route-fail") },
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "route-fail") {
		t.Fatalf("want route-fail, got %v", err)
	}
}

func TestGenerate_addr_error_no_fallback(t *testing.T) {
	cfg := validCfg()
	cfg.FallbackServerAddress = ""
	mgr := &mockMgr{cfg: cfg}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "", errors.New("addr-fail") },
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "no fallback address") {
		t.Fatalf("want fallback error, got %v", err)
	}
}

func TestGenerate_addr_error_with_fallback_success(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "", errors.New("no-ip") },
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if conf.WSSettings.Host != mustHost(mgr.cfg.FallbackServerAddress) {
		t.Fatalf("expected fallback Host, got %q", conf.WSSettings.Host)
	}
	if mgr.incCalls != 1 {
		t.Fatalf("IncrementClientCounter not called")
	}
}

func TestGenerate_adds_peer_with_clientIndex_and_name(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "192.0.2.10", nil },
	})

	_, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(mgr.addedPeers) != 1 {
		t.Fatalf("expected 1 added peer, got %d", len(mgr.addedPeers))
	}
	peer := mgr.addedPeers[0]
	if peer.ClientIndex != 8 {
		t.Fatalf("expected ClientIndex 8, got %d", peer.ClientIndex)
	}
	if peer.Name != "client-8" {
		t.Fatalf("expected peer name client-8, got %q", peer.Name)
	}
}

func TestGenerate_allocate_error_propagates(t *testing.T) {
	cfg := validCfg()
	cfg.TCPSettings.InterfaceSubnet = netip.Prefix{}
	mgr := &mockMgr{cfg: cfg}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "192.0.2.10", nil },
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "TCP interface address allocation fail") {
		t.Fatalf("want TCP allocation error, got %v", err)
	}
}

// --------- tests: allocateClientIPs ---------

func TestAllocateClientIPs_success(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{})

	tcp, udp, ws, err := g.allocateClientIPs(mgr.cfg)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !tcp.IsValid() || !udp.IsValid() || !ws.IsValid() {
		t.Fatalf("invalid addresses returned")
	}
	if mgr.incCalls != 1 {
		t.Fatalf("IncrementClientCounter must be called exactly once")
	}
}

func TestAllocateClientIPs_tcp_error(t *testing.T) {
	cfg := validCfg()
	cfg.TCPSettings.InterfaceSubnet = netip.Prefix{}
	mgr := &mockMgr{cfg: cfg}
	g := generatorWithMocks(mgr, mockIP{})

	_, _, _, err := g.allocateClientIPs(cfg)
	if err == nil || !strings.Contains(err.Error(), "TCP interface address allocation fail") {
		t.Fatalf("want TCP alloc error, got %v", err)
	}
}

func TestAllocateClientIPs_udp_error(t *testing.T) {
	cfg := validCfg()
	cfg.UDPSettings.InterfaceSubnet = netip.Prefix{}
	mgr := &mockMgr{cfg: cfg}
	g := generatorWithMocks(mgr, mockIP{})

	_, _, _, err := g.allocateClientIPs(cfg)
	if err == nil || !strings.Contains(err.Error(), "UDP interface address allocation fail") {
		t.Fatalf("want UDP alloc error, got %v", err)
	}
}

func TestAllocateClientIPs_ws_error(t *testing.T) {
	cfg := validCfg()
	cfg.WSSettings.InterfaceSubnet = netip.Prefix{}
	mgr := &mockMgr{cfg: cfg}
	g := generatorWithMocks(mgr, mockIP{})

	_, _, _, err := g.allocateClientIPs(cfg)
	if err == nil || !strings.Contains(err.Error(), "WS interface address allocation fail") {
		t.Fatalf("want WS alloc error, got %v", err)
	}
}

func TestAllocateClientIPs_increment_error(t *testing.T) {
	cfg := validCfg()
	mgr := &mockMgr{cfg: cfg, incErr: errors.New("inc-fail")}
	g := generatorWithMocks(mgr, mockIP{})

	_, _, _, err := g.allocateClientIPs(cfg)
	if err == nil || !strings.Contains(err.Error(), "inc-fail") {
		t.Fatalf("want increment error, got %v", err)
	}
}

// --------- tests: getDefaultProtocol ---------

func TestGetDefaultProtocol_priority(t *testing.T) {
	cfg := validCfg()

	// 1) UDP has priority
	cfg.EnableUDP, cfg.EnableTCP, cfg.EnableWS = true, true, true
	if got := getDefaultProtocol(cfg); got != settings.UDP {
		t.Fatalf("want UDP, got %v", got)
	}

	// 2) then TCP
	cfg.EnableUDP, cfg.EnableTCP, cfg.EnableWS = false, true, true
	if got := getDefaultProtocol(cfg); got != settings.TCP {
		t.Fatalf("want TCP, got %v", got)
	}

	// 3) else WS
	cfg.EnableUDP, cfg.EnableTCP, cfg.EnableWS = false, false, true
	if got := getDefaultProtocol(cfg); got != settings.WS {
		t.Fatalf("want WS, got %v", got)
	}
}

// --------- tests: deriveClientSettings ---------

func TestDeriveClientSettings_copies_fields_correctly(t *testing.T) {
	serverS := settings.Settings{
		InterfaceName:   "tun-tcp0",
		InterfaceSubnet: mustPrefix("10.0.0.0/24"),
		Port:            443,
		MTU:             1400,
		Encryption:      1,
		DialTimeoutMs:   2000,
	}
	clientIP := netip.MustParseAddr("10.0.0.8")
	host := mustHost("192.0.2.1")

	got := deriveClientSettings(serverS, clientIP, host, settings.TCP)

	if got.InterfaceName != serverS.InterfaceName {
		t.Fatalf("InterfaceName mismatch")
	}
	if got.InterfaceSubnet != serverS.InterfaceSubnet {
		t.Fatalf("InterfaceSubnet mismatch")
	}
	if got.InterfaceIP != clientIP {
		t.Fatalf("InterfaceIP: want %s, got %s", clientIP, got.InterfaceIP)
	}
	if got.Host != host {
		t.Fatalf("Host mismatch")
	}
	if got.Port != serverS.Port {
		t.Fatalf("Port mismatch")
	}
	if got.MTU != serverS.MTU {
		t.Fatalf("MTU: want %d, got %d", serverS.MTU, got.MTU)
	}
	if got.Protocol != settings.TCP {
		t.Fatalf("Protocol mismatch")
	}
	if got.Encryption != serverS.Encryption {
		t.Fatalf("Encryption mismatch")
	}
	if got.DialTimeoutMs != serverS.DialTimeoutMs {
		t.Fatalf("DialTimeoutMs mismatch")
	}
}

func TestDeriveClientSettings_udp_uses_safe_mtu(t *testing.T) {
	serverS := settings.Settings{MTU: 1400}
	got := deriveClientSettings(serverS, netip.Addr{}, "", settings.UDP)
	if got.MTU != settings.SafeMTU {
		t.Fatalf("UDP MTU: want SafeMTU (%d), got %d", settings.SafeMTU, got.MTU)
	}
}
