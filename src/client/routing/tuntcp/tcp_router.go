package tuntcp

import (
	"context"
	"log"
	"net"
	"sync"
	"tungo/client/tunconf"
	"tungo/handshake/ChaCha20"
	"tungo/network"
	"tungo/network/keepalive"
	"tungo/settings"
)

type TCPRouter struct {
	Settings        settings.ConnectionSettings
	Tun             network.TunAdapter
	TunConfigurator tunconf.TunConfigurator
}

func (r *TCPRouter) ForwardTraffic(ctx context.Context) error {
	defer func() {
		_ = r.Tun.Close()
		r.TunConfigurator.Deconfigure(r.Settings)
	}()

	for {
		conn, session, err := newConnectionManager(r.Settings).connect(ctx)
		if err != nil {
			log.Fatalf("failed to establish connection: %s", err)
		}

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
		reconfigureTun(r)
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
		ToTCP(r, *conn, session, connCtx, connCancel, sendKeepaliveCh)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		ToTun(r, *conn, session, connCtx, connCancel, receiveKeepaliveCh)
	}()

	wg.Wait()
}

// Recreates the TUN interface to ensure proper routing after connection loss.
func reconfigureTun(r *TCPRouter) {
	if r.Tun != nil {
		_ = r.Tun.Close()
	}
	r.TunConfigurator.Deconfigure(r.Settings)
	r.Tun = r.TunConfigurator.Configure(r.Settings)
}
