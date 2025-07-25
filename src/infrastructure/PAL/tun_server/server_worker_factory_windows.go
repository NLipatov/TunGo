package tun_server

import (
	"context"
	"io"
	"tungo/application"
	"tungo/infrastructure/PAL/configuration/server"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	settings             settings.Settings
	configurationManager server.ServerConfigurationManager
}

func NewServerWorkerFactory(settings settings.Settings, manager server.ServerConfigurationManager) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings:             settings,
		configurationManager: manager,
	}
}

func (s ServerWorkerFactory) CreateWorker(
	_ context.Context,
	_ io.ReadWriteCloser,
) (application.TunWorker, error) {
	panic("not implemented")
}
