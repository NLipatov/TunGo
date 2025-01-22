package tun_udp_chacha20

import (
	"context"
	"log"
	"net"
	"sync"
	"time"
	"tungo/client/transport_connector"
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
	r.tun = r.TunConfigurator.Configure(r.Settings)
	defer func() {
		_ = r.tun.Close()
		r.TunConfigurator.Deconfigure(r.Settings)
	}()

	for {
		conn, session, err := r.connectToServer(ctx)
		if err != nil {
			log.Printf("could not connect to server at %s: %s", r.Settings.ConnectionIP, err)
		}

		log.Printf("connected to server at %s (UDP)", r.Settings.ConnectionIP)

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(ctx)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-connCtx.Done()
			_ = conn.Close()
			r.TunConfigurator.Deconfigure(r.Settings)
			return
		}()

		//starts forwarding packets from conn to tun-interface and from tun-interface to conn
		startUDPForwarding(r, conn.(*net.UDPConn), session, connCtx, connCancel)

		// After goroutines finish, check if shutdown was initiated
		if ctx.Err() != nil {
			return nil
		} else {
			// Connection lost unexpectedly, attempt to reconnect
			log.Println("connection lost, attempting to reconnect...")
		}

		// Close the connection (if not already closed)
		_ = conn.Close()

		// recreate tun-interface
		if r.tun != nil {
			_ = r.tun.Close()
		}
		r.TunConfigurator.Deconfigure(r.Settings)
		r.tun = r.TunConfigurator.Configure(r.Settings)
	}
}

func startUDPForwarding(r *UDPRouter, conn *net.UDPConn, session *chacha20.Session, connCtx context.Context, connCancel context.CancelFunc) {
	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		tunWorker, buildErr := newUdpTunWorker().
			UseRouter(r).
			UseConn(conn).
			UseSession(session).
			UseEncoder(&chacha20.UDPEncoder{}).
			Build()

		if buildErr != nil {
			log.Fatalf("failed to build TCP TUN worker: %s", buildErr)
		}

		handlingErr := tunWorker.HandlePacketsFromTun(connCtx, connCancel)

		if handlingErr != nil {
			log.Printf("failed to handle TUN-packet: %s", handlingErr)
			connCancel()
			return
		}
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		tunWorker, buildErr := newUdpTunWorker().
			UseRouter(r).
			UseConn(conn).
			UseSession(session).
			UseEncoder(&chacha20.UDPEncoder{}).
			Build()

		if buildErr != nil {
			log.Fatalf("failed to build TCP CONN worker: %s", buildErr)
		}

		handlingErr := tunWorker.HandlePacketsFromConn(connCtx, connCancel)

		if handlingErr != nil {
			log.Printf("failed to handle CONN-packet: %s", handlingErr)
			connCancel()
			return
		}
	}()

	wg.Wait()
}

func (r *UDPRouter) connectToServer(ctx context.Context) (net.Conn, *chacha20.Session, error) {
	connectorDelegate := func() (net.Conn, *chacha20.Session, error) {
		return newConnectionBuilder().
			useSettings(r.Settings).
			useConnectionTimeout(time.Second * 5).
			connect(ctx).
			handshake().
			build()
	}

	return transport_connector.
		NewTransportConnector().
		UseConnectorDelegate(connectorDelegate).
		Connect(ctx)
}
