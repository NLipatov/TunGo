package transportconf

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
	"tungo/handshake/ChaCha20"
)

const (
	initialBackoff       = 1 * time.Second
	maxBackoff           = 32 * time.Second
	maxReconnectAttempts = 30
)

type ConnectionManager struct {
	//concrete logic on creating a connection instance using concrete transport (udp, tcp, etc.)
	ConnectionDelegate func() (net.Conn, *ChaCha20.Session, error)
}

func (m *ConnectionManager) EstablishConnectionWithRetry(ctx context.Context) (net.Conn, *ChaCha20.Session, error) {
	backoff := initialBackoff
	for reconnectAttempts := 0; reconnectAttempts <= maxReconnectAttempts; reconnectAttempts++ {

		conn, session, err := m.ConnectionDelegate()

		if err != nil {
			log.Printf("could not connect to server: %s", err)
			log.Printf("reconnecting in %.0f seconds", backoff.Seconds())

			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(backoff):
			}

			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}

			continue
		}

		return conn, session, nil
	}

	return nil, nil, fmt.Errorf("exceeded maximum connection attempts")
}
