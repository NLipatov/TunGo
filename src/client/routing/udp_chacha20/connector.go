package udp_chacha20

import (
	"context"
	"fmt"
	"net"
	"time"
	"tungo/crypto/chacha20"
	"tungo/settings"
	"tungo/settings/client"
)

type Connector struct {
	settings   settings.ConnectionSettings
	connection Connection
}

func NewConnector(settings settings.ConnectionSettings, connection Connection) *Connector {
	return &Connector{
		settings:   settings,
		connection: connection,
	}
}

func (c *Connector) Connect(ctx context.Context) (*net.UDPConn, *chacha20.UdpSession, error) {
	conn, connErr := c.connection.Establish()
	if connErr != nil {
		return nil, nil, connErr
	}

	session, sessionErr := c.handshake(ctx, conn)
	if sessionErr != nil {
		return nil, nil, sessionErr
	}

	return conn, session, nil
}

func (c *Connector) handshake(ctx context.Context, conn *net.UDPConn) (*chacha20.UdpSession, error) {
	if c.settings.DialTimeoutMs <= 0 || c.settings.DialTimeoutMs >= 300_000 {
		c.settings.DialTimeoutMs = 5_000 //5 seconds is default timeout
	}

	ctx, cancel := context.WithTimeout(ctx, time.Duration(c.settings.DialTimeoutMs)*time.Millisecond)
	defer cancel()

	resultChan := make(chan struct {
		handshake *chacha20.Handshake
		err       error
	}, 1)

	go func(conn net.Conn, settings settings.ConnectionSettings) {
		h := chacha20.NewHandshake()
		handshakeErr := h.ClientSideHandshake(ctx, conn, settings)
		resultChan <- struct {
			handshake *chacha20.Handshake
			err       error
		}{h, handshakeErr}
	}(conn, c.settings)

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-resultChan:
		if res.err != nil {
			return nil, res.err
		}

		session, sessionErr := chacha20.NewUdpSession(res.handshake.Id(), res.handshake.ClientKey(), res.handshake.ServerKey(), false)
		if sessionErr != nil {
			return nil, fmt.Errorf("failed to create client session: %s\n", sessionErr)
		}

		conf, confErr := (&client.Conf{}).Read()
		if confErr != nil {
			return nil, confErr
		}
		session.UseNonceRingBuffer(conf.UDPNonceRingBufferSize)

		return session, nil
	}
}
