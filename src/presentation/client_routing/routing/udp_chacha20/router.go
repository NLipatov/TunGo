package udp_chacha20

import (
	"context"
	"errors"
	"log"
	"math"
	"net"
	"sync"
	"time"
	"tungo/application"
	"tungo/infrastructure/cryptography/chacha20"
	connection2 "tungo/presentation/client_routing/routing/udp_chacha20/connection"
	"tungo/presentation/client_routing/tun_configurator"
	"tungo/settings"
)

type UDPRouter struct {
	Settings        settings.ConnectionSettings
	TunConfigurator tun_configurator.TunConfigurator
	tun             application.TunDevice
}

func (r *UDPRouter) RouteTraffic(ctx context.Context) error {
	defer func() {
		_ = r.tun.Close()
		r.TunConfigurator.Deconfigure(r.Settings)
	}()

	//prepare TUN
	tun, tunErr := r.TunConfigurator.Configure(r.Settings)
	if tunErr != nil {
		return tunErr
	}
	r.tun = tun

	for {
		//establish connection with server
		conn, session, err := r.establishSecureConnection(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) { //client shutdown
				return nil
			}

			log.Printf("connection to server at %s (UDP) failed: %s", r.Settings.ConnectionIP, err)
			time.Sleep(time.Millisecond * 1000)
			continue
		}

		log.Printf("connected to server at %s (UDP)", r.Settings.ConnectionIP)

		// Create a child context for managing data forwarding goroutines
		routingCtx, routingCancel := context.WithCancel(ctx)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-routingCtx.Done()
			_ = conn.Close()
		}()

		//starts forwarding packets from connection to tun-interface and from tun-interface to connection
		r.startUDPForwarding(conn, session, routingCtx, routingCancel)
	}
}

func (r *UDPRouter) startUDPForwarding(conn *net.UDPConn, cryptographyService application.CryptographyService, connCtx context.Context, connCancel context.CancelFunc) {
	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		tunWorker := newChacha20UdpWorker(r, conn, cryptographyService)

		handlingErr := tunWorker.HandleTun(connCtx, connCancel)

		if handlingErr != nil {
			log.Printf("failed to handle TUN-packet: %s", handlingErr)
			connCancel()
			return
		}
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		tunWorker := newChacha20UdpWorker(r, conn, cryptographyService)

		handlingErr := tunWorker.HandleConn(connCtx, connCancel)

		if handlingErr != nil {
			log.Printf("failed to handle CONN-packet: %s", handlingErr)
			connCancel()
			return
		}
	}()

	wg.Wait()
}

func (r *UDPRouter) establishSecureConnection(ctx context.Context) (*net.UDPConn, application.CryptographyService, error) {
	//setup ctx deadline
	deadline := time.Now().Add(time.Duration(math.Max(float64(r.Settings.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	//connect to server and exchange secret
	secret := connection2.NewDefaultSecret(r.Settings, chacha20.NewHandshake())
	cancellableSecret := connection2.NewSecretWithDeadline(handshakeCtx, secret)

	session := connection2.NewDefaultSecureSession(connection2.NewConnection(r.Settings), cancellableSecret)
	cancellableSession := connection2.NewSecureSessionWithDeadline(handshakeCtx, session)
	return cancellableSession.Establish()
}
