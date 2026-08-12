package bubble_tea

import (
	"errors"

	"tungo/internal/config"
	"tungo/internal/daemon/systemd"
)

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

func (s *daemonControlStub) Status() (systemd.UnitStatus, error) {
	if s.status == nil {
		return systemd.UnitStatus{}, nil
	}
	return s.status()
}

func (s *daemonControlStub) Setup(mode config.Mode) (string, error) {
	switch mode {
	case config.ModeClient:
		if s.setupClient != nil {
			return s.setupClient()
		}
	case config.ModeServer:
		if s.setupServer != nil {
			return s.setupServer()
		}
	default:
		return "", errors.New("unknown daemon mode")
	}
	return "/etc/systemd/system/tungo.service", nil
}

func (s *daemonControlStub) RemoveUnit() error {
	if s.delete == nil {
		return nil
	}
	return s.delete()
}

func (s *daemonControlStub) IsUnitActive() (bool, error) {
	if s.isActive == nil {
		return false, nil
	}
	return s.isActive()
}

func (s *daemonControlStub) StartUnit() error {
	if s.start == nil {
		return nil
	}
	return s.start()
}

func (s *daemonControlStub) StopUnit() error {
	if s.stop == nil {
		return nil
	}
	return s.stop()
}

func (s *daemonControlStub) EnableUnit() error {
	if s.enable == nil {
		return nil
	}
	return s.enable()
}

func (s *daemonControlStub) DisableUnit() error {
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
