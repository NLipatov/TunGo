package udp_connection

import (
	"tungo/application"
)

type SecureSession interface {
	Establish() (application.ConnectionAdapter, application.CryptographyService, error)
}

type DefaultSecureSession struct {
	connection application.Connection
	secret     Secret
}

func NewDefaultSecureSession(connection application.Connection, secret Secret) *DefaultSecureSession {
	return &DefaultSecureSession{
		connection: connection,
		secret:     secret,
	}
}

func (c *DefaultSecureSession) Establish() (application.ConnectionAdapter, application.CryptographyService, error) {
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
