package client_factory

import (
	"context"
	"log"
	"net/netip"
	"tungo/application/network/connection"
	application "tungo/application/network/routing"
	"tungo/application/network/routing/tun"
	implementation "tungo/infrastructure/tunnel"
)

type RouterFactory struct {
}

func NewRouterFactory() connection.TrafficRouterFactory {
	return &RouterFactory{}
}

func (u *RouterFactory) CreateRouter(
	ctx context.Context,
	connectionFactory connection.Factory,
	tunManager tun.ClientManager,
	workerFactory connection.ClientWorkerFactory,
) (application.Router, connection.Transport, tun.Device, error) {
	clearRouteEndpoint(tunManager)

	conn, cryptographyService, controller, connErr := connectionFactory.EstablishConnection(ctx)
	if connErr != nil {
		return nil, nil, nil, connErr
	}
	attachRouteEndpoint(conn, tunManager)

	device, deviceErr := tunManager.CreateDevice()
	if deviceErr != nil {
		_ = conn.Close()
		log.Printf("failed to create TUN device: %s", deviceErr)
		return nil, nil, nil, deviceErr
	}

	worker, workerErr := workerFactory.CreateWorker(ctx, conn, device, cryptographyService, controller)
	if workerErr != nil {
		_ = device.Close()
		_ = conn.Close()
		return nil, nil, nil, workerErr
	}

	return implementation.NewRouter(worker), conn, device, nil
}

func attachRouteEndpoint(conn connection.Transport, tunManager tun.ClientManager) {
	remoteProvider, ok := conn.(connection.TransportWithRemoteAddr)
	if !ok {
		return
	}
	ap := remoteProvider.RemoteAddrPort()
	if !ap.IsValid() {
		return
	}
	tunManager.SetRouteEndpoint(ap)
}

func clearRouteEndpoint(tunManager tun.ClientManager) {
	tunManager.SetRouteEndpoint(netip.AddrPort{})
}
