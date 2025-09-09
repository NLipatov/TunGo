package client

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
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

func (c *creatorTestConfigProvider) mockedConfig() Configuration {
	publicKey, _, keyGenerationErr := ed25519.GenerateKey(rand.Reader)
	if keyGenerationErr != nil {
		panic("failed to generate ed25519 key pair: " + keyGenerationErr.Error())
	}

	return Configuration{
		TCPSettings: settings.Settings{
			InterfaceName:    "tcptun0",
			InterfaceIPCIDR:  "10.0.0.0/24",
			InterfaceAddress: "10.0.0.10",
			ConnectionIP:     "192.168.122.194",
			Port:             "8080",
			MTU:              settings.DefaultEthernetMTU,
			Protocol:         settings.TCP,
			Encryption:       settings.ChaCha20Poly1305,
			DialTimeoutMs:    5000,
		},
		UDPSettings: settings.Settings{
			InterfaceName:    "udptun0",
			InterfaceIPCIDR:  "10.0.1.0/24",
			InterfaceAddress: "10.0.1.10",
			ConnectionIP:     "192.168.122.194",
			Port:             "9090",
			MTU:              settings.DefaultEthernetMTU,
			Protocol:         settings.UDP,
			Encryption:       settings.ChaCha20Poly1305,
			DialTimeoutMs:    5000,
		},
		Ed25519PublicKey: publicKey,
		Protocol:         settings.UDP,
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
	if !strings.Contains(body, `"InterfaceName": "tcptun0"`) {
		t.Errorf("Expected InterfaceName tcptun0, got: %s", body)
	}
	if !strings.Contains(body, `"UDPSettings"`) {
		t.Errorf("Expected UDPSettings section, got: %s", body)
	}
	if !strings.Contains(body, `"InterfaceName": "udptun0"`) {
		t.Errorf("Expected InterfaceName udptun0, got: %s", body)
	}
}

func TestDefaultCreator_Create_ResolveError(t *testing.T) {
	resolver := &creatorTestResolver{path: "", err: errors.New("no path")}
	creator := NewDefaultCreator(resolver)
	err := creator.Create((&creatorTestConfigProvider{}).mockedConfig(), "x")
	if err == nil || err.Error() != "no path" {
		t.Errorf("Expected resolve error “no path”, got %v", err)
	}
}

func TestDefaultCreator_Create_WriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("skipping write-error test as root can write anywhere")
	}

	tmp := t.TempDir()

	// Create a real directory but remove write permissions
	roDir := filepath.Join(tmp, "readonly")
	if err := os.MkdirAll(roDir, 0o500); err != nil {
		t.Fatalf("setup MkdirAll failed: %v", err)
	}

	// Resolver returns a path inside the read-only directory
	resolver := &creatorTestResolver{
		path: filepath.Join(roDir, "cfg"),
		err:  nil,
	}
	creator := NewDefaultCreator(resolver)

	// Attempting to write should fail due to insufficient permissions
	err := creator.Create((&creatorTestConfigProvider{}).mockedConfig(), "y")
	if err == nil {
		t.Error("expected write error due to read-only directory, got nil")
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
		t.Fatal("Expected MkdirAll error, got nil")
	}
}
