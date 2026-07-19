package server

import serverConfiguration "tungo/infrastructure/PAL/configuration/server"

type Configuration = serverConfiguration.Configuration
type AllowedPeer = serverConfiguration.AllowedPeer

func NewDefaultConfiguration() *Configuration {
	return serverConfiguration.NewDefaultConfiguration()
}
