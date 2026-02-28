package client

import (
	"net/netip"
	"reflect"
	"strings"
	"testing"
	"tungo/infrastructure/settings"
)

func TestConfiguration_ActiveSettings(t *testing.T) {
	tcp := settings.Settings{MTU: 1400}
	udp := settings.Settings{MTU: 1300}
	ws := settings.Settings{MTU: 1200}

	tests := []struct {
		name      string
		cfg       Configuration
		want      settings.Settings
		wantError bool
	}{
		{
			name: "UDP",
			cfg: Configuration{
				UDPSettings: udp,
				Protocol:    settings.UDP,
			},
			want: udp,
		},
		{
			name: "TCP",
			cfg: Configuration{
				TCPSettings: tcp,
				Protocol:    settings.TCP,
			},
			want: tcp,
		},
		{
			name: "WS",
			cfg: Configuration{
				WSSettings: ws,
				Protocol:   settings.WS,
			},
			want: ws,
		},
		{
			name: "Unsupported protocol",
			cfg: Configuration{
				Protocol: settings.Protocol(255),
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.cfg.ActiveSettings()

			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error for protocol %v, got nil", tt.cfg.Protocol)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("unexpected result: got %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestConfiguration_ResolveActive_SkipsInactiveBrokenSettings(t *testing.T) {
	cfg := Configuration{
		ClientID: 2,
		Protocol: settings.UDP,
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.1.0/24"),
			},
			Protocol: settings.UDP,
		},
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				// Deliberately tiny subnet to trigger client allocation error if resolved.
				IPv4Subnet: netip.MustParsePrefix("10.0.2.1/32"),
			},
			Protocol: settings.TCP,
		},
	}

	if err := cfg.ResolveActive(); err != nil {
		t.Fatalf("ResolveActive should ignore inactive broken settings, got %v", err)
	}
	if !cfg.UDPSettings.IPv4.IsValid() {
		t.Fatal("expected active UDP IPv4 to be derived")
	}
}

func TestConfiguration_Resolve_DerivesIPForAllSettings(t *testing.T) {
	cfg := Configuration{
		ClientID: 3,
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			},
		},
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.1.0/24"),
			},
		},
		WSSettings: settings.Settings{
			Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.2.0/24"),
			},
		},
	}

	if err := cfg.Resolve(); err != nil {
		t.Fatalf("Resolve returned unexpected error: %v", err)
	}

	if !cfg.TCPSettings.IPv4.IsValid() {
		t.Error("expected TCPSettings IPv4 to be derived")
	}
	if !cfg.UDPSettings.IPv4.IsValid() {
		t.Error("expected UDPSettings IPv4 to be derived")
	}
	if !cfg.WSSettings.IPv4.IsValid() {
		t.Error("expected WSSettings IPv4 to be derived")
	}
}

func TestConfiguration_Resolve_PropagatesError(t *testing.T) {
	cfg := Configuration{
		ClientID: 999,
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				// /32 subnet cannot allocate a client address.
				IPv4Subnet: netip.MustParsePrefix("10.0.0.1/32"),
			},
		},
	}

	err := cfg.Resolve()
	if err == nil {
		t.Fatal("expected Resolve to return error for tiny subnet, got nil")
	}
}

func TestConfiguration_Resolve_DualStack(t *testing.T) {
	cfg := Configuration{
		ClientID: 2,
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			},
		},
		UDPSettings: settings.Settings{
			Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.1.0/24"),
			},
		},
		WSSettings: settings.Settings{
			Addressing: settings.Addressing{
				IPv6Subnet: netip.MustParsePrefix("fd01::/64"),
			},
		},
	}

	if err := cfg.Resolve(); err != nil {
		t.Fatalf("Resolve returned unexpected error: %v", err)
	}

	if !cfg.TCPSettings.IPv4.IsValid() {
		t.Error("expected TCPSettings IPv4 to be derived")
	}
	if !cfg.TCPSettings.IPv6.IsValid() {
		t.Error("expected TCPSettings IPv6 to be derived")
	}
	if !cfg.UDPSettings.IPv4.IsValid() {
		t.Error("expected UDPSettings IPv4 to be derived")
	}
	if !cfg.WSSettings.IPv6.IsValid() {
		t.Error("expected WSSettings IPv6 to be derived")
	}
}

func TestConfiguration_ResolveActive_UnsupportedProtocol(t *testing.T) {
	cfg := Configuration{
		ClientID: 1,
		Protocol: settings.UNKNOWN,
	}

	err := cfg.ResolveActive()
	if err == nil {
		t.Fatal("expected error for unsupported protocol, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported protocol") {
		t.Fatalf("expected error about unsupported protocol, got: %v", err)
	}
}

func TestConfiguration_ResolveActive_DeriveError(t *testing.T) {
	cfg := Configuration{
		ClientID: 999,
		Protocol: settings.TCP,
		TCPSettings: settings.Settings{
			Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.0.1/32"),
			},
		},
	}

	err := cfg.ResolveActive()
	if err == nil {
		t.Fatal("expected DeriveIP error, got nil")
	}
}
