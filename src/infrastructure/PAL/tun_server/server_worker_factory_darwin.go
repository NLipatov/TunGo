package tun_server

import (
	"context"
	"io"
	"tungo/application"
	"tungo/infrastructure/settings"
)

type ServerWorkerFactory struct {
	settings settings.Settings
}

func NewServerWorkerFactory(settings settings.Settings) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings: settings,
	}
}

func (s ServerWorkerFactory) CreateWorker(
	_ context.Context,
	_ io.ReadWriteCloser,
) (application.TunWorker, error) {
	panic("not implemented")
}
