package server

import (
	"context"
	"io"
	"tungo/application/network/routing"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type WorkerFactory struct {
	configurationManager server.ConfigurationManager
	runtime              *Runtime
}

func NewWorkerFactory(runtime *Runtime, manager server.ConfigurationManager) (*WorkerFactory, error) {
	return &WorkerFactory{
		configurationManager: manager,
		runtime:              runtime,
	}, nil
}

func (s *WorkerFactory) CreateWorker(
	_ context.Context,
	_ io.ReadWriteCloser,
	_ settings.Settings,
) (routing.Worker, error) {
	return nil, errServerNotSupported
}
