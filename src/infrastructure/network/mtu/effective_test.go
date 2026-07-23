package mtu

import (
	"net/netip"
	"testing"

	"tungo/infrastructure/settings"
)

func TestEffective(t *testing.T) {
	tests := []struct {
		name          string
		configuredMTU int
		ipv6          bool
		want          int
	}{
		{name: "configured IPv4", configuredMTU: 1400, want: 1400},
		{name: "zero IPv4 uses safe default", want: settings.SafeMTU},
		{name: "negative IPv4 uses safe default", configuredMTU: -100, want: settings.SafeMTU},
		{name: "IPv4 value below minimum is clamped", configuredMTU: 500, want: settings.MinimumIPv4MTU},
		{name: "IPv4 minimum is accepted", configuredMTU: settings.MinimumIPv4MTU, want: settings.MinimumIPv4MTU},
		{name: "large IPv4 value is preserved", configuredMTU: 9000, want: 9000},
		{name: "zero IPv6 is clamped to IPv6 minimum", ipv6: true, want: settings.MinimumIPv6MTU},
		{name: "IPv6 value below minimum is clamped", configuredMTU: 1000, ipv6: true, want: settings.MinimumIPv6MTU},
		{name: "IPv6 minimum is accepted", configuredMTU: settings.MinimumIPv6MTU, ipv6: true, want: settings.MinimumIPv6MTU},
		{name: "configured IPv6", configuredMTU: 1400, ipv6: true, want: 1400},
		{name: "large IPv6 value is preserved", configuredMTU: 9000, ipv6: true, want: 9000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			configuration := settings.Settings{MTU: tt.configuredMTU}
			if tt.ipv6 {
				configuration.IPv6 = netip.MustParseAddr("fd00::2")
			}

			if got := Effective(configuration); got != tt.want {
				t.Fatalf("Effective() = %d, want %d", got, tt.want)
			}
		})
	}
}
