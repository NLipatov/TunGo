package tui

import (
	"context"
	"errors"
	"testing"

	appConfiguration "tungo/application/configuration"
	"tungo/application/runtime"
	"tungo/infrastructure/PAL/service_management/linux/systemd"
	"tungo/infrastructure/settings"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
)

func TestNewTUI(t *testing.T) {
	ui, err := New(configurationControlsMock(true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ui == nil {
		t.Fatal("expected non-nil TUI")
	}
	if !ui.initialized() {
		t.Fatal("expected TUI to be initialized")
	}
	if !ui.sessionOptions.ServerSupported {
		t.Fatal("expected serverSupported to come from configuration control")
	}
}

func TestNewTUIRejectsMissingClientControl(t *testing.T) {
	ui, err := New(appConfiguration.Controls{})
	if err == nil || ui != nil {
		t.Fatalf("New() = %v, %v; want nil and error", ui, err)
	}
}

func TestDefaultFactories(t *testing.T) {
	if installer := newDefaultSystemdInstaller(); installer == nil {
		t.Fatal("newDefaultSystemdInstaller() returned nil")
	}
	if _, err := newBubbleTeaUnifiedSession(context.Background(), bubbleTea.ConfiguratorSessionOptions{}); err == nil {
		t.Fatal("newBubbleTeaUnifiedSession() accepted incomplete options")
	}
}

func TestTUI_Configure_NilSessionOptions(t *testing.T) {
	ui := &TUI{
		sessionFactory:          dummySessionFactory,
		systemdInstallerFactory: dummySystemdInstallerFactory,
	}

	gotMode, err := ui.configure(context.Background())
	if err == nil || err.Error() != "tui is not initialized" {
		t.Fatalf("expected initialization error, got %v", err)
	}
	if gotMode != 0 {
		t.Fatalf("expected 0, got %v", gotMode)
	}
}

func TestTUI_Configure_NilFactories(t *testing.T) {
	ui := &TUI{
		sessionOptions: testSessionOptions(),
	}

	gotMode, err := ui.configure(context.Background())
	if err == nil || err.Error() != "tui is not initialized" {
		t.Fatalf("expected initialization error, got %v", err)
	}
	if gotMode != 0 {
		t.Fatalf("expected 0, got %v", gotMode)
	}
}

type mockUnifiedSession struct {
	waitModeResult runtime.Mode
	waitModeErr    error

	activatedOptions       bubbleTea.RuntimeDashboardOptions
	waitRuntimeReconfigure bool
	waitRuntimeErr         error

	activateCalled bool
	closeCalled    bool
}

func (m *mockUnifiedSession) WaitForMode() (runtime.Mode, error) {
	return m.waitModeResult, m.waitModeErr
}

func (m *mockUnifiedSession) ActivateRuntime(_ context.Context, options bubbleTea.RuntimeDashboardOptions) {
	m.activateCalled = true
	m.activatedOptions = options
}

func (m *mockUnifiedSession) WaitForRuntimeExit() (bool, error) {
	return m.waitRuntimeReconfigure, m.waitRuntimeErr
}

func (m *mockUnifiedSession) ShowFatalError(_ string) {}

func (m *mockUnifiedSession) Close() { m.closeCalled = true }

type scriptedUnifiedSession struct {
	modes                []runtime.Mode
	modeErrs             []error
	runtimeReconfigs     []bool
	runtimeErrs          []error
	waitModeCalls        int
	waitRuntimeExitCalls int
	activatedOptions     []bubbleTea.RuntimeDashboardOptions
	closeCalled          bool
}

func (s *scriptedUnifiedSession) WaitForMode() (runtime.Mode, error) {
	call := s.waitModeCalls
	s.waitModeCalls++
	if call < len(s.modeErrs) && s.modeErrs[call] != nil {
		return 0, s.modeErrs[call]
	}
	if call < len(s.modes) {
		return s.modes[call], nil
	}
	return 0, errors.New("unexpected WaitForMode call")
}

func (s *scriptedUnifiedSession) ActivateRuntime(_ context.Context, options bubbleTea.RuntimeDashboardOptions) {
	s.activatedOptions = append(s.activatedOptions, options)
}

func (s *scriptedUnifiedSession) WaitForRuntimeExit() (bool, error) {
	call := s.waitRuntimeExitCalls
	s.waitRuntimeExitCalls++
	if call < len(s.runtimeErrs) && s.runtimeErrs[call] != nil {
		return false, s.runtimeErrs[call]
	}
	if call < len(s.runtimeReconfigs) {
		return s.runtimeReconfigs[call], nil
	}
	return false, errors.New("unexpected WaitForRuntimeExit call")
}

func (s *scriptedUnifiedSession) ShowFatalError(_ string) {}

func (s *scriptedUnifiedSession) Close() { s.closeCalled = true }

type contextWaitingUnifiedSession struct {
	ctx context.Context
	err error
}

func (s *contextWaitingUnifiedSession) WaitForMode() (runtime.Mode, error) {
	return runtime.ModeClient, nil
}

func (s *contextWaitingUnifiedSession) ActivateRuntime(ctx context.Context, _ bubbleTea.RuntimeDashboardOptions) {
	s.ctx = ctx
}

func (s *contextWaitingUnifiedSession) WaitForRuntimeExit() (bool, error) {
	if s.ctx != nil {
		<-s.ctx.Done()
	}
	return false, s.err
}

func (s *contextWaitingUnifiedSession) ShowFatalError(_ string) {}

func (s *contextWaitingUnifiedSession) Close() {}

func withMockUnifiedSession(t *testing.T, ui *TUI, session unifiedSessionHandle) {
	t.Helper()
	ui.session = session
}

func withMockUnifiedSessionFactory(t *testing.T, ui *TUI, factory unifiedSessionFactory) {
	t.Helper()
	ui.sessionFactory = factory
}

func withMockSystemdInstallerFactory(t *testing.T, ui *TUI, factory systemdInstallerFactory) {
	t.Helper()
	ui.systemdInstallerFactory = factory
}

type systemdInstallerStub struct {
	supported bool

	statusRet   systemd.UnitStatus
	statusErr   error
	statusCalls int

	installServerPath string
	installServerErr  error
	installClientPath string
	installClientErr  error

	activeRet   bool
	activeErr   error
	activeCalls int

	stopErr    error
	startErr   error
	enableErr  error
	disableErr error
	removeErr  error
}

func (s *systemdInstallerStub) Supported() bool { return s.supported }

func (s *systemdInstallerStub) InstallServerUnit() (string, error) {
	if s.installServerPath == "" && s.installServerErr == nil {
		return "/etc/systemd/system/tungo.service", nil
	}
	return s.installServerPath, s.installServerErr
}

func (s *systemdInstallerStub) InstallClientUnit() (string, error) {
	if s.installClientPath == "" && s.installClientErr == nil {
		return "/etc/systemd/system/tungo.service", nil
	}
	return s.installClientPath, s.installClientErr
}

func (s *systemdInstallerStub) RemoveUnit() error { return s.removeErr }

func (s *systemdInstallerStub) IsUnitActive() (bool, error) {
	s.activeCalls++
	return s.activeRet, s.activeErr
}

func (s *systemdInstallerStub) StopUnit() error    { return s.stopErr }
func (s *systemdInstallerStub) StartUnit() error   { return s.startErr }
func (s *systemdInstallerStub) EnableUnit() error  { return s.enableErr }
func (s *systemdInstallerStub) DisableUnit() error { return s.disableErr }

func (s *systemdInstallerStub) Status() (systemd.UnitStatus, error) {
	s.statusCalls++
	return s.statusRet, s.statusErr
}

func newTestTUI() *TUI {
	return &TUI{
		sessionOptions: testSessionOptions(),
		sessionFactory: func(context.Context, bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
			return nil, errors.New("session factory not configured")
		},
		systemdInstallerFactory: dummySystemdInstallerFactory,
		newRuntime: func(runtime.Mode) (runtime.Runtime, error) {
			return nil, errors.New("runtime factory not configured")
		},
	}
}

func dummySessionFactory(context.Context, bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
	return nil, errors.New("session factory not configured")
}

func dummySystemdInstallerFactory() systemdInstaller {
	return &systemdInstallerStub{}
}

func testSessionOptions() bubbleTea.ConfiguratorSessionOptions {
	return bubbleTea.ConfiguratorSessionOptions{
		ClientConfigurationControl: configurationControlMock{},
		ServerConfigurationControl: configurationControlMock{},
		ServerSupported:            true,
	}
}

func TestTUI_Configure_HappyPath_ReturnsMode(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeServer}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	gotMode, err := ui.configure(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotMode != runtime.ModeServer {
		t.Fatalf("expected runtime.ModeServer, got %v", gotMode)
	}
	if mock.closeCalled {
		t.Fatal("expected Close not called on success")
	}
}

func TestTUI_Configure_WaitForModeQuit_ReturnsErrUserExit(t *testing.T) {
	mock := &mockUnifiedSession{waitModeErr: bubbleTea.ErrUnifiedSessionQuit}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	_, err := ui.configure(context.Background())
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called on quit")
	}
	if ui.session != nil {
		t.Fatal("expected session cleared on quit")
	}
}

func TestTUI_Configure_WaitForModeClosed_ReturnsErrUserExit(t *testing.T) {
	mock := &mockUnifiedSession{waitModeErr: bubbleTea.ErrUnifiedSessionClosed}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	_, err := ui.configure(context.Background())
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called on closed session")
	}
	if ui.session != nil {
		t.Fatal("expected session cleared on closed session")
	}
}

