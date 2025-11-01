package route

import (
	"fmt"
	"net"
	"tungo/infrastructure/PAL"
	"tungo/infrastructure/settings"
)

type Factory struct {
	commander          PAL.Commander
	connectionSettings settings.Settings
}

func NewFactory(
	commander PAL.Commander,
	connectionSettings settings.Settings,
) Factory {
	return Factory{
		commander:          commander,
		connectionSettings: connectionSettings,
	}
}

func (f *Factory) CreateRoute() (Contract, error) {
	ip := net.ParseIP(f.connectionSettings.ConnectionIP)
	if ip == nil {
		return nil, fmt.Errorf("serverIP is not IP literal: %q", f.connectionSettings.ConnectionIP)
	}
	if ip.To4() != nil {
		return newV4Wrapper(f.commander), nil
	}
	return newV6Wrapper(f.commander), nil
}

func (f *Factory) CreateV4Route() Contract {
	return newV4Wrapper(f.commander)
}

func (f *Factory) CreateV6Route() Contract {
	return newV6Wrapper(f.commander)
}
