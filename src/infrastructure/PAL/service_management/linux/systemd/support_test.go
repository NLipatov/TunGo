package systemd

import (
	"errors"
	"os"
	"testing"
)

func TestAvailable(t *testing.T) {
	hooks := Hooks{
		Stat:     func(string) (os.FileInfo, error) { return nil, nil },
		LookPath: func(string) (string, error) { return "/bin/systemctl", nil },
	}
	if !Available(hooks, "/run/systemd/system") {
		t.Fatal("expected available=true")
	}

	hooks.Stat = func(string) (os.FileInfo, error) { return nil, errors.New("missing") }
	if Available(hooks, "/run/systemd/system") {
		t.Fatal("expected available=false when runtime dir missing")
	}

	hooks.Stat = func(string) (os.FileInfo, error) { return nil, nil }
	hooks.LookPath = func(string) (string, error) { return "", errors.New("missing") }
	if Available(hooks, "/run/systemd/system") {
		t.Fatal("expected available=false when systemctl missing")
	}
}
