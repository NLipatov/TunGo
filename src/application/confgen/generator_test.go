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
	cfg             *serverConfiguration.Configuration
	cfgErr          error
	cfgErrOnCall    int // when > 0, return cfgErr only on this call number
	cfgCalls        int
	incErr          error
	addPeerErr      error
	ensureIPv6Err   error
	incCalls        int
	ensureIPv6Calls int
	addedPeers      []serverConfiguration.AllowedPeer
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
	return nil
}
func (m *mockMgr) AddAllowedPeer(peer serverConfiguration.AllowedPeer) error {
	if m.addPeerErr != nil {
		return m.addPeerErr
	}
	m.addedPeers = append(m.addedPeers, peer)
	return nil
}
func (m *mockMgr) ListAllowedPeers() ([]serverConfiguration.AllowedPeer, error) { return nil, nil }
func (m *mockMgr) SetAllowedPeerEnabled(_ int, _ bool) error                    { return nil }
func (m *mockMgr) RemoveAllowedPeer(_ int) error                                { return nil }
func (m *mockMgr) InvalidateCache()                                             {}

// mockResolver implements hostResolver for tests.
type mockResolver struct {
	ipv4    string
	ipv4Err error
	ipv6    string
	ipv6Err error
}

func (m mockResolver) ResolveIPv4() (string, error) { return m.ipv4, m.ipv4Err }
func (m mockResolver) ResolveIPv6() (string, error) { return m.ipv6, m.ipv6Err }

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
			Addressing: settings.Addressing{
				TunName:    "tun-tcp0",
				IPv4Subnet: mustPrefix("10.0.0.0/24"),
				IPv6Subnet: mustPrefix("fd00::/64"),
				Port:       443,
			},
			MTU:           1400,
			DialTimeoutMs: 1000,
			Protocol:      settings.TCP,
		},
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				TunName:    "tun-udp0",
				IPv4Subnet: mustPrefix("10.1.0.0/24"),
				IPv6Subnet: mustPrefix("fd00:1::/64"),
				Port:       53,
			},
			MTU:           1400,
			DialTimeoutMs: 1000,
			Protocol:      settings.UDP,
		},
		WSSettings: settings.Settings{
			Addressing: settings.Addressing{
				TunName:    "tun-ws0",
				IPv4Subnet: mustPrefix("10.2.0.0/24"),
				IPv6Subnet: mustPrefix("fd00:2::/64"),
				Port:       8080,
			},
			MTU:           1400,
			DialTimeoutMs: 1000,
			Protocol:      settings.WS,
		},
	}
}

func generatorWithMocks(mgr *mockMgr, r mockResolver) *Generator {
	g := NewGenerator(mgr, &primitives.DefaultKeyDeriver{})
	g.resolver = r
	return g
}

// --------- tests: Generate ---------

func TestGenerate_success(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockResolver{
		ipv4: "192.0.2.10",
		ipv6: "2001:db8::1",
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// IPs are derived at Resolve() time, not Generate() time.
	if err := conf.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !conf.TCPSettings.IPv4.IsValid() {
		t.Fatal("TCP IPv4 must be valid after Resolve")
	}
	if !conf.TCPSettings.IPv6.IsValid() {
		t.Fatal("TCP IPv6 must be valid after Resolve")
	}
	if !conf.UDPSettings.IPv6.IsValid() {
		t.Fatal("UDP IPv6 must be valid after Resolve")
	}
	if !conf.WSSettings.IPv6.IsValid() {
		t.Fatal("WS IPv6 must be valid after Resolve")
	}
	if !conf.TCPSettings.Server.HasIPv6() {
		t.Fatal("TCP Server must have IPv6 when server has IPv6")
	}
	expectedIPv6 := netip.MustParseAddr("2001:db8::1")
	if ipv6, ok := conf.TCPSettings.Server.IPv6(); !ok || ipv6 != expectedIPv6 {
		t.Fatalf("TCP Server IPv6: want %s, got %v", expectedIPv6, ipv6)
	}
}

func TestGenerate_config_error(t *testing.T) {
	mgr := &mockMgr{cfgErr: errors.New("cfg-fail")}
	g := generatorWithMocks(mgr, mockResolver{})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "failed to read server configuration") {
		t.Fatalf("want config read error, got %v", err)
	}
}

