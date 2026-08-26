package server

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
)

func TestFileGenerateClient(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	serverConfiguration := newConfiguration()
	serverConfiguration.Host = "192.0.2.1"
	serverConfiguration.X25519PublicKey, serverConfiguration.X25519PrivateKey = testX25519KeyPair(t, 1)
	writeServerConfiguration(t, path, *serverConfiguration)
	file := NewFile(path)

	generated, err := file.GenerateClient()
	if err != nil {
		t.Fatal(err)
	}
	if generated.Path != filepath.Join(filepath.Dir(path), "client_configuration.json.1") {
		t.Fatalf("Path = %q", generated.Path)
	}
	data, err := os.ReadFile(generated.Path)
	if err != nil {
		t.Fatal(err)
	}
	var clientConfiguration clientconfig.Configuration
	if err := json.Unmarshal(data, &clientConfiguration); err != nil {
		t.Fatal(err)
	}
	if clientConfiguration.ClientID != 1 || clientConfiguration.Protocol != settings.UDP {
		t.Fatalf("unexpected client configuration: %+v", clientConfiguration)
	}
	if clientConfiguration.UDPSettings.Server.IPv4 != "192.0.2.1" {
		t.Fatalf("server host = %+v", clientConfiguration.UDPSettings.Server)
	}
	active, err := clientConfiguration.ActiveSettings()
	if err != nil {
		t.Fatal(err)
	}
	if active.IPv4 != netip.MustParseAddr("10.0.1.2") {
		t.Fatalf("client tunnel address = %s, want 10.0.1.2", active.IPv4)
	}

	updated, err := file.Load()
	if err != nil {
		t.Fatal(err)
	}
	if updated.ClientCounter != 1 || len(updated.AllowedPeers) != 1 || updated.AllowedPeers[0].ClientID != 1 {
		t.Fatalf("server registration not persisted: %+v", updated)
	}
}

func TestFileGenerateClientDoesNotRegisterPeerWhenClientFileWriteFails(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "server_configuration.json")
	serverConfiguration := newConfiguration()
	serverConfiguration.Host = "192.0.2.1"
	serverConfiguration.X25519PublicKey, serverConfiguration.X25519PrivateKey = testX25519KeyPair(t, 1)
	writeServerConfiguration(t, path, *serverConfiguration)
	if err := os.Mkdir(filepath.Join(directory, "client_configuration.json.1"), 0700); err != nil {
		t.Fatal(err)
	}

	file := NewFile(path)
	if _, err := file.GenerateClient(); err == nil {
		t.Fatal("GenerateClient() succeeded when the client file could not be written")
	}
	updated, err := file.Load()
	if err != nil {
		t.Fatal(err)
	}
	if updated.ClientCounter != 0 || len(updated.AllowedPeers) != 0 {
		t.Fatalf("failed generation persisted registration: %+v", updated)
	}
}

func TestFileGenerateClientEnablesIPv6Subnets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	serverConfiguration := newConfiguration()
	serverConfiguration.Host = "2001:db8::1"
	serverConfiguration.X25519PublicKey, serverConfiguration.X25519PrivateKey = testX25519KeyPair(t, 1)
	writeServerConfiguration(t, path, *serverConfiguration)

	if _, err := NewFile(path).GenerateClient(); err != nil {
		t.Fatal(err)
	}
	updated, err := NewFile(path).Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, profile := range updated.Profiles() {
		if !profile.Settings.IPv6Subnet.IsValid() {
			t.Fatalf("%s IPv6 subnet was not enabled", profile.Settings.Protocol)
		}
	}
}

func TestFileGenerateClientUsesEnabledProtocol(t *testing.T) {
	tests := []struct {
		name      string
		enableTCP bool
		enableWS  bool
		want      settings.Protocol
	}{
		{name: "TCP", enableTCP: true, want: settings.TCP},
		{name: "WS", enableWS: true, want: settings.WS},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "server_configuration.json")
			configuration := newConfiguration()
			configuration.Host = "192.0.2.1"
			configuration.EnableUDP = false
			configuration.EnableTCP = test.enableTCP
			configuration.EnableWS = test.enableWS
			configuration.X25519PublicKey, configuration.X25519PrivateKey = testX25519KeyPair(t, 1)
			writeServerConfiguration(t, path, *configuration)

			generated, err := NewFile(path).GenerateClient()
			if err != nil {
				t.Fatal(err)
			}
			var clientConfiguration clientconfig.Configuration
			if err := json.Unmarshal([]byte(generated.JSON), &clientConfiguration); err != nil {
				t.Fatal(err)
			}
			if clientConfiguration.Protocol != test.want {
				t.Fatalf("Protocol = %s, want %s", clientConfiguration.Protocol, test.want)
			}
		})
	}
}

func TestFileGenerateClientRejectsMismatchedServerKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	configuration := newConfiguration()
	configuration.Host = "192.0.2.1"
	configuration.X25519PublicKey, _ = testX25519KeyPair(t, 1)
	_, configuration.X25519PrivateKey = testX25519KeyPair(t, 2)
	writeServerConfiguration(t, path, *configuration)

	if _, err := NewFile(path).GenerateClient(); err == nil {
		t.Fatal("GenerateClient() accepted mismatched server keys")
	}
}

func TestResolveServerHostConfigured(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		want       settings.Host
	}{
		{name: "domain", configured: " vpn.example ", want: settings.Host{Domain: "vpn.example"}},
		{name: "IPv4", configured: "192.0.2.1", want: settings.Host{IPv4: "192.0.2.1"}},
		{name: "mapped IPv4", configured: "::ffff:192.0.2.1", want: settings.Host{IPv4: "192.0.2.1"}},
		{name: "bracketed IPv6", configured: "[2001:db8::1]", want: settings.Host{IPv6: "2001:db8::1"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := resolveServerHost(test.configured)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("resolveServerHost(%q) = %+v, want %+v", test.configured, got, test.want)
			}
		})
	}
}

func TestDeriveClientSettings(t *testing.T) {
	serverSettings := settings.Settings{
		Network: settings.Network{
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			Port:       8080,
		},
		MTU:           1500,
		Encryption:    settings.ChaCha20Poly1305,
		DialTimeoutMs: 2500,
	}
	clientSettings, err := deriveClientSettings(serverSettings, settings.Host{Domain: "vpn.example"}, settings.UDP)
	if err != nil {
		t.Fatal(err)
	}
	if clientSettings.TunName != clientUDPTunName || clientSettings.MTU != settings.DefaultMTU || clientSettings.Protocol != settings.UDP {
		t.Fatalf("unexpected settings: %+v", clientSettings)
	}
	if _, err := deriveClientSettings(serverSettings, settings.Host{}, settings.UNKNOWN); err == nil {
		t.Fatal("unsupported protocol was accepted")
	}
}
