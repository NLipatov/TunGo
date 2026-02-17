package tui

import (
	"encoding/json"
	"net/netip"
	"strings"
	"testing"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
)

func mustHostParser(raw string) settings.Host {
	h, err := settings.NewHost(raw)
	if err != nil {
		panic(err)
	}
	return h
}

// makeTestConfig returns a valid Configuration for tests.
func makeTestConfig() client.Configuration {
	return client.Configuration{
		ClientID: 1,
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				TunName:    "tun0",
				Server:     mustHostParser("127.0.0.1"),
				Port:       8080,
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			},
			Protocol: settings.TCP,
		},
		X25519PublicKey:  make([]byte, 32),
		ClientPublicKey:  make([]byte, 32),
		ClientPrivateKey: make([]byte, 32),
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
	if cfg.TCPSettings.Server != want.TCPSettings.Server || cfg.TCPSettings.Port != want.TCPSettings.Port {
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
		t.Errorf("got Port=%d, want %d", cfg.TCPSettings.Port, want.TCPSettings.Port)
	}
}

func TestFromJson_PrettyPrint_CRLF(t *testing.T) {
	parser := NewConfigurationParser()
	want := makeTestConfig()
	raw, _ := json.MarshalIndent(want, "", "  ")
	// Replace LF with CRLF to simulate Windows-style line endings.
	pretty := strings.ReplaceAll(string(raw), "\n", "\r\n")

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
	if cfg.TCPSettings.Server != want.TCPSettings.Server {
		t.Errorf("got Server=%q, want %q", cfg.TCPSettings.Server, want.TCPSettings.Server)
	}
}

func TestFromJson_Invalid(t *testing.T) {
	parser := NewConfigurationParser()
	_, err := parser.FromJson("not a valid { json")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
