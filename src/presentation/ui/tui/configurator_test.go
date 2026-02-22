package tui

import (
	"context"
	"errors"
	"testing"

	"tungo/domain/mode"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
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

	gotMode, err := c.Configure(context.Background())
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

	gotMode, err := c.Configure(context.Background())
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
	gotMode, err := c.Configure(context.Background())
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

	gotMode, err := c.Configure(context.Background())
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

	gotMode, err := c.Configure(context.Background())
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

	gotMode, err := c.Configure(context.Background())
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

	gotMode, err := c.Configure(context.Background())
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

func TestConfigurator_ConfigureContinuous_NilClientConfigurator(t *testing.T) {
	c := &Configurator{
		useContinuousUI:    true,
		clientConfigurator: nil,
		serverConfigurator: newServerConfigurator(&mockManager{}, &mockSelectorFactory{selector: &queueSelector{}}),
	}
	gotMode, err := c.Configure(context.Background())
	if err == nil || err.Error() != "continuous configurator is not initialized" {
		t.Fatalf("expected initialization error, got %v", err)
	}
	if gotMode != mode.Unknown {
		t.Fatalf("expected mode.Unknown, got %v", gotMode)
	}
}

func TestConfigurator_ConfigureContinuous_NilServerConfigurator(t *testing.T) {
	c := &Configurator{
		useContinuousUI: true,
		clientConfigurator: newClientConfigurator(
			&cfgObserverMock{}, &cfgSelectorMock{}, nil, nil,
			&queuedSelectorFactory{selector: &queuedSelector{}}, nil, nil, nil,
		),
		serverConfigurator: nil,
	}
	gotMode, err := c.Configure(context.Background())
	if err == nil || err.Error() != "continuous configurator is not initialized" {
		t.Fatalf("expected initialization error, got %v", err)
	}
	if gotMode != mode.Unknown {
		t.Fatalf("expected mode.Unknown, got %v", gotMode)
	}
}

func TestConfigurator_WithContinuousUI(t *testing.T) {
	c := &Configurator{}
	c.withContinuousUI()
	if !c.useContinuousUI {
		t.Fatal("expected useContinuousUI=true after withContinuousUI()")
	}
}

// --- mock unified session for configureContinuous tests ---

type mockUnifiedSession struct {
	waitModeResult        mode.Mode
	waitModeErr           error
	waitRuntimeReconfigure bool
	waitRuntimeErr        error
	activateCalled        bool
	closeCalled           bool
}

func (m *mockUnifiedSession) WaitForMode() (mode.Mode, error) {
	return m.waitModeResult, m.waitModeErr
}

func (m *mockUnifiedSession) ActivateRuntime(_ context.Context, _ bubbleTea.RuntimeDashboardOptions) {
	m.activateCalled = true
}

func (m *mockUnifiedSession) WaitForRuntimeExit() (bool, error) {
	return m.waitRuntimeReconfigure, m.waitRuntimeErr
}

func (m *mockUnifiedSession) Close() { m.closeCalled = true }

func withMockUnifiedSession(t *testing.T, session unifiedSessionHandle) {
	t.Helper()
	prev := activeUnifiedSession
	activeUnifiedSession = session
	t.Cleanup(func() { activeUnifiedSession = prev })
}

func withMockNewUnifiedSession(t *testing.T, factory func(context.Context, bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error)) {
	t.Helper()
	prev := newUnifiedSession
	newUnifiedSession = factory
	t.Cleanup(func() { newUnifiedSession = prev })
}

func newContinuousConfigurator() *Configurator {
	return &Configurator{
		useContinuousUI: true,
		clientConfigurator: newClientConfigurator(
			&cfgObserverMock{}, &cfgSelectorMock{}, nil, nil,
			&queuedSelectorFactory{selector: &queuedSelector{}}, nil, nil, nil,
		),
		serverConfigurator: newServerConfigurator(&mockManager{}, &mockSelectorFactory{selector: &queueSelector{}}),
	}
}

func TestConfigureContinuous_HappyPath_ReturnsMode(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: mode.Server}
	withMockUnifiedSession(t, mock)

	c := newContinuousConfigurator()
	gotMode, err := c.Configure(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotMode != mode.Server {
		t.Fatalf("expected mode.Server, got %v", gotMode)
	}
	if mock.closeCalled {
		t.Fatal("expected Close not called on success")
	}
}

func TestConfigureContinuous_WaitForModeQuit_ReturnsErrUserExit(t *testing.T) {
	mock := &mockUnifiedSession{waitModeErr: bubbleTea.ErrUnifiedSessionQuit}
	withMockUnifiedSession(t, mock)

	c := newContinuousConfigurator()
	_, err := c.Configure(context.Background())
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called on quit")
	}
	if activeUnifiedSession != nil {
		t.Fatal("expected activeUnifiedSession cleared on quit")
	}
}

func TestConfigureContinuous_WaitForModeError_Propagates(t *testing.T) {
	mock := &mockUnifiedSession{waitModeErr: errors.New("unexpected failure")}
	withMockUnifiedSession(t, mock)

	c := newContinuousConfigurator()
	_, err := c.Configure(context.Background())
	if err == nil || err.Error() != "unexpected failure" {
		t.Fatalf("expected 'unexpected failure', got %v", err)
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called on error")
	}
	if activeUnifiedSession != nil {
		t.Fatal("expected activeUnifiedSession cleared on error")
	}
}

func TestConfigureContinuous_CreatesNewSession_WhenNil(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: mode.Client}
	withMockUnifiedSession(t, nil) // ensure activeUnifiedSession is nil
	withMockNewUnifiedSession(t, func(_ context.Context, _ bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		return mock, nil
	})

	c := newContinuousConfigurator()
	gotMode, err := c.Configure(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotMode != mode.Client {
		t.Fatalf("expected mode.Client, got %v", gotMode)
	}
}

func TestConfigureContinuous_NewSessionError_Propagates(t *testing.T) {
	withMockUnifiedSession(t, nil) // ensure activeUnifiedSession is nil
	withMockNewUnifiedSession(t, func(_ context.Context, _ bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		return nil, errors.New("session creation failed")
	})

	c := newContinuousConfigurator()
	_, err := c.Configure(context.Background())
	if err == nil || err.Error() != "session creation failed" {
		t.Fatalf("expected 'session creation failed', got %v", err)
	}
}

func TestConfigureContinuous_ReusesExistingSession(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: mode.Server}
	withMockUnifiedSession(t, mock)
	factoryCalled := false
	withMockNewUnifiedSession(t, func(_ context.Context, _ bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		factoryCalled = true
		return nil, errors.New("should not be called")
	})

	c := newContinuousConfigurator()
	gotMode, err := c.Configure(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotMode != mode.Server {
		t.Fatalf("expected mode.Server, got %v", gotMode)
	}
	if factoryCalled {
		t.Fatal("expected factory NOT called when session exists")
	}
}
