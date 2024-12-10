package tun_udp

import (
	"context"
	"fmt"
	"net"
	"time"
	"tungo/handshake/chacha20"
	"tungo/handshake/chacha20/chacha20_handshake"
	"tungo/settings"
)

type connectionBuilder struct {
	settings    settings.ConnectionSettings
	conn        *net.UDPConn
	session     *chacha20.Session
	ctx         context.Context
	dialTimeout time.Duration
	err         error
}

func newConnectionBuilder() *connectionBuilder {
	return &connectionBuilder{
		dialTimeout: time.Second * 5,
	}
}

func (b *connectionBuilder) useSettings(s settings.ConnectionSettings) *connectionBuilder {
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

func (b *connectionBuilder) useConnectionTimeout(duration time.Duration) *connectionBuilder {
	if b.err != nil {
		return b
	}

	b.dialTimeout = duration
	return b
}

func (b *connectionBuilder) connect(ctx context.Context) *connectionBuilder {
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

func (b *connectionBuilder) handshake() *connectionBuilder {
	if b.err != nil {
		return b
	}

	ctx, cancel := context.WithTimeout(b.ctx, b.dialTimeout)
	defer cancel()

	resultChan := make(chan struct {
		session *chacha20.Session
		err     error
	})

	go func(conn net.Conn, settings settings.ConnectionSettings) {
		defer close(resultChan)
		session, handshakeErr := chacha20_handshake.OnConnectedToServer(ctx, conn, settings)
		resultChan <- struct {
			session *chacha20.Session
			err     error
		}{session, handshakeErr}
	}(b.conn, b.settings)

	select {
	case <-ctx.Done():
		b.err = fmt.Errorf("server took too long to respond: %w", ctx.Err())
		return b
	case res := <-resultChan:
		b.session = res.session
		b.err = res.err
		return b
	}
}

func (b *connectionBuilder) build() (net.Conn, *chacha20.Session, error) {
	if b.err != nil {
		return nil, nil, b.err
	}

	return b.conn, b.session, b.err
}
