package client

import (
	"encoding/json"
	"errors"
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
				TunName:    "tun0",
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
	config, err := read(path)
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
	_, err := read("/non/existent/file.json")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected wrapped os.ErrNotExist, got %v", err)
	}
}

func TestReaderReadDirectoryError(t *testing.T) {
	_, err := read(t.TempDir())
	if err == nil {
		t.Fatal("expected directory read error")
	}
	if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected non-not-exist error, got %v", err)
	}
	if !strings.Contains(err.Error(), "failed to read client configuration") {
		t.Fatalf("expected read context, got %v", err)
	}
}

func TestReaderReadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client_configuration.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	_, err := read(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("expected wrapped json.SyntaxError, got %v", err)
	}
}

func TestReaderReadInvalidConfiguration(t *testing.T) {
	path := createTempClientConfigFile(t, Configuration{})

	_, err := read(path)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "invalid client configuration") {
		t.Fatalf("expected validation context, got %v", err)
	}
}
