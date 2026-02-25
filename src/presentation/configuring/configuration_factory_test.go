package configuring

import (
	"context"
	"testing"

	"tungo/domain/app"
	"tungo/presentation/ui/cli"
	"tungo/presentation/ui/tui"
)

func TestNewConfigurationFactory(t *testing.T) {
	f := NewConfigurationFactory(app.CLI, nil)
	if f == nil {
		t.Fatal("expected non-nil factory")
	}
}

func TestConfigurationFactory_Configurator_CLI(t *testing.T) {
	f := NewConfigurationFactory(app.CLI, nil)

	got, cleanup := f.Configurator(context.Background())
	defer cleanup()
	if _, ok := got.(*cli.Configurator); !ok {
		t.Fatalf("expected CLI configurator, got %T", got)
	}
}

func TestConfigurationFactory_Configurator_TUI(t *testing.T) {
	f := NewConfigurationFactory(app.TUI, nil)

	got, cleanup := f.Configurator(context.Background())
	defer cleanup()
	if _, ok := got.(*tui.Configurator); !ok {
		t.Fatalf("expected TUI configurator, got %T", got)
	}
}
