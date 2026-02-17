package tui

import (
	"errors"
	"testing"

	"tungo/domain/mode"
)

func TestNewConfigurator(t *testing.T) {
	c := NewConfigurator(
		&cfgObserverMock{},
		&cfgSelectorMock{},
		&cfgCreatorMock{},
		&cfgDeleterMock{},
		&mockManager{},
		&queuedSelectorFactory{selector: &queuedSelector{options: []string{"client"}}},
		nil,
		nil,
	)
	if c == nil {
		t.Fatal("expected non-nil configurator")
	}
	if c.clientConfigurator == nil || c.serverConfigurator == nil {
		t.Fatal("expected inner configurators to be initialized")
	}
}

func TestConfigurator_Configure_ClientMode(t *testing.T) {
	appSelectorFactory := &mockSelectorFactory{selector: &queueSelector{options: []string{"client"}}}
	clientSelectorFactory := &queuedSelectorFactory{selector: &queuedSelector{options: []string{"conf1"}}}

	c := &Configurator{
		appMode: NewAppMode(appSelectorFactory),
		clientConfigurator: newClientConfigurator(
			&cfgObserverMock{results: [][]string{{"conf1"}}},
			&cfgSelectorMock{},
			nil,
			nil,
			clientSelectorFactory,
			nil,
			nil,
			nil,
		),
		serverConfigurator: newServerConfigurator(&mockManager{}, &mockSelectorFactory{
			selector: &queueSelector{options: []string{startServerOption}},
		}),
	}

	gotMode, err := c.Configure()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMode != mode.Client {
		t.Fatalf("expected mode.Client, got %v", gotMode)
	}
}

func TestConfigurator_Configure_ServerMode(t *testing.T) {
	appSelectorFactory := &mockSelectorFactory{selector: &queueSelector{options: []string{"server"}}}

	c := &Configurator{
		appMode: NewAppMode(appSelectorFactory),
		serverConfigurator: newServerConfigurator(&mockManager{}, &mockSelectorFactory{
			selector: &queueSelector{options: []string{startServerOption}},
		}),
	}

	gotMode, err := c.Configure()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMode != mode.Server {
		t.Fatalf("expected mode.Server, got %v", gotMode)
	}
}

func TestConfigurator_Configure_AppModeError(t *testing.T) {
	appSelectorFactory := &mockSelectorFactory{
		selector: &queueSelector{
			options: []string{""},
			errs:    []error{errors.New("selection failed")},
		},
	}

	c := &Configurator{appMode: NewAppMode(appSelectorFactory)}
	gotMode, err := c.Configure()
	if err == nil {
		t.Fatal("expected error")
	}
	if gotMode != mode.Unknown {
		t.Fatalf("expected mode.Unknown, got %v", gotMode)
	}
}
