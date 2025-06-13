package tui_test

import (
	"errors"
	"strings"
	"testing"
	"tungo/domain/mode"
	"tungo/presentation/configuring/tui"
	"tungo/presentation/configuring/tui/components"
)

type ModeMockSelector struct {
	option string
	err    error
}

func (m *ModeMockSelector) SelectOne() (string, error) {
	return m.option, m.err
}

type ModeMockSelectorFactory struct {
	selector components.Selector
	err      error
}

func (m *ModeMockSelectorFactory) NewTuiSelector(_ string, _ []string) (components.Selector, error) {
	return m.selector, m.err
}

func TestAppMode_Mode_Client(t *testing.T) {
	sf := &ModeMockSelectorFactory{
		selector: &ModeMockSelector{option: "client"},
	}
	app := tui.NewAppMode(sf)
	got, err := app.Mode()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != mode.Client {
		t.Fatalf("expected mode.Client, got %v", got)
	}
}

func TestAppMode_Mode_Server(t *testing.T) {
	sf := &ModeMockSelectorFactory{
		selector: &ModeMockSelector{option: "server"},
	}
	app := tui.NewAppMode(sf)
	got, err := app.Mode()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != mode.Server {
		t.Fatalf("expected mode.Server, got %v", got)
	}
}

func TestAppMode_Mode_UnknownSelection(t *testing.T) {
	sf := &ModeMockSelectorFactory{
		selector: &ModeMockSelector{option: "invalid"},
	}
	app := tui.NewAppMode(sf)
	got, err := app.Mode()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != mode.Unknown {
		t.Fatalf("expected mode.Unknown, got %v", got)
	}
	if !strings.Contains(err.Error(), "invalid mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppMode_Mode_SelectorFactoryError(t *testing.T) {
	sf := &ModeMockSelectorFactory{
		err: errors.New("factory failed"),
	}
	app := tui.NewAppMode(sf)
	got, err := app.Mode()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != mode.Unknown {
		t.Fatalf("expected mode.Unknown, got %v", got)
	}
	if !strings.Contains(err.Error(), "factory") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAppMode_Mode_SelectorError(t *testing.T) {
	sf := &ModeMockSelectorFactory{
		selector: &ModeMockSelector{err: errors.New("selection failed")},
	}
	app := tui.NewAppMode(sf)
	got, err := app.Mode()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if got != mode.Unknown {
		t.Fatalf("expected mode.Unknown, got %v", got)
	}
	if !strings.Contains(err.Error(), "selection") {
		t.Fatalf("unexpected error: %v", err)
	}
}
