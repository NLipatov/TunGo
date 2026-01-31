package tun_server

import (
	application "tungo/application/network/routing"
	implementation "tungo/infrastructure/tunnel"
)

type ServerTrafficRouterFactory struct {
}

func NewServerTrafficRouterFactory() *ServerTrafficRouterFactory {
	return &ServerTrafficRouterFactory{}
}

func (s *ServerTrafficRouterFactory) CreateRouter(
	worker application.Worker,
) application.Router {
	return implementation.NewRouter(worker)
}
