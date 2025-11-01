package client

import (
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

			if got != tt.want {
				t.Fatalf("unexpected result: got %+v, want %+v", got, tt.want)
			}
		})
	}
}
