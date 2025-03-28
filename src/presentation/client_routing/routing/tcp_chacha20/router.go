package tcp_chacha20

import (
	"context"
	"errors"
	"log"
	"net"
	"sync"
	"tungo/application"
)

type TCPRouter struct {
	tun                 application.TunDevice
	conn                net.Conn
	cryptographyService application.CryptographyService
}

func NewTCPRouter(
	conn net.Conn, tun application.TunDevice, cryptographyService application.CryptographyService,
) application.TrafficRouter {
	return &TCPRouter{
		tun:                 tun,
		conn:                conn,
		cryptographyService: cryptographyService,
	}
}

func (r *TCPRouter) RouteTraffic(ctx context.Context) error {
	routingCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		worker := newTcpTunWorker(r.conn, r.tun, r.cryptographyService)
		if err := worker.HandleTun(routingCtx, cancel); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("TUN -> TCP error: %v", err)
			cancel()
		}
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		worker := newTcpTunWorker(r.conn, r.tun, r.cryptographyService)
		if err := worker.HandleConn(routingCtx, cancel); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("TCP -> TUN error: %v", err)
			cancel()
		}
	}()

	wg.Wait()

	return nil
}
