package server

import (
	application "tungo/application/network/routing"
	implementation "tungo/infrastructure/tunnel"
)

type TrafficRouterFactory struct {
}

func NewTrafficRouterFactory() *TrafficRouterFactory {
	return &TrafficRouterFactory{}
}

func (s *TrafficRouterFactory) CreateRouter(
	worker application.Worker,
) application.Router {
	return implementation.NewRouter(worker)
}
