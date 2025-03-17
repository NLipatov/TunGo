package tcp_chacha20

import (
	"context"
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
	routingCtx, routingCancel := context.WithCancel(ctx)
	go func() {
		<-routingCtx.Done()
		_ = r.conn.Close()
		_ = r.tun.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		tunWorker := newTcpTunWorker(r.conn, r.tun, r.cryptographyService)
		handleTunErr := tunWorker.HandleTun(routingCtx, routingCancel)
		if handleTunErr != nil {
			log.Fatalf("failed to handle TUN-packet: %s", handleTunErr)
		}
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		tunWorker := newTcpTunWorker(r.conn, r.tun, r.cryptographyService)
		handleConnErr := tunWorker.HandleConn(routingCtx, routingCancel)
		if handleConnErr != nil {
			log.Fatalf("failed to handle CONN-packet: %s", handleConnErr)
		}
	}()

	wg.Wait()

	return nil
}
