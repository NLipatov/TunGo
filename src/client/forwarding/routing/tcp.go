package routing

import (
	"context"
	"etha-tunnel/client/forwarding/clienttcptunforward"
	"etha-tunnel/client/forwarding/ipconfiguration"
	"etha-tunnel/handshake/ChaCha20"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/network/keepalive"
	"etha-tunnel/settings/client"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"time"
)

func StartTCPRouting(conf *client.Conf, tunFile *os.File, ctx *context.Context) error {
	for {
		conn, connectionError := establishTCPConnection(*conf, *ctx)
		if connectionError != nil {
			log.Printf("failed to establish connection: %s", connectionError)
			continue // Retry connection
		}

		log.Printf("Connected to server at %s", conf.TCPSettings.ConnectionIP)
		session, err := handshakeHandlers.OnConnectedToServer(conn, conf)
		if err != nil {
			conn.Close()
			ipconfiguration.Unconfigure()
			log.Printf("registration failed: %s\n", err)
			log.Println("connection is aborted")
			return err
		}

		// Create a child context for managing data forwarding goroutines
		connCtx, connCancel := context.WithCancel(*ctx)
		var wg sync.WaitGroup
		wg.Add(2)

		// Start a goroutine to monitor context cancellation and close the connection
		go func() {
			<-connCtx.Done()
			conn.Close()
			return
		}()

		go startTCPForwarding(&conn, tunFile, session, &connCtx, &connCancel, &wg)

		// Wait for goroutines to finish
		wg.Wait()

		// After goroutines finish, check if shutdown was initiated
		if (*ctx).Err() != nil {
			log.Println("Client is shutting down.")
			return err
		} else {
			// Connection lost unexpectedly, attempt to reconnect
			log.Println("Connection lost, attempting to reconnect...")
		}

		// Close the connection (if not already closed)
		conn.Close()
	}
}

func establishTCPConnection(conf client.Conf, ctx context.Context) (net.Conn, error) {
	reconnectAttempts := 0
	backoff := initialBackoff

	for {
		dialer := &net.Dialer{}
		dialCtx, dialCancel := context.WithTimeout(ctx, connectionTimeout)
		conn, err := dialer.DialContext(dialCtx, "tcp", fmt.Sprintf("%s%s", conf.TCPSettings.ConnectionIP, conf.TCPSettings.ConnectionPort))
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

func startTCPForwarding(conn *net.Conn, tunFile *os.File, session *ChaCha20.Session, connCtx *context.Context, connCancel *context.CancelFunc, wg *sync.WaitGroup) {
	sendKeepAliveCommandChan := make(chan bool, 1)
	connPacketReceivedChan := make(chan bool, 1)
	go keepalive.StartConnectionProbing(*connCtx, *connCancel, sendKeepAliveCommandChan, connPacketReceivedChan)

	// TUN -> TCP
	go func() {
		defer wg.Done()
		clienttcptunforward.ToTCP(*conn, tunFile, session, *connCtx, *connCancel, sendKeepAliveCommandChan)
	}()

	// TCP -> TUN
	go func() {
		defer wg.Done()
		clienttcptunforward.ToTun(*conn, tunFile, session, *connCtx, *connCancel, connPacketReceivedChan)
	}()
}

func startUDPRouting() {

}
