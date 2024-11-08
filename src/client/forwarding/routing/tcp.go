package routing

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
	"tungo/client/forwarding/clientipconf"
	"tungo/client/forwarding/routing/connHandling"
	"tungo/handshake/ChaCha20"
	"tungo/handshake/ChaCha20/handshakeHandlers"
	"tungo/network/keepalive"
	"tungo/settings"
)

func startTCPRouting(settings settings.ConnectionSettings, tunFile *os.File, ctx context.Context) error {
	for {
		conn, connectionError := connect(settings, ctx)
		if connectionError != nil {
			log.Printf("failed to establish connection: %s", connectionError)
			continue // Retry connection
		}

		session, registrationErr := register(&conn, settings)
		if registrationErr != nil {
			log.Fatalf("failed to register: %s", registrationErr)
		}

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(ctx)
		forwardIPPackets(&conn, tunFile, session, connCtx, connCancel)
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
			log.Printf("failed to connect to server: %v", err)
			reconnectAttempts++
			if reconnectAttempts > maxReconnectAttempts {
				clientipconf.Unconfigure(settings)
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

		log.Printf("connected to server at %s (TCP)", settings.ConnectionIP)

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

func forwardIPPackets(conn *net.Conn, tunFile *os.File, session *ChaCha20.Session, connCtx context.Context, connCancel context.CancelFunc) {
	sendKeepaliveCh := make(chan bool, 1)
	receiveKeepaliveCh := make(chan bool, 1)
	go keepalive.StartConnectionProbing(connCtx, connCancel, sendKeepaliveCh, receiveKeepaliveCh)

	var wg sync.WaitGroup
	wg.Add(2)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		connHandling.ToTCP(*conn, tunFile, session, connCtx, connCancel, sendKeepaliveCh)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		connHandling.ToTun(*conn, tunFile, session, connCtx, connCancel, receiveKeepaliveCh)
	}()

	wg.Wait()

	close(sendKeepaliveCh)
	close(receiveKeepaliveCh)
}
