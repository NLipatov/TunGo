package server

import (
	"errors"
	"testing"
	"tungo/application"
	"tungo/settings"
	"tungo/settings/server_configuration"
)

type dummyTunMgr struct{}

func (d *dummyTunMgr) CreateTunDevice(cs settings.ConnectionSettings) (application.TunDevice, error) {
	return nil, nil
}
func (d *dummyTunMgr) DisposeTunDevices(cs settings.ConnectionSettings) error {
	return errors.New("not implemented")
}

type dummyKeyMgr struct{ called bool }

func (d *dummyKeyMgr) PrepareKeys() error {
	d.called = true
	return nil
}

func TestNewDependenciesAndAccessors(t *testing.T) {
	cfg := server_configuration.Configuration{
		EnableTCP: true,
		EnableUDP: false,
		TCPSettings: settings.ConnectionSettings{
			Protocol: settings.TCP,
		},
		UDPSettings: settings.ConnectionSettings{
			Protocol: settings.UDP,
		},
	}
	tm := &dummyTunMgr{}
	km := &dummyKeyMgr{}

	deps := NewDependencies(tm, cfg, km)

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
}
