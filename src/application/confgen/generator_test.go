package confgen

import (
	"errors"
	"net/netip"
	"strings"
	"testing"

	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/cryptography/primitives"
	nip "tungo/infrastructure/network/ip"
	"tungo/infrastructure/settings"
)

// --------- fakes & stubs ---------

type mockMgr struct {
	cfg                *serverConfiguration.Configuration
	cfgErr             error
	cfgErrOnCall       int // when > 0, return cfgErr only on this call number
	cfgCalls           int
	incErr             error
	addPeerErr         error
	ensureIPv6Err      error
	incCalls           int
	ensureIPv6Calls    int
	addedPeers         []serverConfiguration.AllowedPeer
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
	m.cfgCalls++
	if m.cfgErrOnCall > 0 && m.cfgCalls == m.cfgErrOnCall {
		return nil, m.cfgErr
	}
	if m.cfgErrOnCall > 0 {
		return m.cfg, nil
	}
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
func (m *mockMgr) EnsureIPv6Subnets() error {
	m.ensureIPv6Calls++
	if m.ensureIPv6Err != nil {
		return m.ensureIPv6Err
	}
	// Simulate what the real manager does: set default IPv6 subnets if missing.
	for _, s := range []*settings.Settings{
		&m.cfg.TCPSettings,
		&m.cfg.UDPSettings,
		&m.cfg.WSSettings,
	} {
		if !s.IPv6Subnet.IsValid() {
			// leave as-is; tests that need IPv6 subnets pre-set them
		}
	}
	return nil
}
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
func (m mockIP) Route6AddDefaultDev(string) error                    { return nil }
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
			IPv6Subnet:      mustPrefix("fd00::/64"),
			Port:            443,
			MTU:             1400,
			DialTimeoutMs:   1000,
			Protocol:        settings.TCP,
		},
		UDPSettings: settings.Settings{
			InterfaceName:   "tun-udp0",
			InterfaceSubnet: mustPrefix("10.1.0.0/24"),
			IPv6Subnet:      mustPrefix("fd00:1::/64"),
			Port:            53,
			MTU:             1400,
			DialTimeoutMs:   1000,
			Protocol:        settings.UDP,
		},
		WSSettings: settings.Settings{
			InterfaceName:   "tun-ws0",
			InterfaceSubnet: mustPrefix("10.2.0.0/24"),
			IPv6Subnet:      mustPrefix("fd00:2::/64"),
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
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "2001:db8::1", nil
			}
			return "192.0.2.10", nil
		},
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !conf.TCPSettings.InterfaceIP.IsValid() {
		t.Fatal("TCP InterfaceIP must be valid")
	}
	// IPv6 should also be set
	if !conf.TCPSettings.IPv6IP.IsValid() {
		t.Fatal("TCP IPv6IP must be valid")
	}
	if !conf.UDPSettings.IPv6IP.IsValid() {
		t.Fatal("UDP IPv6IP must be valid")
	}
	if !conf.WSSettings.IPv6IP.IsValid() {
		t.Fatal("WS IPv6IP must be valid")
	}
	// IPv6Host should be populated in client settings
	if conf.TCPSettings.IPv6Host.IsZero() {
		t.Fatal("TCP IPv6Host must be set when server has IPv6")
	}
	expectedHost := mustHost("2001:db8::1")
	if conf.TCPSettings.IPv6Host != expectedHost {
		t.Fatalf("TCP IPv6Host: want %s, got %s", expectedHost, conf.TCPSettings.IPv6Host)
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
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			return "", errors.New("addr-fail")
		},
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
	// No IPv6 detected → IPv6Host should be zero
	if !conf.WSSettings.IPv6Host.IsZero() {
		t.Fatal("IPv6Host must be zero when no IPv6 detected")
	}
}

func TestGenerate_clientID_matches_allocated_IPs(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "2001:db8::1", nil
			}
			return "192.0.2.10", nil
		},
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(mgr.addedPeers) != 1 {
		t.Fatalf("expected 1 added peer, got %d", len(mgr.addedPeers))
	}

	peer := mgr.addedPeers[0]

	// The invariant: AllocateClientIP(subnet, peer.ClientID) must produce the
	// same IP that was given to the client configuration.
	for _, tc := range []struct {
		name     string
		subnet   netip.Prefix
		clientIP netip.Addr
	}{
		{"TCP", mgr.cfg.TCPSettings.InterfaceSubnet, conf.TCPSettings.InterfaceIP},
		{"UDP", mgr.cfg.UDPSettings.InterfaceSubnet, conf.UDPSettings.InterfaceIP},
		{"WS", mgr.cfg.WSSettings.InterfaceSubnet, conf.WSSettings.InterfaceIP},
		{"TCP-IPv6", mgr.cfg.TCPSettings.IPv6Subnet, conf.TCPSettings.IPv6IP},
		{"UDP-IPv6", mgr.cfg.UDPSettings.IPv6Subnet, conf.UDPSettings.IPv6IP},
		{"WS-IPv6", mgr.cfg.WSSettings.IPv6Subnet, conf.WSSettings.IPv6IP},
	} {
		got, allocErr := nip.AllocateClientIP(tc.subnet, peer.ClientID)
		if allocErr != nil {
			t.Fatalf("%s: AllocateClientIP(%s, %d) error: %v", tc.name, tc.subnet, peer.ClientID, allocErr)
		}
		if got != tc.clientIP {
			t.Fatalf("%s: server would assign %s but client expects %s (ClientID=%d)",
				tc.name, got, tc.clientIP, peer.ClientID)
		}
	}
}