func TestGenerate_resolve_error_no_fallback(t *testing.T) {
	cfg := validCfg()
	cfg.FallbackServerAddress = ""
	mgr := &mockMgr{cfg: cfg}
	g := generatorWithMocks(mgr, mockResolver{
		ipv4Err: errors.New("resolve-fail"),
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "no fallback address") {
		t.Fatalf("want no-fallback error, got %v", err)
	}
}

func TestGenerate_resolve_error_with_fallback_success(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockResolver{
		ipv4Err: errors.New("no-ip"),
		ipv6Err: errors.New("no-ipv6"),
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if conf.WSSettings.Server != mustHost(mgr.cfg.FallbackServerAddress) {
		t.Fatalf("expected fallback Server, got %q", conf.WSSettings.Server)
	}
	if mgr.incCalls != 1 {
		t.Fatalf("IncrementClientCounter not called")
	}
	if conf.WSSettings.Server.HasIPv6() {
		t.Fatal("Server must not have IPv6 when no IPv6 detected")
	}
}

func TestGenerate_clientID_matches_allocated_IPs(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockResolver{
		ipv4: "192.0.2.10",
		ipv6: "2001:db8::1",
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	// Derive IPs via Resolve
	if err := conf.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(mgr.addedPeers) != 1 {
		t.Fatalf("expected 1 added peer, got %d", len(mgr.addedPeers))
	}

	peer := mgr.addedPeers[0]

	for _, tc := range []struct {
		name     string
		subnet   netip.Prefix
		clientIP netip.Addr
	}{
		{"TCP", mgr.cfg.TCPSettings.IPv4Subnet, conf.TCPSettings.IPv4},
		{"UDP", mgr.cfg.UDPSettings.IPv4Subnet, conf.UDPSettings.IPv4},
		{"WS", mgr.cfg.WSSettings.IPv4Subnet, conf.WSSettings.IPv4},
		{"TCP-IPv6", mgr.cfg.TCPSettings.IPv6Subnet, conf.TCPSettings.IPv6},
		{"UDP-IPv6", mgr.cfg.UDPSettings.IPv6Subnet, conf.UDPSettings.IPv6},
		{"WS-IPv6", mgr.cfg.WSSettings.IPv6Subnet, conf.WSSettings.IPv6},
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

// --------- tests: getDefaultProtocol ---------

func TestGetDefaultProtocol_priority(t *testing.T) {
	cfg := validCfg()

	cfg.EnableUDP, cfg.EnableTCP, cfg.EnableWS = true, true, true
	if got := getDefaultProtocol(cfg); got != settings.UDP {
		t.Fatalf("want UDP, got %v", got)
	}

	cfg.EnableUDP, cfg.EnableTCP, cfg.EnableWS = false, true, true
	if got := getDefaultProtocol(cfg); got != settings.TCP {
		t.Fatalf("want TCP, got %v", got)
	}

	cfg.EnableUDP, cfg.EnableTCP, cfg.EnableWS = false, false, true
	if got := getDefaultProtocol(cfg); got != settings.WS {
		t.Fatalf("want WS, got %v", got)
	}
}

// --------- tests: deriveClientSettings ---------

func TestDeriveClientSettings_copies_fields_correctly(t *testing.T) {
	serverS := settings.Settings{
		Addressing: settings.Addressing{
			TunName:    "tun-tcp0",
			IPv4Subnet: mustPrefix("10.0.0.0/24"),
			IPv6Subnet: mustPrefix("fd00::/64"),
			Port:       443,
		},
		MTU:           1400,
		Encryption:    1,
		DialTimeoutMs: 2000,
	}
	host := mustHost("192.0.2.1").WithIPv6(netip.MustParseAddr("2001:db8::1"))

	got := deriveClientSettings(serverS, host, settings.TCP)

	if got.TunName != serverS.TunName {
		t.Fatalf("TunName mismatch")
	}
	if got.IPv4Subnet != serverS.IPv4Subnet {
		t.Fatalf("IPv4Subnet mismatch")
	}
	if got.IPv6Subnet != serverS.IPv6Subnet {
		t.Fatalf("IPv6Subnet mismatch")
	}
	if got.Server != host {
		t.Fatalf("Server mismatch")
	}
	if ipv6, ok := got.Server.IPv6(); !ok || ipv6 != netip.MustParseAddr("2001:db8::1") {
		t.Fatalf("Server IPv6: want 2001:db8::1, got %v", ipv6)
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
	// IPs should NOT be set by deriveClientSettings — they're derived at Resolve() time.
	if got.IPv4.IsValid() {
		t.Fatalf("IPv4 should not be set by deriveClientSettings")
	}
}

func TestDeriveClientSettings_udp_uses_safe_mtu(t *testing.T) {
	serverS := settings.Settings{MTU: 1400}
	got := deriveClientSettings(serverS, settings.Host{}, settings.UDP)
	if got.MTU != settings.SafeMTU {
		t.Fatalf("UDP MTU: want SafeMTU (%d), got %d", settings.SafeMTU, got.MTU)
	}
}

// --------- tests: IPv6 detection ---------

func TestGenerate_no_ipv6_on_server(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockResolver{
		ipv4:    "192.0.2.10",
		ipv6Err: errors.New("no-ipv6"),
	})

	conf, err := g.Generate()
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if conf.TCPSettings.Server.HasIPv6() {
		t.Fatal("Server must not have IPv6 when server has no IPv6")
	}
	if mgr.ensureIPv6Calls != 0 {
		t.Fatal("EnsureIPv6Subnets should not be called when no IPv6 detected")
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
	g := generatorWithMocks(mgr, mockResolver{
		ipv4: "192.0.2.10",
		ipv6: "2001:db8::1",
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
		cfgErrOnCall: 2,
	}
	g := generatorWithMocks(mgr, mockResolver{
		ipv4: "192.0.2.10",
		ipv6: "2001:db8::1",
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "failed to re-read server configuration") {
		t.Fatalf("want re-read error, got %v", err)
	}
}

func TestGenerate_keypair_error(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockResolver{
		ipv4Err: errors.New("no-ip"),
		ipv6Err: errors.New("no-ipv6"),
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
	g := generatorWithMocks(mgr, mockResolver{
		ipv4Err: errors.New("no-ip"),
		ipv6Err: errors.New("no-ipv6"),
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "failed to add client to AllowedPeers") {
		t.Fatalf("want add-peer error, got %v", err)
	}
}

func TestGenerate_invalid_host_error(t *testing.T) {
	mgr := &mockMgr{cfg: validCfg()}
	g := generatorWithMocks(mgr, mockResolver{
		ipv4:    "http://bad",
		ipv6Err: errors.New("no-ipv6"),
	})

	_, err := g.Generate()
	if err == nil || !strings.Contains(err.Error(), "invalid server host") {
		t.Fatalf("want invalid host error, got %v", err)
	}
}
