package tcp_chacha20

import (
	"context"
	"io"
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

type TcpTunWorker struct {
	ctx              context.Context
	tunFile          io.ReadWriteCloser
	settings         settings.Settings
	sessionManager   session_management.WorkerSessionManager[session]
	tunHandler       application.TunHandler
	transportHandler application.TransportHandler
}

func NewTcpTunWorker(
	ctx context.Context, tunFile io.ReadWriteCloser, settings settings.Settings,
) application.TunWorker {
	sessionManager := session_management.NewDefaultWorkerSessionManager[session]()
	return &TcpTunWorker{
		ctx:              ctx,
		tunFile:          tunFile,
		settings:         settings,
		sessionManager:   sessionManager,
		tunHandler:       NewTunHandler(ctx, tunFile, sessionManager),
		transportHandler: NewTransportHandler(ctx, settings, tunFile, sessionManager),
	}
}

func (t *TcpTunWorker) HandleTun() error {
	return t.tunHandler.HandleTun()
}

func (t *TcpTunWorker) HandleTransport() error {
	return t.transportHandler.HandleTransport()
}
