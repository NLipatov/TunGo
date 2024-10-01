package main

import (
	"bufio"
	"context"
	"encoding/binary"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/network"
	"etha-tunnel/network/ip"
	"etha-tunnel/settings/client"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	initialBackoff       = 1 * time.Second
	maxBackoff           = 32 * time.Second
	maxReconnectAttempts = 5
	shutdownCommand      = "exit"
	connectionTimeout    = 10 * time.Second
	maxPacketLengthBytes = 65535
)

func main() {
	// Create a context that can be canceled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle OS signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		log.Printf("Received signal: %v. Shutting down...", sig)
		cancel()
	}()

	// Start a goroutine to listen for user input
	go listenForExitCommand(cancel)

	// Read client configuration
	conf, err := (&client.Conf{}).Read()
	if err != nil {
		log.Fatalf("Failed to read configuration: %v", err)
	}

	// Deconfigure client at startup to ensure a clean state
	deconfigureClient(*conf)

	// Handle command-line arguments
	args := os.Args
	if len(args[1:]) == 1 && args[1] == "deconfigure" {
		return
	}

	// Configure the client
	if err := configureClient(conf); err != nil {
		log.Fatalf("Failed to configure client: %v", err)
	}

	// Ensure deconfiguration is performed on exit
	defer deconfigureClient(*conf)

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
				deconfigureClient(*conf)
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
				deconfigureClient(*conf)
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
			if err := forwardTunToTCP(conn, tunFile, session, ctx); err != nil {
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
			if err := forwardTCPToTun(conn, tunFile, session, ctx); err != nil {
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

// listenForExitCommand listens for the 'exit' command from the user to initiate shutdown
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

// configureClient configures the client by setting up the TUN interface and routing
func configureClient(conf *client.Conf) error {
	// Delete existing link if any
	_, _ = ip.LinkDel(conf.IfName)

	name, err := network.UpNewTun(conf.IfName)
	if err != nil {
		return fmt.Errorf("failed to create interface %v: %v", conf.IfName, err)
	}
	fmt.Printf("Created TUN interface: %v\n", name)

	// Assign IP address to the TUN interface
	_, err = ip.LinkAddrAdd(conf.IfName, conf.IfIP)
	if err != nil {
		return err
	}
	fmt.Printf("Assigned IP %s to interface %s\n", conf.IfIP, conf.IfName)

	// Parse server IP
	serverIP, _, err := net.SplitHostPort(conf.ServerTCPAddress)
	if err != nil {
		return fmt.Errorf("failed to parse server address: %v", err)
	}

	// Get routing information
	routeInfo, err := ip.RouteGet(serverIP)
	var viaGateway, devInterface string
	fields := strings.Fields(routeInfo)
	for i, field := range fields {
		if field == "via" && i+1 < len(fields) {
			viaGateway = fields[i+1]
		}
		if field == "dev" && i+1 < len(fields) {
			devInterface = fields[i+1]
		}
	}
	if devInterface == "" {
		return fmt.Errorf("failed to parse route to server IP")
	}

	// Add route to server IP
	if viaGateway == "" {
		err = ip.RouteAdd(serverIP, devInterface)
	} else {
		err = ip.RouteAddViaGateway(serverIP, devInterface, viaGateway)
	}
	if err != nil {
		return fmt.Errorf("failed to add route to server IP: %v", err)
	}
	fmt.Printf("Added route to server %s via %s dev %s\n", serverIP, viaGateway, devInterface)

	// Set the TUN interface as the default gateway
	_, err = ip.RouteAddDefaultDev(conf.IfName)
	if err != nil {
		return err
	}
	fmt.Printf("Set %s as default gateway\n", conf.IfName)

	return nil
}

// forwardTunToTCP forwards data from the TUN interface to the TCP connection
func forwardTunToTCP(conn net.Conn, tunFile *os.File, session ChaCha20.Session, ctx context.Context) error {
	buf := make([]byte, maxPacketLengthBytes)
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			n, err := tunFile.Read(buf)
			if err != nil {
				if ctx.Err() != nil {
					// Context was canceled, exit gracefully
					return nil
				}
				return fmt.Errorf("failed to read from TUN: %v", err)
			}

			aad := session.CreateAAD(false, session.C2SCounter)

			encryptedPacket, err := session.Encrypt(buf[:n], aad)
			if err != nil {
				return fmt.Errorf("failed to encrypt packet: %v", err)
			}

			atomic.AddUint64(&session.C2SCounter, 1)

			length := uint32(len(encryptedPacket))
			lengthBuf := make([]byte, 4)
			binary.BigEndian.PutUint32(lengthBuf, length)
			_, err = conn.Write(append(lengthBuf, encryptedPacket...))
			if err != nil {
				return fmt.Errorf("failed to write to server: %v", err)
			}
		}
	}
}

// forwardTCPToTun forwards data from the TCP connection to the TUN interface
func forwardTCPToTun(conn net.Conn, tunFile *os.File, session ChaCha20.Session, ctx context.Context) error {
	buf := make([]byte, maxPacketLengthBytes)
	for {
		select {
		case <-ctx.Done(): // Stop-signal
			return nil
		default:
			// Read the length of the incoming packet
			_, err := io.ReadFull(conn, buf[:4])
			if err != nil {
				if ctx.Err() != nil {
					// Context was canceled, exit gracefully
					return nil
				}
				return fmt.Errorf("failed to read from server: %v", err)
			}
			length := binary.BigEndian.Uint32(buf[:4])

			if length > maxPacketLengthBytes {
				return fmt.Errorf("packet too large: %d", length)
			}

			// Read the encrypted packet based on the length
			_, err = io.ReadFull(conn, buf[:length])
			if err != nil {
				if ctx.Err() != nil {
					// Context was canceled, exit gracefully
					return nil
				}
				return fmt.Errorf("failed to read encrypted packet: %v", err)
			}

			aad := session.CreateAAD(true, session.S2CCounter)
			decrypted, err := session.Decrypt(buf[:length], aad)
			if err != nil {
				return fmt.Errorf("failed to decrypt server packet: %v", err)
			}

			atomic.AddUint64(&session.S2CCounter, 1)

			// Write the decrypted packet to the TUN interface
			_, err = tunFile.Write(decrypted)
			if err != nil {
				return fmt.Errorf("failed to write to TUN: %v", err)
			}
		}
	}
}

// deconfigureClient deconfigures the client by removing routes and deleting the TUN interface
func deconfigureClient(conf client.Conf) {
	hostIp, devName := strings.Split(conf.ServerTCPAddress, ":")[0], conf.IfName
	// Delete the route to the host IP
	if err := ip.RouteDel(hostIp); err != nil {
		log.Printf("Failed to delete route: %s", err)
	}

	// Delete the TUN interface
	if _, err := ip.LinkDel(devName); err != nil {
		log.Printf("Failed to delete interface: %s", err)
	}
}
