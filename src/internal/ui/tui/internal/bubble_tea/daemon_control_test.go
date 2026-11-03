package bubble_tea

import (
	tea "charm.land/bubbletea/v2"
	"context"
	"errors"

	"tungo/internal/daemon/systemd"
	"tungo/internal/mode"
)

// Existing state-transition tests execute commands immediately; concurrency tests
// drive Update and command completion separately.
func completeDaemonCommands(model tea.Model, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	for cmd != nil {
		msg := cmd()
		switch msg.(type) {
		case daemonStatusMsg, daemonActiveMsg, daemonStoppedMsg:
			model, cmd = model.Update(msg)
		default:
			return model, func() tea.Msg { return msg }
		}
	}
	return model, nil
}

func completeDaemonStart(model Configurator, cmd tea.Cmd) Configurator {
	updated, _ := completeDaemonCommands(model, cmd)
	return updated.(Configurator)
}

func newDaemonTestConfigurator(options ConfiguratorOptions, settings *Preferences) (Configurator, error) {
	model, err := NewConfigurator(options, settings)
	if err != nil {
		return model, err
	}
	model = completeDaemonStart(model, model.Init())
	return model, nil
}

type daemonControlStub struct {
	status      func(mode.Mode) (systemd.UnitStatus, error)
	setupClient func() (string, error)
	setupServer func() (string, error)
	delete      func(mode.Mode) error
	isActive    func(mode.Mode) (bool, error)
	start       func(mode.Mode) error
	stop        func(mode.Mode) error
	enable      func(mode.Mode) error
	disable     func(mode.Mode) error
}

func newDaemonControlStub() *daemonControlStub {
	return &daemonControlStub{}
}

func (s *daemonControlStub) Status(_ context.Context, runtimeMode mode.Mode) (systemd.UnitStatus, error) {
	if s.status == nil {
		return systemd.UnitStatus{}, nil
	}
	return s.status(runtimeMode)
}

func (s *daemonControlStub) Setup(_ context.Context, runtimeMode mode.Mode) (string, error) {
	switch runtimeMode {
	case mode.Client:
		if s.setupClient != nil {
			return s.setupClient()
		}
	case mode.Server:
		if s.setupServer != nil {
			return s.setupServer()
		}
	default:
		return "", errors.New("unknown daemon mode")
	}
	return "/etc/systemd/system/tungo.service", nil
}

func (s *daemonControlStub) RemoveUnit(_ context.Context, runtimeMode mode.Mode) error {
	if s.delete == nil {
		return nil
	}
	return s.delete(runtimeMode)
}

func (s *daemonControlStub) IsUnitActive(_ context.Context, runtimeMode mode.Mode) (bool, error) {
	if s.isActive == nil {
		return false, nil
	}
	return s.isActive(runtimeMode)
}

func (s *daemonControlStub) StartUnit(_ context.Context, runtimeMode mode.Mode) error {
	if s.start == nil {
		return nil
	}
	return s.start(runtimeMode)
}

func (s *daemonControlStub) StopUnit(_ context.Context, runtimeMode mode.Mode) error {
	if s.stop == nil {
		return nil
	}
	return s.stop(runtimeMode)
}

func (s *daemonControlStub) EnableUnit(_ context.Context, runtimeMode mode.Mode) error {
	if s.enable == nil {
		return nil
	}
	return s.enable(runtimeMode)
}

func (s *daemonControlStub) DisableUnit(_ context.Context, runtimeMode mode.Mode) error {
	if s.disable == nil {
		return nil
	}
	return s.disable(runtimeMode)
}

func (o *ConfiguratorOptions) testDaemon() *daemonControlStub {
	if daemon, ok := o.Daemon.(*daemonControlStub); ok && daemon != nil {
		return daemon
	}
	daemon := newDaemonControlStub()
	o.Daemon = daemon
	return daemon
}

func daemonTestOptions(actions ...string) []daemonMenuOption {
	options := make([]daemonMenuOption, len(actions))
	for idx, action := range actions {
		runtimeMode := mode.Client
		if action == daemonSetupServerLabel {
			runtimeMode = mode.Server
		}
		options[idx] = daemonMenuOption{mode: runtimeMode, action: action}
	}
	return options
}

func daemonTestActions(options []daemonMenuOption) []string {
	actions := make([]string, len(options))
	for idx, option := range options {
		actions[idx] = option.action
	}
	return actions
}
