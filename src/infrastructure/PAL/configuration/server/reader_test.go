package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"tungo/infrastructure/PAL/stat"
	"tungo/infrastructure/settings"
)

// createTempConfigFile writes JSON-serialized config to a temp file and returns its path.
func createTempConfigFile(t *testing.T, data any) string {
	t.Helper()
	dir := t.TempDir()
	filePath := filepath.Join(dir, "conf.json")
	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		t.Fatalf("failed to write temp config file: %v", err)
	}
	return filePath
}

func TestRead_Success_WithEnvOverrides(t *testing.T) {
	// Arrange: a minimal but valid config; defaults will fill missing fields.
	initial := Configuration{
		FallbackServerAddress: "192.168.1.1",
		EnableUDP:             false, // will be overridden by env
		EnableTCP:             true,  // will be overridden by env
	}
	path := createTempConfigFile(t, initial)

	// Use t.Setenv so values are restored after the test automatically.
	t.Setenv("ServerIP", "10.0.0.1")
	t.Setenv("EnableUDP", "true")
	t.Setenv("EnableTCP", "false")
	// Leave WS unset to test "no env" path.

	// Act
	r := newDefaultReader(path, stat.NewDefaultStat())
	conf, err := r.read()
	if err != nil {
		t.Fatalf("read() returned error: %v", err)
	}

	// Assert
	if conf.FallbackServerAddress != "10.0.0.1" {
		t.Errorf("expected ServerIP override to be applied, got %q", conf.FallbackServerAddress)
	}
	if conf.EnableUDP != true {
		t.Errorf("expected EnableUDP true, got %v", conf.EnableUDP)
	}
	if conf.EnableTCP != false {
		t.Errorf("expected EnableTCP false, got %v", conf.EnableTCP)
	}
	// WS was not set via env; default from file (zero) stays false
	if conf.EnableWS != false {
		t.Errorf("expected EnableWS false by default, got %v", conf.EnableWS)
	}
}

func TestRead_FileDoesNotExist(t *testing.T) {
	r := newDefaultReader("/non/existent/conf.json", stat.NewDefaultStat())
	_, err := r.read()
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' in error, got %v", err)
	}
}

func TestRead_StatOtherError(t *testing.T) {
	// Using a path with a null byte typically causes a non-ErrNotExist error from Stat.
	invalidPath := string([]byte{0})
	r := newDefaultReader(invalidPath, stat.NewDefaultStat())
	_, err := r.read()
	if err == nil {
		t.Fatal("expected error for invalid file path, got nil")
	}
	if !strings.Contains(err.Error(), "configuration file not found") {
		t.Errorf("expected 'configuration file not found' in error, got %v", err)
	}
}

func TestRead_FileUnreadable(t *testing.T) {
	// Create a directory instead of a file to trigger os.ReadFile error.
	dir := t.TempDir()
	unreadablePath := filepath.Join(dir, "conf.json")
	if err := os.Mkdir(unreadablePath, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	r := newDefaultReader(unreadablePath, stat.NewDefaultStat())
	_, err := r.read()
	if err == nil {
		t.Fatal("expected error for unreadable file, got nil")
	}
	if !strings.Contains(err.Error(), "is unreadable") {
		t.Errorf("expected 'is unreadable' in error, got %v", err)
	}
}

func TestRead_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conf.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0644); err != nil {
		t.Fatalf("failed to write invalid JSON file: %v", err)
	}

	r := newDefaultReader(path, stat.NewDefaultStat())
	_, err := r.read()
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected 'invalid' in error, got %v", err)
	}
}

func TestRead_EnsureDefaultsAndValidate_PassWithEmptyConfig(t *testing.T) {
	// Arrange: empty JSON object — EnsureDefaults must populate settings,
	// Validate should pass because all protocols are disabled by default.
	path := createTempConfigFile(t, struct{}{})
	r := newDefaultReader(path, stat.NewDefaultStat())

	// Act
	conf, err := r.read()
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	// Assert defaults applied
	if conf.TCPSettings.InterfaceName == "" || conf.UDPSettings.InterfaceName == "" || conf.WSSettings.InterfaceName == "" {
		t.Fatalf("EnsureDefaults did not populate interface names: TCP=%q UDP=%q WS=%q",
			conf.TCPSettings.InterfaceName, conf.UDPSettings.InterfaceName, conf.WSSettings.InterfaceName)
	}
}

func TestRead_ValidateFails_ReportsPathAndReason(t *testing.T) {
	// Arrange: make UDP port equal TCP port to trigger duplicate-port validation error.
	// We also enable both TCP and UDP to ensure they are validated.
	initial := Configuration{
		EnableTCP: true,
		EnableUDP: true,
		UDPSettings: settings.Settings{
			Port: "8080", // same as TCP default port after EnsureDefaults
		},
	}
	path := createTempConfigFile(t, initial)
	r := newDefaultReader(path, stat.NewDefaultStat())

	// Act
	_, err := r.read()
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}

	// Assert the error mentions both the path and validation reason (duplicate port).
	if !strings.Contains(err.Error(), path) {
		t.Errorf("expected error to contain file path %q, got: %v", path, err)
	}
	if !strings.Contains(err.Error(), "duplicate port") {
		t.Errorf("expected error to contain validation reason 'duplicate port', got: %v", err)
	}
}

func TestRead_EnvEnabledProtocols_InvalidValuesAreIgnored(t *testing.T) {
	// Arrange: file sets EnableUDP=true; env tries to set invalid bool strings => ignored.
	initial := Configuration{
		EnableUDP: true,
	}
	path := createTempConfigFile(t, initial)
	t.Setenv("EnableUDP", "maybe") // invalid -> should be ignored
	t.Setenv("EnableTCP", "yea")   // invalid -> should be ignored
	t.Setenv("EnableWS", "nope")   // invalid -> should be ignored

	r := newDefaultReader(path, stat.NewDefaultStat())
	conf, err := r.read()
	if err != nil {
		t.Fatalf("read() returned error: %v", err)
	}

	// Assert that invalid envs did not clobber the file values.
	if conf.EnableUDP != true {
		t.Errorf("expected EnableUDP to remain true, got %v", conf.EnableUDP)
	}
	if conf.EnableTCP != false {
		t.Errorf("expected EnableTCP to remain false, got %v", conf.EnableTCP)
	}
	if conf.EnableWS != false {
		t.Errorf("expected EnableWS to remain false, got %v", conf.EnableWS)
	}
}

func TestRead_ServerIP_NotSet_DoesNotOverride(t *testing.T) {
	initial := Configuration{
		FallbackServerAddress: "1.2.3.4",
	}
	path := createTempConfigFile(t, initial)
	// Do not set ServerIP env var.

	r := newDefaultReader(path, stat.NewDefaultStat())
	conf, err := r.read()
	if err != nil {
		t.Fatalf("read() returned error: %v", err)
	}
	if conf.FallbackServerAddress != "1.2.3.4" {
		t.Errorf("expected FallbackServerAddress to be unchanged, got %q", conf.FallbackServerAddress)
	}
}
