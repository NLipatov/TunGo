package client

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClientResolverResolve(t *testing.T) {
	resolved, err := NewDefaultResolver().Resolve()
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}
	expected := filepath.Join(string(os.PathSeparator), "etc", "tungo", "client_configuration.json")
	if resolved != expected {
		t.Errorf("expected %q, got %q", expected, resolved)
	}
}