func TestTUI_Configure_WaitForModeError_Propagates(t *testing.T) {
	mock := &mockUnifiedSession{waitModeErr: errors.New("unexpected failure")}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	_, err := ui.configure(context.Background())
	if err == nil || err.Error() != "unexpected failure" {
		t.Fatalf("expected 'unexpected failure', got %v", err)
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called on error")
	}
	if ui.session != nil {
		t.Fatal("expected session cleared on error")
	}
}

func TestTUI_Configure_CreatesNewSession_WhenNil(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeClient}
	ui := newTestTUI()
	withMockUnifiedSessionFactory(t, ui, func(_ context.Context, _ bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		return mock, nil
	})

	gotMode, err := ui.configure(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotMode != runtime.ModeClient {
		t.Fatalf("expected runtime.ModeClient, got %v", gotMode)
	}
	if ui.session != mock {
		t.Fatal("expected session stored on TUI")
	}
}

func TestTUI_Configure_NewSessionError_Propagates(t *testing.T) {
	ui := newTestTUI()
	withMockUnifiedSessionFactory(t, ui, func(_ context.Context, _ bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		return nil, errors.New("session creation failed")
	})

	_, err := ui.configure(context.Background())
	if err == nil || err.Error() != "session creation failed" {
		t.Fatalf("expected 'session creation failed', got %v", err)
	}
}

