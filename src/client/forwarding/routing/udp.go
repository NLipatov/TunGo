package routing

import (
	"context"
	"etha-tunnel/client/forwarding/ipconfiguration"
	"etha-tunnel/handshake/ChaCha20/handshakeHandlers"
	"etha-tunnel/settings"
	"fmt"
	"log"
	"net"
	"os"
	"time"
)

func StartUDPRouting(settings settings.ConnectionSettings, tunFile *os.File, ctx *context.Context) error {
	for {
		conn, connectionError := establishUDPConnection(settings, *ctx)
		if connectionError != nil {
			log.Printf("failed to establish connection: %s", connectionError)
			continue // Retry connection
		}

		conn.Write([]byte("INIT"))

		log.Printf("Connected to server at %s (UDP)", settings.ConnectionIP)
		session, err := handshakeHandlers.OnConnectedToServer(conn, settings)
		if err != nil {
			conn.Close()
			ipconfiguration.Unconfigure()
			log.Printf("registration failed: %s\n", err)
			log.Println("connection is aborted")
			return err
		}
		log.Println("sessionId: %s", session.SessionId)
	}
}

func establishUDPConnection(settings settings.ConnectionSettings, ctx context.Context) (net.Conn, error) {
	reconnectAttempts := 0
	backoff := initialBackoff

	for {
		serverAddr := fmt.Sprintf("%s%s", settings.ConnectionIP, settings.ConnectionPort)
		dialer := &net.Dialer{}
		dialCtx, dialCancel := context.WithTimeout(ctx, connectionTimeout)
		defer dialCancel()

		conn, err := dialer.DialContext(dialCtx, "udp", serverAddr)
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
