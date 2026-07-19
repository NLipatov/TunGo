package configuration

import (
	"errors"
	"net/netip"
	"testing"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	serverConfiguration "tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type runtimeInfoClientManager struct {
	cfg *clientConfiguration.Configuration
	err error
}

func (m runtimeInfoClientManager) Configuration() (*clientConfiguration.Configuration, error) {
	return m.cfg, m.err
}

type runtimeInfoServerManager struct {
	cfg        *serverConfiguration.Configuration
	err        error
	peers      []serverConfiguration.AllowedPeer
	peersErr   error
	setID      int
	setEnabled bool
	setErr     error
	removeID   int
	removeErr  error
}

func (m runtimeInfoServerManager) Configuration() (*serverConfiguration.Configuration, error) {
	return m.cfg, m.err
}
func (m runtimeInfoServerManager) IncrementClientCounter() error { return nil }
func (m runtimeInfoServerManager) InjectX25519Keys(_, _ []byte) error {
	return nil
}
func (m runtimeInfoServerManager) AddAllowedPeer(serverConfiguration.AllowedPeer) error {
	return nil
}
func (m *runtimeInfoServerManager) ListAllowedPeers() ([]serverConfiguration.AllowedPeer, error) {
	return m.peers, m.peersErr
}
func (m *runtimeInfoServerManager) SetAllowedPeerEnabled(id int, enabled bool) error {
	m.setID = id
	m.setEnabled = enabled
	return m.setErr
}
func (m *runtimeInfoServerManager) RemoveAllowedPeer(id int) error {
	m.removeID = id
	return m.removeErr
}
func (m *runtimeInfoServerManager) EnsureIPv6Subnets() error { return nil }
func (m *runtimeInfoServerManager) InvalidateCache()         {}

type pathResolverStub struct {
	path string
	err  error
}

func (r pathResolverStub) Resolve() (string, error) { return r.path, r.err }

func TestClientControlRuntimeConfiguration(t *testing.T) {
	publicKey := make([]byte, 32)
	publicKey[0] = 7
	conf := &clientConfiguration.Configuration{
		ClientID: 3,
		Protocol: settings.UDP,
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.1.0/24"),
			},
		},
		ClientPublicKey: publicKey,
	}
	control := clientControl{
		manager: runtimeInfoClientManager{cfg: conf},
	}

	got, err := control.ClientRuntimeConfiguration()
	if err != nil {
		t.Fatalf("ClientRuntimeConfiguration() error = %v", err)
	}
	if got.UDPSettings.IPv4 != netip.MustParseAddr("10.0.1.4") {
		t.Fatalf("derived IPv4 = %v", got.UDPSettings.IPv4)
	}
	got.ClientPublicKey[0] = 9
	if conf.ClientPublicKey[0] != 7 {
		t.Fatal("runtime configuration aliases persisted client key")
	}
}

func TestServerControlRuntimeConfiguration(t *testing.T) {
	peerKey := make([]byte, 32)
	peerKey[0] = 11
	conf := &serverConfiguration.Configuration{
		X25519PublicKey:  make([]byte, 32),
		X25519PrivateKey: make([]byte, 32),
		EnableTCP:        true,
		AllowedPeers: []serverConfiguration.AllowedPeer{{
			Name:      "client-1",
			PublicKey: peerKey,
			Enabled:   true,
			ClientID:  1,
		}},
	}
	manager := &runtimeInfoServerManager{cfg: conf}
	control := serverControl{
		resolver: pathResolverStub{path: "/tmp/server.json"},
		manager:  manager,
	}

	got, err := control.ServerRuntimeConfiguration()
	if err != nil {
		t.Fatalf("ServerRuntimeConfiguration() error = %v", err)
	}
	if !got.EnableTCP || len(got.AllowedPeers) != 1 || got.AllowedPeers[0].ClientID != 1 {
		t.Fatalf("unexpected runtime configuration: %+v", got)
	}
	got.AllowedPeers[0].PublicKey[0] = 12
	if conf.AllowedPeers[0].PublicKey[0] != 11 {
		t.Fatal("runtime configuration aliases persisted peer key")
	}
}