func TestTUI_Configure_ReusesExistingSession(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeServer}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)
	factoryCalled := false
	withMockUnifiedSessionFactory(t, ui, func(_ context.Context, _ bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		factoryCalled = true
		return nil, errors.New("should not be called")
	})

	gotMode, err := ui.configure(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotMode != runtime.ModeServer {
		t.Fatalf("expected runtime.ModeServer, got %v", gotMode)
	}
	if factoryCalled {
		t.Fatal("expected factory NOT called when session exists")
	}
}

func TestTUI_Configure_SystemdSupported_WiresCallbacks(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeClient}
	ui := newTestTUI()
	ui.sessionOptions.ServerSupported = true

	installer := &systemdInstallerStub{
		supported: true,
		statusRet: systemd.UnitStatus{
			Installed:     true,
			Managed:       true,
			UnitFileState: "enabled",
			ActiveState:   "active",
			Role:          systemd.UnitRoleServer,
			ExecStart:     "/usr/local/bin/tungo s",
			FragmentPath:  "/etc/systemd/system/tungo.service",
		},
	}
	withMockSystemdInstallerFactory(t, ui, func() systemdInstaller { return installer })

	var captured bubbleTea.ConfiguratorSessionOptions
	withMockUnifiedSessionFactory(t, ui, func(_ context.Context, opts bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		captured = opts
		return mock, nil
	})

	gotMode, err := ui.configure(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotMode != runtime.ModeClient {
		t.Fatalf("expected runtime.ModeClient, got %v", gotMode)
	}
	if !captured.SystemdSupported {
		t.Fatal("expected SystemdSupported=true in session options")
	}
	if captured.GetSystemdDaemonStatus == nil ||
		captured.InstallClientSystemdUnit == nil ||
		captured.CheckSystemdUnitActive == nil ||
		captured.StopSystemdUnit == nil ||
		captured.StartSystemdUnit == nil ||
		captured.EnableSystemdUnit == nil ||
		captured.DisableSystemdUnit == nil ||
		captured.RemoveSystemdUnit == nil {
		t.Fatal("expected systemd callbacks to be wired when supported")
	}
	if captured.InstallServerSystemdUnit == nil {
		t.Fatal("expected server unit installer wired when server is supported")
	}

	installer.activeRet = true
	active, err := captured.CheckSystemdUnitActive()
	if err != nil {
		t.Fatalf("unexpected active-check error: %v", err)
	}
	if !active {
		t.Fatal("expected active check to reflect installer IsUnitActive result")
	}
	if installer.activeCalls != 1 {
		t.Fatalf("expected exactly one IsUnitActive call, got %d", installer.activeCalls)
	}
	if installer.statusCalls != 0 {
		t.Fatalf("expected active check to avoid Status calls, got %d", installer.statusCalls)
	}

	status, err := captured.GetSystemdDaemonStatus()
	if err != nil {
		t.Fatalf("unexpected status error: %v", err)
	}
	if !status.Installed ||
		!status.Managed ||
		status.UnitFileState != "enabled" ||
		status.ActiveState != "active" ||
		status.Mode != runtime.ModeServer ||
		status.ExecStart != "/usr/local/bin/tungo s" ||
		status.FragmentPath != "/etc/systemd/system/tungo.service" {
		t.Fatalf("unexpected mapped daemon status: %+v", status)
	}

	installer.statusRet.Role = systemd.UnitRoleClient
	status, err = captured.GetSystemdDaemonStatus()
	if err != nil {
		t.Fatalf("unexpected status error: %v", err)
	}
	if status.Mode != runtime.ModeClient {
		t.Fatalf("expected mapped client mode, got %v", status.Mode)
	}

	installer.statusRet.Role = systemd.UnitRoleUnknown
	status, err = captured.GetSystemdDaemonStatus()
	if err != nil {
		t.Fatalf("unexpected status error: %v", err)
	}
	if status.Mode != 0 {
		t.Fatalf("expected mapped unknown mode, got %v", status.Mode)
	}

	installer.statusErr = errors.New("status failed")
	_, err = captured.GetSystemdDaemonStatus()
	if err == nil || err.Error() != "status failed" {
		t.Fatalf("expected status failure from closure, got %v", err)
	}
}

