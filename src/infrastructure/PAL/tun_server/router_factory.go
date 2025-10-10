package tun_server

import (
	"tungo/application"
	"tungo/application/network/tun"
	"tungo/infrastructure/routing"
)

type ServerTrafficRouterFactory struct {
}

func NewServerTrafficRouterFactory() *ServerTrafficRouterFactory {
	return &ServerTrafficRouterFactory{}
}

func (s *ServerTrafficRouterFactory) CreateRouter(
	worker tun.Worker,
) application.TrafficRouter {
	return routing.NewRouter(worker)
}
