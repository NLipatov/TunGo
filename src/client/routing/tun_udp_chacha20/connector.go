package tun_udp_chacha20

import (
	"context"
	"net"
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

func (c *Connector) Connect(ctx context.Context) (*net.UDPConn, *chacha20.Session, error) {
	conn, connErr := c.dial(c.settings)
	if connErr != nil {
		return nil, nil, connErr
	}

	session, sessionErr := c.handshake(ctx, conn)
	if sessionErr != nil {
		return nil, nil, sessionErr
	}

	return conn, session, nil
}

func (c *Connector) dial(settings settings.ConnectionSettings) (*net.UDPConn, error) {
	serverAddr := net.JoinHostPort(settings.ConnectionIP, settings.Port)
	udpAddr, err := net.ResolveUDPAddr("udp", serverAddr)
	if err != nil {
		return nil, err
	}

	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (c *Connector) handshake(ctx context.Context, conn *net.UDPConn) (*chacha20.Session, error) {
	if c.settings.DialTimeoutMs <= 0 || c.settings.DialTimeoutMs >= 300_000 {
		c.settings.DialTimeoutMs = 5_000 //5 seconds is default timeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.settings.DialTimeoutMs)*time.Millisecond)
	defer cancel()

	resultChan := make(chan struct {
		session *chacha20.Session
		err     error
	}, 1)

	go func(conn net.Conn, settings settings.ConnectionSettings) {
		session, handshakeErr := handshake.OnConnectedToServer(ctx, conn, settings)
		resultChan <- struct {
			session *chacha20.Session
			err     error
		}{session, handshakeErr}
	}(conn, c.settings)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			return nil, res.err
		}

		return res.session, nil
	}
}
