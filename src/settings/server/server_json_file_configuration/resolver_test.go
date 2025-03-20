package server_json_file_configuration

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

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() returned error: %v", err)
	}

	expected := filepath.Join(filepath.Dir(cwd), "src", "settings", "server", "conf.json")
	if actual != expected {
		t.Errorf("expected path %q, actual %q", expected, actual)
	}
}
