//go:build windows

package route

import (
	"fmt"
	"net"
	"tungo/infrastructure/settings"
)

type Factory struct {
	connectionSettings settings.Settings
}

func NewFactory(
	connectionSettings settings.Settings,
) Factory {
	return Factory{
		connectionSettings: connectionSettings,
	}
}

func (f *Factory) CreateRoute() (Contract, error) {
	ip := net.ParseIP(f.connectionSettings.ConnectionIP)
	if ip == nil {
		return nil, fmt.Errorf("serverIP is not IP literal: %q", f.connectionSettings.ConnectionIP)
	}
	if ip.To4() != nil {
		return newV4Wrapper(), nil
	}
	return newV6Wrapper(), nil
}

func (f *Factory) CreateRouteV4() Contract { return newV4Wrapper() }
func (f *Factory) CreateRouteV6() Contract { return newV6Wrapper() }
