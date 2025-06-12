package udp_chacha20

import (
	"context"
	"io"
	"tungo/application"
	"tungo/infrastructure/routing/server_routing/session_management"
	"tungo/infrastructure/settings"
)

type UdpTunWorker struct {
	sessionManager   session_management.WorkerSessionManager[session]
	tunHandler       application.TunHandler
	transportHandler application.TransportHandler
}

func NewUdpTunWorker(
	ctx context.Context, tun io.ReadWriteCloser, settings settings.Settings,
) application.TunWorker {
	sessionManager := session_management.NewDefaultWorkerSessionManager[session]()
	concurrentSessionManager := session_management.NewConcurrentManager(sessionManager)
	return &UdpTunWorker{
		tunHandler:       NewTunHandler(ctx, tun, concurrentSessionManager),
		transportHandler: NewTransportHandler(ctx, settings, tun, concurrentSessionManager),
	}
}

func (u *UdpTunWorker) HandleTun() error {
	return u.tunHandler.HandleTun()
}

func (u *UdpTunWorker) HandleTransport() error {
	return u.transportHandler.HandleTransport()
}