func TestEndpointInfoFromSettings(t *testing.T) {
	settingsValue := settings.Settings{
		Protocol: settings.TCP,
		Addressing: settings.Addressing{
			Server: settings.Host{}.
				WithIPv4(netip.MustParseAddr("198.51.100.10")).
				WithIPv6(netip.MustParseAddr("2001:db8::10")),
			Port: 443,
			IPv4: netip.MustParseAddr("10.0.0.2"),
			IPv6: netip.MustParseAddr("fd00::2"),
		},
	}

	got, ok := endpointInfoFromSettings(settings.UDP, settingsValue)
	if !ok {
		t.Fatal("expected endpoint entry")
	}
	if got.Protocol != settings.TCP {
		t.Fatalf("Protocol: got %v", got.Protocol)
	}
	if ipv4, ok := got.Server.IPv4(); !ok || ipv4 != netip.MustParseAddr("198.51.100.10") {
		t.Fatalf("Server.IPv4: got %v ok=%v", ipv4, ok)
	}
	if ipv6, ok := got.Server.IPv6(); !ok || ipv6 != netip.MustParseAddr("2001:db8::10") {
		t.Fatalf("Server.IPv6: got %v ok=%v", ipv6, ok)
	}
	if got.Port != 443 {
		t.Fatalf("Port: got %v", got.Port)
	}
	if got.TunnelIPv4 != netip.MustParseAddr("10.0.0.2") {
		t.Fatalf("TunnelIPv4: got %v", got.TunnelIPv4)
	}
	if got.TunnelIPv6 != netip.MustParseAddr("fd00::2") {
		t.Fatalf("TunnelIPv6: got %v", got.TunnelIPv6)
	}
}

func TestEndpointInfoFromSettings_UsesFallbackProtocol(t *testing.T) {
	settingsValue := settings.Settings{
		Protocol: settings.UNKNOWN,
		Addressing: settings.Addressing{
			IPv4: netip.MustParseAddr("10.0.0.1"),
		},
	}

	got, ok := endpointInfoFromSettings(settings.WS, settingsValue)
	if !ok {
		t.Fatal("expected endpoint entry")
	}
	if got.Protocol != settings.WS {
		t.Fatalf("Protocol: got %v", got.Protocol)
	}
}

func TestEndpointInfoFromSettings_InvalidAddressesReturnFalse(t *testing.T) {
	got, ok := endpointInfoFromSettings(settings.TCP, settings.Settings{})
	if ok {
		t.Fatal("expected invalid endpoint to be rejected")
	}
	if got != (EndpointInfo{}) {
		t.Fatalf("expected zero endpoint on invalid input, got %+v", got)
	}
}

func TestClientControlRuntimeInfo(t *testing.T) {
	control := clientControl{
		manager: runtimeInfoClientManager{
			cfg: &clientConfiguration.Configuration{
				ClientID: 1,
				Protocol: settings.TCP,
				TCPSettings: settings.Settings{
					Protocol: settings.TCP,
					Addressing: settings.Addressing{
						Server:     settings.Host{}.WithIPv4(netip.MustParseAddr("198.51.100.10")),
						Port:       443,
						IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
					},
				},
			},
		},
	}

	got, err := control.RuntimeInfo()
	if err != nil {
		t.Fatalf("RuntimeInfo() error = %v", err)
	}
	if got.Protocol != settings.TCP {
		t.Fatalf("Protocol: got %v", got.Protocol)
	}
	if len(got.Endpoints) != 1 {
		t.Fatalf("expected one endpoint, got %d", len(got.Endpoints))
	}
	if got.Endpoints[0].TunnelIPv4 != netip.MustParseAddr("10.0.0.2") {
		t.Fatalf("TunnelIPv4: got %v", got.Endpoints[0].TunnelIPv4)
	}
}

func TestClientControlRuntimeInfo_ConfigurationError(t *testing.T) {
	want := errors.New("read failed")
	control := clientControl{manager: runtimeInfoClientManager{err: want}}

	_, err := control.RuntimeInfo()
	if !errors.Is(err, want) {
		t.Fatalf("expected configuration error, got %v", err)
	}
}

func TestClientControlRuntimeInfo_ResolveActiveError(t *testing.T) {
	control := clientControl{
		manager: runtimeInfoClientManager{
			cfg: &clientConfiguration.Configuration{Protocol: settings.UNKNOWN},
		},
	}

	_, err := control.RuntimeInfo()
	if err == nil || err.Error() != "unsupported protocol: UNKNOWN" {
		t.Fatalf("expected unsupported protocol error, got %v", err)
	}
}

