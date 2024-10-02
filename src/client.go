package main

import (
	"bufio"
	"context"
	"etha-tunnel/client/forwarding/configuration"
	"etha-tunnel/client/forwarding/tcptunforward"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/network"
	"etha-tunnel/settings/client"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	initialBackoff       = 1 * time.Second
	maxBackoff           = 32 * time.Second
	maxReconnectAttempts = 5
	shutdownCommand      = "exit"
	connectionTimeout    = 10 * time.Second
)

func main() {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a goroutine to listen for user input
	go listenForExitCommand(cancel)

	// Client configuration (enabling TUN/TCP forwarding)
	configuration.Unconfigure()
	defer configuration.Unconfigure()
	if err := configuration.Configure(); err != nil {
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

	var reconnectAttempts int
	backoff := initialBackoff

	for {
		// Attempt to connect using DialContext with a pointer to net.Dialer
		dialer := &net.Dialer{}
		dialCtx, dialCancel := context.WithTimeout(ctx, connectionTimeout)
		conn, err := dialer.DialContext(dialCtx, "tcp", conf.ServerTCPAddress)
		dialCancel()

		if err != nil {
			log.Printf("Failed to connect to server: %v", err)
			reconnectAttempts++
			if reconnectAttempts > maxReconnectAttempts {
				configuration.Unconfigure()
				log.Fatalf("Exceeded maximum reconnect attempts (%d)", maxReconnectAttempts)
			}
			log.Printf("Retrying to connect in %v...", backoff)
			select {
			case <-ctx.Done():
				log.Println("Client is shutting down.")
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		log.Printf("Connected to server at %s", conf.ServerTCPAddress)
		session, err := handshakeHandlers.OnConnectedToServer(conn, conf)
		if err != nil {
			log.Printf("Registration failed: %s", err)
			conn.Close()
			reconnectAttempts++
			if reconnectAttempts > maxReconnectAttempts {
				configuration.Unconfigure()
				log.Fatalf("Exceeded maximum reconnect attempts (%d)", maxReconnectAttempts)
			}
			log.Printf("Retrying to connect in %v...", backoff)
			select {
			case <-ctx.Done():
				log.Println("Client is shutting down.")
				return
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		// Reset backoff after successful connection
		reconnectAttempts = 0
		backoff = initialBackoff

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(ctx)
		var wg sync.WaitGroup
		wg.Add(2)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-connCtx.Done()
			conn.Close()
		}()

		// Goroutine for forwarding data from TUN to TCP
		go func(conn net.Conn, tunFile *os.File, session ChaCha20.Session, ctx context.Context) {
			defer wg.Done()
			if err := tcptunforward.ToTCP(conn, tunFile, session, ctx); err != nil {
				if ctx.Err() != nil {
					// Context was canceled, no need to log as an error
					return
				}
				log.Printf("Error in TUN->TCP: %v", err)
				connCancel()
			}
		}(conn, tunFile, *session, connCtx)

		// Goroutine for forwarding data from TCP to TUN
		go func(conn net.Conn, tunFile *os.File, session ChaCha20.Session, ctx context.Context) {
			defer wg.Done()
			if err := tcptunforward.ToTun(conn, tunFile, session, ctx); err != nil {
				if ctx.Err() != nil {
					// Context was canceled, no need to log as an error
					return
				}
				log.Printf("Error in TCP->TUN: %v", err)
				connCancel()
			}
		}(conn, tunFile, *session, connCtx)

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

func listenForExitCommand(cancelFunc context.CancelFunc) {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Type 'exit' to close the connection and shut down the client.")
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if strings.EqualFold(text, shutdownCommand) {
			log.Println("Exit command received. Shutting down...")
			cancelFunc()
			return
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("Error reading standard input: %v", err)
	}
}
