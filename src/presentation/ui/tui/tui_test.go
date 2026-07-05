package tui

import (
	"context"
	"errors"
	"testing"

	clientConfiguration "tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/PAL/platform"
	systemdDomain "tungo/infrastructure/PAL/service_management/linux/systemd/domain"
	"tungo/infrastructure/settings"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
	"tungo/runtime"
)

func TestNewTUI(t *testing.T) {
	ui, err := New()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ui == nil {
		t.Fatal("expected non-nil TUI")
	}
	if !ui.initialized() {
		t.Fatal("expected TUI to be initialized")
	}
	if ui.sessionOptions.ServerSupported != platform.Capabilities().ServerModeSupported() {
		t.Fatal("expected serverSupported to match platform capabilities")
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

	statusRet   systemdDomain.UnitStatus
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

func (s *systemdInstallerStub) Status() (systemdDomain.UnitStatus, error) {
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
		Observer:            &cfgObserverMock{},
		Selector:            &cfgSelectorMock{},
		Creator:             &cfgCreatorMock{},
		Deleter:             &cfgDeleterMock{},
		ClientConfigManager: clientConfiguration.NewManager(),
		ServerConfigManager: &mockManager{},
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
		statusRet: systemdDomain.UnitStatus{
			Installed:     true,
			Managed:       true,
			UnitFileState: "enabled",
			ActiveState:   "active",
			Role:          systemdDomain.UnitRoleServer,
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

	installer.statusRet.Role = systemdDomain.UnitRoleClient
	status, err = captured.GetSystemdDaemonStatus()
	if err != nil {
		t.Fatalf("unexpected status error: %v", err)
	}
	if status.Mode != runtime.ModeClient {
		t.Fatalf("expected mapped client mode, got %v", status.Mode)
	}

	installer.statusRet.Role = systemdDomain.UnitRoleUnknown
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

type mockRuntimeLifecycle struct {
	session runtime.Session
	modes   []runtime.Mode
	err     error
}

func (m *mockRuntimeLifecycle) Start(_ context.Context, mode runtime.Mode) (runtime.Session, error) {
	m.modes = append(m.modes, mode)
	if m.err != nil {
		return nil, m.err
	}
	return m.session, nil
}

type mockRuntimeSession struct {
	info       runtime.Info
	readyCh    chan struct{}
	doneCh     chan struct{}
	err        error
	stopCalled bool
}

func newMockRuntimeSession(info runtime.Info, err error) *mockRuntimeSession {
	readyCh := make(chan struct{})
	close(readyCh)
	return &mockRuntimeSession{
		info:    info,
		readyCh: readyCh,
		doneCh:  make(chan struct{}),
		err:     err,
	}
}

func (m *mockRuntimeSession) Info() runtime.Info {
	return m.info
}

func (m *mockRuntimeSession) Ready() <-chan struct{} {
	return m.readyCh
}

func (m *mockRuntimeSession) Done() <-chan struct{} {
	return m.doneCh
}

func (m *mockRuntimeSession) Err() error {
	return m.err
}

func (m *mockRuntimeSession) Stop() {
	if !m.stopCalled {
		m.stopCalled = true
		close(m.doneCh)
	}
}

func TestTUI_Run_StartsRuntimeLifecycle(t *testing.T) {
	uiSession := &mockUnifiedSession{
		waitModeResult: runtime.ModeClient,
		waitRuntimeErr: bubbleTea.ErrUnifiedSessionQuit,
	}
	ui := newTestTUI()
	ui.sessionOptions.ServerSupported = true
	withMockUnifiedSession(t, ui, uiSession)

	runtimeSession := newMockRuntimeSession(runtime.Info{Mode: runtime.ModeClient, Protocol: settings.TCP}, context.Canceled)
	lifecycle := &mockRuntimeLifecycle{session: runtimeSession}

	err := ui.Run(context.Background(), lifecycle)
	if err != nil {
		t.Fatalf("expected clean user exit, got %v", err)
	}
	if len(lifecycle.modes) != 1 || lifecycle.modes[0] != runtime.ModeClient {
		t.Fatalf("expected lifecycle to start client mode, got %v", lifecycle.modes)
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
	if uiSession.activatedOptions.ReadyCh != runtimeSession.readyCh {
		t.Fatal("expected runtime ready channel forwarded")
	}
	if !runtimeSession.stopCalled {
		t.Fatal("expected runtime session stopped after user exit")
	}
}

func TestTUI_Run_NilLifecycle_ReturnsError(t *testing.T) {
	ui := newTestTUI()

	err := ui.Run(context.Background(), nil)
	if err == nil || err.Error() != "runtime lifecycle is nil" {
		t.Fatalf("expected nil lifecycle error, got %v", err)
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