func TestServerControlRuntimeInfo(t *testing.T) {
	control := serverControl{
		manager: &runtimeInfoServerManager{
			cfg: &serverConfiguration.Configuration{
				EnableTCP: true,
				EnableUDP: true,
				EnableWS:  true,
				TCPSettings: settings.Settings{
					Protocol: settings.TCP,
					Addressing: settings.Addressing{
						Server: settings.Host{}.WithIPv4(netip.MustParseAddr("198.51.100.10")),
						IPv4:   netip.MustParseAddr("10.0.0.1"),
					},
				},
				UDPSettings: settings.Settings{
					Protocol: settings.UDP,
					Addressing: settings.Addressing{
						Server: settings.Host{}.WithIPv6(netip.MustParseAddr("2001:db8::20")),
						IPv4:   netip.MustParseAddr("10.0.1.1"),
					},
				},
				WSSettings: settings.Settings{
					Protocol: settings.WS,
					Addressing: settings.Addressing{
						Server: settings.Host{}.WithIPv4(netip.MustParseAddr("198.51.100.30")),
						IPv4:   netip.MustParseAddr("10.0.2.1"),
					},
				},
			},
		},
	}

	got, err := control.RuntimeInfo()
	if err != nil {
		t.Fatalf("RuntimeInfo() error = %v", err)
	}
	if len(got.Endpoints) != 3 {
		t.Fatalf("expected three endpoints, got %d", len(got.Endpoints))
	}
	if got.Endpoints[0].Protocol != settings.TCP || got.Endpoints[0].TunnelIPv4 != netip.MustParseAddr("10.0.0.1") {
		t.Fatalf("unexpected TCP endpoint: %+v", got.Endpoints[0])
	}
	if got.Endpoints[1].Protocol != settings.UDP || got.Endpoints[1].TunnelIPv4 != netip.MustParseAddr("10.0.1.1") {
		t.Fatalf("unexpected UDP endpoint: %+v", got.Endpoints[1])
	}
	if got.Endpoints[2].Protocol != settings.WS || got.Endpoints[2].TunnelIPv4 != netip.MustParseAddr("10.0.2.1") {
		t.Fatalf("unexpected WS endpoint: %+v", got.Endpoints[2])
	}
}

func TestServerControlRuntimeInfo_ConfigurationError(t *testing.T) {
	want := errors.New("read failed")
	control := serverControl{manager: &runtimeInfoServerManager{err: want}}

	_, err := control.RuntimeInfo()
	if !errors.Is(err, want) {
		t.Fatalf("expected configuration error, got %v", err)
	}
}

func TestServerControlGenerateClientConfiguration_ConfigurationError(t *testing.T) {
	want := errors.New("read failed")
	control := serverControl{manager: &runtimeInfoServerManager{err: want}}

	_, err := control.GenerateClientConfiguration()
	if !errors.Is(err, want) {
		t.Fatalf("expected generation error, got %v", err)
	}
}

func TestServerControlListPeersCopiesPublicKey(t *testing.T) {
	key := []byte{1, 2, 3}
	manager := &runtimeInfoServerManager{
		peers: []serverConfiguration.AllowedPeer{{
			Name:      "client-1",
			PublicKey: key,
			Enabled:   true,
			ClientID:  7,
		}},
	}
	control := serverControl{manager: manager}

	got, err := control.ListPeers()
	if err != nil {
		t.Fatalf("ListPeers() error = %v", err)
	}
	if len(got) != 1 || got[0].Name != "client-1" || got[0].ClientID != 7 || !got[0].Enabled {
		t.Fatalf("unexpected peer list: %+v", got)
	}
	key[0] = 9
	if got[0].PublicKey[0] != 1 {
		t.Fatalf("expected public key copy, got %v", got[0].PublicKey)
	}
}

func TestServerControlListPeersError(t *testing.T) {
	want := errors.New("list failed")
	control := serverControl{manager: &runtimeInfoServerManager{peersErr: want}}

	_, err := control.ListPeers()
	if !errors.Is(err, want) {
		t.Fatalf("expected list error, got %v", err)
	}
}

func TestServerControlSetAndRemovePeer(t *testing.T) {
	manager := &runtimeInfoServerManager{}
	control := serverControl{manager: manager}

	if err := control.SetPeerEnabled(7, true); err != nil {
		t.Fatalf("SetPeerEnabled() error = %v", err)
	}
	if manager.setID != 7 || !manager.setEnabled {
		t.Fatalf("unexpected set call: id=%d enabled=%v", manager.setID, manager.setEnabled)
	}

	if err := control.RemovePeer(8); err != nil {
		t.Fatalf("RemovePeer() error = %v", err)
	}
	if manager.removeID != 8 {
		t.Fatalf("unexpected remove id: %d", manager.removeID)
	}
}
