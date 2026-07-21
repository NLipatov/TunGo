package client

import (
	"path/filepath"
	"testing"
)

func TestClientResolverResolve(t *testing.T) {
	resolved, err := NewDefaultResolver().Resolve()
	if err != nil {
		t.Fatalf("Resolve() returned error: %v", err)
	}

	sep := string(filepath.Separator)
	expected := filepath.Join("C:", sep, "ProgramData", sep, "TunGo", sep, "client_configuration.json")
	if resolved != expected {
		t.Errorf("expected %q, got %q", expected, resolved)
	}
}
