package client_configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientResolverResolve(t *testing.T) {
	resolved, err := newClientResolver().resolve()
	if err != nil {
		t.Fatalf("resolve() returned error: %v", err)
	}
	expected := filepath.Join(string(os.PathSeparator), "etc", "tungo", "client_configuration.json")
	if resolved != expected {
		t.Errorf("expected %q, got %q", expected, resolved)
	}
}
