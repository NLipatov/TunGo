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
	status      func() (systemd.UnitStatus, error)
	setupClient func() (string, error)
	setupServer func() (string, error)
	delete      func() error
	isActive    func() (bool, error)
	start       func() error
	stop        func() error
	enable      func() error
	disable     func() error
}

func newDaemonControlStub() *daemonControlStub {
	return &daemonControlStub{}
}

func (s *daemonControlStub) Status(_ context.Context) (systemd.UnitStatus, error) {
	if s.status == nil {
		return systemd.UnitStatus{}, nil
	}
	return s.status()
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

func (s *daemonControlStub) RemoveUnit(_ context.Context) error {
	if s.delete == nil {
		return nil
	}
	return s.delete()
}

func (s *daemonControlStub) IsUnitActive(_ context.Context) (bool, error) {
	if s.isActive == nil {
		return false, nil
	}
	return s.isActive()
}

func (s *daemonControlStub) StartUnit(_ context.Context) error {
	if s.start == nil {
		return nil
	}
	return s.start()
}

func (s *daemonControlStub) StopUnit(_ context.Context) error {
	if s.stop == nil {
		return nil
	}
	return s.stop()
}

func (s *daemonControlStub) EnableUnit(_ context.Context) error {
	if s.enable == nil {
		return nil
	}
	return s.enable()
}

func (s *daemonControlStub) DisableUnit(_ context.Context) error {
	if s.disable == nil {
		return nil
	}
	return s.disable()
}

func (o *ConfiguratorOptions) testDaemon() *daemonControlStub {
	if daemon, ok := o.Daemon.(*daemonControlStub); ok && daemon != nil {
		return daemon
	}
	daemon := newDaemonControlStub()
	o.Daemon = daemon
	return daemon
}
