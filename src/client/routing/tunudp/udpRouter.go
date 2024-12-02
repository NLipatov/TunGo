package tunudp

import (
	"context"
	"log"
	"net"
	"sync"
	"tungo/client/transportconf"
	"tungo/client/tunconf"
	"tungo/handshake/ChaCha20"
	"tungo/network"
	"tungo/network/keepalive"
	"tungo/settings"
)

type UDPRouter struct {
	Settings        settings.ConnectionSettings
	TunConfigurator tunconf.TunConfigurator
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
		startUDPForwarding(r, conn.(*net.UDPConn), session, &connCtx, &connCancel)

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

func startUDPForwarding(r *UDPRouter, conn *net.UDPConn, session *ChaCha20.Session, connCtx *context.Context, connCancel *context.CancelFunc) {
	sendKeepAliveCommandChan := make(chan bool, 1)
	connPacketReceivedChan := make(chan bool, 1)
	go keepalive.StartConnectionProbing(*connCtx, *connCancel, sendKeepAliveCommandChan, connPacketReceivedChan)

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		FromTun(r, conn, session, *connCtx, *connCancel, sendKeepAliveCommandChan)
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		ToTun(r, conn, session, *connCtx, *connCancel, connPacketReceivedChan)
	}()

	wg.Wait()
}

func (r *UDPRouter) connectToServer(ctx context.Context) (net.Conn, *ChaCha20.Session, error) {
	connectionDelegate := func() (net.Conn, *ChaCha20.Session, error) {
		return newConnectionBuilder().
			useSettings(r.Settings).
			connect(ctx).
			handshake().
			build()
	}
	transportConnManager := &transportconf.ConnectionManager{
		ConnectionDelegate: connectionDelegate,
	}

	return transportConnManager.EstablishConnectionWithRetry(ctx)
}
