package tun_server

import (
	"context"
	"io"
	"tungo/application"
	"tungo/application/network/tun"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	configurationManager server.ServerConfigurationManager
}

func NewServerWorkerFactory(manager server.ServerConfigurationManager) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		configurationManager: manager,
	}
}

func (s ServerWorkerFactory) CreateWorker(
	_ context.Context,
	_ io.ReadWriteCloser,
	_ settings.Settings) (tun.Worker, error) {
	panic("not implemented")
}