func TestTUI_Configure_SystemdUnsupported_DoesNotWireCallbacks(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeServer}
	ui := newTestTUI()
	ui.sessionOptions.ServerSupported = true

	installer := &systemdInstallerStub{supported: false}
	withMockSystemdInstallerFactory(t, ui, func() systemdInstaller { return installer })

	var captured bubbleTea.ConfiguratorSessionOptions
	withMockUnifiedSessionFactory(t, ui, func(_ context.Context, opts bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		captured = opts
		return mock, nil
	})

	gotMode, err := ui.configure(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotMode != runtime.ModeServer {
		t.Fatalf("expected runtime.ModeServer, got %v", gotMode)
	}
	if captured.SystemdSupported {
		t.Fatal("expected SystemdSupported=false in session options")
	}
	if captured.GetSystemdDaemonStatus != nil ||
		captured.InstallClientSystemdUnit != nil ||
		captured.InstallServerSystemdUnit != nil ||
		captured.CheckSystemdUnitActive != nil ||
		captured.StopSystemdUnit != nil ||
		captured.StartSystemdUnit != nil ||
		captured.EnableSystemdUnit != nil ||
		captured.DisableSystemdUnit != nil ||
		captured.RemoveSystemdUnit != nil {
		t.Fatal("expected systemd callbacks to be nil when unsupported")
	}
}

