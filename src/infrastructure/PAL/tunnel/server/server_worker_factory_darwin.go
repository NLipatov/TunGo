package server

import (
	"context"
	"io"
	appConfiguration "tungo/application/configuration"
	"tungo/application/configuration/settings"
	"tungo/application/network/routing"
)

type WorkerFactory struct {
	configuration appConfiguration.ServerRuntimeConfiguration
	runtime       *Runtime
}

func NewWorkerFactory(runtime *Runtime, configuration appConfiguration.ServerRuntimeConfiguration) (*WorkerFactory, error) {
	return &WorkerFactory{
		configuration: configuration,
		runtime:       runtime,
	}, nil
}

func (s *WorkerFactory) CreateWorker(
	_ context.Context,
	_ io.ReadWriteCloser,
	_ settings.Settings,
) (routing.Endpoints, error) {
	return routing.Endpoints{}, errServerNotSupported
}
