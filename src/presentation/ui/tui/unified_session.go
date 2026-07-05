package tui

import (
	"context"
	"errors"
	"fmt"
	"tungo/infrastructure/PAL/exec_commander"
	"tungo/infrastructure/PAL/service_management/linux/systemd"
	systemdDomain "tungo/infrastructure/PAL/service_management/linux/systemd/domain"
	bubbleTea "tungo/presentation/ui/tui/internal/bubble_tea"
	"tungo/runtime"
)

// unifiedSessionHandle is the subset of *bubbleTea.UnifiedSession used by TUI.
type unifiedSessionHandle interface {
	WaitForMode() (runtime.Mode, error)
	ActivateRuntime(ctx context.Context, options bubbleTea.RuntimeDashboardOptions)
	WaitForRuntimeExit() (reconfigure bool, err error)
	ShowFatalError(message string)
	Close()
}

type unifiedSessionFactory func(context.Context, bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error)

type systemdInstaller interface {
	Supported() bool
	InstallServerUnit() (string, error)
	InstallClientUnit() (string, error)
	RemoveUnit() error
	IsUnitActive() (bool, error)
	StopUnit() error
	StartUnit() error
	EnableUnit() error
	DisableUnit() error
	Status() (systemdDomain.UnitStatus, error)
}

func newBubbleTeaUnifiedSession(ctx context.Context, opts bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
	return bubbleTea.NewUnifiedSession(ctx, opts)
}

type systemdInstallerFactory func() systemdInstaller

func newDefaultSystemdInstaller() systemdInstaller {
	return systemd.NewUnitInstaller(exec_commander.NewExecCommander())
}

func (t *TUI) Configure(ctx context.Context) (runtime.Mode, error) {
	if !t.initialized() {
		return 0, fmt.Errorf("tui is not initialized")
	}

	systemdInstaller := t.systemdInstallerFactory()
	systemdSupported := systemdInstaller.Supported()
	configOpts := t.sessionOptions
	configOpts.SystemdSupported = systemdSupported
	if systemdSupported {
		configOpts.GetSystemdDaemonStatus = func() (bubbleTea.SystemdDaemonStatus, error) {
			status, err := systemdInstaller.Status()
			if err != nil {
				return bubbleTea.SystemdDaemonStatus{}, err
			}
			var daemonMode runtime.Mode
			switch status.Role {
			case systemdDomain.UnitRoleClient:
				daemonMode = runtime.ModeClient
			case systemdDomain.UnitRoleServer:
				daemonMode = runtime.ModeServer
			}
			return bubbleTea.SystemdDaemonStatus{
				Installed:      status.Installed,
				Managed:        status.Managed,
				Mode:           daemonMode,
				LoadState:      string(status.LoadState),
				UnitFileState:  string(status.UnitFileState),
				ActiveState:    string(status.ActiveState),
				SubState:       status.SubState,
				Result:         status.Result,
				ExecMainStatus: status.ExecMainStatus,
				ExecStart:      status.ExecStart,
				FragmentPath:   status.FragmentPath,
			}, nil
		}
		configOpts.InstallClientSystemdUnit = systemdInstaller.InstallClientUnit
		configOpts.CheckSystemdUnitActive = func() (bool, error) {
			return systemdInstaller.IsUnitActive()
		}
		configOpts.StopSystemdUnit = systemdInstaller.StopUnit
		configOpts.StartSystemdUnit = systemdInstaller.StartUnit
		configOpts.EnableSystemdUnit = systemdInstaller.EnableUnit
		configOpts.DisableSystemdUnit = systemdInstaller.DisableUnit
		configOpts.RemoveSystemdUnit = systemdInstaller.RemoveUnit
		if configOpts.ServerSupported {
			configOpts.InstallServerSystemdUnit = systemdInstaller.InstallServerUnit
		}
	}

	if t.session == nil {
		session, err := t.sessionFactory(ctx, configOpts)
		if err != nil {
			return 0, err
		}
		t.session = session
	}

	selectedMode, err := t.session.WaitForMode()
	if err != nil {
		if errors.Is(err, bubbleTea.ErrUnifiedSessionQuit) || errors.Is(err, bubbleTea.ErrUnifiedSessionClosed) {
			t.closeSession()
			return 0, ErrUserExit
		}
		t.closeSession()
		return 0, err
	}
	return selectedMode, nil
}

func (t *TUI) initialized() bool {
	opts := t.sessionOptions
	return opts.Observer != nil &&
		opts.Selector != nil &&
		opts.Creator != nil &&
		opts.Deleter != nil &&
		opts.ClientConfigManager != nil &&
		opts.ServerConfigManager != nil &&
		t.sessionFactory != nil &&
		t.systemdInstallerFactory != nil
}

// closeSession closes the active unified session.
func (t *TUI) closeSession() {
	if t.session != nil {
		t.session.Close()
		t.session = nil
	}
}

// Close releases the active unified session. Safe to call multiple times.
func (t *TUI) Close() {
	t.closeSession()
}
