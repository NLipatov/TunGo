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

// unifiedSessionHandle is the subset of *bubbleTea.UnifiedSession used by the configurator
// and runtime backend. Extracted as an interface for testability.
type unifiedSessionHandle interface {
	WaitForMode() (runtime.Mode, error)
	ActivateRuntime(ctx context.Context, options bubbleTea.RuntimeDashboardOptions)
	WaitForRuntimeExit() (reconfigure bool, err error)
	ShowFatalError(message string)
	Close()
}

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

// sessionHolder shares session state between Configurator and runtimeBackend.
// Both hold a pointer to the same holder; when either clears handle, the other sees it.
type sessionHolder struct {
	handle unifiedSessionHandle
}

// newUnifiedSession creates a new unified session. Replaced in tests.
var newUnifiedSession = func(ctx context.Context, opts bubbleTea.ConfiguratorSessionOptions) (unifiedSessionHandle, error) {
	return bubbleTea.NewUnifiedSession(ctx, opts)
}

var newSystemdInstaller = func() systemdInstaller {
	return systemd.NewUnitInstaller(exec_commander.NewExecCommander())
}

func (p *Configurator) Configure(ctx context.Context) (runtime.Mode, error) {
	if p.clientConfigurator == nil || p.serverConfigurator == nil {
		return 0, fmt.Errorf("configurator is not initialized")
	}

	systemdInstaller := newSystemdInstaller()
	systemdSupported := systemdInstaller.Supported()
	configOpts := bubbleTea.ConfiguratorSessionOptions{
		Observer:            p.clientConfigurator.observer,
		Selector:            p.clientConfigurator.selector,
		Creator:             p.clientConfigurator.creator,
		Deleter:             p.clientConfigurator.deleter,
		ClientConfigManager: p.clientConfigurator.configurationManager,
		ServerConfigManager: p.serverConfigurator.manager,
		ServerSupported:     p.serverSupported,
		SystemdSupported:    systemdSupported,
	}
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
		if p.serverSupported {
			configOpts.InstallServerSystemdUnit = systemdInstaller.InstallServerUnit
		}
	}

	if p.sh == nil || p.sh.handle == nil {
		session, err := newUnifiedSession(ctx, configOpts)
		if err != nil {
			return 0, err
		}
		p.sh = &sessionHolder{handle: session}
		p.runtimeUI.setSessionHolder(p.sh)
	}

	selectedMode, err := p.sh.handle.WaitForMode()
	if err != nil {
		if errors.Is(err, bubbleTea.ErrUnifiedSessionQuit) || errors.Is(err, bubbleTea.ErrUnifiedSessionClosed) {
			p.closeSession()
			return 0, ErrUserExit
		}
		p.closeSession()
		return 0, err
	}
	return selectedMode, nil
}

// closeSession closes the active unified session and clears the holder.
func (p *Configurator) closeSession() {
	if p.sh != nil && p.sh.handle != nil {
		p.sh.handle.Close()
		p.sh.handle = nil
	}
}

// Close releases the active unified session. Safe to call multiple times.
func (p *Configurator) Close() {
	p.closeSession()
}
