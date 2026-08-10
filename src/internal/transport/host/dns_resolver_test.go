package host

import (
	"context"
	"testing"

	"tungo/internal/config/settings"
)

func TestResolveConfiguredHost(t *testing.T) {
	t.Parallel()

	host := settings.Host{IPv4: "192.0.2.1", IPv6: "2001:db8::1"}
	if got, err := ResolveIP(context.Background(), host); err != nil || got != host.IPv4 {
		t.Fatalf("ResolveIP = (%q, %v)", got, err)
	}
	if got, err := ResolveIPv4(context.Background(), host); err != nil || got != host.IPv4 {
		t.Fatalf("ResolveIPv4 = (%q, %v)", got, err)
	}
	if got, err := ResolveIPv6(context.Background(), host); err != nil || got != host.IPv6 {
		t.Fatalf("ResolveIPv6 = (%q, %v)", got, err)
	}
}

func TestResolveRejectsWrongAddressFamily(t *testing.T) {
	t.Parallel()

	if _, err := ResolveIPv4(context.Background(), settings.Host{IPv6: "2001:db8::1"}); err == nil {
		t.Fatal("expected IPv4 resolution error")
	}
	if _, err := ResolveIPv6(context.Background(), settings.Host{IPv4: "192.0.2.1"}); err == nil {
		t.Fatal("expected IPv6 resolution error")
	}
}
