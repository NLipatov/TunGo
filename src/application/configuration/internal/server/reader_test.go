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
	path := filepath.Join(dir, "conf.json")
	b, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

//
// ---------- Happy paths & defaults ----------
//

func TestRead_Happy_NoEnv_EnsuresDefaults(t *testing.T) {
	path := createTempConfigFile(t, struct{}{}) // empty JSON
	r := newDefaultReader(path, stat.NewDefaultStat())

	// Ensure no env noise
	_ = os.Unsetenv("ServerIP")
	_ = os.Unsetenv("EnableUDP")
	_ = os.Unsetenv("EnableTCP")
	_ = os.Unsetenv("EnableWS")

	conf, err := r.read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	// defaults must be applied
	if conf.TCPSettings.TunName == "" || conf.UDPSettings.TunName == "" || conf.WSSettings.TunName == "" {
		t.Fatalf("defaults not applied: TCP=%q UDP=%q WS=%q",
			conf.TCPSettings.TunName, conf.UDPSettings.TunName, conf.WSSettings.TunName)
	}
}

//
// ---------- Stat() error branches ----------
//

func TestRead_FileDoesNotExist(t *testing.T) {
	r := newDefaultReader("/definitely/missing/conf.json", stat.NewDefaultStat())
	_, err := r.read()
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected 'does not exist', got: %v", err)
	}
}

func TestRead_StatOtherError(t *testing.T) {
	// Path with NUL triggers a non-ENOENT error on most systems.
	bad := string([]byte{0})
	r := newDefaultReader(bad, stat.NewDefaultStat())
	_, err := r.read()
	if err == nil || !strings.Contains(err.Error(), "configuration file not found") {
		t.Fatalf("expected 'configuration file not found', got: %v", err)
	}
}

//
// ---------- ReadFile / Unmarshal error branches ----------
//

func TestRead_FileUnreadable_IsDirectory(t *testing.T) {
	dir := t.TempDir()
	asDir := filepath.Join(dir, "conf.json")
	if err := os.Mkdir(asDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	r := newDefaultReader(asDir, stat.NewDefaultStat())
	_, err := r.read()
	if err == nil || !strings.Contains(err.Error(), "is unreadable") {
		t.Fatalf("expected 'is unreadable', got: %v", err)
	}
}

func TestRead_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "conf.json")
	if err := os.WriteFile(path, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}
	r := newDefaultReader(path, stat.NewDefaultStat())
	_, err := r.read()
	if err == nil || !strings.Contains(err.Error(), "is invalid") {
		t.Fatalf("expected 'is invalid', got: %v", err)
	}
}

//
// ---------- Validate() error propagation ----------
//

func TestRead_ValidateFails_DuplicatePort_IncludesPathAndReason(t *testing.T) {
	// Enable TCP+UDP and force duplicate port (TCP default 8080; set UDP to 8080).
	initial := Configuration{
		EnableTCP: true,
		EnableUDP: true,
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{Port: 8080},
		},
	}
	path := createTempConfigFile(t, initial)
	r := newDefaultReader(path, stat.NewDefaultStat())

	_, err := r.read()
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "duplicate port") {
		t.Fatalf("expected path and 'duplicate port' in error, got: %v", err)
	}
}

//
// ---------- Env overrides: unset, empty, invalid, valid(true/false) ----------
//

func TestRead_ServerIP_Override_SetAndEmpty(t *testing.T) {
	initial := Configuration{FallbackServerAddress: "1.2.3.4"}
	path := createTempConfigFile(t, initial)

	// Case 1: set
	t.Setenv("ServerIP", "10.0.0.1")
	conf, err := newDefaultReader(path, stat.NewDefaultStat()).read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if conf.FallbackServerAddress != "10.0.0.1" {
		t.Fatalf("ServerIP override failed: %q", conf.FallbackServerAddress)
	}

	// Case 2: empty string -> ignored (branch env == "")
	t.Setenv("ServerIP", "")
	conf, err = newDefaultReader(path, stat.NewDefaultStat()).read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if conf.FallbackServerAddress != "1.2.3.4" {
		t.Fatalf("empty env should not override, got %q", conf.FallbackServerAddress)
	}
}

func TestRead_EnableFlags_Valid_Invalid_Unset_AllBranches(t *testing.T) {
	// Start with all false in file to make flips visible.
	base := Configuration{EnableUDP: false, EnableTCP: false, EnableWS: false}
	path := createTempConfigFile(t, base)

	// invalid values -> ignored
	t.Setenv("EnableUDP", "meh")
	t.Setenv("EnableTCP", "nope")
	t.Setenv("EnableWS", "y")
	conf, err := newDefaultReader(path, stat.NewDefaultStat()).read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if conf.EnableUDP || conf.EnableTCP || conf.EnableWS {
		t.Fatalf("invalid envs must be ignored, got UDP=%v TCP=%v WS=%v", conf.EnableUDP, conf.EnableTCP, conf.EnableWS)
	}

	// valid true
	t.Setenv("EnableUDP", "true")
	t.Setenv("EnableTCP", "true")
	t.Setenv("EnableWS", "true")
	conf, err = newDefaultReader(path, stat.NewDefaultStat()).read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if !conf.EnableUDP || !conf.EnableTCP || !conf.EnableWS {
		t.Fatalf("expected all enabled true, got UDP=%v TCP=%v WS=%v", conf.EnableUDP, conf.EnableTCP, conf.EnableWS)
	}

	// valid false
	t.Setenv("EnableUDP", "false")
	t.Setenv("EnableTCP", "false")
	t.Setenv("EnableWS", "false")
	conf, err = newDefaultReader(path, stat.NewDefaultStat()).read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if conf.EnableUDP || conf.EnableTCP || conf.EnableWS {
		t.Fatalf("expected all disabled false, got UDP=%v TCP=%v WS=%v", conf.EnableUDP, conf.EnableTCP, conf.EnableWS)
	}

	// unset (env not present) -> branch env == "" not taken, file values remain
	_ = os.Unsetenv("EnableUDP")
	_ = os.Unsetenv("EnableTCP")
	_ = os.Unsetenv("EnableWS")
	conf, err = newDefaultReader(path, stat.NewDefaultStat()).read()
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	// file had all false
	if conf.EnableUDP || conf.EnableTCP || conf.EnableWS {
		t.Fatalf("unset env should keep file values, got UDP=%v TCP=%v WS=%v", conf.EnableUDP, conf.EnableTCP, conf.EnableWS)
	}
}
