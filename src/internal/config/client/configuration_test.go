package client

import (
	"bytes"
	"log/slog"
	"net/netip"
	"reflect"
	"strconv"
	"strings"
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

func TestConfiguration_ActiveSettingsReturnsAddressDerivationError(t *testing.T) {
	cfg := Configuration{
		ClientID: 2,
		Protocol: settings.UDP,
		UDPSettings: settings.Settings{Addressing: settings.Addressing{
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/30"),
		}},
	}

	if _, err := cfg.ActiveSettings(); err == nil || !strings.Contains(err.Error(), "derive IPv4") {
		t.Fatalf("ActiveSettings() error = %v, want IPv4 derivation error", err)
	}
}

func TestConfiguration_ApplyDefaults(t *testing.T) {
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	tests := []struct {
		name        string
		s           settings.Settings
		want        int
		wantWarning bool
	}{
		{
			name: "IPv4",
			s: settings.Settings{Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			}},
			want: settings.DefaultMTU,
		},
		{
			name: "dual stack",
			s: settings.Settings{Addressing: settings.Addressing{
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			}},
			want: settings.DefaultMTU,
		},
		{
			name: "IPv4 MTU below minimum",
			s: settings.Settings{
				Addressing: settings.Addressing{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:        settings.MinimumIPv4MTU - 1,
			},
			want:        settings.DefaultMTU,
			wantWarning: true,
		},
		{
			name: "IPv6 MTU below minimum",
			s: settings.Settings{
				Addressing: settings.Addressing{IPv6Subnet: netip.MustParsePrefix("fd00::/64")},
				MTU:        settings.DefaultMTU - 1,
			},
			want:        settings.DefaultMTU,
			wantWarning: true,
		},
		{
			name: "negative IPv6 MTU",
			s: settings.Settings{
				Addressing: settings.Addressing{IPv6Subnet: netip.MustParsePrefix("fd00::/64")},
				MTU:        -1,
			},
			want:        settings.DefaultMTU,
			wantWarning: true,
		},
		{
			name: "MTU above maximum",
			s: settings.Settings{
				Addressing: settings.Addressing{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:        settings.MaximumMTU + 1,
			},
			want:        settings.DefaultMTU,
			wantWarning: true,
		},
		{
			name: "IPv4 MTU below default",
			s: settings.Settings{
				Addressing: settings.Addressing{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:        settings.DefaultMTU - 1,
			},
			want: settings.DefaultMTU - 1,
		},
		{
			name: "minimum IPv4 MTU",
			s: settings.Settings{
				Addressing: settings.Addressing{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:        settings.MinimumIPv4MTU,
			},
			want: settings.MinimumIPv4MTU,
		},
		{
			name: "maximum MTU",
			s: settings.Settings{
				Addressing: settings.Addressing{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:        settings.MaximumMTU,
			},
			want: settings.MaximumMTU,
		},
		{
			name: "explicit MTU",
			s: settings.Settings{
				Addressing: settings.Addressing{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:        1400,
			},
			want: 1400,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logs.Reset()
			cfg := Configuration{Protocol: settings.UDP, UDPSettings: tt.s}
			cfg.applyDefaults()
			if cfg.UDPSettings.MTU != tt.want {
				t.Fatalf("MTU = %d, want %d", cfg.UDPSettings.MTU, tt.want)
			}
			gotWarning := strings.Contains(logs.String(), "level=WARN")
			if gotWarning != tt.wantWarning {
				t.Fatalf("warning logged = %t, want %t; log: %q", gotWarning, tt.wantWarning, logs.String())
			}
			if tt.wantWarning {
				for _, field := range []string{
					"configured=" + strconv.Itoa(tt.s.MTU),
					"effective=" + strconv.Itoa(tt.want),
				} {
					if !strings.Contains(logs.String(), field) {
						t.Errorf("log does not contain %q: %q", field, logs.String())
					}
				}
			}
		})
	}
}
