package server

import serverConfiguration "tungo/application/configuration/server"

type Configuration = serverConfiguration.Configuration
type AllowedPeer = serverConfiguration.AllowedPeer

func NewConfiguration() *Configuration {
	return serverConfiguration.New()
}
