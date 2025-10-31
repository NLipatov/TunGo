package netsh

import (
	"errors"
	"fmt"
	"net"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/PAL/configuration/client"
	"tungo/infrastructure/settings"
)

type Factory struct {
	configuration client.Configuration
	commander     PAL.Commander
}

func NewFactory(
	configuration client.Configuration,
	commander PAL.Commander,
) *Factory {
	return &Factory{
		configuration: configuration,
		commander:     commander,
	}
}

func (f *Factory) CreateNetsh() (Contract, error) {
	connectionSettings, connectionSettingsErr := f.settingsToUse()
	if connectionSettingsErr != nil {
		return nil, connectionSettingsErr
	}
	ip := net.ParseIP(connectionSettings.ConnectionIP)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP: %q", connectionSettings.ConnectionIP)
	}
	if ip.To4() != nil {
		return NewV4Wrapper(f.commander), nil
	}
	return NewV6Wrapper(f.commander), nil
}

func (f *Factory) settingsToUse() (settings.Settings, error) {
	var zero settings.Settings
	switch f.configuration.Protocol {
	case settings.UDP:
		return f.configuration.UDPSettings, nil
	case settings.TCP:
		return f.configuration.TCPSettings, nil
	case settings.WS, settings.WSS:
		return f.configuration.WSSettings, nil
	default:
		return zero, errors.New("unsupported protocol")
	}
}
