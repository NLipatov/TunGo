package udp_chacha20

import (
	"context"
	"errors"
	"log"
	"math"
	"net"
	"sync"
	"time"
	"tungo/client/routing/udp_chacha20/connection"
	"tungo/client/tun_configurator"
	"tungo/crypto/chacha20"
	"tungo/network"
	"tungo/settings"
)

type UDPRouter struct {
	Settings        settings.ConnectionSettings
	TunConfigurator tun_configurator.TunConfigurator
	tun             network.TunAdapter
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

func (r *UDPRouter) startUDPForwarding(conn *net.UDPConn, session *chacha20.DefaultUdpSession, connCtx context.Context, connCancel context.CancelFunc) {
	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		tunWorker := newChacha20UdpWorker(r, conn, session)

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
		tunWorker := newChacha20UdpWorker(r, conn, session)

		handlingErr := tunWorker.HandleConn(connCtx, connCancel)

		if handlingErr != nil {
			log.Printf("failed to handle CONN-packet: %s", handlingErr)
			connCancel()
			return
		}
	}()

	wg.Wait()
}

func (r *UDPRouter) establishSecureConnection(ctx context.Context) (*net.UDPConn, *chacha20.DefaultUdpSession, error) {
	//setup ctx deadline
	deadline := time.Now().Add(time.Duration(math.Max(float64(r.Settings.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	//connect to server and exchange secret
	secret := connection.NewDefaultSecret(r.Settings, chacha20.NewHandshake())
	cancellableSecret := connection.NewSecretWithDeadline(handshakeCtx, secret)

	session := connection.NewDefaultSecureSession(connection.NewConnection(r.Settings), cancellableSecret)
	cancellableSession := connection.NewSecureSessionWithDeadline(handshakeCtx, session)
	return cancellableSession.Establish()
}