func TestGenerate_allocate_error_propagates(t *testing.T) {
	cfg := validCfg()
	cfg.TCPSettings.InterfaceSubnet = netip.Prefix{}
	mgr := &mockMgr{cfg: cfg}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "2001:db8::1", nil
			}
			return "192.0.2.10", nil
		},
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

	ips, err := g.allocateClientIPs(mgr.cfg, mgr.cfg.ClientCounter+1)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !ips.tcp.IsValid() || !ips.udp.IsValid() || !ips.ws.IsValid() {
		t.Fatalf("invalid IPv4 addresses returned")
	}
	if !ips.tcpV6.IsValid() || !ips.udpV6.IsValid() || !ips.wsV6.IsValid() {
		t.Fatalf("invalid IPv6 addresses returned")
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

	_, err := g.allocateClientIPs(cfg, cfg.ClientCounter+1)
	if err == nil || !strings.Contains(err.Error(), "TCP interface address allocation fail") {
		t.Fatalf("want TCP alloc error, got %v", err)
	}
}

func TestAllocateClientIPs_udp_error(t *testing.T) {
	cfg := validCfg()
	cfg.UDPSettings.InterfaceSubnet = netip.Prefix{}
	mgr := &mockMgr{cfg: cfg}
	g := generatorWithMocks(mgr, mockIP{})

	_, err := g.allocateClientIPs(cfg, cfg.ClientCounter+1)
	if err == nil || !strings.Contains(err.Error(), "UDP interface address allocation fail") {
		t.Fatalf("want UDP alloc error, got %v", err)
	}
}

func TestAllocateClientIPs_ws_error(t *testing.T) {
	cfg := validCfg()
	cfg.WSSettings.InterfaceSubnet = netip.Prefix{}
	mgr := &mockMgr{cfg: cfg}
	g := generatorWithMocks(mgr, mockIP{})

	_, err := g.allocateClientIPs(cfg, cfg.ClientCounter+1)
	if err == nil || !strings.Contains(err.Error(), "WS interface address allocation fail") {
		t.Fatalf("want WS alloc error, got %v", err)
	}
}

func TestAllocateClientIPs_increment_error(t *testing.T) {
	cfg := validCfg()
	mgr := &mockMgr{cfg: cfg, incErr: errors.New("inc-fail")}
	g := generatorWithMocks(mgr, mockIP{})

	_, err := g.allocateClientIPs(cfg, cfg.ClientCounter+1)
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
		IPv6Subnet:      mustPrefix("fd00::/64"),
		Port:            443,
		MTU:             1400,
		Encryption:      1,
		DialTimeoutMs:   2000,
	}
	clientIP := netip.MustParseAddr("10.0.0.8")
	clientIPv6 := netip.MustParseAddr("fd00::8")
	host := mustHost("192.0.2.1")
	ipv6Host := mustHost("2001:db8::1")

	got := deriveClientSettings(serverS, clientIP, clientIPv6, host, ipv6Host, settings.TCP)

	if got.InterfaceName != serverS.InterfaceName {
		t.Fatalf("InterfaceName mismatch")
	}
	if got.InterfaceSubnet != serverS.InterfaceSubnet {
		t.Fatalf("InterfaceSubnet mismatch")
	}
	if got.InterfaceIP != clientIP {
		t.Fatalf("InterfaceIP: want %s, got %s", clientIP, got.InterfaceIP)
	}
	if got.IPv6Subnet != serverS.IPv6Subnet {
		t.Fatalf("IPv6Subnet mismatch")
	}
	if got.IPv6IP != clientIPv6 {
		t.Fatalf("IPv6IP: want %s, got %s", clientIPv6, got.IPv6IP)
	}
	if got.Host != host {
		t.Fatalf("Host mismatch")
	}
	if got.IPv6Host != ipv6Host {
		t.Fatalf("IPv6Host: want %s, got %s", ipv6Host, got.IPv6Host)
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
	got := deriveClientSettings(serverS, netip.Addr{}, netip.Addr{}, "", "", settings.UDP)
	if got.MTU != settings.SafeMTU {
		t.Fatalf("UDP MTU: want SafeMTU (%d), got %d", settings.SafeMTU, got.MTU)
	}
}

