package connection

import (
	"net"
	"tungo/crypto/chacha20"
)

type SecureSession interface {
	Establish() (*net.Conn, *chacha20.TcpSession, error)
}

type DefaultSecureSession struct {
	connection Connection
	secret     Secret
}

func NewDefaultSecureSession(connection Connection, secret Secret) *DefaultSecureSession {
	return &DefaultSecureSession{
		connection: connection,
		secret:     secret,
	}
}

func (c *DefaultSecureSession) Establish() (*net.Conn, *chacha20.TcpSession, error) {
	conn, connErr := c.connection.Establish()
	if connErr != nil {
		return nil, nil, connErr
	}

	session, sessionErr := c.secret.Exchange(conn)
	if sessionErr != nil {
		return nil, nil, sessionErr
	}

	return conn, session, nil
}
