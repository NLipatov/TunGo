package infrastructure

import (
	"errors"
	"os"
	"testing"
)

func TestSupported(t *testing.T) {
	hooks := Hooks{
		Stat:     func(string) (os.FileInfo, error) { return nil, nil },
		LookPath: func(string) (string, error) { return "/bin/systemctl", nil },
	}
	if !Supported(hooks, "/run/systemd/system") {
		t.Fatal("expected supported=true")
	}

	hooks.Stat = func(string) (os.FileInfo, error) { return nil, errors.New("missing") }
	if Supported(hooks, "/run/systemd/system") {
		t.Fatal("expected supported=false when runtime dir missing")
	}

	hooks.Stat = func(string) (os.FileInfo, error) { return nil, nil }
	hooks.LookPath = func(string) (string, error) { return "", errors.New("missing") }
	if Supported(hooks, "/run/systemd/system") {
		t.Fatal("expected supported=false when systemctl missing")
	}
}
