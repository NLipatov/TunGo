package client_configuration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tungo/settings"
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

func TestReaderReadSuccess(t *testing.T) {
	expectedConfig := Configuration{
		TCPSettings:      settings.ConnectionSettings{},
		UDPSettings:      settings.ConnectionSettings{},
		Ed25519PublicKey: nil,
		Protocol:         settings.UDP,
	}
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
