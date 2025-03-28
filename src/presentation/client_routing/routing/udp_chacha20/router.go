package udp_chacha20

import (
	"context"
	"errors"
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
	go func() {
		<-routingCtx.Done()
		_ = r.conn.Close()
		_ = r.tun.Close()
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		worker := newUdpWorker(r.conn, r.tun, r.cryptographyService)
		if err := worker.HandleTun(ctx, routingCancel); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("TUN -> UDP error: %v", err)
			routingCancel()
			return
		}
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		worker := newUdpWorker(r.conn, r.tun, r.cryptographyService)
		if err := worker.HandleConn(ctx, routingCancel); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("UDP -> TUN error: %v", err)
			routingCancel()
			return
		}
	}()

	wg.Wait()

	return nil
}
