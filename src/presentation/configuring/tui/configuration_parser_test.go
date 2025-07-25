package tui

import (
	"encoding/json"
	"testing"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
)

// makeTestConfig returns a minimal Configuration for tests.
func makeTestConfig() client.Configuration {
	return client.Configuration{
		TCPSettings: settings.Settings{
			ConnectionIP: "127.0.0.1",
			Port:         "8080",
		},
		UDPSettings:      settings.Settings{},
		Ed25519PublicKey: nil,
		Protocol:         settings.TCP,
	}
}

func TestFromJson_Simple(t *testing.T) {
	parser := NewConfigurationParser()
	want := makeTestConfig()
	raw, _ := json.Marshal(want)

	cfg, err := parser.FromJson(string(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TCPSettings.ConnectionIP != want.TCPSettings.ConnectionIP || cfg.TCPSettings.Port != want.TCPSettings.Port {
		t.Errorf("got %+v, want %+v", cfg, want)
	}
	if cfg.Protocol != want.Protocol {
		t.Errorf("got Protocol=%v, want %v", cfg.Protocol, want.Protocol)
	}
}

func TestFromJson_WithBOMAndZeroWidthAndControl(t *testing.T) {
	parser := NewConfigurationParser()
	want := makeTestConfig()
	raw, _ := json.Marshal(want)
	// Surround with BOM, ZWSP, null, bell, vertical tab
	dirty := "\uFEFF\u200B\x00\x07  " + string(raw) + "  \x0B\u200B\uFEFF"

	cfg, err := parser.FromJson(dirty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TCPSettings.Port != want.TCPSettings.Port {
		t.Errorf("got Port=%q, want %q", cfg.TCPSettings.Port, want.TCPSettings.Port)
	}
}

func TestFromJson_PrettyPrint_CRLF(t *testing.T) {
	parser := NewConfigurationParser()
	pretty := "{\r\n" +
		"  \"TCPSettings\": {\r\n" +
		"    \"ConnectionIP\": \"127.0.0.1\",\r\n" +
		"    \"Port\": \"8080\"\r\n" +
		"  },\r\n" +
		"  \"UDPSettings\": {},\r\n" +
		"  \"Protocol\": \"TCP\"\r\n" +
		"}"

	cfg, err := parser.FromJson(pretty)
	if err != nil {
		t.Fatalf("failed to parse CRLF JSON: %v", err)
	}
	if cfg.Protocol != settings.TCP {
		t.Errorf("got Protocol=%v, want %v", cfg.Protocol, settings.TCP)
	}
}

func TestFromJson_NonBreakingSpaceTrim(t *testing.T) {
	parser := NewConfigurationParser()
	want := makeTestConfig()
	raw, _ := json.Marshal(want)
	// Surround with non-breaking spaces
	dirty := "\u00A0\u00A0" + string(raw) + "\u00A0"

	cfg, err := parser.FromJson(dirty)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.TCPSettings.ConnectionIP != want.TCPSettings.ConnectionIP {
		t.Errorf("got ConnectionIP=%q, want %q", cfg.TCPSettings.ConnectionIP, want.TCPSettings.ConnectionIP)
	}
}

func TestFromJson_Invalid(t *testing.T) {
	parser := NewConfigurationParser()
	_, err := parser.FromJson("not a valid { json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
