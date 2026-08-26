package server

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"

	clientconfig "tungo/internal/config/client"
	"tungo/internal/config/settings"
)

func TestFileGenerateClient(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	serverConfiguration := newConfiguration()
	serverConfiguration.Host = "192.0.2.1"
	serverConfiguration.X25519PublicKey = make([]byte, 32)
	serverConfiguration.X25519PrivateKey = make([]byte, 32)
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

func TestFileGenerateClientSerializesConcurrentRegistrations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	serverConfiguration := newConfiguration()
	serverConfiguration.Host = "vpn.example"
	serverConfiguration.X25519PublicKey = make([]byte, 32)
	serverConfiguration.X25519PrivateKey = make([]byte, 32)
	writeServerConfiguration(t, path, *serverConfiguration)
	file := NewFile(path)

	const count = 8
	ids := make(chan int, count)
	errs := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			generated, err := file.GenerateClient()
			if err != nil {
				errs <- err
				return
			}
			var configuration clientconfig.Configuration
			if err := json.Unmarshal([]byte(generated.JSON), &configuration); err != nil {
				errs <- err
				return
			}
			if configuration.UDPSettings.Server.Domain != "vpn.example" {
				errs <- fmt.Errorf("server host = %+v", configuration.UDPSettings.Server)
				return
			}
			ids <- configuration.ClientID
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	close(ids)
	generatedIDs := make([]int, 0, count)
	for id := range ids {
		generatedIDs = append(generatedIDs, id)
	}
	sort.Ints(generatedIDs)
	for i, id := range generatedIDs {
		if id != i+1 {
			t.Fatalf("generated client IDs = %v", generatedIDs)
		}
	}
	updated, err := file.Load()
	if err != nil {
		t.Fatal(err)
	}
	if updated.ClientCounter != count || len(updated.AllowedPeers) != count {
		t.Fatalf("server registration state = counter %d, peers %d", updated.ClientCounter, len(updated.AllowedPeers))
	}
}

func TestFileGenerateClientEnablesIPv6Subnets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server_configuration.json")
	serverConfiguration := newConfiguration()
	serverConfiguration.Host = "2001:db8::1"
	serverConfiguration.X25519PublicKey = make([]byte, 32)
	serverConfiguration.X25519PrivateKey = make([]byte, 32)
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
