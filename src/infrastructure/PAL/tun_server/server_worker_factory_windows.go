package tun_server

import (
	"context"
	"io"
	"tungo/application/network/connection"
	"tungo/application/network/routing"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	configurationManager server.ServerConfigurationManager
}

func NewServerWorkerFactory(manager server.ServerConfigurationManager) connection.ServerWorkerFactory {
	return &ServerWorkerFactory{
		configurationManager: manager,
	}
}

func (s ServerWorkerFactory) CreateWorker(
	_ context.Context,
	_ io.ReadWriteCloser,
	_ settings.Settings) (routing.Worker, error) {
	panic("not implemented")
}
