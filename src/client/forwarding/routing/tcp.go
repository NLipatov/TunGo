package routing

import (
	"context"
	"etha-tunnel/client/forwarding"
	"etha-tunnel/client/forwarding/ipconfiguration"
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

func StartTCPRouting(settings settings.ConnectionSettings, tunFile *os.File, ctx *context.Context) error {
	for {
		conn, connectionError := connect(settings, *ctx)
		if connectionError != nil {
			log.Printf("failed to establish connection: %s", connectionError)
			continue // Retry connection
		}

		session, registrationErr := register(&conn, settings)
		if registrationErr != nil {
			log.Fatalf("failed to register: %s", registrationErr)
		}

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(*ctx)
		forwardIPPackets(&conn, tunFile, session, &connCtx, &connCancel)
		return conn.Close()
	}
}

func connect(settings settings.ConnectionSettings, ctx context.Context) (net.Conn, error) {
	reconnectAttempts := 0
	backoff := initialBackoff

	for {
		dialer := &net.Dialer{}
		dialCtx, dialCancel := context.WithTimeout(ctx, connectionTimeout)
		conn, err := dialer.DialContext(dialCtx, "tcp", fmt.Sprintf("%s%s", settings.ConnectionIP, settings.ConnectionPort))
		dialCancel()

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

		log.Printf("Connected to server at %s (TCP)", settings.ConnectionIP)

		return conn, nil
	}
}

func register(conn *net.Conn, settings settings.ConnectionSettings) (*ChaCha20.Session, error) {
	session, err := handshakeHandlers.OnConnectedToServer(*conn, settings)
	if err != nil {
		log.Printf("aborting connection: registration failed: %s\n", err)
		return nil, err
	}

	return session, err
}

func forwardIPPackets(conn *net.Conn, tunFile *os.File, session *ChaCha20.Session, connCtx *context.Context, connCancel *context.CancelFunc) {
	sendKeepAliveCommandChan := make(chan bool, 1)
	connPacketReceivedChan := make(chan bool, 1)
	go keepalive.StartConnectionProbing(*connCtx, *connCancel, sendKeepAliveCommandChan, connPacketReceivedChan)

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		forwarding.ToTCP(*conn, tunFile, session, *connCtx, *connCancel, sendKeepAliveCommandChan)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		forwarding.ToTun(*conn, tunFile, session, *connCtx, *connCancel, connPacketReceivedChan)
	}()

	wg.Wait()
}
