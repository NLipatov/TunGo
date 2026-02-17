package tui_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"tungo/domain/mode"
	"tungo/presentation/configuring/tui"
	"tungo/presentation/configuring/tui/components/domain/contracts/selector"
	"tungo/presentation/configuring/tui/components/domain/value_objects"
)

// AppModeMockSelector implements selector.Selector for tests.
type AppModeMockSelector struct {
	option string
	err    error
}

func (m *AppModeMockSelector) SelectOne() (string, error) {
	return m.option, m.err
}

// AppModeMockSelectorFactory records inputs and returns the preset selector/error.
type AppModeMockSelectorFactory struct {
	selector selector.Selector
	err      error

	gotLabel      string
	gotOptions    []string
	gotForeground value_objects.Color
	gotBackground value_objects.Color
}

func (m *AppModeMockSelectorFactory) NewTuiSelector(
	label string,
	options []string,
	foreground value_objects.Color,
	background value_objects.Color,
) (selector.Selector, error) {
	m.gotLabel = label
	m.gotOptions = append([]string(nil), options...)
	m.gotForeground = foreground
	m.gotBackground = background
	// use colors so compiler doesn't warn in case of changes
	_ = m.gotForeground.Enabled()
	_ = m.gotBackground.Enabled()
	return m.selector, m.err
}

func TestAppMode_Mode_Client(t *testing.T) {
	sf := &AppModeMockSelectorFactory{
		selector: &AppModeMockSelector{option: "client"},
	}
	app := tui.NewAppMode(sf)
	got, err := app.Mode()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != mode.Client {
		t.Fatalf("expected mode.Client, got %v", got)
	}
	// assert factory inputs
	if sf.gotLabel != "Select mode" {
		t.Fatalf("expected label %q, got %q", "Select mode", sf.gotLabel)
	}
	wantOpts := []string{"client", "server"}
	if !reflect.DeepEqual(sf.gotOptions, wantOpts) {
		t.Fatalf("expected options %v, got %v", wantOpts, sf.gotOptions)
	}
}

func TestAppMode_Mode_Server(t *testing.T) {
	sf := &AppModeMockSelectorFactory{
		selector: &AppModeMockSelector{option: "server"},
	}
	app := tui.NewAppMode(sf)
	got, err := app.Mode()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got != mode.Server {
		t.Fatalf("expected mode.Server, got %v", got)
	}
	// sanity check inputs once more
	if sf.gotLabel != "Select mode" {
		t.Fatalf("expected label %q, got %q", "Select mode", sf.gotLabel)
	}
	wantOpts := []string{"client", "server"}
	if !reflect.DeepEqual(sf.gotOptions, wantOpts) {
		t.Fatalf("expected options %v, got %v", wantOpts, sf.gotOptions)
	}
}

func TestAppMode_Mode_UnknownSelection(t *testing.T) {
	sf := &AppModeMockSelectorFactory{
		selector: &AppModeMockSelector{option: "invalid"},
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
	sf := &AppModeMockSelectorFactory{
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
	sf := &AppModeMockSelectorFactory{
		selector: &AppModeMockSelector{err: errors.New("selection failed")},
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