func TestTUI_Close_ClosesAndClearsSession(t *testing.T) {
	mock := &mockUnifiedSession{}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	ui.Close()
	if !mock.closeCalled {
		t.Fatal("expected Close() to call underlying session Close")
	}
	if ui.session != nil {
		t.Fatal("expected session handle to be cleared")
	}
}

func TestTUI_Close_IdempotentWithNilSession(t *testing.T) {
	ui := newTestTUI()

	ui.Close()
	ui.Close()
}

type mockRuntimeFactory struct {
	runtime runtime.Runtime
	modes   []runtime.Mode
	err     error
}

func (m *mockRuntimeFactory) New(mode runtime.Mode) (runtime.Runtime, error) {
	m.modes = append(m.modes, mode)
	if m.err != nil {
		return nil, m.err
	}
	return m.runtime, nil
}

type mockRuntime struct {
	err       error
	ready     bool
	runCalled bool
}

func newMockRuntime(err error) *mockRuntime {
	return &mockRuntime{err: err}
}

func (m *mockRuntime) Ready() bool { return m.ready }

func (m *mockRuntime) Run(ctx context.Context) error {
	m.runCalled = true
	<-ctx.Done()
	return m.err
}

type completedRuntime struct {
	err       error
	runCalled bool
}

func newCompletedRuntime(err error) *completedRuntime {
	return &completedRuntime{err: err}
}

func (*completedRuntime) Ready() bool { return true }

func (r *completedRuntime) Run(context.Context) error {
	r.runCalled = true
	return r.err
}

type scriptedRuntimeFactory struct {
	runtimes []runtime.Runtime
	modes    []runtime.Mode
}

func (s *scriptedRuntimeFactory) New(mode runtime.Mode) (runtime.Runtime, error) {
	s.modes = append(s.modes, mode)
	if len(s.runtimes) == 0 {
		return nil, errors.New("unexpected runtime factory call")
	}
	runtimeInstance := s.runtimes[0]
	s.runtimes = s.runtimes[1:]
	return runtimeInstance, nil
}

type runtimeInfoErrorControl struct {
	configurationControlMock
	err error
}

func (c runtimeInfoErrorControl) RuntimeInfo() (appConfiguration.RuntimeInfo, error) {
	return appConfiguration.RuntimeInfo{}, c.err
}

func TestTUI_Run_RunsRuntime(t *testing.T) {
	uiSession := &mockUnifiedSession{
		waitModeResult: runtime.ModeClient,
		waitRuntimeErr: bubbleTea.ErrUnifiedSessionQuit,
	}
	ui := newTestTUI()
	ui.sessionOptions.ServerSupported = true
	withMockUnifiedSession(t, ui, uiSession)

	runtimeInstance := newMockRuntime(nil)
	runtimeInstance.ready = true
	factory := &mockRuntimeFactory{runtime: runtimeInstance}
	ui.newRuntime = factory.New

	err := ui.Run(context.Background())
	if err != nil {
		t.Fatalf("expected clean user exit, got %v", err)
	}
	if len(factory.modes) != 1 || factory.modes[0] != runtime.ModeClient {
		t.Fatalf("expected factory to create client mode, got %v", factory.modes)
	}
	if !uiSession.activateCalled {
		t.Fatal("expected dashboard activation")
	}
	if uiSession.activatedOptions.Mode != runtime.ModeClient {
		t.Fatalf("expected client dashboard mode, got %v", uiSession.activatedOptions.Mode)
	}
	if uiSession.activatedOptions.Protocol != settings.TCP {
		t.Fatalf("expected protocol forwarded, got %v", uiSession.activatedOptions.Protocol)
	}
	if !uiSession.activatedOptions.ServerSupported {
		t.Fatal("expected ServerSupported forwarded to dashboard")
	}
	if uiSession.activatedOptions.Ready == nil {
		t.Fatal("expected runtime readiness callback forwarded")
	}
	if !uiSession.activatedOptions.Ready() {
		t.Fatal("forwarded runtime readiness callback returned false")
	}
	if !runtimeInstance.runCalled {
		t.Fatal("expected runtime to run")
	}
}

