package client

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tungo/infrastructure/settings"
)

type managerTestMockResolver struct {
	path string
	err  error
}

func (r managerTestMockResolver) Resolve() (string, error) {
	if r.err != nil {
		return "", r.err
	}
	return r.path, nil
}

func createTempConfigFile(t *testing.T, data interface{}) string {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "client_configuration.json")
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal data: %v", err)
	}
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	return filePath
}

func TestManagerConfigurationResolverError(t *testing.T) {
	manager := NewManager()
	manager.(*Manager).resolver = managerTestMockResolver{err: errors.New("resolver error")}
	_, err := manager.Configuration()
	if err == nil {
		t.Fatal("expected resolver error, got nil")
	}
	if !strings.Contains(err.Error(), "resolver error") {
		t.Errorf("expected error to contain 'resolver error', got %v", err)
	}
}

func TestManagerConfigurationFileNotExist(t *testing.T) {
	manager := NewManager()
	manager.(*Manager).resolver = managerTestMockResolver{path: "/non/existent/path/client_configuration.json"}
	_, err := manager.Configuration()
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected error to mention 'does not exist', got %v", err)
	}
}

func TestManagerConfigurationInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client_configuration.json")
	// Write invalid JSON.
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	manager := NewManager()
	manager.(*Manager).resolver = managerTestMockResolver{path: path}
	_, err := manager.Configuration()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestManagerConfigurationSuccess(t *testing.T) {
	// Create a valid configuration file.
	defaultConfig := Configuration{
		TCPSettings:      settings.Settings{},
		UDPSettings:      settings.Settings{},
		Ed25519PublicKey: nil,
		Protocol:         settings.TCP,
	}
	path := createTempConfigFile(t, defaultConfig)
	manager := NewManager()
	manager.(*Manager).resolver = managerTestMockResolver{path: path}
	config, err := manager.Configuration()
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if config.Protocol != settings.TCP {
		t.Errorf("expected Protocol %q, got %d", settings.TCP, config.Protocol)
	}
}
