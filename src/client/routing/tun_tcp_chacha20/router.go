package tun_tcp_chacha20

import (
	"context"
	"log"
	"net"
	"sync"
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
		connector := NewConnector(r.Settings)
		conn, session, err := connector.Connect(ctx)
		if err != nil {
			log.Fatalf("failed to establish connection: %s", err)
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

func forwardIPPackets(r *TCPRouter, conn *net.Conn, session *chacha20.Session, connCtx context.Context, connCancel context.CancelFunc) {
	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		tunWorker, buildErr := newTcpTunWorker().
			UseRouter(r).
			UseConn(*conn).
			UseSession(session).
			UseEncoder(&chacha20.TCPEncoder{}).
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
			UseEncoder(&chacha20.TCPEncoder{}).
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
