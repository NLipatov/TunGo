package tunudp

import (
	"context"
	"fmt"
	"net"
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

	session, err := handshakeHandlers.OnConnectedToServer(b.conn, b.settings)
	b.session = session
	b.err = err
	return b
}

func (b *udpConnectionBuilder) build() (*net.UDPConn, *ChaCha20.Session, error) {
	if b.err != nil {
		return nil, nil, b.err
	}

	return b.conn, b.session, b.err
}
