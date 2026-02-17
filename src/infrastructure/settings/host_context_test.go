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