func TestTUI_Run_ReconfigureStartsNextRuntime(t *testing.T) {
	uiSession := &scriptedUnifiedSession{
		modes:            []runtime.Mode{runtime.ModeClient, runtime.ModeServer},
		runtimeReconfigs: []bool{true},
		runtimeErrs:      []error{nil, bubbleTea.ErrUnifiedSessionQuit},
	}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, uiSession)

	clientRuntime := newMockRuntime(nil)
	serverRuntime := newMockRuntime(nil)
	factory := &scriptedRuntimeFactory{runtimes: []runtime.Runtime{clientRuntime, serverRuntime}}
	ui.newRuntime = factory.New

	err := ui.Run(context.Background())
	if err != nil {
		t.Fatalf("expected clean exit after reconfigure, got %v", err)
	}
	if len(factory.modes) != 2 ||
		factory.modes[0] != runtime.ModeClient ||
		factory.modes[1] != runtime.ModeServer {
		t.Fatalf("expected client then server runtimes, got %v", factory.modes)
	}
	if len(uiSession.activatedOptions) != 2 {
		t.Fatalf("expected two runtime dashboard activations, got %d", len(uiSession.activatedOptions))
	}
	if !clientRuntime.runCalled || !serverRuntime.runCalled {
		t.Fatal("expected both runtimes to run")
	}
}

func TestTUI_Run_ConfigureUserExit_ReturnsNil(t *testing.T) {
	uiSession := &mockUnifiedSession{waitModeErr: bubbleTea.ErrUnifiedSessionQuit}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, uiSession)

	factory := &mockRuntimeFactory{}
	ui.newRuntime = factory.New

	err := ui.Run(context.Background())
	if err != nil {
		t.Fatalf("expected clean user exit, got %v", err)
	}
	if len(factory.modes) != 0 {
		t.Fatalf("expected runtime not to be created, got modes %v", factory.modes)
	}
}

func TestTUI_Run_ConfigureError_ReturnsWrappedError(t *testing.T) {
	uiSession := &mockUnifiedSession{waitModeErr: errors.New("select failed")}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, uiSession)

	err := ui.Run(context.Background())
	if err == nil || err.Error() != "configuration error: select failed" {
		t.Fatalf("expected wrapped configuration error, got %v", err)
	}
}

func TestTUI_Run_ConfigureSessionClosed_ReturnsShutdownError(t *testing.T) {
	ui := newTestTUI()
	withMockUnifiedSessionFactory(t, ui, func(context.Context, bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		return nil, ErrSessionClosed
	})

	err := ui.Run(context.Background())
	if err == nil || err.Error() != "ui session ended during shutdown: unified session closed" {
		t.Fatalf("expected session closed error, got %v", err)
	}
}

func TestTUI_Run_RuntimeCreationError_ReturnsError(t *testing.T) {
	uiSession := &mockUnifiedSession{waitModeResult: runtime.ModeClient}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, uiSession)

	factory := &mockRuntimeFactory{err: errors.New("runtime creation failed")}
	ui.newRuntime = factory.New

	err := ui.Run(context.Background())
	if err == nil || err.Error() != "runtime creation failed" {
		t.Fatalf("expected runtime creation error, got %v", err)
	}
	if len(factory.modes) != 1 || factory.modes[0] != runtime.ModeClient {
		t.Fatalf("expected client creation attempt, got %v", factory.modes)
	}
}

func TestTUI_Run_CanceledContext_ReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ui := newTestTUI()

	err := ui.Run(ctx)
	if err != nil {
		t.Fatalf("expected canceled context to stop run loop cleanly, got %v", err)
	}
}

