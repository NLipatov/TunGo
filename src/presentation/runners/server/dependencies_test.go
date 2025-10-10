package server

import (
	"errors"
	"testing"
	"tungo/application/network/tun"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type dummyTunMgr struct{}

func (d *dummyTunMgr) CreateDevice(_ settings.Settings) (tun.Device, error) {
	return nil, nil
}
func (d *dummyTunMgr) DisposeDevices(_ settings.Settings) error {
	return errors.New("not implemented")
}

func (d *dummyTunMgr) DisableDevMasquerade() error {
	return nil
}

type dummyKeyMgr struct{ called bool }

func (d *dummyKeyMgr) PrepareKeys() error {
	d.called = true
	return nil
}

func TestNewDependenciesAndAccessors(t *testing.T) {
	cfg := server.Configuration{
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

	deps := NewDependencies(tm, cfg, km, nil)

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

	if deps.ConfigurationManager() != nil {
		t.Error("expected nil ConfigurationManager")
	}
}
