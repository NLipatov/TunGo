package client

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tungo/infrastructure/settings"
)

func createTempClientConfigFile(t *testing.T, data interface{}) string {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "client_configuration.json")
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	return filePath
}

func validTestConfig() Configuration {
	host, _ := settings.NewHost("127.0.0.1")
	return Configuration{
		ClientID: 1,
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				Server:     host,
				Port:       9090,
				IPv4Subnet: netip.MustParsePrefix("10.0.1.0/24"),
			},
			Protocol: settings.UDP,
		},
		X25519PublicKey:  make([]byte, 32),
		ClientPublicKey:  make([]byte, 32),
		ClientPrivateKey: make([]byte, 32),
		Protocol:         settings.UDP,
	}
}

func TestReaderReadSuccess(t *testing.T) {
	expectedConfig := validTestConfig()
	path := createTempClientConfigFile(t, expectedConfig)
	r := newReader(path)
	config, err := r.read()
	if err != nil {
		t.Fatalf("read() returned error: %v", err)
	}
	expectedBytes, _ := json.MarshalIndent(expectedConfig, "", "  ")
	actualBytes, _ := json.MarshalIndent(config, "", "  ")
	if strings.TrimSpace(string(expectedBytes)) != strings.TrimSpace(string(actualBytes)) {
		t.Errorf("expected %s, got %s", expectedBytes, actualBytes)
	}
}

func TestReaderReadFileError(t *testing.T) {
	r := newReader("/non/existent/file.json")
	_, err := r.read()
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
}

func TestReaderReadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client_configuration.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	r := newReader(path)
	_, err := r.read()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
