package factory

import (
	"tungo/application"
	"tungo/infrastructure/routing_layer"
)

type ServerRouterFactory struct {
}

func NewServerRouterFactory() application.ServerTrafficRouterFactory {
	return &ServerRouterFactory{}
}

func (s ServerRouterFactory) CreateRouter(worker application.TunWorker) application.TrafficRouter {
	return routing_layer.NewRouter(worker)
}
