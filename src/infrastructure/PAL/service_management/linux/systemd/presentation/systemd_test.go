package presentation

import (
	"errors"
	"os"
	"testing"

	"tungo/infrastructure/PAL/service_management/linux/systemd/application"
	"tungo/infrastructure/PAL/service_management/linux/systemd/infrastructure"
)

type noopCommander struct{}

func (noopCommander) CombinedOutput(string, ...string) ([]byte, error) { return nil, nil }
func (noopCommander) Run(string, ...string) error                      { return nil }

func unsupportedService() *application.Service {
	hooks := infrastructure.Hooks{
		Stat:     func(string) (os.FileInfo, error) { return nil, errors.New("missing") },
		LookPath: func(string) (string, error) { return "", errors.New("missing") },
		Lstat:    func(string) (os.FileInfo, error) { return nil, errors.New("unused") },
		WriteFile: func(string, []byte, os.FileMode) error {
			return nil
		},
		ReadFile: func(string) ([]byte, error) { return nil, nil },
		Remove:   func(string) error { return nil },
		Geteuid:  func() int { return 0 },
	}
	return application.NewService(noopCommander{}, hooks, infrastructure.DefaultConfig())
}

func TestSystemd_ForwardsMethods(t *testing.T) {
	api := NewSystemd(unsupportedService())
	if api == nil {
		t.Fatal("expected non-nil api")
	}
	if api.Supported() {
		t.Fatal("expected unsupported")
	}

	if _, err := api.InstallServerUnit(); err == nil {
		t.Fatal("expected error")
	}
	if _, err := api.InstallClientUnit(); err == nil {
		t.Fatal("expected error")
	}
	if err := api.RemoveUnit(); err == nil {
		t.Fatal("expected error")
	}
	if _, err := api.IsUnitActive(); err == nil {
		t.Fatal("expected error")
	}
	if err := api.StopUnit(); err == nil {
		t.Fatal("expected error")
	}
	if err := api.StartUnit(); err == nil {
		t.Fatal("expected error")
	}
	if err := api.EnableUnit(); err == nil {
		t.Fatal("expected error")
	}
	if err := api.DisableUnit(); err == nil {
		t.Fatal("expected error")
	}
	if _, err := api.Status(); err == nil {
		t.Fatal("expected error")
	}
}