func TestTUI_RunRuntime_RuntimeInfoError(t *testing.T) {
	want := errors.New("runtime info failed")
	ui := newTestTUI()
	ui.sessionOptions.ClientConfigurationControl = runtimeInfoErrorControl{err: want}

	factory := &mockRuntimeFactory{}
	ui.newRuntime = factory.New

	err := ui.runRuntime(context.Background(), runtime.ModeClient)
	if err == nil || err.Error() != "runtime info error: runtime info failed" {
		t.Fatalf("expected runtime info error, got %v", err)
	}
	if len(factory.modes) != 0 {
		t.Fatalf("expected runtime not to be created, got modes %v", factory.modes)
	}
}

func TestTUI_RunRuntime_NewRuntimeError(t *testing.T) {
	ui := newTestTUI()
	factory := &mockRuntimeFactory{err: errors.New("runtime creation failed")}
	ui.newRuntime = factory.New

	err := ui.runRuntime(context.Background(), runtime.ModeClient)
	if err == nil || err.Error() != "runtime creation failed" {
		t.Fatalf("expected runtime creation error, got %v", err)
	}
	if len(factory.modes) != 1 || factory.modes[0] != runtime.ModeClient {
		t.Fatalf("expected client creation attempt, got %v", factory.modes)
	}
}

func TestTUI_RunRuntime_RuntimeUIErrorAfterWorkerExit(t *testing.T) {
	uiSession := &contextWaitingUnifiedSession{err: errors.New("dashboard failed")}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, uiSession)

	runtimeInstance := newCompletedRuntime(nil)
	factory := &mockRuntimeFactory{runtime: runtimeInstance}
	ui.newRuntime = factory.New

	err := ui.runRuntime(context.Background(), runtime.ModeClient)
	if err == nil || err.Error() != "runtime UI failed: dashboard failed" {
		t.Fatalf("expected runtime UI error, got %v", err)
	}
	if !runtimeInstance.runCalled {
		t.Fatal("expected runtime to run")
	}
}

func TestTUI_Run_NilRuntimeFactory_ReturnsError(t *testing.T) {
	ui := newTestTUI()
	ui.newRuntime = nil

	err := ui.Run(context.Background())
	if err == nil || err.Error() != "runtime factory is nil" {
		t.Fatalf("expected nil runtime factory error, got %v", err)
	}
}

func TestTUI_RunRuntimePhase_WithoutSession_ReturnsError(t *testing.T) {
	ui := newTestTUI()

	reconfigure, err := ui.runRuntimePhase(context.Background(), bubbleTea.RuntimeDashboardOptions{})
	if err == nil || err.Error() != "runtime dashboard requires active tui session" {
		t.Fatalf("expected active session error, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
}

func TestTUI_RunRuntimePhase_HappyPath_Reconfigure(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeReconfigure: true}
	ui := newTestTUI()
	ui.sessionOptions.ServerSupported = true
	withMockUnifiedSession(t, ui, mock)

	reconfigure, err := ui.runRuntimePhase(context.Background(), bubbleTea.RuntimeDashboardOptions{
		Mode:            runtime.ModeServer,
		ServerSupported: true,
		Protocol:        settings.UDP,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !reconfigure {
		t.Fatal("expected reconfigure=true")
	}
	if !mock.activateCalled {
		t.Fatal("expected ActivateRuntime called")
	}
	if mock.activatedOptions.Mode != runtime.ModeServer {
		t.Fatalf("expected server mode mapping, got %v", mock.activatedOptions.Mode)
	}
	if mock.activatedOptions.Protocol != settings.UDP {
		t.Fatalf("expected protocol forwarded, got %v", mock.activatedOptions.Protocol)
	}
	if !mock.activatedOptions.ServerSupported {
		t.Fatal("expected ServerSupported forwarded")
	}
}

func TestTUI_RunRuntimePhase_Quit_ReturnsErrUserExit(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeErr: bubbleTea.ErrUnifiedSessionQuit}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	reconfigure, err := ui.runRuntimePhase(context.Background(), bubbleTea.RuntimeDashboardOptions{
		Mode: runtime.ModeClient,
	})
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called")
	}
	if ui.session != nil {
		t.Fatal("expected session cleared")
	}
}

func TestTUI_RunRuntimePhase_Closed_ReturnsErrUserExit(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeErr: bubbleTea.ErrUnifiedSessionClosed}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	reconfigure, err := ui.runRuntimePhase(context.Background(), bubbleTea.RuntimeDashboardOptions{
		Mode: runtime.ModeClient,
	})
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called")
	}
	if ui.session != nil {
		t.Fatal("expected session cleared")
	}
}

