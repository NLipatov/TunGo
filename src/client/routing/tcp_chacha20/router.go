package tcp_chacha20

import (
	"context"
	"errors"
	"log"
	"math"
	"net"
	"sync"
	"time"
	"tungo/client/routing/tcp_chacha20/connection"
	"tungo/client/tun_configurator"
	"tungo/crypto/chacha20"
	"tungo/network"
	"tungo/settings"
)

type TCPRouter struct {
	Settings        settings.ConnectionSettings
	TunConfigurator tun_configurator.TunConfigurator
	tun             network.TunAdapter
}

func (r *TCPRouter) RouteTraffic(ctx context.Context) error {
	r.tun = r.TunConfigurator.Configure(r.Settings)
	defer func() {
		_ = r.tun.Close()
		r.TunConfigurator.Deconfigure(r.Settings)
	}()

	for {
		conn, session, err := r.establishSecureConnection(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) { //client shutdown
				return nil
			}

			log.Printf("connection to server at %s (TCP) failed: %s", r.Settings.ConnectionIP, err)
			continue
		}

		log.Printf("connected to server at %s (TCP)", r.Settings.ConnectionIP)

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(ctx)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-connCtx.Done()
			_ = conn.Close()
			r.TunConfigurator.Deconfigure(r.Settings)
		}()

		forwardIPPackets(r, &conn, session, connCtx, connCancel)

		// After goroutines finish, check if shutdown was initiated
		if ctx.Err() != nil {
			return nil
		} else {
			// Connection lost unexpectedly, attempt to reconnect
			log.Println("connection lost, attempting to reconnect...")
		}

		//cancel connection context
		<-connCtx.Done()

		// Close the connection (if not already closed)
		_ = conn.Close()

		// recreate tun interface
		if r.tun != nil {
			_ = r.tun.Close()
		}
		r.TunConfigurator.Deconfigure(r.Settings)
		r.tun = r.TunConfigurator.Configure(r.Settings)
	}
}

func forwardIPPackets(r *TCPRouter, conn *net.Conn, session *chacha20.TcpSession, connCtx context.Context, connCancel context.CancelFunc) {
	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		tunWorker, buildErr := newTcpTunWorker().
			UseRouter(r).
			UseConn(*conn).
			UseSession(session).
			UseEncoder(&chacha20.DefaultTCPEncoder{}).
			Build()

		if buildErr != nil {
			log.Fatalf("failed to build TCP TUN worker: %s", buildErr)
		}

		tunWorkerErr := tunWorker.HandlePacketsFromTun(connCtx, connCancel)

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
			UseSession(session).
			UseEncoder(&chacha20.DefaultTCPEncoder{}).
			Build()

		if buildErr != nil {
			log.Fatalf("failed to build TCP TUN worker: %s", buildErr)
		}

		handlingErr := tunWorker.HandlePacketsFromConn(connCtx, connCancel)

		if handlingErr != nil {
			log.Fatalf("failed to handle CONN-packet: %s", handlingErr)
		}
	}()

	wg.Wait()
}

func (r *TCPRouter) establishSecureConnection(ctx context.Context) (net.Conn, *chacha20.TcpSession, error) {
	//setup ctx deadline
	deadline := time.Now().Add(time.Duration(math.Max(float64(r.Settings.DialTimeoutMs), 5000)) * time.Millisecond)
	handshakeCtx, handshakeCtxCancel := context.WithDeadline(ctx, deadline)
	defer handshakeCtxCancel()

	//connect to server and exchange secret
	secret := connection.NewDefaultSecret(r.Settings, chacha20.NewHandshake())
	cancellableSecret := connection.NewSecretWithDeadline(handshakeCtx, secret)

	session := connection.NewDefaultSecureSession(connection.NewDefaultConnection(r.Settings), cancellableSecret)
	cancellableSession := connection.NewSecureSessionWithDeadline(handshakeCtx, session)
	conn, tcpSession, err := cancellableSession.Establish()
	if err != nil {
		return nil, nil, err
	}

	return *conn, tcpSession, nil
}
