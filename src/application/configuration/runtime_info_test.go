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
	cfg *serverConfiguration.Configuration
	err error
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
func (m runtimeInfoServerManager) ListAllowedPeers() ([]serverConfiguration.AllowedPeer, error) {
	return nil, nil
}
func (m runtimeInfoServerManager) SetAllowedPeerEnabled(int, bool) error { return nil }
func (m runtimeInfoServerManager) RemoveAllowedPeer(int) error           { return nil }
func (m runtimeInfoServerManager) EnsureIPv6Subnets() error              { return nil }
func (m runtimeInfoServerManager) InvalidateCache()                      {}

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

func TestServerControlRuntimeInfo(t *testing.T) {
	control := serverControl{
		manager: runtimeInfoServerManager{
			cfg: &serverConfiguration.Configuration{
				EnableTCP: true,
				EnableUDP: true,
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
			},
		},
	}

	got, err := control.RuntimeInfo()
	if err != nil {
		t.Fatalf("RuntimeInfo() error = %v", err)
	}
	if len(got.Endpoints) != 2 {
		t.Fatalf("expected two endpoints, got %d", len(got.Endpoints))
	}
	if got.Endpoints[0].Protocol != settings.TCP || got.Endpoints[0].TunnelIPv4 != netip.MustParseAddr("10.0.0.1") {
		t.Fatalf("unexpected TCP endpoint: %+v", got.Endpoints[0])
	}
	if got.Endpoints[1].Protocol != settings.UDP || got.Endpoints[1].TunnelIPv4 != netip.MustParseAddr("10.0.1.1") {
		t.Fatalf("unexpected UDP endpoint: %+v", got.Endpoints[1])
	}
}
