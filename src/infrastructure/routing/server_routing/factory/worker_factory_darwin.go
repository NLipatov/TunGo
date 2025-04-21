package factory

import (
	"context"
	"io"
	"tungo/application"
	"tungo/settings"
)

type ServerWorkerFactory struct {
	settings settings.ConnectionSettings
}

func NewServerWorkerFactory(settings settings.ConnectionSettings) application.ServerWorkerFactory {
	return &ServerWorkerFactory{
		settings: settings,
	}
}

func (s ServerWorkerFactory) CreateWorker(_ context.Context, _ io.ReadWriteCloser) (application.TunWorker, error) {
	panic("not implemented")
}
