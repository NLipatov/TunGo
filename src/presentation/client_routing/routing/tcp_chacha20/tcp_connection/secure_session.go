package tcp_connection

import (
	"net"
	"tungo/application"
)

type SecureSession interface {
	Establish() (*net.Conn, application.CryptographyService, error)
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

func (c *DefaultSecureSession) Establish() (*net.Conn, application.CryptographyService, error) {
	conn, connErr := c.connection.Establish()
	if connErr != nil {
		return nil, nil, connErr
	}

	cryptographyService, cryptographyServiceErr := c.secret.Exchange(conn)
	if cryptographyServiceErr != nil {
		return nil, nil, cryptographyServiceErr
	}

	return conn, cryptographyService, nil
}
