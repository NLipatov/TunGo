package tcp_chacha20

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
	"tungo/crypto/chacha20"
	"tungo/crypto/chacha20/handshake"
	"tungo/settings"
)

type Connector struct {
	settings settings.ConnectionSettings
}

func NewConnector(settings settings.ConnectionSettings) *Connector {
	return &Connector{
		settings: settings,
	}
}

func (c *Connector) Connect(ctx context.Context) (net.Conn, *chacha20.Session, error) {
	conn, connErr := c.dial(ctx)
	if connErr != nil {
		return nil, nil, connErr
	}

	session, sessionErr := c.handshake(ctx, conn)
	if sessionErr != nil {
		_ = conn.Close()
		return nil, nil, sessionErr
	}

	return conn, session, nil
}

func (c *Connector) dial(ctx context.Context) (net.Conn, error) {
	dialer := &net.Dialer{}
	dialTimeout := time.Duration(c.settings.DialTimeoutMs) * time.Millisecond
	if dialTimeout <= 0 || dialTimeout > 300*time.Second {
		dialTimeout = 5 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)

	defer cancel()
	conn, err := dialer.DialContext(dialCtx, "tcp", net.JoinHostPort(c.settings.ConnectionIP, c.settings.Port))
	if err != nil {
		return nil, err
	}

	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		_ = conn.Close()
		return nil, fmt.Errorf("connection is not TCP, received %T", conn)
	}

	err = tcpConn.SetKeepAlive(true)
	if err != nil {
		return nil, fmt.Errorf("failed to enable keep-alive: %s", err)
	}

	err = tcpConn.SetKeepAlivePeriod(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("failed to set keep-alive period: %s", err)
	}

	return conn, nil
}

func (c *Connector) handshake(ctx context.Context, conn net.Conn) (*chacha20.Session, error) {
	var closeOnce sync.Once
	closeConn := func() {
		_ = conn.Close()
	}

	resultChan := make(chan struct {
		session *chacha20.Session
		err     error
	}, 1)

	go func(conn net.Conn, settings settings.ConnectionSettings) {
		session, handshakeErr := handshake.OnConnectedToServer(ctx, conn, settings)
		if handshakeErr != nil {
			closeOnce.Do(closeConn)
		}
		resultChan <- struct {
			session *chacha20.Session
			err     error
		}{session, handshakeErr}
	}(conn, c.settings)

	select {
	case <-ctx.Done():
		closeOnce.Do(closeConn)
		return nil, ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			return nil, res.err
		}

		return res.session, nil
	}
}
