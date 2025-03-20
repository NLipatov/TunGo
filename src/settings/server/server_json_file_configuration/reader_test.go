package server_json_file_configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tungo/settings/server"
)

func TestReadSuccess(t *testing.T) {
	// Create a valid configuration file.
	initialConfig := server.Configuration{
		FallbackServerAddress:  "192.168.1.1",
		EnableUDP:              false,
		EnableTCP:              true,
		UDPNonceRingBufferSize: 100,
	}
	filePath := createTempConfigFile(t, initialConfig)

	// Override environment variables.
	_ = os.Setenv("ServerIP", "10.0.0.1")
	_ = os.Setenv("EnableUDP", "true")
	_ = os.Setenv("EnableTCP", "false")
	_ = os.Setenv("UDPNonceRingBufferSize", "200")
	defer resetEnv("ServerIP", "EnableUDP", "EnableTCP", "UDPNonceRingBufferSize")

	r := newReader(filePath)
	conf, err := r.read()
	if err != nil {
		t.Fatalf("read() returned error: %v", err)
	}

	if conf.FallbackServerAddress != "10.0.0.1" {
		t.Errorf("Expected FallbackServerAddress '10.0.0.1', got %s", conf.FallbackServerAddress)
	}
	if conf.EnableUDP != true {
		t.Errorf("Expected EnableUDP true, got %v", conf.EnableUDP)
	}
	if conf.EnableTCP != false {
		t.Errorf("Expected EnableTCP false, got %v", conf.EnableTCP)
	}
	if conf.UDPNonceRingBufferSize != 200 {
		t.Errorf("Expected UDPNonceRingBufferSize 200, got %d", conf.UDPNonceRingBufferSize)
	}
}

func TestReadFileDoesNotExist(t *testing.T) {
	nonExistentPath := "/non/existent/conf.json"
	r := newReader(nonExistentPath)
	_, err := r.read()
	if err == nil {
		t.Fatal("Expected error for non-existent file, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("Expected error to mention 'does not exist', got %v", err)
	}
}

func TestReadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "conf.json")
	invalidJSON := []byte("{invalid json")
	if err := os.WriteFile(filePath, invalidJSON, 0644); err != nil {
		t.Fatalf("Failed to write invalid JSON file: %v", err)
	}

	r := newReader(filePath)
	_, err := r.read()
	if err == nil {
		t.Fatal("Expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("Expected error to mention 'invalid', got %v", err)
	}
}

// Test for branch: "configuration file not found: %s"
// Uses an invalid path (null byte) which causes os.Stat to error with a non-ErrNotExist error.
func TestReadStatOtherError(t *testing.T) {
	invalidPath := string([]byte{0})
	r := newReader(invalidPath)
	_, err := r.read()
	if err == nil {
		t.Fatal("Expected error for invalid file path, got nil")
	}
	if !strings.Contains(err.Error(), "configuration file not found") {
		t.Errorf("Expected error to mention 'configuration file not found', got %v", err)
	}
}

// Test for branch: unreadable file (os.ReadFile error)
func TestReadFileUnreadable(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "conf.json")
	config := server.Configuration{
		FallbackServerAddress:  "192.168.1.1",
		EnableUDP:              false,
		EnableTCP:              true,
		UDPNonceRingBufferSize: 100,
	}
	content, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	if err := os.WriteFile(filePath, content, 0000); err != nil {
		t.Fatalf("Failed to write file: %v", err)
	}
	defer os.Chmod(filePath, 0644) // restore permissions

	r := newReader(filePath)
	_, err = r.read()
	if err == nil {
		t.Fatal("Expected error for unreadable file, got nil")
	}
	if !strings.Contains(err.Error(), "is unreadable") {
		t.Errorf("Expected error to mention 'is unreadable', got %v", err)
	}
}

func createTempConfigFile(t *testing.T, data interface{}) string {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "conf.json")
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal config: %v", err)
	}
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("Failed to write temp config file: %v", err)
	}
	return filePath
}

func resetEnv(keys ...string) {
	for _, key := range keys {
		_ = os.Unsetenv(key)
	}
}
