package server

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSuccess(t *testing.T) {
	resolver := NewServerResolver()
	actual, err := resolver.Resolve()
	if err != nil {
		t.Fatalf("resolve() returned error: %v", err)
	}

	expected := filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json")
	if actual != expected {
		t.Errorf("expected path %q, got %q", expected, actual)
	}
}
