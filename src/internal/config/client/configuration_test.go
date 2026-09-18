package client

import (
	"bytes"
	"log/slog"
	"net/netip"
	"reflect"
	"slices"
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
			want: settings.Settings{Network: udp.Network, MTU: udp.MTU, Protocol: settings.UDP},
		},
		{
			name: "TCP",
			cfg: Configuration{
				TCPSettings: tcp,
				Protocol:    settings.TCP,
			},
			want: settings.Settings{Network: tcp.Network, MTU: tcp.MTU, Protocol: settings.TCP},
		},
		{
			name: "WS",
			cfg: Configuration{
				WSSettings: ws,
				Protocol:   settings.WS,
			},
			want: settings.Settings{Network: ws.Network, MTU: ws.MTU, Protocol: settings.WS},
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
		UDPSettings: settings.Settings{Network: settings.Network{
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
		UDPSettings: settings.Settings{Network: settings.Network{
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
			s: settings.Settings{Network: settings.Network{
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			}},
			want: settings.DefaultMTU,
		},
		{
			name: "dual stack",
			s: settings.Settings{Network: settings.Network{
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
			}},
			want: settings.DefaultMTU,
		},
		{
			name: "IPv4 MTU below minimum",
			s: settings.Settings{
				Network: settings.Network{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:     settings.MinimumIPv4MTU - 1,
			},
			want:        settings.DefaultMTU,
			wantWarning: true,
		},
		{
			name: "IPv6 MTU below minimum",
			s: settings.Settings{
				Network: settings.Network{IPv6Subnet: netip.MustParsePrefix("fd00::/64")},
				MTU:     settings.DefaultMTU - 1,
			},
			want:        settings.DefaultMTU,
			wantWarning: true,
		},
		{
			name: "negative IPv6 MTU",
			s: settings.Settings{
				Network: settings.Network{IPv6Subnet: netip.MustParsePrefix("fd00::/64")},
				MTU:     -1,
			},
			want:        settings.DefaultMTU,
			wantWarning: true,
		},
		{
			name: "MTU above maximum",
			s: settings.Settings{
				Network: settings.Network{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:     settings.MaximumMTU + 1,
			},
			want:        settings.DefaultMTU,
			wantWarning: true,
		},
		{
			name: "IPv4 MTU below default",
			s: settings.Settings{
				Network: settings.Network{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:     settings.DefaultMTU - 1,
			},
			want: settings.DefaultMTU - 1,
		},
		{
			name: "minimum IPv4 MTU",
			s: settings.Settings{
				Network: settings.Network{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:     settings.MinimumIPv4MTU,
			},
			want: settings.MinimumIPv4MTU,
		},
		{
			name: "maximum MTU",
			s: settings.Settings{
				Network: settings.Network{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:     settings.MaximumMTU,
			},
			want: settings.MaximumMTU,
		},
		{
			name: "explicit MTU",
			s: settings.Settings{
				Network: settings.Network{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
				MTU:     1400,
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
					"protocol=UDP",
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

func TestConfiguration_ApplyDefaultsSetsDNSForConfiguredFamilies(t *testing.T) {
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	tests := []struct {
		name     string
		network  settings.Network
		wantDNS4 []string
		wantDNS6 []string
		wantLogs []string
	}{
		{
			name:     "IPv4",
			network:  settings.Network{IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24")},
			wantDNS4: []string{"1.1.1.1", "8.8.8.8"},
			wantLogs: []string{`level=INFO msg="client DNSv4 defaults applied" effective="[1.1.1.1 8.8.8.8]"`},
		},
		{
			name:     "IPv6",
			network:  settings.Network{IPv6Subnet: netip.MustParsePrefix("fd00::/64")},
			wantDNS6: []string{"2606:4700:4700::1111", "2001:4860:4860::8888"},
			wantLogs: []string{`level=INFO msg="client DNSv6 defaults applied" effective="[2606:4700:4700::1111 2001:4860:4860::8888]"`},
		},
		{
			name: "dual stack preserves explicit DNS",
			network: settings.Network{
				IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
				IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
				DNSv4:      []string{"9.9.9.9"},
				DNSv6:      []string{"2620:fe::9"},
			},
			wantDNS4: []string{"9.9.9.9"},
			wantDNS6: []string{"2620:fe::9"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			logs.Reset()
			cfg := Configuration{
				Protocol: settings.UDP,
				UDPSettings: settings.Settings{
					Network: test.network,
				},
			}
			cfg.applyDefaults()

			if !reflect.DeepEqual(cfg.UDPSettings.DNSv4, test.wantDNS4) {
				t.Fatalf("DNSv4 = %v, want %v", cfg.UDPSettings.DNSv4, test.wantDNS4)
			}
			if !reflect.DeepEqual(cfg.UDPSettings.DNSv6, test.wantDNS6) {
				t.Fatalf("DNSv6 = %v, want %v", cfg.UDPSettings.DNSv6, test.wantDNS6)
			}
			for _, message := range test.wantLogs {
				if !strings.Contains(logs.String(), message) {
					t.Errorf("log does not contain %q: %q", message, logs.String())
				}
			}
			if got := strings.Count(logs.String(), "\n"); got != len(test.wantLogs) {
				t.Errorf("got %d log entries, want %d: %q", got, len(test.wantLogs), logs.String())
			}
			logs.Reset()
			cfg.applyDefaults()
			if logs.Len() != 0 {
				t.Fatalf("reapplying defaults logged changes: %s", &logs)
			}
		})
	}
}

func TestConfiguration_ApplyDefaultsKeepsDNSIndependent(t *testing.T) {
	cfg := Configuration{
		Protocol: settings.UDP,
		UDPSettings: settings.Settings{Network: settings.Network{
			IPv4Subnet: netip.MustParsePrefix("10.0.0.0/24"),
			IPv6Subnet: netip.MustParsePrefix("fd00::/64"),
		}},
	}
	other := cfg
	cfg.applyDefaults()
	other.applyDefaults()
	wantDNS4 := append([]string(nil), other.UDPSettings.DNSv4...)
	wantDNS6 := append([]string(nil), other.UDPSettings.DNSv6...)

	cfg.UDPSettings.DNSv4[0] = "9.9.9.9"
	cfg.UDPSettings.DNSv6[0] = "2620:fe::9"
	if !reflect.DeepEqual(other.UDPSettings.DNSv4, wantDNS4) ||
		!reflect.DeepEqual(other.UDPSettings.DNSv6, wantDNS6) {
		t.Fatal("applyDefaults shared DNS backing arrays between configurations")
	}
}

func TestConfiguration_ApplyDefaultsRoutesDNS(t *testing.T) {
	for _, tt := range []struct {
		name   string
		v4     []string
		v6     []string
		dns4   []string
		dns6   []string
		wantV4 []string
		wantV6 []string
	}{
		{
			name:   "narrow lists gain custom DNS routes",
			v4:     []string{"192.0.2.42/24"},
			v6:     []string{"2001:db8::42/64"},
			dns4:   []string{"9.9.9.9"},
			dns6:   []string{"2620:fe::9"},
			wantV4: []string{"192.0.2.0/24", "9.9.9.9/32"},
			wantV6: []string{"2001:db8::/64", "2620:fe::9/128"},
		},
		{
			name:   "empty lists gain default DNS routes",
			v4:     []string{},
			v6:     []string{},
			wantV4: []string{"1.1.1.1/32", "8.8.8.8/32"},
			wantV6: []string{"2606:4700:4700::1111/128", "2001:4860:4860::8888/128"},
		},
		{
			name:   "existing prefixes cover some resolvers",
			v4:     []string{"9.9.9.0/24", "8.8.8.8/32"},
			v6:     []string{"2620:fe::/64", "2001:4860:4860::8888/128"},
			dns4:   []string{"9.9.9.9", "8.8.8.8", "1.1.1.1"},
			dns6:   []string{"2620:fe::9", "2001:4860:4860::8888", "2606:4700:4700::1111"},
			wantV4: []string{"9.9.9.0/24", "8.8.8.8/32", "1.1.1.1/32"},
			wantV6: []string{"2620:fe::/64", "2001:4860:4860::8888/128", "2606:4700:4700::1111/128"},
		},
		{
			name:   "TUN subnets cover resolvers without extra routes",
			v4:     []string{},
			v6:     []string{},
			dns4:   []string{"10.0.1.1"},
			dns6:   []string{"fd00::1"},
			wantV4: []string{},
			wantV6: []string{},
		},
		{
			name:   "duplicate resolvers with different spelling",
			v4:     []string{},
			v6:     []string{},
			dns4:   []string{" 9.9.9.9 ", "9.9.9.9"},
			dns6:   []string{" 2620:00fe::9 ", "2620:fe::9"},
			wantV4: []string{"9.9.9.9/32"},
			wantV6: []string{"2620:fe::9/128"},
		},
		{
			name:   "scoped IPv6 resolvers share one host route",
			v6:     []string{},
			dns6:   []string{"fe80::1%tun0", "fe80::1%tun0"},
			wantV4: []string{"0.0.0.0/1", "128.0.0.0/1"},
			wantV6: []string{"fe80::1/128"},
		},
		{
			name:   "scoped IPv6 resolver covered by a prefix",
			v6:     []string{"fe80::/64"},
			dns6:   []string{"fe80::1%tun0"},
			wantV4: []string{"0.0.0.0/1", "128.0.0.0/1"},
			wantV6: []string{"fe80::/64"},
		},
		{
			name:   "full tunnel already covers DNS",
			v4:     []string{"0.0.0.0/0"},
			v6:     []string{"::/0"},
			wantV4: []string{"0.0.0.0/1", "128.0.0.0/1"},
			wantV6: []string{"::/1", "8000::/1"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfiguration()
			cfg.UDPSettings.IPv4Subnet = netip.MustParsePrefix("10.0.1.42/24")
			cfg.UDPSettings.IPv6Subnet = netip.MustParsePrefix("fd00::42/64")
			cfg.UDPSettings.DNSv4, cfg.UDPSettings.DNSv6 = tt.dns4, tt.dns6
			cfg.TunnelRoutesV4, cfg.TunnelRoutesV6 = tt.v4, tt.v6
			cfg.applyDefaults()
			if !slices.Equal(cfg.TunnelRoutesV4, tt.wantV4) {
				t.Fatalf("TunnelRoutesV4 = %v, want %v", cfg.TunnelRoutesV4, tt.wantV4)
			}
			if !slices.Equal(cfg.TunnelRoutesV6, tt.wantV6) {
				t.Fatalf("TunnelRoutesV6 = %v, want %v", cfg.TunnelRoutesV6, tt.wantV6)
			}
			cfg.applyDefaults()
			if !slices.Equal(cfg.TunnelRoutesV4, tt.wantV4) || !slices.Equal(cfg.TunnelRoutesV6, tt.wantV6) {
				t.Fatal("reapplying defaults changed the DNS routes")
			}
		})
	}
}

func TestConfiguration_ApplyDefaultsRoutesDNSForActiveProtocolAndFamilies(t *testing.T) {
	for _, protocol := range []settings.Protocol{settings.UDP, settings.TCP, settings.WS, settings.WSS} {
		for _, ipv6 := range []bool{false, true} {
			t.Run(protocol.String()+"/ipv6="+strconv.FormatBool(ipv6), func(t *testing.T) {
				inactive := settings.Settings{Network: settings.Network{
					IPv4Subnet: netip.MustParsePrefix("10.0.2.0/24"),
					IPv6Subnet: netip.MustParsePrefix("fd01::/64"),
					DNSv4:      []string{"8.8.8.8"},
					DNSv6:      []string{"2001:4860:4860::8888"},
				}}
				cfg := Configuration{
					Protocol: protocol, UDPSettings: inactive, TCPSettings: inactive, WSSettings: inactive,
					TunnelRoutesV4: []string{}, TunnelRoutesV6: []string{},
				}
				active, err := cfg.selectedSettings()
				if err != nil {
					t.Fatal(err)
				}
				active.Network = settings.Network{DNSv4: []string{"9.9.9.9"}, DNSv6: []string{"2620:fe::9"}}
				wantV4, wantV6 := []string{"9.9.9.9/32"}, []string{}
				if ipv6 {
					active.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
					wantV4, wantV6 = []string{}, []string{"2620:fe::9/128"}
				} else {
					active.IPv4Subnet = netip.MustParsePrefix("10.0.1.0/24")
				}
				cfg.applyDefaults()
				if !slices.Equal(cfg.TunnelRoutesV4, wantV4) || !slices.Equal(cfg.TunnelRoutesV6, wantV6) {
					t.Fatalf("TunnelRoutes = %v, %v; want %v, %v", cfg.TunnelRoutesV4, cfg.TunnelRoutesV6, wantV4, wantV6)
				}
			})
		}
	}
}

func TestConfiguration_ApplyDefaultsWarnsAboutDNSRoutes(t *testing.T) {
	var logs bytes.Buffer
	originalLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(originalLogger) })

	cfg := validTestConfiguration()
	cfg.UDPSettings.IPv6Subnet = netip.MustParsePrefix("fd00::/64")
	cfg.UDPSettings.DNSv4 = []string{"9.9.9.9"}
	cfg.UDPSettings.DNSv6 = []string{"2620:fe::9"}
	cfg.TunnelRoutesV4, cfg.TunnelRoutesV6 = []string{}, []string{}
	cfg.applyDefaults()
	for _, message := range []string{
		`level=WARN msg="client TunnelRoutesV4 were changed" configured=[] effective=[9.9.9.9/32]`,
		`level=WARN msg="client TunnelRoutesV6 were changed" configured=[] effective=[2620:fe::9/128]`,
	} {
		if !strings.Contains(logs.String(), message) {
			t.Errorf("log does not contain %q: %q", message, logs.String())
		}
	}
	logs.Reset()
	cfg.applyDefaults()
	if logs.Len() != 0 {
		t.Fatalf("reapplying defaults logged changes: %s", &logs)
	}
}
