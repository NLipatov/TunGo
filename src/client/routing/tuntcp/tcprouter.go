package tuntcp

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

type TCPRouter struct {
	Settings settings.ConnectionSettings
}

func (tr *TCPRouter) ForwardTraffic(ctx context.Context) error {
	var tunFile *os.File
	defer func() {
		_ = tunFile.Close()
	}()
	defer tunconf.Deconfigure(tr.Settings)

	for {
		_ = tunFile.Close()
		tunFile = tunconf.Configure(tr.Settings)

		conn, connectionError := connect(tr.Settings, ctx)
		if connectionError != nil {
			log.Printf("failed to establish connection: %s", connectionError)
			continue // Retry connection
		}

		session, registrationErr := register(&conn, tr.Settings)
		if registrationErr != nil {
			log.Printf("failed to register: %s", registrationErr)
			time.Sleep(time.Second * 1)
		}

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(ctx)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-connCtx.Done()
			_ = conn.Close()
			tunconf.Deconfigure(tr.Settings)
			return
		}()

		forwardIPPackets(&conn, tunFile, session, connCtx, connCancel)

		// After goroutines finish, check if shutdown was initiated
		if ctx.Err() != nil {
			return nil
		} else {
			// Connection lost unexpectedly, attempt to reconnect
			log.Println("connection lost, attempting to reconnect...")
		}

		// Close the connection (if not already closed)
		_ = conn.Close()
	}
}

func connect(settings settings.ConnectionSettings, ctx context.Context) (net.Conn, error) {
	reconnectAttempts := 0
	backoff := initialBackoff

	for {
		dialer := &net.Dialer{}
		dialCtx, dialCancel := context.WithTimeout(ctx, connectionTimeout)
		conn, err := dialer.DialContext(dialCtx, "tcp", fmt.Sprintf("%s%s", settings.ConnectionIP, settings.Port))
		dialCancel()

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
		ToTCP(*conn, tunFile, session, connCtx, connCancel, sendKeepaliveCh)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		ToTun(*conn, tunFile, session, connCtx, connCancel, receiveKeepaliveCh)
	}()

	wg.Wait()
}
