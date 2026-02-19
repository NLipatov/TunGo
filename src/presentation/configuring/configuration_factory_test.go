package configuring

import (
	"os"
	"testing"

	"tungo/presentation/ui/cli"
	"tungo/presentation/ui/tui"
)

func TestNewConfigurationFactory(t *testing.T) {
	f := NewConfigurationFactory(nil)
	if f == nil {
		t.Fatal("expected non-nil factory")
	}
}

func TestConfigurationFactory_Configurator_CLI(t *testing.T) {
	original := os.Args
	t.Cleanup(func() {
		os.Args = original
	})

	os.Args = []string{"tungo", "--help"}
	f := NewConfigurationFactory(nil)

	got := f.Configurator()
	if _, ok := got.(*cli.Configurator); !ok {
		t.Fatalf("expected CLI configurator, got %T", got)
	}
}

func TestConfigurationFactory_Configurator_TUI(t *testing.T) {
	original := os.Args
	t.Cleanup(func() {
		os.Args = original
	})

	os.Args = []string{"tungo"}
	f := NewConfigurationFactory(nil)

	got := f.Configurator()
	if _, ok := got.(*tui.Configurator); !ok {
		t.Fatalf("expected TUI configurator, got %T", got)
	}
}
