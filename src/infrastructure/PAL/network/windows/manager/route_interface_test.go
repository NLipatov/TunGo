//go:build windows

package manager

import "testing"

func TestRouteInterfaceName_UsesAliasWhenProvided(t *testing.T) {
	got, err := routeInterfaceName(" Ethernet0 ", 15)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "Ethernet0" {
		t.Fatalf("unexpected interface name: %q", got)
	}
}

func TestRouteInterfaceName_FallsBackToIndex(t *testing.T) {
	got, err := routeInterfaceName("", 12)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "12" {
		t.Fatalf("unexpected interface fallback: %q", got)
	}
}

func TestRouteInterfaceName_ErrorsWithoutAliasAndIndex(t *testing.T) {
	if _, err := routeInterfaceName("   ", 0); err == nil {
		t.Fatal("expected error for empty alias and invalid index")
	}
}
