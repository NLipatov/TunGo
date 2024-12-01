package tuntcp

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
	"tungo/handshake/ChaCha20"
	"tungo/settings"
)

type connManager struct {
	settings settings.ConnectionSettings
}

func newConnectionManager(settings settings.ConnectionSettings) *connManager {
	return &connManager{
		settings: settings,
	}
}

func (m *connManager) connect(ctx context.Context) (net.Conn, *ChaCha20.Session, error) {
	backoff := initialBackoff
	for reconnectAttempts := 0; reconnectAttempts <= maxReconnectAttempts; reconnectAttempts++ {
		conn, session, err := newTCPConnectionBuilder().useSettings(m.settings).connect(ctx).handshake().build()
		if err != nil {
			log.Printf("could not connect to server at %s: %s", m.settings.ConnectionIP, err)
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

		log.Printf("connected to server at %s (TCP)", m.settings.ConnectionIP)

		return conn, session, nil
	}

	return nil, nil, fmt.Errorf("exceeded maximum reconnect attempts (%d)", maxReconnectAttempts)
}