func TestTUI_RunRuntimePhase_Disconnected_KeepsSession(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeErr: bubbleTea.ErrUnifiedSessionRuntimeDisconnected}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	reconfigure, err := ui.runRuntimePhase(context.Background(), bubbleTea.RuntimeDashboardOptions{
		Mode: runtime.ModeServer,
	})
	if err != nil {
		t.Fatalf("expected nil error for disconnect, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false for disconnect")
	}
	if mock.closeCalled {
		t.Fatal("expected Close NOT called on disconnect")
	}
	if ui.session == nil {
		t.Fatal("expected session preserved on disconnect")
	}
}

func TestTUI_RunRuntimePhase_GenericError_ClearsSession(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeErr: errors.New("unexpected")}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	reconfigure, err := ui.runRuntimePhase(context.Background(), bubbleTea.RuntimeDashboardOptions{
		Mode: runtime.ModeClient,
	})
	if err == nil || err.Error() != "unexpected" {
		t.Fatalf("expected 'unexpected', got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called")
	}
	if ui.session != nil {
		t.Fatal("expected session cleared")
	}
}

func TestTUI_RunRuntimePhase_NoError_ReturnsReconfigure(t *testing.T) {
	mock := &mockUnifiedSession{waitRuntimeReconfigure: false}
	ui := newTestTUI()
	withMockUnifiedSession(t, ui, mock)

	reconfigure, err := ui.runRuntimePhase(context.Background(), bubbleTea.RuntimeDashboardOptions{
		Mode: runtime.ModeServer,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if reconfigure {
		t.Fatal("expected reconfigure=false")
	}
}

func TestTUI_RuntimeInfo_Client(t *testing.T) {
	ui := newTestTUI()

	got, err := ui.runtimeInfo(runtime.ModeClient)
	if err != nil {
		t.Fatalf("runtimeInfo() error = %v", err)
	}
	if got.Protocol != settings.TCP {
		t.Fatalf("expected client protocol TCP, got %v", got.Protocol)
	}
}

func TestTUI_RuntimeInfo_Server(t *testing.T) {
	ui := newTestTUI()

	got, err := ui.runtimeInfo(runtime.ModeServer)
	if err != nil {
		t.Fatalf("runtimeInfo() error = %v", err)
	}
	if got.Protocol != settings.TCP {
		t.Fatalf("expected server protocol TCP, got %v", got.Protocol)
	}
}

func TestTUI_RuntimeInfo_MissingClientControl(t *testing.T) {
	ui := newTestTUI()
	ui.sessionOptions.ClientConfigurationControl = nil

	_, err := ui.runtimeInfo(runtime.ModeClient)
	if err == nil || err.Error() != "client configuration control is nil" {
		t.Fatalf("expected missing client control error, got %v", err)
	}
}

func TestTUI_RuntimeInfo_MissingServerControl(t *testing.T) {
	ui := newTestTUI()
	ui.sessionOptions.ServerConfigurationControl = nil

	_, err := ui.runtimeInfo(runtime.ModeServer)
	if err == nil || err.Error() != "server configuration control is nil" {
		t.Fatalf("expected missing server control error, got %v", err)
	}
}

func TestTUI_RuntimeInfo_InvalidMode(t *testing.T) {
	ui := newTestTUI()

	_, err := ui.runtimeInfo(0)
	if err == nil || err.Error() != "invalid runtime mode: 0" {
		t.Fatalf("expected invalid runtime mode error, got %v", err)
	}
}
