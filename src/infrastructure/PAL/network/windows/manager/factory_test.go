//go:build windows

package manager

import (
	"net/netip"
	"testing"
	"tungo/infrastructure/settings"
)

func TestFactory_Create_SelectsManagerByAddressFamilies(t *testing.T) {
	base := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "198.51.100.10"),
		},
	}

	t.Run("dual stack", func(t *testing.T) {
		s := base
		s.IPv4 = netip.MustParseAddr("10.0.0.2")
		s.IPv6 = netip.MustParseAddr("fd00::2")

		got, err := NewFactory(s).Create()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got.(*dualStackManager); !ok {
			t.Fatalf("expected *dualStackManager, got %T", got)
		}
	})

	t.Run("ipv4 only", func(t *testing.T) {
		s := base
		s.IPv4 = netip.MustParseAddr("10.0.0.2")

		got, err := NewFactory(s).Create()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got.(*v4Manager); !ok {
			t.Fatalf("expected *v4Manager, got %T", got)
		}
	})

	t.Run("ipv6 only", func(t *testing.T) {
		s := base
		s.IPv6 = netip.MustParseAddr("fd00::2")

		got, err := NewFactory(s).Create()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, ok := got.(*v6Manager); !ok {
			t.Fatalf("expected *v6Manager, got %T", got)
		}
	})
}

func TestFactory_Create_NoValidAddresses(t *testing.T) {
	s := settings.Settings{
		Addressing: settings.Addressing{
			TunName: "tun0",
			Server:  mustHost(t, "198.51.100.10"),
		},
	}

	got, err := NewFactory(s).Create()
	if err == nil {
		t.Fatalf("expected error, got manager %T", got)
	}
}

func mustHost(t *testing.T, raw string) settings.Host {
	t.Helper()
	h, err := settings.NewHost(raw)
	if err != nil {
		t.Fatalf("NewHost(%q): %v", raw, err)
	}
	return h
}
