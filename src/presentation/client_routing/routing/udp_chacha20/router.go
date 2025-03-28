package udp_chacha20

import (
	"context"
	"log"
	"net"
	"sync"
	"tungo/application"
)

type UDPRouter struct {
	tun                 application.TunDevice
	conn                *net.UDPConn
	cryptographyService application.CryptographyService
}

func NewUDPRouter(
	conn *net.UDPConn, tun application.TunDevice, cryptographyService application.CryptographyService,
) application.TrafficRouter {
	return &UDPRouter{
		tun:                 tun,
		conn:                conn,
		cryptographyService: cryptographyService,
	}
}

func (r *UDPRouter) RouteTraffic(ctx context.Context) error {
	routingCtx, routingCancel := context.WithCancel(ctx)
	// Start a goroutine to monitor context cancellation and close the udp_connection

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		tunWorker := newUdpWorker(r.conn, r.tun, r.cryptographyService)

		handlingErr := tunWorker.HandleTun(routingCtx, routingCancel)

		if handlingErr != nil {
			log.Printf("TUN -> UDP error: %v", handlingErr)
			routingCancel()
			return
		}
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		tunWorker := newUdpWorker(r.conn, r.tun, r.cryptographyService)

		handlingErr := tunWorker.HandleConn(routingCtx, routingCancel)

		if handlingErr != nil {
			log.Printf("UDP -> TUN error: %v", handlingErr)
			routingCancel()
			return
		}
	}()

	wg.Wait()

	return nil
}
