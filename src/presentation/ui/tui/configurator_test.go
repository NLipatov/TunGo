package tui

import (
	"errors"
	"testing"

	"tungo/domain/mode"
	selectorContract "tungo/presentation/ui/tui/internal/ui/contracts/selector"
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

func TestNewDefaultConfigurator(t *testing.T) {
	c := NewDefaultConfigurator(&mockManager{})
	if c == nil {
		t.Fatal("expected non-nil default configurator")
	}
	if c.clientConfigurator == nil || c.serverConfigurator == nil {
		t.Fatal("expected default configurator to initialize internal configurators")
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

func TestConfigurator_Configure_BackToModeFromClient_ThenServer(t *testing.T) {
	appSelectorFactory := &mockSelectorFactory{
		selector: &queueSelector{options: []string{"client", "server"}},
	}
	clientSelectorFactory := &queuedSelectorFactory{
		selector: &queuedSelector{
			options: []string{""},
			errs:    []error{selectorContract.ErrNavigateBack},
		},
	}

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
	if gotMode != mode.Server {
		t.Fatalf("expected mode.Server after back to mode selection, got %v", gotMode)
	}
}

func TestConfigurator_Configure_BackToModeFromServer_ThenClient(t *testing.T) {
	appSelectorFactory := &mockSelectorFactory{
		selector: &queueSelector{options: []string{"server", "client"}},
	}
	serverSelectorFactory := &mockSelectorFactory{
		selector: &queueSelector{
			options: []string{""},
			errs:    []error{selectorContract.ErrNavigateBack},
		},
	}
	clientSelectorFactory := &queuedSelectorFactory{
		selector: &queuedSelector{options: []string{"conf1"}},
	}

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
		serverConfigurator: newServerConfigurator(&mockManager{}, serverSelectorFactory),
	}

	gotMode, err := c.Configure()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotMode != mode.Client {
		t.Fatalf("expected mode.Client after server back, got %v", gotMode)
	}
}

func TestConfigurator_Configure_ClientErrorPropagates(t *testing.T) {
	appSelectorFactory := &mockSelectorFactory{
		selector: &queueSelector{options: []string{"client"}},
	}
	clientSelectorFactory := &queuedSelectorFactory{
		selector: &queuedSelector{
			options: []string{""},
			errs:    []error{errors.New("client fail")},
		},
	}

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
	if err == nil || err.Error() != "client fail" {
		t.Fatalf("expected client fail, got %v", err)
	}
	if gotMode != mode.Client {
		t.Fatalf("expected selected mode client on client error, got %v", gotMode)
	}
}

func TestConfigurator_Configure_ServerErrorPropagates(t *testing.T) {
	appSelectorFactory := &mockSelectorFactory{
		selector: &queueSelector{options: []string{"server"}},
	}
	serverSelectorFactory := &mockSelectorFactory{
		selector: &queueSelector{
			options: []string{""},
			errs:    []error{errors.New("server fail")},
		},
	}

	c := &Configurator{
		appMode:            NewAppMode(appSelectorFactory),
		clientConfigurator: newClientConfigurator(&cfgObserverMock{}, &cfgSelectorMock{}, nil, nil, &queuedSelectorFactory{selector: &queuedSelector{}}, nil, nil, nil),
		serverConfigurator: newServerConfigurator(&mockManager{}, serverSelectorFactory),
	}

	gotMode, err := c.Configure()
	if err == nil || err.Error() != "server fail" {
		t.Fatalf("expected server fail, got %v", err)
	}
	if gotMode != mode.Server {
		t.Fatalf("expected selected mode server on server error, got %v", gotMode)
	}
}

func TestConfigurator_ConfigureFromState_UnknownState(t *testing.T) {
	c := &Configurator{}
	gotMode, err := c.configureFromState(configuratorState(99))
	if err == nil || err.Error() != "unknown configurator state: 99" {
		t.Fatalf("expected unknown state error, got %v", err)
	}
	if gotMode != mode.Unknown {
		t.Fatalf("expected mode.Unknown, got %v", gotMode)
	}
}
