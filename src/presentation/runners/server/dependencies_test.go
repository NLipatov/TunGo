package server

import (
	"errors"
	"testing"
	"tungo/application"
	"tungo/infrastructure/PAL/server_configuration"
	"tungo/infrastructure/settings"
)

type dummyTunMgr struct{}

func (d *dummyTunMgr) CreateTunDevice(_ settings.Settings) (application.TunDevice, error) {
	return nil, nil
}
func (d *dummyTunMgr) DisposeTunDevices(_ settings.Settings) error {
	return errors.New("not implemented")
}

type dummyKeyMgr struct{ called bool }

func (d *dummyKeyMgr) PrepareKeys() error {
	d.called = true
	return nil
}

type dummySessionLifetimeMgr struct{ called bool }

func (d *dummySessionLifetimeMgr) PrepareSessionLifetime() error {
	d.called = true
	return nil
}

func TestNewDependenciesAndAccessors(t *testing.T) {
	cfg := server_configuration.Configuration{
		EnableTCP: true,
		EnableUDP: false,
		TCPSettings: settings.Settings{
			Protocol: settings.TCP,
		},
		UDPSettings: settings.Settings{
			Protocol: settings.UDP,
		},
	}
	tm := &dummyTunMgr{}
	km := &dummyKeyMgr{}
	sm := &dummySessionLifetimeMgr{}

	deps := NewDependencies(tm, cfg, km, sm)

	gotCfg := deps.Configuration()
	if gotCfg.EnableTCP != cfg.EnableTCP {
		t.Errorf("Configuration().EnableTCP = %v; want %v", gotCfg.EnableTCP, cfg.EnableTCP)
	}
	if gotCfg.EnableUDP != cfg.EnableUDP {
		t.Errorf("Configuration().EnableUDP = %v; want %v", gotCfg.EnableUDP, cfg.EnableUDP)
	}
	if gotCfg.TCPSettings.Protocol != cfg.TCPSettings.Protocol {
		t.Errorf("Configuration().TCPSettings.Protocol = %v; want %v", gotCfg.TCPSettings.Protocol, cfg.TCPSettings.Protocol)
	}
	if gotCfg.UDPSettings.Protocol != cfg.UDPSettings.Protocol {
		t.Errorf("Configuration().UDPSettings.Protocol = %v; want %v", gotCfg.UDPSettings.Protocol, cfg.UDPSettings.Protocol)
	}

	gotTm := deps.TunManager()
	if gotTm != tm {
		t.Errorf("TunManager() = %v; want %v", gotTm, tm)
	}

	gotKm := deps.KeyManager()
	if gotKm != km {
		t.Errorf("KeyManager() = %v; want %v", gotKm, km)
	}

	if err := deps.KeyManager().PrepareKeys(); err != nil {
		t.Errorf("KeyManager().PrepareKeys() returned error: %v", err)
	}
	if !km.called {
		t.Error("KeyManager().PrepareKeys() was not invoked on underlying manager")
	}

	gotSm := deps.SessionLifetimeManager()
	if gotSm != sm {
		t.Errorf("SessionLifetimeManager() = %v; want %v", gotSm, sm)
	}

	if err := deps.SessionLifetimeManager().PrepareSessionLifetime(); err != nil {
		t.Errorf("SessionLifetimeManager().PrepareSessionLifetime() returned error: %v", err)
	}
	if !sm.called {
		t.Error("SessionLifetimeManager().PrepareSessionLifetime() was not invoked on underlying manager")
	}
}
