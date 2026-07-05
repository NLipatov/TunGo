package tui

import (
	"context"
	"errors"
	"testing"

	systemdDomain "tungo/infrastructure/PAL/service_management/linux/systemd/domain"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
	"tungo/runtime"
)

func TestNewConfigurator(t *testing.T) {
	runtimeUI := NewRuntimeUI()
	c := NewConfigurator(&mockManager{}, true, runtimeUI)
	if c == nil {
		t.Fatal("expected non-nil configurator")
	}
	if c.clientConfigurator == nil || c.serverConfigurator == nil {
		t.Fatal("expected inner configurators to be initialized")
	}
	if c.runtimeUI != runtimeUI {
		t.Fatal("expected runtime UI to be wired")
	}
	if !c.serverSupported {
		t.Fatal("expected serverSupported to be forwarded")
	}
}

func TestConfigurator_Configure_NilClientConfigurator(t *testing.T) {
	c := &Configurator{
		clientConfigurator: nil,
		serverConfigurator: newServerConfigurator(&mockManager{}, &mockSelectorFactory{selector: &queueSelector{}}),
	}
	gotMode, err := c.Configure(context.Background())
	if err == nil || err.Error() != "configurator is not initialized" {
		t.Fatalf("expected initialization error, got %v", err)
	}
	if gotMode != 0 {
		t.Fatalf("expected 0, got %v", gotMode)
	}
}

func TestConfigurator_Configure_NilServerConfigurator(t *testing.T) {
	c := &Configurator{
		clientConfigurator: newClientConfigurator(
			&cfgObserverMock{}, &cfgSelectorMock{}, nil, nil,
			&queuedSelectorFactory{selector: &queuedSelector{}}, nil, nil, nil,
		),
		serverConfigurator: nil,
	}
	gotMode, err := c.Configure(context.Background())
	if err == nil || err.Error() != "configurator is not initialized" {
		t.Fatalf("expected initialization error, got %v", err)
	}
	if gotMode != 0 {
		t.Fatalf("expected 0, got %v", gotMode)
	}
}

// --- mock unified session for Configure tests ---

type mockUnifiedSession struct {
	waitModeResult         runtime.Mode
	waitModeErr            error
	waitRuntimeReconfigure bool
	waitRuntimeErr         error
	activateCalled         bool
	closeCalled            bool
}

func (m *mockUnifiedSession) WaitForMode() (runtime.Mode, error) {
	return m.waitModeResult, m.waitModeErr
}

func (m *mockUnifiedSession) ActivateRuntime(_ context.Context, _ bubbleTea.RuntimeDashboardOptions) {
	m.activateCalled = true
}

func (m *mockUnifiedSession) WaitForRuntimeExit() (bool, error) {
	return m.waitRuntimeReconfigure, m.waitRuntimeErr
}

func (m *mockUnifiedSession) ShowFatalError(_ string) {}

func (m *mockUnifiedSession) Close() { m.closeCalled = true }

func withMockUnifiedSession(t *testing.T, c *Configurator, session unifiedSessionHandle) {
	t.Helper()
	if session != nil {
		c.sh = &sessionHolder{handle: session}
		c.runtimeUI.setSessionHolder(c.sh)
	} else {
		c.sh = nil
	}
}

func withMockNewUnifiedSession(t *testing.T, factory func(context.Context, bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error)) {
	t.Helper()
	prev := newUnifiedSession
	newUnifiedSession = factory
	t.Cleanup(func() { newUnifiedSession = prev })
}

func withMockNewSystemdInstaller(t *testing.T, factory func() systemdInstaller) {
	t.Helper()
	prev := newSystemdInstaller
	newSystemdInstaller = factory
	t.Cleanup(func() { newSystemdInstaller = prev })
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

func newTestConfigurator() *Configurator {
	return &Configurator{
		clientConfigurator: newClientConfigurator(
			&cfgObserverMock{}, &cfgSelectorMock{}, nil, nil,
			&queuedSelectorFactory{selector: &queuedSelector{}}, nil, nil, nil,
		),
		serverConfigurator: newServerConfigurator(&mockManager{}, &mockSelectorFactory{selector: &queueSelector{}}),
		runtimeUI:          NewRuntimeUI(),
	}
}

func TestConfigurator_Configure_HappyPath_ReturnsMode(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeServer}
	c := newTestConfigurator()
	withMockUnifiedSession(t, c, mock)

	gotMode, err := c.Configure(context.Background())
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

func TestConfigurator_Configure_WaitForModeQuit_ReturnsErrUserExit(t *testing.T) {
	mock := &mockUnifiedSession{waitModeErr: bubbleTea.ErrUnifiedSessionQuit}
	c := newTestConfigurator()
	withMockUnifiedSession(t, c, mock)

	_, err := c.Configure(context.Background())
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called on quit")
	}
	if c.sh.handle != nil {
		t.Fatal("expected session cleared on quit")
	}
}

