package server_json_file_configuration

import (
	"encoding/json"
	"os"
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

func createTempConfigFile(t *testing.T, data interface{}) string {
	t.Helper()
	dir := t.TempDir()
	filePath := dir + "/conf.json"
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
	filePath := dir + "/conf.json"
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
