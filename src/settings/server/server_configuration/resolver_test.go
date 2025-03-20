package server_configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSuccess(t *testing.T) {
	resolver := newResolver()
	actual, err := resolver.resolve()
	if err != nil {
		t.Fatalf("resolve() returned error: %v", err)
	}

	expected := filepath.Join(string(os.PathSeparator), "etc", "tungo", "server_configuration.json")
	if actual != expected {
		t.Errorf("expected path %q, got %q", expected, actual)
	}
}
