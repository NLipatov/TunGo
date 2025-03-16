package tcp_chacha20

import (
	"context"
	"log"
	"net"
	"sync"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	"tungo/presentation/client_routing/routing"
)

type TCPRouter struct {
	Tun                 application.TunDevice
	conn                net.Conn
	cryptographyService application.CryptographyService
}

func NewTCPRouter(
	conn *net.Conn, tun application.TunDevice, cryptographyService application.CryptographyService,
) routing.TrafficRouter {
	return &TCPRouter{
		Tun:                 tun,
		conn:                *conn,
		cryptographyService: cryptographyService,
	}
}

func (r *TCPRouter) RouteTraffic(ctx context.Context) error {
	routingCtx, routingCancel := context.WithCancel(ctx)
	go func() {
		<-routingCtx.Done()
		_ = r.conn.Close()
		_ = r.Tun.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		tunWorker, buildErr := newTcpTunWorker().
			UseRouter(r).
			UseConn(r.conn).
			UseCryptographyService(r.cryptographyService).
			UseEncoder(&chacha20.DefaultTCPEncoder{}).
			Build()

		if buildErr != nil {
			log.Fatalf("failed to build TCP TUN worker: %s", buildErr)
		}

		tunWorkerErr := tunWorker.HandleTun(routingCtx, routingCancel)

		if tunWorkerErr != nil {
			log.Fatalf("failed to handle TUN-packet: %s", tunWorkerErr)
		}
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		tunWorker, buildErr := newTcpTunWorker().
			UseRouter(r).
			UseConn(r.conn).
			UseCryptographyService(r.cryptographyService).
			UseEncoder(&chacha20.DefaultTCPEncoder{}).
			Build()

		if buildErr != nil {
			log.Fatalf("failed to build TCP TUN worker: %s", buildErr)
		}

		handlingErr := tunWorker.HandleConn(routingCtx, routingCancel)

		if handlingErr != nil {
			log.Fatalf("failed to handle CONN-packet: %s", handlingErr)
		}
	}()

	wg.Wait()

	return nil
}
