package tun_tcp_chacha20

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"
	"tungo/Domain/settings"
	"tungo/Infrastructure/crypto/chacha20"
	"tungo/Infrastructure/crypto/chacha20/handshake"
)

type tcpConnectionBuilder struct {
	settings    settings.ConnectionSettings
	conn        net.Conn
	session     *chacha20.Session
	ctx         context.Context
	dialCancel  context.CancelFunc
	dialTimeout time.Duration
	err         error
}

func newTCPConnectionBuilder() *tcpConnectionBuilder {
	return &tcpConnectionBuilder{
		dialTimeout: time.Second * 5,
	}
}

func (b *tcpConnectionBuilder) useSettings(s settings.ConnectionSettings) *tcpConnectionBuilder {
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

func (b *tcpConnectionBuilder) useConnectionTimeout(duration time.Duration) *tcpConnectionBuilder {
	if b.err != nil {
		return b
	}

	b.dialTimeout = duration
	return b
}

func (b *tcpConnectionBuilder) connect(ctx context.Context) *tcpConnectionBuilder {
	if b.err != nil {
		return b
	}

	dialer := &net.Dialer{}
	dialCtx, cancel := context.WithTimeout(ctx, b.dialTimeout)
	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "tcp", fmt.Sprintf("%s%s", b.settings.ConnectionIP, b.settings.Port))
	if err != nil {
		if b.conn != nil {
			_ = b.conn.Close()
		}
		b.err = err
		return b
	}

	tcpConn := conn.(*net.TCPConn)
	err = tcpConn.SetKeepAlive(true)
	if err != nil {
		log.Fatalf("Failed to enable keep-alive: %v", err)
	}

	err = tcpConn.SetKeepAlivePeriod(30 * time.Second)
	if err != nil {
		log.Fatalf("Failed to set keep-alive period: %v", err)
	}

	b.conn = conn
	b.ctx = ctx
	return b
}

func (b *tcpConnectionBuilder) handshake() *tcpConnectionBuilder {
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
		session, handshakeErr := handshake.OnConnectedToServer(ctx, conn, settings)
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

func (b *tcpConnectionBuilder) build() (net.Conn, *chacha20.Session, error) {
	if b.err != nil {
		return nil, nil, b.err
	}
	return b.conn, b.session, b.err
}
