package client

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tungo/infrastructure/settings"
)

type creatorTestResolver struct {
	path string
	err  error
}

func (f *creatorTestResolver) Resolve() (string, error) {
	return f.path, f.err
}

type creatorTestConfigProvider struct{}

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

func mustAddr(raw string) netip.Addr {
	return netip.MustParseAddr(raw)
}

func (c *creatorTestConfigProvider) mockedConfig() Configuration {
	publicKey, _, keyGenerationErr := ed25519.GenerateKey(rand.Reader)
	if keyGenerationErr != nil {
		panic("failed to generate ed25519 key pair: " + keyGenerationErr.Error())
	}

	return Configuration{
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				TunName:    "tcptun0",
				IPv4Subnet: mustPrefix("10.0.0.0/24"),
				IPv4:       mustAddr("10.0.0.10"),
				Server:     mustHost("192.168.122.194"),
				Port:       8080,
			},
			MTU:           settings.DefaultEthernetMTU,
			Protocol:      settings.TCP,
			Encryption:    settings.ChaCha20Poly1305,
			DialTimeoutMs: 5000,
		},
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				TunName:    "udptun0",
				IPv4Subnet: mustPrefix("10.0.1.0/24"),
				IPv4:       mustAddr("10.0.1.10"),
				Server:     mustHost("192.168.122.194"),
				Port:       9090,
			},
			MTU:           settings.DefaultEthernetMTU,
			Protocol:      settings.UDP,
			Encryption:    settings.ChaCha20Poly1305,
			DialTimeoutMs: 5000,
		},
		X25519PublicKey: publicKey,
		Protocol:        settings.UDP,
	}
}

func TestDefaultCreator_Create_Success(t *testing.T) {
	tmp := t.TempDir()
	resolver := &creatorTestResolver{path: filepath.Join(tmp, "config"), err: nil}
	creator := NewDefaultCreator(resolver)
	cfg := (&creatorTestConfigProvider{}).mockedConfig()

	if err := creator.Create(cfg, "test"); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, "config.test"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	body := string(data)

	if !strings.Contains(body, `"TCPSettings"`) {
		t.Errorf("Expected TCPSettings section, got: %s", body)
	}
	if !strings.Contains(body, `"TunName": "tcptun0"`) {
		t.Errorf("Expected TunName tcptun0, got: %s", body)
	}
	if !strings.Contains(body, `"UDPSettings"`) {
		t.Errorf("Expected UDPSettings section, got: %s", body)
	}
	if !strings.Contains(body, `"TunName": "udptun0"`) {
		t.Errorf("Expected TunName udptun0, got: %s", body)
	}
}

func TestDefaultCreator_Create_ResolveError(t *testing.T) {
	wantErr := errors.New("no path")
	resolver := &creatorTestResolver{path: "", err: wantErr}
	creator := NewDefaultCreator(resolver)
	err := creator.Create((&creatorTestConfigProvider{}).mockedConfig(), "x")
	if err == nil {
		t.Fatal("expected resolve error, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("expected wrapped resolve error %v, got %v", wantErr, err)
	}
	if !strings.Contains(err.Error(), "resolve default client configuration path") {
		t.Errorf("expected resolve context, got %v", err)
	}
}

func TestDefaultCreator_Create_WriteError(t *testing.T) {
	tmp := t.TempDir()
	resolver := &creatorTestResolver{path: filepath.Join(tmp, "config")}
	confPath := resolver.path + ".y"
	if err := os.Mkdir(confPath, 0700); err != nil {
		t.Fatalf("create directory at destination path: %v", err)
	}
	creator := NewDefaultCreator(resolver)

	err := creator.Create((&creatorTestConfigProvider{}).mockedConfig(), "y")
	if err == nil {
		t.Fatal("expected write error for directory destination")
	}
	if !strings.Contains(err.Error(), "write client configuration") {
		t.Errorf("expected write context, got %v", err)
	}
}

func TestDefaultCreator_Create_RejectsPathTraversal(t *testing.T) {
	tmp := t.TempDir()
	resolver := &creatorTestResolver{path: filepath.Join(tmp, "config"), err: nil}
	creator := NewDefaultCreator(resolver)
	cfg := (&creatorTestConfigProvider{}).mockedConfig()

	badNames := []string{
		"../etc/passwd",
		"foo/bar",
		`foo\bar`,
		".",
		"..",
		"name\x00evil",
	}
	for _, name := range badNames {
		if err := creator.Create(cfg, name); err == nil {
			t.Errorf("expected error for name %q, got nil", name)
		}
	}
}

func TestDefaultCreator_Create_MkdirAllError(t *testing.T) {
	tmp := t.TempDir()

	// Create a file where a directory should be
	parentFile := filepath.Join(tmp, "parent")
	if err := os.WriteFile(parentFile, []byte{}, 0600); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Resolver returns a path under the file, so MkdirAll should fail
	resolver := &creatorTestResolver{path: filepath.Join(parentFile, "conf"), err: nil}
	creator := NewDefaultCreator(resolver)
	cfg := (&creatorTestConfigProvider{}).mockedConfig()

	err := creator.Create(cfg, "test")
	if err == nil {
		t.Fatal("expected MkdirAll error, got nil")
	}
	if !strings.Contains(err.Error(), "create client configuration directory") {
		t.Errorf("expected directory creation context, got %v", err)
	}
}
