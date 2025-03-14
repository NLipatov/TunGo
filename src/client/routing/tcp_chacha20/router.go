package tcp_chacha20

import (
	"context"
	"errors"
	"log"
	"math"
	"net"
	"sync"
	"time"
	"tungo/application"
	"tungo/client/routing/tcp_chacha20/connection"
	"tungo/client/tun_configurator"
	chacha21 "tungo/infrastructure/cryptography/chacha20"
	"tungo/settings"
)

type TCPRouter struct {
	Settings        settings.ConnectionSettings
	TunConfigurator tun_configurator.TunConfigurator
	tun             application.TunDevice
}

func (r *TCPRouter) RouteTraffic(ctx context.Context) error {
	defer func() {
		_ = r.tun.Close()
		r.TunConfigurator.Deconfigure(r.Settings)
	}()

	tun, tunErr := r.TunConfigurator.Configure(r.Settings)
	if tunErr != nil {
		return tunErr
	}
	r.tun = tun

	for {
		conn, cryptographyService, err := r.establishSecureConnection(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) { //client shutdown
				return nil
			}

			log.Printf("connection to server at %s (TCP) failed: %s", r.Settings.ConnectionIP, err)
			time.Sleep(time.Millisecond * 1000)
			continue
		}

		log.Printf("connected to server at %s (TCP)", r.Settings.ConnectionIP)

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(ctx)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-connCtx.Done()
			_ = conn.Close()
		}()

		forwardIPPackets(r, &conn, cryptographyService, connCtx, connCancel)
	}
}

func forwardIPPackets(r *TCPRouter, conn *net.Conn, cryptographyService application.CryptographyService, connCtx context.Context, connCancel context.CancelFunc) {
	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		tunWorker, buildErr := newTcpTunWorker().
			UseRouter(r).
			UseConn(*conn).
			UseCryptographyService(cryptographyService).
			UseEncoder(&chacha21.DefaultTCPEncoder{}).
			Build()

		if buildErr != nil {
			log.Fatalf("failed to build TCP TUN worker: %s", buildErr)
		}

		tunWorkerErr := tunWorker.HandleTun(connCtx, connCancel)

		if tunWorkerErr != nil {
			log.Fatalf("failed to handle TUN-packet: %s", tunWorkerErr)
		}
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		tunWorker, buildErr := newTcpTunWorker().
			UseRouter(r).
			UseConn(*conn).
			UseCryptographyService(cryptographyService).
			UseEncoder(&chacha21.DefaultTCPEncoder{}).
			Build()

		if buildErr != nil {
			log.Fatalf("failed to build TCP TUN worker: %s", buildErr)
		}

		handlingErr := tunWorker.HandleConn(connCtx, connCancel)

		if handlingErr != nil {
			log.Fatalf("failed to handle CONN-packet: %s", handlingErr)
		}
	}()

	wg.Wait()
}

func (r *TCPRouter) establishSecureConnection(ctx context.Context) (net.Conn, application.CryptographyService, error) {
	//setup ctx deadline
	deadline := time.Now().Add(time.Duration(math.Max(float64(r.Settings.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	//connect to server and exchange secret
	secret := connection.NewDefaultSecret(r.Settings, chacha21.NewHandshake())
	cancellableSecret := connection.NewSecretWithDeadline(handshakeCtx, secret)

	session := connection.NewDefaultSecureSession(connection.NewDefaultConnection(r.Settings), cancellableSecret)
	cancellableSession := connection.NewSecureSessionWithDeadline(handshakeCtx, session)
	conn, tcpSession, err := cancellableSession.Establish()
	if err != nil {
		return nil, nil, err
	}

	return *conn, tcpSession, nil
}
