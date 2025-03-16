package udp_chacha20

import (
	"context"
	"log"
	"net"
	"sync"
	"tungo/application"
	"tungo/presentation/client_routing/routing"
)

type UDPRouter struct {
	Tun                 application.TunDevice
	conn                *net.UDPConn
	cryptographyService application.CryptographyService
}

func NewUDPRouter(
	conn *net.UDPConn, tun application.TunDevice, cryptographyService application.CryptographyService,
) routing.TrafficRouter {
	return &UDPRouter{
		Tun:                 tun,
		conn:                conn,
		cryptographyService: cryptographyService,
	}
}

func (r *UDPRouter) RouteTraffic(ctx context.Context) error {
	routingCtx, routingCancel := context.WithCancel(ctx)
	// Start a goroutine to monitor context cancellation and close the udp_connection
	go func() {
		<-routingCtx.Done()
		_ = r.conn.Close()
		_ = r.Tun.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		tunWorker := newChacha20UdpWorker(r, r.conn, r.cryptographyService)

		handlingErr := tunWorker.HandleTun(routingCtx, routingCancel)

		if handlingErr != nil {
			log.Printf("failed to handle TUN-packet: %s", handlingErr)
			routingCancel()
			return
		}
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		tunWorker := newChacha20UdpWorker(r, r.conn, r.cryptographyService)

		handlingErr := tunWorker.HandleConn(routingCtx, routingCancel)

		if handlingErr != nil {
			log.Printf("failed to handle CONN-packet: %s", handlingErr)
			routingCancel()
			return
		}
	}()

	wg.Wait()

	return nil
}
