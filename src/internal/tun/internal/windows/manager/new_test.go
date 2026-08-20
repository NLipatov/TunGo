//go:build windows

package manager

import (
	"net/netip"
	"testing"
	"tungo/internal/config/settings"
)

func TestNew_SelectsManagerBySubnets(t *testing.T) {
	base := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "198.51.100.10"),
		},
	}

	t.Run("dual stack", func(t *testing.T) {
		s := base
		s.IPv4Subnet = netip.MustParsePrefix("10.0.0.0/24")
		s.IPv6Subnet = netip.MustParsePrefix("fd00::/64")

		got, err := New(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got.(*dualStackManager); !ok {
			t.Fatalf("expected *dualStackManager, got %T", got)
		}
	})

	t.Run("ipv4 only", func(t *testing.T) {
		s := base
		s.IPv4Subnet = netip.MustParsePrefix("10.0.0.0/24")

		got, err := New(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got.(*v4Manager); !ok {
			t.Fatalf("expected *v4Manager, got %T", got)
		}
	})

	t.Run("ipv6 only", func(t *testing.T) {
		s := base
		s.IPv6Subnet = netip.MustParsePrefix("fd00::/64")

		got, err := New(s)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got.(*v6Manager); !ok {
			t.Fatalf("expected *v6Manager, got %T", got)
		}
	})
}

func TestNew_RejectsSettingsWithoutSubnets(t *testing.T) {
	tests := []struct {
		name       string
		addressing settings.Addressing
	}{
		{name: "empty"},
		{
			name: "IPv4 address only",
			addressing: settings.Addressing{
				IPv4: netip.MustParseAddr("10.0.0.2"),
			},
		},
		{
			name: "IPv6 address only",
			addressing: settings.Addressing{
				IPv6: netip.MustParseAddr("fd00::2"),
			},
		},
		{
			name: "dual-stack addresses only",
			addressing: settings.Addressing{
				IPv4: netip.MustParseAddr("10.0.0.2"),
				IPv6: netip.MustParseAddr("fd00::2"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(settings.Settings{Addressing: tt.addressing})
			if err == nil {
				t.Fatalf("expected error, got manager %T", got)
			}
		})
	}
}

func mustHost(t *testing.T, raw string) settings.Host {
	t.Helper()
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return settings.Host{Domain: raw}
	}
	if ip.Unmap().Is4() {
		return settings.Host{IPv4: ip.Unmap().String()}
	}
	return settings.Host{IPv6: ip.String()}
}
