package tun_tcp

import (
	"context"
	"log"
	"net"
	"sync"
	"time"
	"tungo/client/transport_connector"
	"tungo/client/tun_configurator"
	"tungo/handshake/ChaCha20"
	"tungo/network"
	"tungo/network/keepalive"
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
		conn, session, err := r.connectToServer(ctx)
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

func forwardIPPackets(r *TCPRouter, conn *net.Conn, session *ChaCha20.Session, connCtx context.Context, connCancel context.CancelFunc) {
	sendKeepaliveCh := make(chan bool, 1)
	receiveKeepaliveCh := make(chan bool, 1)
	go keepalive.StartConnectionProbing(connCtx, connCancel, sendKeepaliveCh, receiveKeepaliveCh)

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		tunWorkerErr := newTcpTunWorker().
			UseRouter(*r).
			UseConn(*conn).
			UseSession(session).
			UseSendKeepAliveChan(sendKeepaliveCh).
			HandlePacketsFromTun(connCtx, connCancel)

		if tunWorkerErr != nil {
			log.Fatalf("failed to handle TUN package: %s", tunWorkerErr)
		}
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		tunWorkerErr := newTcpTunWorker().
			UseRouter(*r).
			UseConn(*conn).
			UseSession(session).
			UseReceiveKeepAliveChan(receiveKeepaliveCh).
			HandlePacketsFromConn(connCtx, connCancel)

		if tunWorkerErr != nil {
			log.Fatalf("failed to handle CONN-packet: %s", tunWorkerErr)
		}
	}()

	wg.Wait()
}

func (r *TCPRouter) connectToServer(ctx context.Context) (net.Conn, *ChaCha20.Session, error) {
	connectorDelegate := func() (net.Conn, *ChaCha20.Session, error) {
		return newTCPConnectionBuilder().
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
