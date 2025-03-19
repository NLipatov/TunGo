package server_json_file_configuration

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"tungo/settings/server"
)

// testResolver implements the same resolve() method for testing purposes.
type testResolver struct {
	path string
}

func (r testResolver) resolve() (string, error) {
	return r.path, nil
}

// createTestConfigPath returns a temporary file path for configuration.
func createTestConfigPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "conf.json")
}

func TestConfigurationCreatesDefault(t *testing.T) {
	path := createTestConfigPath(t)
	manager := NewServerConfigurationManager()
	manager.resolver = testResolver{path: path}

	// Ensure the file does not exist.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file to not exist, but it does")
	}

	conf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() returned error: %v", err)
	}

	defaultConf := server.NewDefaultConfiguration()

	expectedJSON, err := json.MarshalIndent(defaultConf, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal default configuration: %v", err)
	}

	actualJSON, err := json.MarshalIndent(conf, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal actual configuration: %v", err)
	}

	if strings.TrimSpace(string(expectedJSON)) != strings.TrimSpace(string(actualJSON)) {
		t.Errorf("expected configuration JSON:\n%s\n\ngot:\n%s", expectedJSON, actualJSON)
	}

	// File should now exist.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist after default creation, but got error: %v", err)
	}
}

func TestIncrementClientCounter(t *testing.T) {
	path := createTestConfigPath(t)
	manager := NewServerConfigurationManager()
	manager.resolver = testResolver{path: path}

	// Create initial default configuration.
	initialConf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() returned error: %v", err)
	}
	initialCounter := initialConf.ClientCounter

	// Increment counter.
	if err := manager.IncrementClientCounter(); err != nil {
		t.Fatalf("IncrementClientCounter() returned error: %v", err)
	}

	// Read configuration again.
	updatedConf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() returned error after increment: %v", err)
	}

	if updatedConf.ClientCounter != initialCounter+1 {
		t.Errorf("expected ClientCounter to be %d, got %d", initialCounter+1, updatedConf.ClientCounter)
	}
}

func TestInjectEdKeys(t *testing.T) {
	path := createTestConfigPath(t)
	manager := NewServerConfigurationManager()
	manager.resolver = testResolver{path: path}

	// Generate ed25519 keys.
	public, private, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate keys: %v", err)
	}

	if err := manager.InjectEdKeys(public, private); err != nil {
		t.Fatalf("InjectEdKeys() returned error: %v", err)
	}

	conf, err := manager.Configuration()
	if err != nil {
		t.Fatalf("Configuration() returned error: %v", err)
	}

	if !reflect.DeepEqual(conf.Ed25519PublicKey, public) {
		t.Errorf("expected public key %v, got %v", public, conf.Ed25519PublicKey)
	}
	if !reflect.DeepEqual(conf.Ed25519PrivateKey, private) {
		t.Errorf("expected private key %v, got %v", private, conf.Ed25519PrivateKey)
	}
}
