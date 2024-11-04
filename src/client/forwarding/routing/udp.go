package routing

import (
	"context"
	"etha-tunnel/client/forwarding/ipconfiguration"
	"etha-tunnel/client/forwarding/routing/connHandling"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/network/keepalive"
	"etha-tunnel/settings"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

func startUDPRouting(settings settings.ConnectionSettings, tunFile *os.File, ctx context.Context) error {
	for {
		conn, connectionError := establishUDPConnection(settings, ctx)
		if connectionError != nil {
			log.Printf("failed to establish connection: %s", connectionError)
			continue // Retry connection
		}

		log.Printf("Connected to server at %s (UDP)", settings.ConnectionIP)

		_, err := conn.Write([]byte("REG"))
		if err != nil {
			log.Fatalf("failed to send reg request to server")
		}

		session, err := handshakeHandlers.OnConnectedToServer(conn, settings)
		if err != nil {
			_ = conn.Close()
			log.Printf("registration failed: %s\n", err)
			log.Println("connection is aborted")
			return err
		}

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(ctx)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-connCtx.Done()
			_ = conn.Close()
			return
		}()

		startUDPForwarding(conn, tunFile, session, &connCtx, &connCancel)

		// After goroutines finish, check if shutdown was initiated
		if ctx.Err() != nil {
			log.Println("Client is shutting down.")
			return err
		} else {
			// Connection lost unexpectedly, attempt to reconnect
			log.Println("Connection lost, attempting to reconnect...")
		}

		// Close the connection (if not already closed)
		_ = conn.Close()
	}
}

func establishUDPConnection(settings settings.ConnectionSettings, ctx context.Context) (*net.UDPConn, error) {
	reconnectAttempts := 0
	backoff := initialBackoff

	for {
		serverAddr := fmt.Sprintf("%s%s", settings.ConnectionIP, settings.ConnectionPort)

		udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
		if err != nil {
			return nil, err
		}

		conn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			log.Printf("Failed to connect to server: %v", err)
			reconnectAttempts++
			if reconnectAttempts > maxReconnectAttempts {
				ipconfiguration.Unconfigure(settings)
				log.Fatalf("Exceeded maximum reconnect attempts (%d)", maxReconnectAttempts)
			}
			log.Printf("Retrying to connect in %v...", backoff)

			select {
			case <-ctx.Done():
				log.Println("Client is shutting down.")
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

func startUDPForwarding(conn *net.UDPConn, tunFile *os.File, session *ChaCha20.Session, connCtx *context.Context, connCancel *context.CancelFunc) {
	sendKeepAliveCommandChan := make(chan bool, 1)
	connPacketReceivedChan := make(chan bool, 1)
	go keepalive.StartConnectionProbing(*connCtx, *connCancel, sendKeepAliveCommandChan, connPacketReceivedChan)

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> UDP
	go func() {
		defer wg.Done()
		connHandling.TunToUDP(conn, tunFile, session, *connCtx, *connCancel, sendKeepAliveCommandChan)
	}()

	// UDP -> TUN
	go func() {
		defer wg.Done()
		connHandling.UDPToTun(conn, tunFile, session, *connCtx, *connCancel, connPacketReceivedChan)
	}()

	wg.Wait()
}
