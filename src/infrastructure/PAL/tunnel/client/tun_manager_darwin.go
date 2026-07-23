//go:build darwin

package client

import (
	appConfiguration "tungo/application/configuration"
	"tungo/application/network/routing/tun"
	neManager "tungo/infrastructure/PAL/network/darwin/ne/manager"
	rawManager "tungo/infrastructure/PAL/network/darwin/raw/manager"
)

func NewPlatformTunManager(configuration appConfiguration.ClientRuntimeConfiguration) (tun.ClientManager, error) {
	if manager, ok := neManager.New(); ok {
		return manager, nil
	}

	settings, err := configuration.ActiveSettings()
	if err != nil {
		return nil, err
	}
	return rawManager.NewFactory(settings).Create()
}
