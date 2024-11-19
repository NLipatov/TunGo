package tunudp

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
	"tungo/client/tunconf"
	"tungo/handshake/ChaCha20"
	"tungo/handshake/ChaCha20/handshakeHandlers"
	"tungo/network/keepalive"
	"tungo/settings"
)

type UDPRouter struct {
	Settings settings.ConnectionSettings
}

func (ur *UDPRouter) ForwardTraffic(ctx context.Context) error {
	var tunFile *os.File
	defer func() {
		_ = tunFile.Close()
	}()
	defer tunconf.Deconfigure(ur.Settings)

	for {
		_ = tunFile.Close()
		tunFile = tunconf.Configure(ur.Settings)

		conn, connectionError := establishUDPConnection(ur.Settings, ctx)
		if connectionError != nil {
			log.Printf("failed to establish connection: %s", connectionError)
			continue // Retry connection
		}

		_, err := conn.Write([]byte(ur.Settings.SessionMarker))
		if err != nil {
			log.Fatalf("failed to send reg request to server")
		}

		session, err := handshakeHandlers.OnConnectedToServer(conn, ur.Settings)
		if err != nil {
			_ = conn.Close()
			log.Printf("registration failed: %s\n", err)
			time.Sleep(time.Second * 1)
			continue
		}

		log.Printf("connected to server at %s (UDP)", ur.Settings.ConnectionIP)

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(ctx)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-connCtx.Done()
			_ = conn.Close()
			return
		}()

		startUDPForwarding(ur.Settings, conn, tunFile, session, &connCtx, &connCancel)

		// After goroutines finish, check if shutdown was initiated
		if ctx.Err() != nil {
			log.Println("client is shutting down.")
			return nil
		} else {
			// Connection lost unexpectedly, attempt to reconnect
			log.Println("connection lost, attempting to reconnect...")
		}

		// Close the connection (if not already closed)
		_ = conn.Close()
	}
}

func establishUDPConnection(settings settings.ConnectionSettings, ctx context.Context) (*net.UDPConn, error) {
	reconnectAttempts := 0
	backoff := initialBackoff

	for {
		serverAddr := fmt.Sprintf("%s%s", settings.ConnectionIP, settings.Port)

		udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			return nil, err
		}

		conn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			log.Printf("failed to connect to server: %v", err)
			reconnectAttempts++
			if reconnectAttempts > maxReconnectAttempts {
				tunconf.Deconfigure(settings)
				log.Fatalf("exceeded maximum reconnect attempts (%d)", maxReconnectAttempts)
			}
			log.Printf("retrying to connect in %v...", backoff)

			select {
			case <-ctx.Done():
				log.Println("client is shutting down.")
				return nil, err
			case <-time.After(backoff):
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		return conn, nil
	}
}

func startUDPForwarding(settings settings.ConnectionSettings, conn *net.UDPConn, tunFile *os.File, session *ChaCha20.Session, connCtx *context.Context, connCancel *context.CancelFunc) {
	sendKeepAliveCommandChan := make(chan bool, 1)
	connPacketReceivedChan := make(chan bool, 1)
	go keepalive.StartConnectionProbing(*connCtx, *connCancel, sendKeepAliveCommandChan, connPacketReceivedChan)

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		FromTun(conn, tunFile, session, *connCtx, *connCancel, sendKeepAliveCommandChan)
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		ToTun(settings, conn, tunFile, session, *connCtx, *connCancel, connPacketReceivedChan)
	}()

	wg.Wait()
}
