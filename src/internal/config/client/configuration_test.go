package client

import (
	"net/netip"
	"reflect"
	"testing"

	"tungo/internal/config/settings"
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
			want: settings.Settings{Addressing: udp.Addressing, MTU: udp.MTU, Protocol: settings.UDP},
		},
		{
			name: "TCP",
			cfg: Configuration{
				TCPSettings: tcp,
				Protocol:    settings.TCP,
			},
			want: settings.Settings{Addressing: tcp.Addressing, MTU: tcp.MTU, Protocol: settings.TCP},
		},
		{
			name: "WS",
			cfg: Configuration{
				WSSettings: ws,
				Protocol:   settings.WS,
			},
			want: settings.Settings{Addressing: ws.Addressing, MTU: ws.MTU, Protocol: settings.WS},
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

func TestConfiguration_ActiveSettingsDerivesLegacyClientAddress(t *testing.T) {
	cfg := Configuration{
		ClientID: 3,
		Protocol: settings.UDP,
		UDPSettings: settings.Settings{Addressing: settings.Addressing{
			IPv4Subnet: netip.MustParsePrefix("10.0.1.0/24"),
		}},
	}

	active, err := cfg.ActiveSettings()
	if err != nil {
		t.Fatalf("ActiveSettings: %v", err)
	}
	if want := netip.MustParseAddr("10.0.1.4"); active.IPv4 != want {
		t.Fatalf("IPv4 = %v, want %v", active.IPv4, want)
	}
}
