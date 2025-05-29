package udp_connection

import (
	"net"
	"tungo/application"
)

type SecureSession interface {
	Establish() (*net.UDPConn, application.CryptographyService, error)
}

type DefaultSecureSession struct {
	connection application.Connection[*net.UDPConn]
	secret     Secret
}

func NewDefaultSecureSession(connection application.Connection[*net.UDPConn], secret Secret) *DefaultSecureSession {
	return &DefaultSecureSession{
		connection: connection,
		secret:     secret,
	}
}

func (c *DefaultSecureSession) Establish() (*net.UDPConn, application.CryptographyService, error) {
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
