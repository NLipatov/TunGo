package client

import (
	clientConfiguration "tungo/application/configuration/client"
	"tungo/application/configuration/settings"
)

func selectedSettings(conf *clientConfiguration.Configuration) (settings.Settings, error) {
	selected, err := conf.ActiveSettings()
	if err != nil {
		return settings.Settings{}, err
	}
	selected.Protocol = conf.Protocol
	if err := selected.DeriveIP(conf.ClientID); err != nil {
		return settings.Settings{}, err
	}
	return selected, nil
}
