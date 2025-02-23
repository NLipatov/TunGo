package transport_connector

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
	"tungo/crypto"
)

const (
	initialBackoff       = 1 * time.Second
	maxBackoff           = 32 * time.Second
	maxReconnectAttempts = 30
)

type TransportConnector struct {
	//concrete logic on creating a connection instance using concrete transport (udp, tcp, etc.)
	connectDelegate func() (net.Conn, crypto.Session, error)
}

func NewTransportConnector() Connector {
	return &TransportConnector{}
}

func (m *TransportConnector) UseConnectorDelegate(f func() (net.Conn, crypto.Session, error)) Connector {
	m.connectDelegate = f
	return m
}

func (m *TransportConnector) Connect(ctx context.Context) (net.Conn, crypto.Session, error) {
	backoff := initialBackoff
	for reconnectAttempts := 0; reconnectAttempts <= maxReconnectAttempts; reconnectAttempts++ {

		conn, session, err := m.connectDelegate()

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
