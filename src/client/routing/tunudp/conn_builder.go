package tunudp

import (
	"context"
	"fmt"
	"net"
	"time"
	"tungo/handshake/ChaCha20"
	"tungo/handshake/ChaCha20/handshakeHandlers"
	"tungo/settings"
)

type udpConnectionBuilder struct {
	settings settings.ConnectionSettings
	conn     *net.UDPConn
	session  *ChaCha20.Session
	ctx      context.Context
	err      error
}

func newUDPConnectionBuilder() *udpConnectionBuilder {
	return &udpConnectionBuilder{}
}

func (b *udpConnectionBuilder) useSettings(s settings.ConnectionSettings) *udpConnectionBuilder {
	if b.err != nil {
		return b
	}

	if s.ConnectionIP == "" || s.Port == "" {
		b.err = fmt.Errorf("invalid connection settings: IP and Port are required")
		return b
	}

	b.settings = s
	return b
}

func (b *udpConnectionBuilder) connect(ctx context.Context) *udpConnectionBuilder {
	if b.err != nil {
		return b
	}

	serverAddr := fmt.Sprintf("%s%s", b.settings.ConnectionIP, b.settings.Port)
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		b.err = fmt.Errorf("server address resolution failed: %w", err)
		return b
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		b.err = fmt.Errorf("failed to dial server's udp address: %w", err)
		return b
	}

	b.conn = conn
	b.ctx = ctx
	return b
}

func (b *udpConnectionBuilder) handshake() *udpConnectionBuilder {
	if b.err != nil {
		return b
	}

	reconnectAttempts := 0
	backoff := initialBackoff

	for {
		session, err := handshakeHandlers.OnConnectedToServer(b.conn, b.settings)
		if err != nil {
			reconnectAttempts++
			if reconnectAttempts > maxReconnectAttempts {
				b.err = fmt.Errorf("exceeded maximum reconnect attempts (%d)", maxReconnectAttempts)
				_ = b.conn.Close()
				return b
			}
			select {
			case <-b.ctx.Done():
				b.err = b.ctx.Err()
				return b
			case <-time.After(backoff):
			}
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
			continue
		}

		b.session = session
		return b
	}
}

func (b *udpConnectionBuilder) build() (*net.UDPConn, *ChaCha20.Session, error) {
	return b.conn, b.session, b.err
}
