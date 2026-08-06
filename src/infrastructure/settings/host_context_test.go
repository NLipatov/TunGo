package settings

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestHost_RouteIPv4Context_UsesContextResolver(t *testing.T) {
	orig := lookupHostContext
	t.Cleanup(func() { lookupHostContext = orig })

	lookupHostContext = func(_ context.Context, domain string) ([]string, error) {
		if domain != "vpn.example.com" {
			t.Fatalf("unexpected domain: %s", domain)
		}
		return []string{"2001:db8::1", "198.51.100.20"}, nil
	}

	h, err := DomainHost("vpn.example.com")
	if err != nil {
		t.Fatalf("DomainHost failed: %v", err)
	}

	ip, routeErr := h.RouteIPv4Context(context.Background())
	if routeErr != nil {
		t.Fatalf("RouteIPv4Context failed: %v", routeErr)
	}
	if ip != "198.51.100.20" {
		t.Fatalf("unexpected IPv4 route result: %s", ip)
	}
}

func TestHost_RouteIPContext_PropagatesContextCancel(t *testing.T) {
	orig := lookupHostContext
	t.Cleanup(func() { lookupHostContext = orig })

	lookupHostContext = func(ctx context.Context, _ string) ([]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	h, err := DomainHost("vpn.example.com")
	if err != nil {
		t.Fatalf("DomainHost failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, routeErr := h.RouteIPContext(ctx)
	if routeErr == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(routeErr.Error(), context.Canceled.Error()) && !errors.Is(routeErr, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", routeErr)
	}
}

func TestHost_RouteIPContext_NilContext(t *testing.T) {
	orig := lookupHostContext
	t.Cleanup(func() { lookupHostContext = orig })

	lookupHostContext = func(ctx context.Context, _ string) ([]string, error) {
		if ctx == nil {
			t.Fatal("resolver received nil context")
		}
		return []string{"198.51.100.20"}, nil
	}

	h, err := DomainHost("vpn.example.com")
	if err != nil {
		t.Fatalf("DomainHost failed: %v", err)
	}
	//nolint:staticcheck // This test verifies the nil-context fallback.
	if ip, routeErr := h.RouteIPContext(nil); routeErr != nil || ip != "198.51.100.20" {
		t.Fatalf("RouteIPContext(nil) = (%q, %v)", ip, routeErr)
	}
}

func TestHost_RouteIPv6Context_SkipsInvalidAndIPv4Addresses(t *testing.T) {
	orig := lookupHostContext
	t.Cleanup(func() { lookupHostContext = orig })

	lookupHostContext = func(_ context.Context, _ string) ([]string, error) {
		return []string{"not-an-ip", "198.51.100.20"}, nil
	}

	h, err := DomainHost("vpn.example.com")
	if err != nil {
		t.Fatalf("DomainHost failed: %v", err)
	}
	if _, routeErr := h.RouteIPv6Context(context.Background()); routeErr == nil {
		t.Fatal("expected no matching address family error")
	}
}

func TestHost_RouteIPv6Context_ResolvesIPv6(t *testing.T) {
	orig := lookupHostContext
	t.Cleanup(func() { lookupHostContext = orig })

	lookupHostContext = func(_ context.Context, _ string) ([]string, error) {
		return []string{"2001:db8::20"}, nil
	}

	h, err := DomainHost("vpn.example.com")
	if err != nil {
		t.Fatalf("DomainHost failed: %v", err)
	}
	if ip, routeErr := h.RouteIPv6Context(context.Background()); routeErr != nil || ip != "2001:db8::20" {
		t.Fatalf("RouteIPv6Context() = (%q, %v)", ip, routeErr)
	}
}
