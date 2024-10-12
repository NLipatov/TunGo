package main

import (
	"context"
	"etha-tunnel/client/forwarding/clienttcptunforward"
	"etha-tunnel/client/forwarding/ipconfiguration"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/inputcommands"
	"etha-tunnel/network"
	"etha-tunnel/network/keepalive"
	"etha-tunnel/settings/client"
	"log"
	"net"
	"sync"
	"time"
)

const (
	initialBackoff       = 1 * time.Second
	maxBackoff           = 32 * time.Second
	maxReconnectAttempts = 5
	connectionTimeout    = 10 * time.Second
)

func main() {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go inputcommands.ListenForCommand(cancel)

	// Client configuration (enabling TUN/TCP forwarding)
	ipconfiguration.Unconfigure()
	defer ipconfiguration.Unconfigure()
	if err := ipconfiguration.Configure(); err != nil {
		log.Fatalf("Failed to configure client: %v", err)
	}

	// Read client configuration
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("Failed to read configuration: %v", err)
	}

	// Open the TUN interface
	tunFile, err := network.OpenTunByName(conf.IfName)
	if err != nil {
		log.Fatalf("Failed to open TUN interface: %v", err)
	}
	defer tunFile.Close()

	for {
		conn, connectionError := establishConnection(*conf, ctx)
		if connectionError != nil {
			log.Fatalf("failed establish connection: %s", connectionError)
		}

		log.Printf("Connected to server at %s", conf.ServerTCPAddress)
		session, err := handshakeHandlers.OnConnectedToServer(conn, conf)
		if err != nil {
			conn.Close()
			ipconfiguration.Unconfigure()
			log.Printf("registration failed: %s\n", err)
			log.Fatalf("connection is aborted")
		}

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		wg.Add(2)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-connCtx.Done()
			conn.Close()
			connCancel()
			return
		}()

		sendKeepAliveCommandChan := make(chan bool, 1)
		connPacketReceivedChan := make(chan bool, 1)
		go keepalive.StartConnectionProbing(connCtx, connCancel, sendKeepAliveCommandChan, connPacketReceivedChan)

		// TUN -> TCP
		go func() {
			defer wg.Done()
			clienttcptunforward.ToTCP(conn, tunFile, session, ctx, sendKeepAliveCommandChan)
		}()

		// TCP -> TUN
		go func() {
			defer wg.Done()
			clienttcptunforward.ToTun(conn, tunFile, session, ctx, connPacketReceivedChan)
		}()

		// Wait for goroutines to finish
		wg.Wait()

		// After goroutines finish, check if shutdown was initiated
		if ctx.Err() != nil {
			log.Println("Client is shutting down.")
			return
		} else {
			// Connection lost unexpectedly, attempt to reconnect
			log.Println("Connection lost, attempting to reconnect...")
		}

		// Close the connection (if not already closed)
		conn.Close()
	}
}

func establishConnection(conf client.Conf, ctx context.Context) (net.Conn, error) {
	reconnectAttempts := 0
	backoff := initialBackoff

	for {
		dialer := &net.Dialer{}
		dialCtx, dialCancel := context.WithTimeout(ctx, connectionTimeout)
		conn, err := dialer.DialContext(dialCtx, "tcp", conf.ServerTCPAddress)
		dialCancel()

		if err != nil {
			log.Printf("Failed to connect to server: %v", err)
			reconnectAttempts++
			if reconnectAttempts > maxReconnectAttempts {
				ipconfiguration.Unconfigure()
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