func TestConfigurator_Configure_WaitForModeClosed_ReturnsErrUserExit(t *testing.T) {
	mock := &mockUnifiedSession{waitModeErr: bubbleTea.ErrUnifiedSessionClosed}
	c := newTestConfigurator()
	withMockUnifiedSession(t, c, mock)

	_, err := c.Configure(context.Background())
	if !errors.Is(err, ErrUserExit) {
		t.Fatalf("expected ErrUserExit, got %v", err)
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called on closed session")
	}
	if c.sh.handle != nil {
		t.Fatal("expected session cleared on closed session")
	}
}

func TestConfigurator_Configure_WaitForModeError_Propagates(t *testing.T) {
	mock := &mockUnifiedSession{waitModeErr: errors.New("unexpected failure")}
	c := newTestConfigurator()
	withMockUnifiedSession(t, c, mock)

	_, err := c.Configure(context.Background())
	if err == nil || err.Error() != "unexpected failure" {
		t.Fatalf("expected 'unexpected failure', got %v", err)
	}
	if !mock.closeCalled {
		t.Fatal("expected Close called on error")
	}
	if c.sh.handle != nil {
		t.Fatal("expected session cleared on error")
	}
}

func TestConfigurator_Configure_CreatesNewSession_WhenNil(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeClient}
	c := newTestConfigurator()
	// session is nil by default — factory should create new one
	withMockNewUnifiedSession(t, func(_ context.Context, _ bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		return mock, nil
	})

	gotMode, err := c.Configure(context.Background())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotMode != runtime.ModeClient {
		t.Fatalf("expected runtime.ModeClient, got %v", gotMode)
	}
	if c.sh == nil || c.sh.handle != mock {
		t.Fatal("expected session stored on configurator")
	}
}

func TestConfigurator_Configure_NewSessionError_Propagates(t *testing.T) {
	c := newTestConfigurator()
	// session is nil by default — factory will fail
	withMockNewUnifiedSession(t, func(_ context.Context, _ bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		return nil, errors.New("session creation failed")
	})

	_, err := c.Configure(context.Background())
	if err == nil || err.Error() != "session creation failed" {
		t.Fatalf("expected 'session creation failed', got %v", err)
	}
}

func TestConfigurator_Configure_ReusesExistingSession(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeServer}
	c := newTestConfigurator()
	withMockUnifiedSession(t, c, mock)
	factoryCalled := false
	withMockNewUnifiedSession(t, func(_ context.Context, _ bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		factoryCalled = true
		return nil, errors.New("should not be called")
	})

	gotMode, err := c.Configure(context.Background())
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

func TestConfigurator_Configure_SystemdSupported_WiresCallbacks(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeClient}
	c := newTestConfigurator()
	c.serverSupported = true

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
	withMockNewSystemdInstaller(t, func() systemdInstaller { return installer })

	var captured bubbleTea.ConfiguratorSessionOptions
	withMockNewUnifiedSession(t, func(_ context.Context, opts bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		captured = opts
		return mock, nil
	})

	gotMode, err := c.Configure(context.Background())
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

func TestConfigurator_Configure_SystemdUnsupported_DoesNotWireCallbacks(t *testing.T) {
	mock := &mockUnifiedSession{waitModeResult: runtime.ModeServer}
	c := newTestConfigurator()
	c.serverSupported = true

	installer := &systemdInstallerStub{supported: false}
	withMockNewSystemdInstaller(t, func() systemdInstaller { return installer })

	var captured bubbleTea.ConfiguratorSessionOptions
	withMockNewUnifiedSession(t, func(_ context.Context, opts bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
		captured = opts
		return mock, nil
	})

	gotMode, err := c.Configure(context.Background())
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

func TestConfigurator_Close_ClosesAndClearsSession(t *testing.T) {
	mock := &mockUnifiedSession{}
	c := newTestConfigurator()
	withMockUnifiedSession(t, c, mock)

	c.Close()
	if !mock.closeCalled {
		t.Fatal("expected Close() to call underlying session Close")
	}
	if c.sh.handle != nil {
		t.Fatal("expected session handle to be cleared")
	}
}

func TestConfigurator_Close_IdempotentWithNilSession(t *testing.T) {
	c := newTestConfigurator()
	c.sh = &sessionHolder{handle: nil}

	c.Close()
	c.Close()
}