// --------- tests: IPv6 detection ---------

func TestGenerate_no_ipv6_on_server(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "", errors.New("no-ipv6")
			}
			return "192.0.2.10", nil
		},
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !conf.TCPSettings.IPv6Host.IsZero() {
		t.Fatal("IPv6Host must be zero when server has no IPv6")
	}
	if mgr.ensureIPv6Calls != 0 {
		t.Fatal("EnsureIPv6Subnets should not be called when no IPv6 detected")
	}
}

func TestGenerate_ipv6_skips_link_local(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "fe80::1\n2001:db8::42", nil
			}
			return "192.0.2.10", nil
		},
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	expectedHost := mustHost("2001:db8::42")
	if conf.TCPSettings.IPv6Host != expectedHost {
		t.Fatalf("IPv6Host: want %s, got %s (should skip fe80:: link-local)", expectedHost, conf.TCPSettings.IPv6Host)
	}
}

func TestGenerate_ipv6_only_link_local(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "fe80::1", nil
			}
			return "192.0.2.10", nil
		},
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !conf.TCPSettings.IPv6Host.IsZero() {
		t.Fatal("IPv6Host must be zero when only link-local IPv6 available")
	}
}

type mockKeyDeriver struct {
	genErr error
}

func (m *mockKeyDeriver) GenerateX25519KeyPair() ([]byte, [32]byte, error) {
	if m.genErr != nil {
		return nil, [32]byte{}, m.genErr
	}
	return make([]byte, 32), [32]byte{1}, nil
}

func (m *mockKeyDeriver) DeriveKey(_, _, _ []byte) ([]byte, error) {
	return nil, nil
}

func TestGenerate_ensure_ipv6_subnets_error(t *testing.T) {
	mgr := &mockMgr{
		cfg:           validCfg(),
		ensureIPv6Err: errors.New("ensure-fail"),
	}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "2001:db8::1", nil
			}
			return "192.0.2.10", nil
		},
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "failed to auto-enable IPv6 subnets") {
		t.Fatalf("want auto-enable error, got %v", err)
	}
}

func TestGenerate_reread_config_error(t *testing.T) {
	mgr := &mockMgr{
		cfg:          validCfg(),
		cfgErr:       errors.New("reread-fail"),
		cfgErrOnCall: 2, // succeed on 1st call, fail on 2nd (re-read after EnsureIPv6Subnets)
	}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "2001:db8::1", nil
			}
			return "192.0.2.10", nil
		},
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "failed to re-read server configuration") {
		t.Fatalf("want re-read error, got %v", err)
	}
}

func TestGenerate_keypair_error(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "", errors.New("no-ip") },
	})
	g.keyDeriver = &mockKeyDeriver{genErr: errors.New("keygen-fail")}

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "failed to generate client keypair") {
		t.Fatalf("want keypair error, got %v", err)
	}
}

func TestGenerate_add_peer_error(t *testing.T) {
	mgr := &mockMgr{
		cfg:        validCfg(),
		addPeerErr: errors.New("add-fail"),
	}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc:  func(int, string) (string, error) { return "", errors.New("no-ip") },
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "failed to add client to AllowedPeers") {
		t.Fatalf("want add-peer error, got %v", err)
	}
}

func TestGenerate_ipv6_unparseable_addr_skipped(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "not-an-ip\n2001:db8::1", nil
			}
			return "192.0.2.10", nil
		},
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	expectedHost := mustHost("2001:db8::1")
	if conf.TCPSettings.IPv6Host != expectedHost {
		t.Fatalf("IPv6Host: want %s, got %s (should skip unparseable line)", expectedHost, conf.TCPSettings.IPv6Host)
	}
}

func TestGenerate_invalid_host_error(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "", errors.New("no-ipv6")
			}
			return "http://bad", nil // not a valid IP or domain
		},
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "invalid server host") {
		t.Fatalf("want invalid host error, got %v", err)
	}
}

func TestGenerate_ipv6_empty_line_skipped(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockIP{
		RouteDefaultFunc: func() (string, error) { return "eth0", nil },
		AddrShowDevFunc: func(v int, _ string) (string, error) {
			if v == 6 {
				return "\n2001:db8::1", nil // leading empty line
			}
			return "192.0.2.10", nil
		},
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	expectedHost := mustHost("2001:db8::1")
	if conf.TCPSettings.IPv6Host != expectedHost {
		t.Fatalf("IPv6Host: want %s, got %s", expectedHost, conf.TCPSettings.IPv6Host)
	}
}
