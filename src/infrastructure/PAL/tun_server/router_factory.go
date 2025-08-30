package tun_server

import (
	"tungo/application"
	"tungo/infrastructure/routing"
)

type ServerTrafficRouterFactory struct {
}

func NewServerTrafficRouterFactory() *ServerTrafficRouterFactory {
	return &ServerTrafficRouterFactory{}
}

func (s *ServerTrafficRouterFactory) CreateRouter(
	worker application.TunWorker,
) application.TrafficRouter {
	return routing.NewRouter(worker)
}
